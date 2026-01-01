package main

import (
	"context"
	"crypto/sha1"
	"database/sql"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

func shouldLoadDevicesFromDB() bool {
	// signup 专用：SIGNUP_DEVICES_SOURCE=db/mysql
	if getEnvBool("SIGNUP_DEVICES_FROM_DB", false) {
		return true
	}
	v := strings.ToLower(strings.TrimSpace(getEnvStr("SIGNUP_DEVICES_SOURCE", "")))
	return v == "db" || v == "mysql"
}

func shouldWriteStartupAccountsToDB() bool {
	// signup 注册成功后把账号（含 cookies）写入 MySQL cookies 池
	if getEnvBool("SAVE_STARTUP_COOKIES_TO_DB", false) {
		return true
	}
	v := strings.ToLower(strings.TrimSpace(getEnvStr("COOKIES_SINK", "")))
	return v == "db" || v == "mysql"
}

func dbName() string {
	v := strings.TrimSpace(getEnvStr("DB_NAME", "tiktok_go_play"))
	if v == "" {
		return "tiktok_go_play"
	}
	return v
}

func devicePoolTable() string {
	v := strings.TrimSpace(getEnvStr("DB_DEVICE_POOL_TABLE", "device_pool_devices"))
	if v == "" {
		return "device_pool_devices"
	}
	return v
}

func cookiePoolTable() string {
	v := strings.TrimSpace(getEnvStr("DB_COOKIE_POOL_TABLE", "startup_cookie_accounts"))
	if v == "" {
		return "startup_cookie_accounts"
	}
	return v
}

func dbDeviceShards() int {
	n := getEnvInt("DB_DEVICE_POOL_SHARDS", 1)
	if n <= 0 {
		return 1
	}
	return n
}

func dbCookieShards() int {
	n := getEnvInt("DB_COOKIE_POOL_SHARDS", 1)
	if n <= 0 {
		return 1
	}
	return n
}

func stableShard(key string, shards int) int {
	key = strings.TrimSpace(key)
	if shards <= 1 || key == "" {
		return 0
	}
	sum := sha1.Sum([]byte(key))
	// 取前 4 bytes
	n := int(sum[0])<<24 | int(sum[1])<<16 | int(sum[2])<<8 | int(sum[3])
	if n < 0 {
		n = -n
	}
	return n % shards
}

func deviceKeyFromDeviceRaw(d map[string]interface{}) string {
	// 优先 device_id，其次 cdid
	if d == nil {
		return ""
	}
	if v := strings.TrimSpace(deviceIDFromAny(d)); v != "" {
		return v
	}
	if v, ok := d["cdid"].(string); ok && strings.TrimSpace(v) != "" {
		return strings.TrimSpace(v)
	}
	return ""
}

var (
	signupDBOnce sync.Once
	signupDB     *sql.DB
	signupDBErr  error
)

func getSignupDB() (*sql.DB, error) {
	signupDBOnce.Do(func() {
		host := getEnvStr("DB_HOST", "127.0.0.1")
		port := getEnvStr("DB_PORT", "3306")
		user := getEnvStr("DB_USER", "root")
		pass := getEnvStr("DB_PASSWORD", "123456")
		name := dbName()

		dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?parseTime=true&charset=utf8mb4&collation=utf8mb4_unicode_ci&loc=Local",
			user, pass, host, port, name)
		db, err := sql.Open("mysql", dsn)
		if err != nil {
			signupDBErr = err
			return
		}
		db.SetMaxOpenConns(10)
		db.SetMaxIdleConns(5)
		db.SetConnMaxLifetime(30 * time.Minute)
		ctx, cancel := contextWithTimeout(5 * time.Second)
		defer cancel()
		if err := db.PingContext(ctx); err != nil {
			_ = db.Close()
			signupDBErr = err
			return
		}
		signupDB = db
	})
	return signupDB, signupDBErr
}

func contextWithTimeout(d time.Duration) (context.Context, func()) {
	// 小工具，避免在多个文件引入 context 的重复样板
	type cancelFunc = func()
	ctx, cancel := context.WithTimeout(context.Background(), d)
	var cf cancelFunc = cancel
	return ctx, cf
}

// parseCreateTimeFromAccount 从账号 JSON 中解析 create_time 字段，返回 time.Time 或 nil
// 支持多种时间格式：
//   - "2006-01-02 15:04:05" (MySQL DATETIME 格式)
//   - "2006-01-02T15:04:05Z" (ISO 8601)
//   - RFC3339 格式
//   - Unix 时间戳（秒或毫秒，数字或字符串）
func parseCreateTimeFromAccount(acc map[string]interface{}) interface{} {
	if acc == nil {
		return nil
	}
	createTimeVal, ok := acc["create_time"]
	if !ok {
		return nil
	}

	// 字符串格式
	if createTimeStr, ok := createTimeVal.(string); ok {
		createTimeStr = strings.TrimSpace(createTimeStr)
		if createTimeStr == "" {
			return nil
		}

		// 尝试常见时间格式
		timeFormats := []string{
			"2006-01-02 15:04:05",       // MySQL DATETIME
			"2006-01-02T15:04:05",       // ISO 8601 (无时区)
			"2006-01-02T15:04:05Z",      // ISO 8601 (UTC)
			"2006-01-02T15:04:05Z07:00", // ISO 8601 (带时区)
			time.RFC3339,                // RFC3339
			time.RFC3339Nano,            // RFC3339Nano
		}
		for _, fmt := range timeFormats {
			if t, err := time.Parse(fmt, createTimeStr); err == nil {
				return t
			}
		}

		// 尝试 Unix 时间戳（字符串格式）
		if ts, err := strconv.ParseFloat(createTimeStr, 64); err == nil {
			return parseUnixTimestamp(ts)
		}
		return nil
	}

	// 数字格式（Unix 时间戳）
	if ts, ok := createTimeVal.(float64); ok {
		return parseUnixTimestamp(ts)
	}
	if ts, ok := createTimeVal.(int64); ok {
		return parseUnixTimestamp(float64(ts))
	}
	if ts, ok := createTimeVal.(int); ok {
		return parseUnixTimestamp(float64(ts))
	}

	return nil
}

// parseUnixTimestamp 解析 Unix 时间戳（秒或毫秒），返回 time.Time 或 nil
func parseUnixTimestamp(ts float64) interface{} {
	// 如果时间戳大于 1e10，认为是毫秒时间戳，需要转换为秒
	if ts > 1e10 {
		ts = ts / 1000
	}
	// 时间戳不能为负数或过大
	if ts < 0 || ts > 1e10 {
		return nil
	}
	t := time.Unix(int64(ts), 0)
	// 检查时间是否合理（1970-2100 之间）
	if t.Year() < 1970 || t.Year() > 2100 {
		return nil
	}
	return t
}

func loadDevicesFromDB(limit int) ([]map[string]interface{}, error) {
	if limit <= 0 {
		limit = getEnvInt("DEVICES_LIMIT", getEnvInt("MAX_GENERATE", 0))
	}
	if limit <= 0 {
		limit = 1000
	}
	db, err := getSignupDB()
	if err != nil {
		return nil, fmt.Errorf("mysql init: %w", err)
	}
	tbl := devicePoolTable()

	// 可选：固定 shard（多实例并行）
	shard := -1
	if v := strings.TrimSpace(os.Getenv("SIGNUP_DB_DEVICE_SHARD")); v != "" {
		if n, e := strconv.Atoi(v); e == nil && n >= 0 {
			shard = n
		}
	}

	// 获取最小年龄配置（小时）
	minAgeHours := getSignupDeviceMinAgeHours()

	// 构建 SQL 查询：在数据库层面按 device_create_time 过滤
	var query string
	var args []interface{}

	if shard >= 0 {
		// 指定 shard
		if minAgeHours > 0 {
			// 需要时间过滤：device_create_time <= NOW() - INTERVAL ? HOUR
			query = fmt.Sprintf("SELECT device_json FROM `%s` WHERE shard_id=? AND device_create_time IS NOT NULL AND device_create_time <= DATE_SUB(NOW(), INTERVAL ? HOUR) ORDER BY id ASC LIMIT ?", tbl)
			args = []interface{}{shard, minAgeHours, limit}
		} else {
			// 不需要时间过滤
			query = fmt.Sprintf("SELECT device_json FROM `%s` WHERE shard_id=? ORDER BY id ASC LIMIT ?", tbl)
			args = []interface{}{shard, limit}
		}
	} else {
		// 不指定 shard
		if minAgeHours > 0 {
			// 需要时间过滤
			query = fmt.Sprintf("SELECT device_json FROM `%s` WHERE device_create_time IS NOT NULL AND device_create_time <= DATE_SUB(NOW(), INTERVAL ? HOUR) ORDER BY id ASC LIMIT ?", tbl)
			args = []interface{}{minAgeHours, limit}
		} else {
			// 不需要时间过滤
			query = fmt.Sprintf("SELECT device_json FROM `%s` ORDER BY id ASC LIMIT ?", tbl)
			args = []interface{}{limit}
		}
	}

	rows, err := db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("mysql query devices: %w", err)
	}
	defer rows.Close()

	out := make([]map[string]interface{}, 0, limit)
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			continue
		}
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		var m map[string]interface{}
		if err := json.Unmarshal([]byte(raw), &m); err != nil {
			continue
		}
		out = append(out, m)
		if len(out) >= limit {
			break
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("mysql rows: %w", err)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("mysql device pool empty: table=%s (min_age_hours=%d)", tbl, minAgeHours)
	}
	return out, nil
}

func writeStartupAccountToDB(acc map[string]interface{}) error {
	if acc == nil {
		return nil
	}
	db, err := getSignupDB()
	if err != nil {
		return err
	}
	tbl := cookiePoolTable()
	deviceKey := strings.TrimSpace(deviceKeyFromDeviceRaw(acc))
	if deviceKey == "" {
		return fmt.Errorf("startup account missing device_id/cdid")
	}
	// cookies 必须存在
	if v, ok := acc["cookies"]; !ok || strings.TrimSpace(fmt.Sprintf("%v", v)) == "" {
		return fmt.Errorf("startup account missing cookies")
	}
	b, _ := json.Marshal(acc)
	raw := string(b)

	shards := dbCookieShards()
	// poll 补齐模式：允许强制写入指定 shard（用于“每个 cookies 池缺口”按池补齐）
	// - DGEMAIL_COOKIE_SHARD: >=0 时强制使用该 shard
	// - 否则按稳定 hash 分片
	forceShard := -1
	if v := strings.TrimSpace(os.Getenv("DGEMAIL_COOKIE_SHARD")); v != "" {
		if n, e := strconv.Atoi(v); e == nil && n >= 0 {
			forceShard = n
		}
	}
	shard := stableShard(deviceKey, shards)
	if forceShard >= 0 {
		if shards <= 1 {
			shard = 0
		} else {
			shard = forceShard % shards
		}
	}

	// 提取 create_time 并转换为 TIMESTAMP（用于 device_create_time 字段）
	deviceCreateTime := parseCreateTimeFromAccount(acc)

	// upsert（包含 device_create_time 字段）
	if deviceCreateTime != nil {
		_, err = db.Exec(
			fmt.Sprintf("INSERT INTO `%s` (shard_id, device_key, account_json, device_create_time) VALUES (?,?,?,?) ON DUPLICATE KEY UPDATE account_json=VALUES(account_json), device_create_time=VALUES(device_create_time), updated_at=CURRENT_TIMESTAMP", tbl),
			shard, deviceKey, raw, deviceCreateTime,
		)
		// 如果因为 unknown column 报错，则降级重试
		if err != nil && (strings.Contains(err.Error(), "Unknown column") || strings.Contains(err.Error(), "device_create_time")) {
			// Fallback: 尝试不带 device_create_time
			_, err = db.Exec(
				fmt.Sprintf("INSERT INTO `%s` (shard_id, device_key, account_json) VALUES (?,?,?) ON DUPLICATE KEY UPDATE account_json=VALUES(account_json), updated_at=CURRENT_TIMESTAMP", tbl),
				shard, deviceKey, raw,
			)
		}
	} else {
		// 如果没有 create_time，不更新 device_create_time 字段（保持 NULL 或旧值）
		_, err = db.Exec(
			fmt.Sprintf("INSERT INTO `%s` (shard_id, device_key, account_json) VALUES (?,?,?) ON DUPLICATE KEY UPDATE account_json=VALUES(account_json), updated_at=CURRENT_TIMESTAMP", tbl),
			shard, deviceKey, raw,
		)
	}
	return err
}

func deleteDeviceFromDBPool(deviceKey string) error {
	deviceKey = strings.TrimSpace(deviceKey)
	if deviceKey == "" {
		return nil
	}
	db, err := getSignupDB()
	if err != nil {
		return err
	}
	tbl := devicePoolTable()
	_, err = db.Exec(fmt.Sprintf("DELETE FROM `%s` WHERE device_id=?", tbl), deviceKey)
	if err == nil {
		return nil
	}
	// 兼容：如果写入端用的是 cdid 当 device_id，则上面能删；否则再尝试按 JSON 字段不可靠，这里只做二次尝试：device_key==cdid 时依然走 device_id 列
	return err
}
