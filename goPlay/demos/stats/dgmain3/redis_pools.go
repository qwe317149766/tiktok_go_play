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

// incrDevicePlayGet 写“播放次数”（play_count）并返回写入后的计数（用于达到阈值淘汰）。
func incrDevicePlayGet(poolID string, delta int64) (int64, error) {
	if strings.TrimSpace(poolID) == "" {
		return 0, fmt.Errorf("empty poolID")
	}
	rdb, err := getRedisClient()
	if err != nil {
		return 0, err
	}
	_, playKey := devicePoolCountKeys()
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	v, err := rdb.ZIncrBy(ctx, playKey, float64(delta), poolID).Result()
	if err != nil {
		return 0, err
	}
	if v < 0 {
		return 0, nil
	}
	return int64(v), nil
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

// loadDevicesFromRedisN 用于“按需取用”：
// - Redis 模式下不再一次性加载全量设备，而是启动时按需要数量（通常等于并发数）拉取 N 个设备到内存。
// - 会持续扫描 ids，直到拿到 N 条有效 data 或扫完为止。
func loadDevicesFromRedisN(target int) ([]string, error) {
	if target <= 0 {
		return []string{}, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()

	rdb, err := getRedisClient()
	if err != nil {
		return nil, fmt.Errorf("redis init: %w", err)
	}

	prefix := envStr("REDIS_DEVICE_POOL_KEY", "tiktok:device_pool")
	idsKey := prefix + ":ids"
	dataKey := prefix + ":data"

	out := make([]string, 0, target)
	seen := make(map[string]bool, target)

	var cursor uint64 = 0
	for page := 0; page < 200; page++ { // 上限保护：最多扫 200 页
		ids, next, err := rdb.SScan(ctx, idsKey, cursor, "*", 1000).Result()
		if err != nil {
			return nil, fmt.Errorf("redis sscan ids: %w", err)
		}
		cursor = next

		// 筛选本页候选
		fields := make([]string, 0, len(ids))
		for _, id := range ids {
			id = strings.TrimSpace(id)
			if id == "" || seen[id] {
				continue
			}
			seen[id] = true
			fields = append(fields, id)
		}
		if len(fields) > 0 {
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
				if len(out) >= target {
					return out, nil
				}
			}
		}

		if cursor == 0 {
			break
		}
	}

	if len(out) == 0 {
		return nil, fmt.Errorf("redis device pool empty or no valid data: %s", idsKey)
	}
	return out, nil
}

// pickOneDeviceFromRedis 从 Redis 设备池里挑一个“未在 exclude 里的设备”，用于坏设备补位。
// 返回：(poolID, deviceJSON)
func pickOneDeviceFromRedis(exclude map[string]bool) (string, string, error) {
	rdb, err := getRedisClient()
	if err != nil {
		return "", "", fmt.Errorf("redis init: %w", err)
	}

	prefix := envStr("REDIS_DEVICE_POOL_KEY", "tiktok:device_pool")
	idsKey := prefix + ":ids"
	dataKey := prefix + ":data"

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()

	var cursor uint64 = 0
	// 最多扫 50 页，避免极端情况下无限循环
	for i := 0; i < 50; i++ {
		ids, next, err := rdb.SScan(ctx, idsKey, cursor, "*", 1000).Result()
		if err != nil {
			return "", "", fmt.Errorf("redis sscan ids: %w", err)
		}
		for _, id := range ids {
			if strings.TrimSpace(id) == "" {
				continue
			}
			if exclude != nil && exclude[id] {
				continue
			}
			raw, err := rdb.HGet(ctx, dataKey, id).Result()
			if err != nil || strings.TrimSpace(raw) == "" {
				continue
			}
			return id, raw, nil
		}

		cursor = next
		if cursor == 0 {
			break
		}
	}
	return "", "", fmt.Errorf("no replacement device available in redis (all excluded or empty): %s", idsKey)
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
		// 兜底：如果 Redis cookie 池为空，允许使用默认 cookies（通过 env 提供）
		// DEFAULT_COOKIES_JSON 格式：{"sessionid":"...","sid_tt":"...","uid_tt":"...", ...}
		if raw := strings.TrimSpace(os.Getenv("DEFAULT_COOKIES_JSON")); raw != "" {
			var ck map[string]string
			if err := json.Unmarshal([]byte(raw), &ck); err == nil && len(ck) > 0 {
				return []CookieRecord{{ID: "default", Cookies: ck}}, nil
			}
			return nil, fmt.Errorf("redis startup cookie pool empty: %s；DEFAULT_COOKIES_JSON 解析失败或为空", idsKey)
		}
		return nil, fmt.Errorf(
			"redis startup cookie pool empty: %s；请先运行 goPlay/demos/signup/dgemail 产出 cookies 并写入 Redis，或配置 DEFAULT_COOKIES_JSON 作为兜底",
			idsKey,
		)
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

// -------- order progress (Linux 抢单模式：实时写 Redis) --------

func orderProgressKey(orderID string) string {
	prefix := envStr("REDIS_ORDER_PROGRESS_PREFIX", "tiktok:order_progress")
	return prefix + ":" + strings.TrimSpace(orderID)
}

// incrOrderDeliveredInRedis 实时更新 Redis 中订单完成量（worker 每成功一次就 +1）。
// 结构：HSET {prefix}:{order_id} delivered/total/updated_at
func incrOrderDeliveredInRedis(orderID string, delta int64, total int64) error {
	orderID = strings.TrimSpace(orderID)
	if orderID == "" || delta <= 0 {
		return nil
	}
	rdb, err := getRedisClient()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	key := orderProgressKey(orderID)
	pipe := rdb.Pipeline()
	pipe.HSetNX(ctx, key, "total", total)
	pipe.HIncrBy(ctx, key, "delivered", delta)
	pipe.HSet(ctx, key, "updated_at", time.Now().Unix())
	_, err = pipe.Exec(ctx)
	return err
}

func getOrderDeliveredFromRedis(orderID string) (delivered int64, total int64, ok bool, err error) {
	orderID = strings.TrimSpace(orderID)
	if orderID == "" {
		return 0, 0, false, nil
	}
	rdb, err := getRedisClient()
	if err != nil {
		return 0, 0, false, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	key := orderProgressKey(orderID)
	m, err := rdb.HGetAll(ctx, key).Result()
	if err != nil {
		return 0, 0, false, err
	}
	if len(m) == 0 {
		return 0, 0, false, nil
	}
	ok = true
	if v, ok2 := m["delivered"]; ok2 {
		if n, err2 := strconv.ParseInt(strings.TrimSpace(v), 10, 64); err2 == nil {
			delivered = n
		}
	}
	if v, ok2 := m["total"]; ok2 {
		if n, err2 := strconv.ParseInt(strings.TrimSpace(v), 10, 64); err2 == nil {
			total = n
		}
	}
	return delivered, total, ok, nil
}

func deleteOrderProgressInRedis(orderID string) error {
	orderID = strings.TrimSpace(orderID)
	if orderID == "" {
		return nil
	}
	rdb, err := getRedisClient()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return rdb.Del(ctx, orderProgressKey(orderID)).Err()
}


