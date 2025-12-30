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

// saveStartupCookiesToRedis 将 startUp 注册得到的 cookies 写入 Redis cookie 池
// key 结构：
// - {prefix}:ids  (SET)
// - {prefix}:data (HASH) id -> json(cookies map)
func saveStartupCookiesToRedis(all []RegisterResult) (int, error) {
	if !envBool("SAVE_STARTUP_COOKIES_TO_REDIS", false) {
		return 0, nil
	}

	target := envInt("STARTUP_REGISTER_COUNT", 0)
	if target <= 0 {
		// 默认：用 MAX_GENERATE 控制数量（与你统一配置一致）
		target = envInt("MAX_GENERATE", 0)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
	defer cancel()

	rdb, err := newRedisClient()
	if err != nil {
		return 0, fmt.Errorf("redis init: %w", err)
	}
	defer rdb.Close()
	if err := rdb.Ping(ctx).Err(); err != nil {
		return 0, fmt.Errorf("redis ping: %w", err)
	}

	prefix := envStr("REDIS_STARTUP_COOKIE_POOL_KEY", "tiktok:startup_cookie_pool")
	idsKey := prefix + ":ids"
	dataKey := prefix + ":data"

	wrote := 0
	for _, r := range all {
		if !r.Success || len(r.Cookies) == 0 {
			continue
		}
		id := cookieIDFromMap(r.Cookies)
		val, _ := json.Marshal(r.Cookies)

		pipe := rdb.TxPipeline()
		pipe.SAdd(ctx, idsKey, id)
		pipe.HSet(ctx, dataKey, id, string(val))
		if _, err := pipe.Exec(ctx); err != nil {
			return wrote, fmt.Errorf("redis write cookie: %w", err)
		}

		wrote++
		if target > 0 && wrote >= target {
			break
		}
	}
	return wrote, nil
}


