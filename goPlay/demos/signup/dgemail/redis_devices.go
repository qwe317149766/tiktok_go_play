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
	if url := strings.TrimSpace(os.Getenv("REDIS_URL")); url != "" {
		opt, err := redis.ParseURL(url)
		if err != nil {
			return nil, err
		}
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
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	rdb, err := newRedisClient()
	if err != nil {
		return nil, fmt.Errorf("redis init: %w", err)
	}
	defer rdb.Close()

	if err := rdb.Ping(ctx).Err(); err != nil {
		return nil, fmt.Errorf("redis ping: %w", err)
	}

	prefix := getEnvStr("REDIS_DEVICE_POOL_KEY", "tiktok:device_pool")
	idsKey := prefix + ":ids"
	dataKey := prefix + ":data"

	// 通过 SSCAN 拉取，避免 SMEMBERS 一次性拉爆内存
	var ids []string
	iter := rdb.SScan(ctx, idsKey, 0, "*", 1000).Iterator()
	for iter.Next(ctx) {
		ids = append(ids, iter.Val())
		if limit > 0 && len(ids) >= limit {
			break
		}
	}
	if err := iter.Err(); err != nil {
		return nil, fmt.Errorf("redis sscan ids: %w", err)
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("redis device pool empty: %s", idsKey)
	}

	// 批量 HMGET
	const chunk = 500
	devices := make([]map[string]interface{}, 0, len(ids))
	for i := 0; i < len(ids); i += chunk {
		end := i + chunk
		if end > len(ids) {
			end = len(ids)
		}
		fields := ids[i:end]
		vals, err := rdb.HMGet(ctx, dataKey, fields...).Result()
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
			devices = append(devices, dev)
			if limit > 0 && len(devices) >= limit {
				return devices, nil
			}
		}
	}
	return devices, nil
}



