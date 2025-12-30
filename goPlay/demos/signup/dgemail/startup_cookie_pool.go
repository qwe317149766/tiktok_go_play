package main

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

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
	if !getEnvBool("SAVE_STARTUP_COOKIES_TO_REDIS", false) {
		return 0, nil
	}

	target := getEnvInt("STARTUP_REGISTER_COUNT", 0)
	if target <= 0 {
		// 默认：用 MAX_GENERATE 控制数量（与你统一配置一致）
		target = getEnvInt("MAX_GENERATE", 0)
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

	prefix := getEnvStr("REDIS_STARTUP_COOKIE_POOL_KEY", "tiktok:startup_cookie_pool")
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


