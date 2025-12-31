package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

func getEnvSec(name string, defSec int) time.Duration {
	v := strings.TrimSpace(os.Getenv(name))
	if v == "" {
		return time.Duration(defSec) * time.Second
	}
	n, err := strconv.Atoi(v)
	if err != nil || n <= 0 {
		return time.Duration(defSec) * time.Second
	}
	return time.Duration(n) * time.Second
}

func redisLoadTimeout() time.Duration {
	// 批量扫描/批量 HMGET 更慢，单独给更大的 timeout
	// 与 stats/dgmain3 保持一致
	return getEnvSec("REDIS_LOAD_TIMEOUT_SEC", 120)
}

func getHMGETChunk() int {
	c := getEnvInt("REDIS_HMGET_CHUNK", 200)
	if c <= 0 {
		return 200
	}
	return c
}

func devicePoolKeys() (idsKey, dataKey, useKey string) {
	// signup 从 Redis 读取设备：按分库读取（通过 REDIS_DEVICE_POOL_KEY / 启动参数决定）
	prefix := getEnvStr("REDIS_DEVICE_POOL_KEY", "tiktok:device_pool")
	return prefix + ":ids", prefix + ":data", prefix + ":use"
}

func devicePoolOrderKeys() (seqKey, inKey string) {
	// signup 从 Redis 读取设备：按分库读取
	prefix := getEnvStr("REDIS_DEVICE_POOL_KEY", "tiktok:device_pool")
	return prefix + ":seq", prefix + ":in"
}

func sampleFactor() int {
	f := getEnvInt("REDIS_DEVICE_SAMPLE_FACTOR", 3)
	if f < 1 {
		return 1
	}
	if f > 20 {
		return 20
	}
	return f
}

func sampleRounds() int {
	r := getEnvInt("REDIS_DEVICE_SAMPLE_ROUNDS", 10)
	if r < 1 {
		return 1
	}
	if r > 200 {
		return 200
	}
	return r
}

func sampleDeviceIDsFast(ctx context.Context, rdb *redis.Client, idsKey, inKey, useKey string, need int, inOffset *int64) ([]string, error) {
	if need <= 0 {
		return []string{}, nil
	}
	// 1) FIFO 队列：按入队顺序取
	if strings.TrimSpace(inKey) != "" {
		start := int64(0)
		if inOffset != nil && *inOffset > 0 {
			start = *inOffset
		}
		stop := start + int64(need) - 1
		ids, err := rdb.ZRange(ctx, inKey, start, stop).Result()
		if err == nil && len(ids) > 0 {
			if inOffset != nil {
				*inOffset = stop + 1
			}
			return ids, nil
		}
	}
	// 2) 兼容：useKey（没有 FIFO 时也能稳定取数）
	if strings.TrimSpace(useKey) != "" {
		start := int64(0)
		if inOffset != nil && *inOffset > 0 {
			start = *inOffset
		}
		stop := start + int64(need) - 1
		ids, err := rdb.ZRange(ctx, useKey, start, stop).Result()
		if err == nil && len(ids) > 0 {
			if inOffset != nil {
				*inOffset = stop + 1
			}
			return ids, nil
		}
	}
	// 3) 随机抽样（避免 10 万池子全量 SSCAN）
	ids, err := rdb.SRandMemberN(ctx, idsKey, int64(need)).Result()
	if err == nil && len(ids) > 0 {
		return ids, nil
	}
	// 4) 兜底：SSCAN
	var out []string
	iter := rdb.SScan(ctx, idsKey, 0, "*", int64(need)).Iterator()
	for iter.Next(ctx) {
		out = append(out, iter.Val())
		if len(out) >= need {
			break
		}
	}
	if err := iter.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func getEnvBool(name string, def bool) bool {
	v := strings.TrimSpace(os.Getenv(name))
	if v == "" {
		return def
	}
	switch strings.ToLower(v) {
	case "1", "true", "yes", "y", "on":
		return true
	case "0", "false", "no", "n", "off":
		return false
	default:
		return def
	}
}

func getEnvInt(name string, def int) int {
	v := strings.TrimSpace(os.Getenv(name))
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

func getEnvStr(name, def string) string {
	v := strings.TrimSpace(os.Getenv(name))
	if v == "" {
		return def
	}
	return v
}

func shouldLoadDevicesFromRedis() bool {
	if getEnvBool("DEVICES_FROM_REDIS", false) {
		return true
	}
	if strings.EqualFold(getEnvStr("DEVICES_SOURCE", ""), "redis") {
		return true
	}
	return false
}

func newRedisClient() (*redis.Client, error) {
	// 对齐 stats：显式超时，避免高并发/大批量 HMGET 时出现 i/o timeout
	dialTO := getEnvSec("REDIS_DIAL_TIMEOUT_SEC", 10)
	readTO := getEnvSec("REDIS_READ_TIMEOUT_SEC", 20)
	writeTO := getEnvSec("REDIS_WRITE_TIMEOUT_SEC", 20)
	poolTO := getEnvSec("REDIS_POOL_TIMEOUT_SEC", 30)

	if url := strings.TrimSpace(os.Getenv("REDIS_URL")); url != "" {
		opt, err := redis.ParseURL(url)
		if err != nil {
			return nil, err
		}
		opt.DialTimeout = dialTO
		opt.ReadTimeout = readTO
		opt.WriteTimeout = writeTO
		opt.PoolTimeout = poolTO
		return redis.NewClient(opt), nil
	}

	host := getEnvStr("REDIS_HOST", "127.0.0.1")
	port := getEnvInt("REDIS_PORT", 6379)
	db := getEnvInt("REDIS_DB", 0)
	user := strings.TrimSpace(os.Getenv("REDIS_USERNAME"))
	pass := strings.TrimSpace(os.Getenv("REDIS_PASSWORD"))
	useTLS := getEnvBool("REDIS_SSL", false)

	opt := &redis.Options{
		Addr:     fmt.Sprintf("%s:%d", host, port),
		DB:       db,
		Username: user,
		Password: pass,
		DialTimeout:  dialTO,
		ReadTimeout:  readTO,
		WriteTimeout: writeTO,
		PoolTimeout:  poolTO,
	}
	if useTLS {
		opt.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	}
	return redis.NewClient(opt), nil
}

// loadDevicesFromRedis 从 Python 注册流程写入的 Redis 设备池读取设备。
// 约定 key 结构：
// - {prefix}:ids  (SET)
// - {prefix}:data (HASH) id -> json
func loadDevicesFromRedis(limit int) ([]map[string]interface{}, error) {
	ctx, cancel := context.WithTimeout(context.Background(), redisLoadTimeout())
	defer cancel()

	rdb, err := newRedisClient()
	if err != nil {
		return nil, fmt.Errorf("redis init: %w", err)
	}
	defer rdb.Close()

	if err := rdb.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("redis ping: %w", err)
	}

	idsKey, dataKey, useKey := devicePoolKeys()
	_, inKey := devicePoolOrderKeys()

	minAge := getSignupDeviceMinAgeHours()
	chunk := getHMGETChunk()
	f := sampleFactor()
	rounds := sampleRounds()
	var inOff int64 = 0

	target := limit
	if target <= 0 {
		target = 0
	}
	// 目标=limit（按需取），但“先拿够候选再过滤”；过滤不足就继续按 FIFO 往后取
	out := make([]map[string]interface{}, 0, max(0, limit))
	seen := map[string]bool{}

	for round := 0; round < rounds && (limit <= 0 || len(out) < limit); round++ {
		needBase := 1000
		if limit > 0 {
			needBase = (limit - len(out)) * f
			if needBase < 50 {
				needBase = 50
			}
		}
		if needBase > 5000 {
			needBase = 5000
		}
		ids, err := sampleDeviceIDsFast(ctx, rdb, idsKey, inKey, useKey, needBase, &inOff)
		if err != nil {
			return nil, fmt.Errorf("redis sample device ids: %w", err)
		}
		if len(ids) == 0 {
			break
		}
		fields := make([]string, 0, len(ids))
		for _, id := range ids {
			id = strings.TrimSpace(id)
			if id == "" || seen[id] {
				continue
			}
			seen[id] = true
			fields = append(fields, id)
		}
		if len(fields) == 0 {
			continue
		}
		for i := 0; i < len(fields); i += chunk {
			end := i + chunk
			if end > len(fields) {
				end = len(fields)
			}
			sub := fields[i:end]
			vals, err := rdb.HMGet(ctx, dataKey, sub...).Result()
			if err != nil {
				return nil, fmt.Errorf("redis hmget data: %w", err)
			}
			for _, v := range vals {
				s, ok := v.(string)
				if !ok || strings.TrimSpace(s) == "" {
					continue
				}
				var dev map[string]interface{}
				if err := json.Unmarshal([]byte(s), &dev); err != nil {
					continue
				}
				if !deviceCreateTimeOK(dev, minAge) {
					continue
				}
				out = append(out, dev)
				if limit > 0 && len(out) >= limit {
					return out, nil
				}
			}
		}
	}

	if len(out) == 0 {
		return nil, fmt.Errorf("redis device pool empty or no valid data after minAgeHours=%d: %s", minAge, idsKey)
	}
	_ = target
	return out, nil
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}



