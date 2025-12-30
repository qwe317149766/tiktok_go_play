package main

import (
	"context"
	"crypto/sha1"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

func envBool(name string, def bool) bool {
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

func envInt(name string, def int) int {
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

func envStr(name, def string) string {
	v := strings.TrimSpace(os.Getenv(name))
	if v == "" {
		return def
	}
	return v
}

func newRedisClient() (*redis.Client, error) {
	if url := strings.TrimSpace(os.Getenv("REDIS_URL")); url != "" {
		opt, err := redis.ParseURL(url)
		if err != nil {
			return nil, err
		}
		return redis.NewClient(opt), nil
	}
	host := envStr("REDIS_HOST", "127.0.0.1")
	port := envInt("REDIS_PORT", 6379)
	db := envInt("REDIS_DB", 0)
	user := strings.TrimSpace(os.Getenv("REDIS_USERNAME"))
	pass := strings.TrimSpace(os.Getenv("REDIS_PASSWORD"))
	useTLS := envBool("REDIS_SSL", false)
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

var (
	globalRedis     *redis.Client
	globalRedisOnce sync.Once
	globalRedisErr  error
)

func getRedisClient() (*redis.Client, error) {
	globalRedisOnce.Do(func() {
		rdb, err := newRedisClient()
		if err != nil {
			globalRedisErr = err
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := rdb.Ping(ctx).Err(); err != nil {
			_ = rdb.Close()
			globalRedisErr = err
			return
		}
		globalRedis = rdb
	})
	return globalRedis, globalRedisErr
}

// -------- device pool (来自 Python 注册成功写入) --------

func shouldLoadDevicesFromRedis() bool {
	if envBool("DEVICES_FROM_REDIS", false) {
		return true
	}
	if strings.EqualFold(envStr("DEVICES_SOURCE", ""), "redis") {
		return true
	}
	return false
}

func devicePoolKeys() (idsKey, dataKey, useKey, failKey string) {
	prefix := envStr("REDIS_DEVICE_POOL_KEY", "tiktok:device_pool")
	return prefix + ":ids", prefix + ":data", prefix + ":use", prefix + ":fail"
}

func devicePoolCountKeys() (attemptKey, playKey string) {
	prefix := envStr("REDIS_DEVICE_POOL_KEY", "tiktok:device_pool")
	return prefix + ":attempt", prefix + ":play"
}

func devicePoolIDFromDevice(device map[string]interface{}) string {
	idField := envStr("REDIS_DEVICE_ID_FIELD", "cdid")
	// 先按配置字段取
	if v, ok := device[idField]; ok {
		switch t := v.(type) {
		case string:
			if strings.TrimSpace(t) != "" {
				return strings.TrimSpace(t)
			}
		case float64:
			return fmt.Sprintf("%.0f", t)
		}
	}
	// fallback：常见字段
	if v, ok := device["device_id"].(string); ok && strings.TrimSpace(v) != "" {
		return strings.TrimSpace(v)
	}
	if v, ok := device["cdid"].(string); ok && strings.TrimSpace(v) != "" {
		return strings.TrimSpace(v)
	}
	return ""
}

func getSeedTokenFromRedis(poolID string) (seed string, seedType int, token string, ok bool) {
	if strings.TrimSpace(poolID) == "" {
		return "", 0, "", false
	}
	rdb, err := getRedisClient()
	if err != nil {
		return "", 0, "", false
	}
	_, dataKey, _, _ := devicePoolKeys()
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()

	raw, err := rdb.HGet(ctx, dataKey, poolID).Result()
	if err != nil || strings.TrimSpace(raw) == "" {
		return "", 0, "", false
	}

	var dev map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &dev); err != nil {
		return "", 0, "", false
	}

	if s, ok := dev["seed"].(string); ok {
		seed = s
	}
	// seed_type 兼容 int/float/string
	switch v := dev["seed_type"].(type) {
	case float64:
		seedType = int(v)
	case int:
		seedType = v
	case string:
		if n, err := strconv.Atoi(strings.TrimSpace(v)); err == nil {
			seedType = n
		}
	}
	if t, ok := dev["token"].(string); ok {
		token = t
	}
	return seed, seedType, token, (seed != "" && token != "" && seedType != 0)
}

func setSeedTokenToRedis(poolID string, seed string, seedType int, token string) error {
	if strings.TrimSpace(poolID) == "" {
		return fmt.Errorf("empty poolID")
	}
	rdb, err := getRedisClient()
	if err != nil {
		return err
	}
	_, dataKey, _, _ := devicePoolKeys()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	raw, err := rdb.HGet(ctx, dataKey, poolID).Result()
	if err != nil || strings.TrimSpace(raw) == "" {
		return fmt.Errorf("device not found in redis data: %s", poolID)
	}
	var dev map[string]interface{}
	if err := json.Unmarshal([]byte(raw), &dev); err != nil {
		return err
	}
	dev["seed"] = seed
	dev["seed_type"] = seedType
	dev["token"] = token
	// 兼容字段名
	dev["seedType"] = seedType

	out, _ := json.Marshal(dev)
	return rdb.HSet(ctx, dataKey, poolID, string(out)).Err()
}

func incrDeviceUse(poolID string, delta int64) error {
	if strings.TrimSpace(poolID) == "" {
		return fmt.Errorf("empty poolID")
	}
	rdb, err := getRedisClient()
	if err != nil {
		return err
	}
	_, _, useKey, _ := devicePoolKeys()
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	return rdb.ZIncrBy(ctx, useKey, float64(delta), poolID).Err()
}

// incrDeviceAttempt 写“尝试次数”（attempt_count）
// 同时也写入 :use 作为兼容字段（历史上 :use 被当作 use_count 参与淘汰规则）。
func incrDeviceAttempt(poolID string, delta int64) error {
	if err := incrDeviceUse(poolID, delta); err != nil {
		return err
	}
	rdb, err := getRedisClient()
	if err != nil {
		return err
	}
	attemptKey, _ := devicePoolCountKeys()
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	return rdb.ZIncrBy(ctx, attemptKey, float64(delta), poolID).Err()
}

// incrDevicePlay 写“播放次数”（play_count），只在 stats 成功时调用
func incrDevicePlay(poolID string, delta int64) error {
	if strings.TrimSpace(poolID) == "" {
		return fmt.Errorf("empty poolID")
	}
	rdb, err := getRedisClient()
	if err != nil {
		return err
	}
	_, playKey := devicePoolCountKeys()
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	return rdb.ZIncrBy(ctx, playKey, float64(delta), poolID).Err()
}

func incrDeviceFail(poolID string, delta int64) error {
	if strings.TrimSpace(poolID) == "" {
		return fmt.Errorf("empty poolID")
	}
	rdb, err := getRedisClient()
	if err != nil {
		return err
	}
	_, _, _, failKey := devicePoolKeys()
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	return rdb.ZIncrBy(ctx, failKey, float64(delta), poolID).Err()
}

func loadDevicesFromRedis(limit int) ([]string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()

	rdb, err := getRedisClient()
	if err != nil {
		return nil, fmt.Errorf("redis init: %w", err)
	}

	prefix := envStr("REDIS_DEVICE_POOL_KEY", "tiktok:device_pool")
	idsKey := prefix + ":ids"
	dataKey := prefix + ":data"

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

	const chunk = 500
	out := make([]string, 0, len(ids))
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
			out = append(out, s)
			if limit > 0 && len(out) >= limit {
				return out, nil
			}
		}
	}
	return out, nil
}

// -------- startup cookie pool (来自 Go startUp 注册写入) --------

func shouldLoadCookiesFromRedis() bool {
	if envBool("COOKIES_FROM_REDIS", false) {
		return true
	}
	if strings.EqualFold(envStr("COOKIES_SOURCE", ""), "redis") {
		return true
	}
	return false
}

type CookieRecord struct {
	ID      string            `json:"id"`
	Cookies map[string]string `json:"cookies"`
}

func loadStartupCookiesFromRedis(limit int) ([]CookieRecord, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()

	rdb, err := getRedisClient()
	if err != nil {
		return nil, fmt.Errorf("redis init: %w", err)
	}

	prefix := envStr("REDIS_STARTUP_COOKIE_POOL_KEY", "tiktok:startup_cookie_pool")
	idsKey := prefix + ":ids"
	dataKey := prefix + ":data"

	var ids []string
	iter := rdb.SScan(ctx, idsKey, 0, "*", 1000).Iterator()
	for iter.Next(ctx) {
		ids = append(ids, iter.Val())
		if limit > 0 && len(ids) >= limit {
			break
		}
	}
	if err := iter.Err(); err != nil {
		return nil, fmt.Errorf("redis sscan cookie ids: %w", err)
	}
	if len(ids) == 0 {
		return nil, fmt.Errorf("redis startup cookie pool empty: %s", idsKey)
	}

	const chunk = 500
	out := make([]CookieRecord, 0, len(ids))
	for i := 0; i < len(ids); i += chunk {
		end := i + chunk
		if end > len(ids) {
			end = len(ids)
		}
		fields := ids[i:end]
		vals, err := rdb.HMGet(ctx, dataKey, fields...).Result()
		if err != nil {
			return nil, fmt.Errorf("redis hmget cookie data: %w", err)
		}
		for idx, v := range vals {
			s, ok := v.(string)
			if !ok || strings.TrimSpace(s) == "" {
				continue
			}
			var ck map[string]string
			if err := json.Unmarshal([]byte(s), &ck); err != nil {
				continue
			}
			out = append(out, CookieRecord{ID: fields[idx], Cookies: ck})
			if limit > 0 && len(out) >= limit {
				return out, nil
			}
		}
	}
	return out, nil
}

func cookieIDFromMap(cookies map[string]string) string {
	if v := strings.TrimSpace(cookies["sessionid"]); v != "" {
		return v
	}
	if v := strings.TrimSpace(cookies["sid_tt"]); v != "" {
		return v
	}
	b, _ := json.Marshal(cookies)
	h := sha1.Sum(b)
	return hex.EncodeToString(h[:])
}


