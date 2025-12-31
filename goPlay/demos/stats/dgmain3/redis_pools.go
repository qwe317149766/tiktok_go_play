package main

import (
	"context"
	"crypto/sha1"
	"crypto/tls"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

func GetDeviceMinAgeHours() int {
	// stats 项目：按你的要求【不需要】按时间过滤设备（无论 file/redis/source）。
	// 为避免“设备少但 cookies 多”的不一致，这里统一关闭过滤。
	//
	// 说明：STATS_DEVICE_MIN_AGE_HOURS 仍可能存在于 env 配置里，但会被忽略。
	return 0
}

func extractJSONStringFieldFast(jsonStr string, field string) (string, bool) {
	// 轻量级提取：只支持 string 字段，例如 "create_time":"2025-12-31 12:00:00"
	// 找不到/格式不符合则返回 false，由上层回退到 json.Unmarshal
	if strings.TrimSpace(jsonStr) == "" || strings.TrimSpace(field) == "" {
		return "", false
	}
	key := `"` + field + `"`
	i := strings.Index(jsonStr, key)
	if i < 0 {
		return "", false
	}
	// 找到冒号
	j := strings.Index(jsonStr[i+len(key):], ":")
	if j < 0 {
		return "", false
	}
	k := i + len(key) + j + 1
	// 跳过空白
	for k < len(jsonStr) && (jsonStr[k] == ' ' || jsonStr[k] == '\t' || jsonStr[k] == '\r' || jsonStr[k] == '\n') {
		k++
	}
	// 期待字符串
	if k >= len(jsonStr) || jsonStr[k] != '"' {
		return "", false
	}
	k++
	// 取到下一个未转义引号
	start := k
	escaped := false
	for k < len(jsonStr) {
		c := jsonStr[k]
		if escaped {
			escaped = false
			k++
			continue
		}
		if c == '\\' {
			escaped = true
			k++
			continue
		}
		if c == '"' {
			return jsonStr[start:k], true
		}
		k++
	}
	return "", false
}

func devicePassMinAge(deviceJSON string, minAgeHours int) bool {
	if minAgeHours <= 0 {
		return true
	}
	deviceJSON = strings.TrimSpace(deviceJSON)
	if deviceJSON == "" {
		return false
	}

	// 优先走快速路径：避免大量 json.Unmarshal 带来的 CPU 压力
	ctStr, ok := extractJSONStringFieldFast(deviceJSON, "create_time")
	if !ok {
		// 回退：设备 JSON 里有 create_time: "YYYY-MM-DD HH:MM:SS"
		var m map[string]any
		if err := json.Unmarshal([]byte(deviceJSON), &m); err != nil {
			return false
		}
		ctRaw, ok2 := m["create_time"]
		if !ok2 {
			return false
		}
		ctStr, ok2 = ctRaw.(string)
		if !ok2 {
			return false
		}
		ctStr = strings.TrimSpace(ctStr)
	}
	ctStr = strings.TrimSpace(ctStr)
	if ctStr == "" {
		return false
	}
	t, err := time.ParseInLocation("2006-01-02 15:04:05", ctStr, time.Local)
	if err != nil {
		return false
	}
	threshold := time.Now().Add(-time.Duration(minAgeHours) * time.Hour)
	return t.Before(threshold) || t.Equal(threshold)
}

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

func envSec(name string, defSec int) time.Duration {
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
	// 读取设备/读取cookies 这类“批量扫描+HMGET”的操作更慢，单独给更大的 timeout
	// 默认 120 秒，可通过 env 覆盖
	return envSec("REDIS_LOAD_TIMEOUT_SEC", 120)
}

func newRedisClient() (*redis.Client, error) {
	// 可配置超时（避免 HMGET 在高并发/弱网络下出现 i/o timeout）
	dialTO := envSec("REDIS_DIAL_TIMEOUT_SEC", 10)
	readTO := envSec("REDIS_READ_TIMEOUT_SEC", 20)
	writeTO := envSec("REDIS_WRITE_TIMEOUT_SEC", 20)
	poolTO := envSec("REDIS_POOL_TIMEOUT_SEC", 30)

	if url := strings.TrimSpace(os.Getenv("REDIS_URL")); url != "" {
		opt, err := redis.ParseURL(url)
		if err != nil {
			return nil, err
		}
		// 覆盖默认超时（ParseURL 可能带默认值过小）
		opt.DialTimeout = dialTO
		opt.ReadTimeout = readTO
		opt.WriteTimeout = writeTO
		opt.PoolTimeout = poolTO
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

func shouldLoadDevicesFromStartupRedis() bool {
	// 显式指定从“startup 注册成功写入的设备池”读取设备
	if envBool("DEVICES_FROM_STARTUP_REDIS", false) {
		return true
	}
	if strings.EqualFold(envStr("DEVICES_SOURCE", ""), "startup_redis") {
		return true
	}
	return false
}

func shouldLoadDevicesFromStartupCookieRedis() bool {
	// 你要求的“账号池模式”：设备+cookies 都来自 startup_cookie_pool:data 中的完整账号 JSON
	if envBool("DEVICES_FROM_STARTUP_COOKIE_REDIS", false) {
		return true
	}
	if strings.EqualFold(envStr("DEVICES_SOURCE", ""), "startup_cookie_redis") {
		return true
	}
	return false
}

func shouldLoadDevicesFromRedis() bool {
	// startup_redis 是 redis 的一个子类型：仍然走同一套 redis 读取逻辑，只是 prefix 不同
	if shouldLoadDevicesFromStartupRedis() {
		return true
	}
	// startup_cookie_redis 也是 redis 的一种：但读取逻辑不是 device_pool，而是 startup_cookie_pool
	if shouldLoadDevicesFromStartupCookieRedis() {
		return true
	}
	if envBool("DEVICES_FROM_REDIS", false) {
		return true
	}
	if strings.EqualFold(envStr("DEVICES_SOURCE", ""), "redis") {
		return true
	}
	return false
}

func startupDevicePoolPrefix() string {
	// 默认与 REDIS_DEVICE_POOL_KEY 一致；若你希望“只用 startup 成功设备”，可配置独立 key
	if p := strings.TrimSpace(envStr("REDIS_STARTUP_DEVICE_POOL_KEY", "")); p != "" {
		return p
	}
	return envStr("REDIS_DEVICE_POOL_KEY", "tiktok:device_pool")
}

func devicePoolKeys() (idsKey, dataKey, useKey, failKey string) {
	prefix := envStr("REDIS_DEVICE_POOL_KEY", "tiktok:device_pool")
	if shouldLoadDevicesFromStartupRedis() {
		prefix = startupDevicePoolPrefix()
	}
	return prefix + ":ids", prefix + ":data", prefix + ":use", prefix + ":fail"
}

func devicePoolOrderKeys() (seqKey, inKey string) {
	prefix := envStr("REDIS_DEVICE_POOL_KEY", "tiktok:device_pool")
	if shouldLoadDevicesFromStartupRedis() {
		prefix = startupDevicePoolPrefix()
	}
	// 约定：Python 注册成功写入时维护 :seq(自增) 与 :in(ZSET FIFO)
	// 如未维护，则 Go 会自动回退到其它取数方式（随机/扫描）
	return prefix + ":seq", prefix + ":in"
}

func devicePoolCountKeys() (attemptKey, playKey string) {
	prefix := envStr("REDIS_DEVICE_POOL_KEY", "tiktok:device_pool")
	if shouldLoadDevicesFromStartupRedis() {
		prefix = startupDevicePoolPrefix()
	}
	return prefix + ":attempt", prefix + ":play"
}

func redisDeviceSampleFactor() int {
	// minAge 过滤/空洞数据时会导致“拿到的候选不够”，这里用抽样倍数提高命中率
	// 默认 3，建议 2~10
	f := envInt("REDIS_DEVICE_SAMPLE_FACTOR", 3)
	if f < 1 {
		return 1
	}
	if f > 20 {
		return 20
	}
	return f
}

func redisDeviceSampleRounds() int {
	// 采样轮数上限：防止极端情况下无限尝试
	r := envInt("REDIS_DEVICE_SAMPLE_ROUNDS", 10)
	if r < 1 {
		return 1
	}
	if r > 200 {
		return 200
	}
	return r
}

func sampleDeviceIDsFast(ctx context.Context, rdb *redis.Client, idsKey, inKey, useKey string, need int, inOffset *int64) ([]string, error) {
	// 优先从 inKey(ZSET FIFO) 按入队顺序拿；没有则从 idsKey(SET) 随机抽样；最后才 SSCAN
	// 返回的 ids 可能包含重复/无效，调用方再去重/排除
	if need <= 0 {
		return []string{}, nil
	}
	// 1) ZRANGE(inKey) 快速拿一批（严格 FIFO：越早入队越先取）
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
		// err 不直接返回：可能 key 不存在或临时错误；继续走随机抽样兜底
	}
	// 2) ZRANGE(useKey)（兼容旧策略：没有 FIFO 队列时，可用 useKey 作为稳定取数来源）
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
	// 3) SRANDMEMBER(idsKey) 随机抽样（避免 SSCAN 10 万）
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
	ctx, cancel := context.WithTimeout(context.Background(), redisLoadTimeout())
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

	ctx, cancel := context.WithTimeout(context.Background(), redisLoadTimeout())
	defer cancel()

	rdb, err := getRedisClient()
	if err != nil {
		return nil, fmt.Errorf("redis init: %w", err)
	}

	idsKey, dataKey, useKey, _ := devicePoolKeys()
	_, inKey := devicePoolOrderKeys()

	out := make([]string, 0, target)
	seen := make(map[string]bool, target)
	minAge := GetDeviceMinAgeHours()
	skippedTooNew := 0
	// 分批 HMGET，避免一次 HMGET 太大导致 i/o timeout
	chunk := envInt("REDIS_HMGET_CHUNK", 200)
	if chunk <= 0 {
		chunk = 200
	}
	sampleFactor := redisDeviceSampleFactor()
	rounds := redisDeviceSampleRounds()
	var inOff int64 = 0

	for round := 0; round < rounds && len(out) < target; round++ {
		need := (target - len(out)) * sampleFactor
		if need < 50 {
			need = 50
		}
		if need > 5000 {
			need = 5000
		}
		ids, err := sampleDeviceIDsFast(ctx, rdb, idsKey, inKey, useKey, need, &inOff)
		if err != nil {
			// 采样失败就回退错误，避免静默卡死
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
				if !devicePassMinAge(s, minAge) {
					skippedTooNew++
					continue
				}
				out = append(out, s)
				if len(out) >= target {
					return out, nil
				}
			}
		}
	}

	if len(out) == 0 {
		if minAge > 0 {
			return nil, fmt.Errorf("redis device pool empty or no valid data after minAgeHours=%d filter (skipped=%d): %s", minAge, skippedTooNew, idsKey)
		}
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

	idsKey, dataKey, useKey, _ := devicePoolKeys()
	_, inKey := devicePoolOrderKeys()

	ctx, cancel := context.WithTimeout(context.Background(), redisLoadTimeout())
	defer cancel()
	minAge := GetDeviceMinAgeHours()
	chunk := envInt("REDIS_HMGET_CHUNK", 200)
	if chunk <= 0 {
		chunk = 200
	}
	rounds := redisDeviceSampleRounds()
	// 替换场景：更倾向快速拿到 1 个可用设备
	pickBatch := envInt("REDIS_DEVICE_PICK_BATCH", 200)
	if pickBatch < 20 {
		pickBatch = 20
	}
	if pickBatch > 2000 {
		pickBatch = 2000
	}
	var inOff int64 = 0

	for round := 0; round < rounds; round++ {
		ids, err := sampleDeviceIDsFast(ctx, rdb, idsKey, inKey, useKey, pickBatch, &inOff)
		if err != nil {
			return "", "", fmt.Errorf("redis sample ids: %w", err)
		}
		if len(ids) == 0 {
			break
		}
		fields := make([]string, 0, len(ids))
		for _, id := range ids {
			id = strings.TrimSpace(id)
			if id == "" {
				continue
			}
			if exclude != nil && exclude[id] {
				continue
			}
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
				return "", "", fmt.Errorf("redis hmget data: %w", err)
			}
			for idx, v := range vals {
				raw, ok := v.(string)
				if !ok || strings.TrimSpace(raw) == "" {
					continue
				}
				if !devicePassMinAge(raw, minAge) {
					continue
				}
				return sub[idx], raw, nil
			}
		}
	}
	if minAge > 0 {
		return "", "", fmt.Errorf("no replacement device available in redis (all excluded/too new/empty) minAgeHours=%d: %s", minAge, idsKey)
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

func shouldLoadCookiesFromDevicesFile() bool {
	// 新模式：从“设备文件每行的 cookies 字段”构建 cookie 池（startUp 导出的设备文件）
	// 典型用法：
	// - DEVICES_SOURCE=file
	// - STATS_DEVICES_FILE=goPlay/demos/signup/dgemail/res/devices1221/devices12_21_3.txt
	// - COOKIES_SOURCE=devices_file
	return strings.EqualFold(envStr("COOKIES_SOURCE", ""), "devices_file") ||
		strings.EqualFold(envStr("COOKIES_SOURCE", ""), "startup_devices_file")
}

type CookieRecord struct {
	ID      string            `json:"id"`
	Cookies map[string]string `json:"cookies"`
}

func defaultCookieFromEnv() (CookieRecord, bool, error) {
	// DEFAULT_COOKIES_JSON 格式：{"sessionid":"...","sid_tt":"...","uid_tt":"...", ...}
	raw := strings.TrimSpace(os.Getenv("DEFAULT_COOKIES_JSON"))
	if raw == "" {
		return CookieRecord{}, false, nil
	}
	var ck map[string]string
	if err := json.Unmarshal([]byte(raw), &ck); err != nil || len(ck) == 0 {
		return CookieRecord{}, false, fmt.Errorf("DEFAULT_COOKIES_JSON 解析失败或为空")
	}
	return CookieRecord{ID: "default", Cookies: ck}, true, nil
}

func loadStartupCookiesFromRedis(limit int) ([]CookieRecord, error) {
	ctx, cancel := context.WithTimeout(context.Background(), redisLoadTimeout())
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
		if rec, ok, err := defaultCookieFromEnv(); err == nil && ok {
			return []CookieRecord{rec}, nil
		} else if err != nil {
			return nil, fmt.Errorf("redis startup cookie pool empty: %s；%v", idsKey, err)
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
			// 兼容两种格式：
			// 1) 旧：data=id->json(cookies map[string]string)
			// 2) 新：data=id->json(account)，account 里包含 cookies 字段（可能是 python dict string）
			var ck map[string]string
			if err := json.Unmarshal([]byte(s), &ck); err == nil && len(ck) > 0 {
				out = append(out, CookieRecord{ID: fields[idx], Cookies: ck})
			} else {
				var m map[string]any
				if err := json.Unmarshal([]byte(s), &m); err != nil {
					continue
				}
				raw, ok := m["cookies"]
				if !ok {
					continue
				}
				ck2 := parseCookiesAny(raw)
				if len(ck2) == 0 {
					continue
				}
				out = append(out, CookieRecord{ID: fields[idx], Cookies: ck2})
			}
			if limit > 0 && len(out) >= limit {
				return out, nil
			}
		}
	}
	return out, nil
}

// pickOneStartupCookieFromRedis 从 Redis cookie 池里挑一个“未在 exclude 里的 cookies”，用于连续失败后更换。
func pickOneStartupCookieFromRedis(exclude map[string]bool) (CookieRecord, error) {
	rdb, err := getRedisClient()
	if err != nil {
		return CookieRecord{}, fmt.Errorf("redis init: %w", err)
	}
	prefix := envStr("REDIS_STARTUP_COOKIE_POOL_KEY", "tiktok:startup_cookie_pool")
	idsKey := prefix + ":ids"
	dataKey := prefix + ":data"

	ctx, cancel := context.WithTimeout(context.Background(), redisLoadTimeout())
	defer cancel()

	var cursor uint64 = 0
	for i := 0; i < 50; i++ {
		ids, next, err := rdb.SScan(ctx, idsKey, cursor, "*", 1000).Result()
		if err != nil {
			return CookieRecord{}, fmt.Errorf("redis sscan cookie ids: %w", err)
		}
		for _, id := range ids {
			id = strings.TrimSpace(id)
			if id == "" || id == "default" {
				continue
			}
			if exclude != nil && exclude[id] {
				continue
			}
			raw, err := rdb.HGet(ctx, dataKey, id).Result()
			if err != nil || strings.TrimSpace(raw) == "" {
				continue
			}
			// 兼容：纯 cookies / 完整账号 JSON
			var ck map[string]string
			if err := json.Unmarshal([]byte(raw), &ck); err == nil && len(ck) > 0 {
				return CookieRecord{ID: id, Cookies: ck}, nil
			}
			var m map[string]any
			if err := json.Unmarshal([]byte(raw), &m); err != nil {
				continue
			}
			ck2 := parseCookiesAny(m["cookies"])
			if len(ck2) == 0 {
				continue
			}
			return CookieRecord{ID: id, Cookies: ck2}, nil
		}
		cursor = next
		if cursor == 0 {
			break
		}
	}
	return CookieRecord{}, fmt.Errorf("no replacement cookie available in redis (all excluded or empty): %s", idsKey)
}

// loadStartupAccountDevicesFromRedisN 从“账号池(startup_cookie_pool)”读取 N 条完整账号 JSON（每条包含设备字段 + cookies 字段）
// 注意：这是你要求的 stats 设备来源，不再读取 device_pool。
func loadStartupAccountDevicesFromRedisN(target int) ([]string, error) {
	if target <= 0 {
		return []string{}, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), redisLoadTimeout())
	defer cancel()

	rdb, err := getRedisClient()
	if err != nil {
		return nil, fmt.Errorf("redis init: %w", err)
	}

	prefix := envStr("REDIS_STARTUP_COOKIE_POOL_KEY", "tiktok:startup_cookie_pool")
	idsKey := prefix + ":ids"
	dataKey := prefix + ":data"

	out := make([]string, 0, target)
	seen := make(map[string]bool, target)

	// 分批 HMGET，避免一次过大导致 i/o timeout
	chunk := envInt("REDIS_HMGET_CHUNK", 200)
	if chunk <= 0 {
		chunk = 200
	}
	// 抽样倍数：账号池可能有空洞/坏数据
	sampleFactor := envInt("REDIS_ACCOUNT_SAMPLE_FACTOR", 3)
	if sampleFactor < 1 {
		sampleFactor = 1
	}
	if sampleFactor > 20 {
		sampleFactor = 20
	}
	rounds := envInt("REDIS_ACCOUNT_SAMPLE_ROUNDS", 10)
	if rounds < 1 {
		rounds = 1
	}
	if rounds > 200 {
		rounds = 200
	}

	for r := 0; r < rounds && len(out) < target; r++ {
		needIDs := (target - len(out)) * sampleFactor
		if needIDs <= 0 {
			break
		}
		ids, err := rdb.SRandMemberN(ctx, idsKey, int64(needIDs)).Result()
		if err != nil || len(ids) == 0 {
			// 兜底：SSCAN
			var cursor uint64 = 0
			for i := 0; i < 50 && len(ids) < needIDs; i++ {
				got, next, e := rdb.SScan(ctx, idsKey, cursor, "*", int64(needIDs)).Result()
				if e != nil {
					break
				}
				ids = append(ids, got...)
				cursor = next
				if cursor == 0 {
					break
				}
			}
		}
		// 去重 + 过滤空
		cands := make([]string, 0, len(ids))
		for _, id := range ids {
			id = strings.TrimSpace(id)
			if id == "" {
				continue
			}
			if seen[id] {
				continue
			}
			seen[id] = true
			cands = append(cands, id)
		}
		if len(cands) == 0 {
			continue
		}
		for i := 0; i < len(cands) && len(out) < target; i += chunk {
			end := i + chunk
			if end > len(cands) {
				end = len(cands)
			}
			fields := cands[i:end]
			vals, e := rdb.HMGet(ctx, dataKey, fields...).Result()
			if e != nil {
				return nil, fmt.Errorf("redis hmget startup account data: %w", e)
			}
			for _, v := range vals {
				s, ok := v.(string)
				if !ok || strings.TrimSpace(s) == "" {
					continue
				}
				// 只接受“带 cookies 字段”的账号 JSON（新格式）；旧格式纯 cookies 不作为设备来源
				var m map[string]any
				if err := json.Unmarshal([]byte(s), &m); err != nil {
					continue
				}
				if _, ok := m["cookies"]; !ok {
					continue
				}
				out = append(out, s)
				if len(out) >= target {
					break
				}
			}
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("startup account pool empty or invalid: %s", idsKey)
	}
	return out, nil
}

func incrStartupCookieUseInRedis(cookieID string, delta int64) error {
	cookieID = strings.TrimSpace(cookieID)
	if cookieID == "" || cookieID == "default" || delta == 0 {
		return nil
	}
	rdb, err := getRedisClient()
	if err != nil {
		return err
	}
	prefix := envStr("REDIS_STARTUP_COOKIE_POOL_KEY", "tiktok:startup_cookie_pool")
	useKey := prefix + ":use"
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return rdb.ZIncrBy(ctx, useKey, float64(delta), cookieID).Err()
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

var rePyCookiePair = regexp.MustCompile(`'([^']+)'\s*:\s*'([^']*)'`)

func parseCookiesAny(v any) map[string]string {
	// 支持：
	// 1) JSON object: {"k":"v"}
	// 2) Python dict string: "{'k':'v', 'k2':'v2'}"（你给的 devices12_21_3.txt 就是这种）
	// 3) Cookie header: "k=v; k2=v2"
	switch t := v.(type) {
	case map[string]string:
		if len(t) == 0 {
			return nil
		}
		out := make(map[string]string, len(t))
		for k, v := range t {
			k = strings.TrimSpace(k)
			if k == "" {
				continue
			}
			out[k] = strings.TrimSpace(v)
		}
		if len(out) == 0 {
			return nil
		}
		return out
	case map[string]any:
		if len(t) == 0 {
			return nil
		}
		out := make(map[string]string, len(t))
		for k, vv := range t {
			k = strings.TrimSpace(k)
			if k == "" {
				continue
			}
			if s, ok := vv.(string); ok {
				out[k] = strings.TrimSpace(s)
			}
		}
		if len(out) == 0 {
			return nil
		}
		return out
	case string:
		s := strings.TrimSpace(t)
		if s == "" {
			return nil
		}
		// JSON object
		if strings.HasPrefix(s, "{") && strings.HasSuffix(s, "}") && strings.Contains(s, "\":") {
			var m map[string]string
			if err := json.Unmarshal([]byte(s), &m); err == nil && len(m) > 0 {
				return m
			}
			var m2 map[string]any
			if err := json.Unmarshal([]byte(s), &m2); err == nil && len(m2) > 0 {
				return parseCookiesAny(m2)
			}
		}
		// Python dict string
		if strings.HasPrefix(s, "{") && strings.HasSuffix(s, "}") && strings.Contains(s, "':") {
			matches := rePyCookiePair.FindAllStringSubmatch(s, -1)
			if len(matches) == 0 {
				return nil
			}
			out := make(map[string]string, len(matches))
			for _, mm := range matches {
				if len(mm) != 3 {
					continue
				}
				k := strings.TrimSpace(mm[1])
				if k == "" {
					continue
				}
				out[k] = strings.TrimSpace(mm[2])
			}
			if len(out) == 0 {
				return nil
			}
			return out
		}
		// Cookie header
		if strings.Contains(s, "=") {
			out := map[string]string{}
			parts := strings.Split(s, ";")
			for _, p := range parts {
				p = strings.TrimSpace(p)
				if p == "" {
					continue
				}
				kv := strings.SplitN(p, "=", 2)
				if len(kv) != 2 {
					continue
				}
				k := strings.TrimSpace(kv[0])
				if k == "" {
					continue
				}
				out[k] = strings.TrimSpace(kv[1])
			}
			if len(out) == 0 {
				return nil
			}
			return out
		}
		return nil
	default:
		return nil
	}
}

func cookiesFromStartupDeviceJSONLine(line string) (CookieRecord, bool) {
	line = strings.TrimSpace(line)
	if line == "" {
		return CookieRecord{}, false
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(line), &m); err != nil {
		return CookieRecord{}, false
	}
	raw, ok := m["cookies"]
	if !ok {
		return CookieRecord{}, false
	}
	ck := parseCookiesAny(raw)
	if len(ck) == 0 {
		return CookieRecord{}, false
	}
	id := cookieIDFromMap(ck)
	return CookieRecord{ID: id, Cookies: ck}, true
}

func loadCookiesFromStartupDevices(lines []string, limit int) []CookieRecord {
	out := make([]CookieRecord, 0, len(lines))
	seen := map[string]bool{}
	for _, line := range lines {
		rec, ok := cookiesFromStartupDeviceJSONLine(line)
		if !ok || rec.ID == "" {
			continue
		}
		if seen[rec.ID] {
			continue
		}
		seen[rec.ID] = true
		out = append(out, rec)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out
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


