package main

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strings"
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

func deviceIDFromAny(m map[string]interface{}) string {
	if m == nil {
		return ""
	}
	if v, ok := m["device_id"]; ok {
		switch t := v.(type) {
		case string:
			return strings.TrimSpace(t)
		case float64:
			return fmt.Sprintf("%.0f", t)
		case int:
			return fmt.Sprintf("%d", t)
		case int64:
			return fmt.Sprintf("%d", t)
		default:
			return strings.TrimSpace(fmt.Sprintf("%v", t))
		}
	}
	return ""
}

// saveStartupCookiesToRedis 将 startUp 注册成功的“账号数据”写入 Redis 账号池（沿用 startup_cookie_pool 的 key 结构）
// 账号数据格式：完整设备字段 + cookies 字段（cookies 是该 JSON 的一个字段）
// key 结构：
// - {prefix}:ids  (SET)
// - {prefix}:data (HASH) id -> json(account)  (account 里包含 cookies 字段)
func saveStartupCookiesToRedis(accounts []map[string]interface{}, targetOverride int) (int, error) {
	if !getEnvBool("SAVE_STARTUP_COOKIES_TO_REDIS", false) {
		return 0, nil
	}

	target := targetOverride
	if target <= 0 {
		target = getEnvInt("STARTUP_REGISTER_COUNT", 0)
	}
	if target <= 0 {
		// 默认：用 MAX_GENERATE 控制数量（与你统一配置一致）
		target = getEnvInt("MAX_GENERATE", 0)
	}

	// 对齐 stats/dgemail poll：统一使用可配置的 bulk load timeout，避免大批量写入时超时
	ctx, cancel := context.WithTimeout(context.Background(), redisLoadTimeout())
	defer cancel()

	rdb, err := newRedisClient()
	if err != nil {
		return 0, fmt.Errorf("redis init: %w", err)
	}
	defer rdb.Close()

	wrote := 0
	// 为了复用“按 shard 分组 pipeline”的实现，这里按 target 做切片上限，然后统一写入
	if target > 0 && len(accounts) > target {
		accounts = accounts[:target]
	}
	n, err := writeStartupAccountsToRedisWithClient(ctx, rdb, accounts)
	wrote += n
	if err != nil {
		return wrote, err
	}
	return wrote, nil
}


