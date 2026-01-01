package main

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"sync"
	"time"

	_ "github.com/go-sql-driver/mysql"
)

// 全 DB：stats 从 MySQL startup_cookie_accounts 读取账号池（每条 account_json 含 cookies 字段，device+cookies 同源）

func shouldLoadStartupAccountsFromDB() bool {
	v := strings.ToLower(strings.TrimSpace(envStr("DEVICES_SOURCE", "")))
	return v == "db" || v == "mysql" || v == "db_startup_accounts"
}

func dbCookiePoolTable() string {
	t := strings.TrimSpace(envStr("DB_COOKIE_POOL_TABLE", "startup_cookie_accounts"))
	if t == "" {
		return "startup_cookie_accounts"
	}
	return t
}

func dbCookiePoolShards() int {
	n := envInt("DB_COOKIE_POOL_SHARDS", 1)
	if n <= 0 {
		return 1
	}
	return n
}

var (
	statsDBOnce sync.Once
	statsDB     *sql.DB
	statsDBErr  error
)

func getStatsDB() (*sql.DB, error) {
	statsDBOnce.Do(func() {
		host := envStr("DB_HOST", "127.0.0.1")
		port := envStr("DB_PORT", "3306")
		user := envStr("DB_USER", "root")
		pass := envStr("DB_PASSWORD", "123456")
		name := envStr("DB_NAME", "tiktok_go_play")
		dsn := fmt.Sprintf("%s:%s@tcp(%s:%s)/%s?parseTime=true&charset=utf8mb4&collation=utf8mb4_unicode_ci&loc=Local",
			user, pass, host, port, name)
		db, err := sql.Open("mysql", dsn)
		if err != nil {
			statsDBErr = err
			return
		}
		db.SetMaxOpenConns(20)
		db.SetMaxIdleConns(5)
		db.SetConnMaxLifetime(30 * time.Minute)
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := db.PingContext(ctx); err != nil {
			_ = db.Close()
			statsDBErr = err
			return
		}
		statsDB = db
	})
	return statsDB, statsDBErr
}

func loadStartupAccountsFromDBN(target int) ([]string, error) {
	if target <= 0 {
		return []string{}, nil
	}
	db, err := getStatsDB()
	if err != nil {
		return nil, fmt.Errorf("mysql init: %w", err)
	}
	tbl := dbCookiePoolTable()
	shards := dbCookiePoolShards()
	if shards <= 0 {
		shards = 1
	}
	// 可选：只读某个 shard（用于多实例并行）
	shard := envInt("STATS_DB_COOKIE_SHARD", -1)

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	var rows *sql.Rows
	if shard >= 0 {
		// 按 device_create_time 排序（优先使用注册时间早的设备），如果为 NULL 则按 id 排序
		rows, err = db.QueryContext(ctx, fmt.Sprintf("SELECT account_json FROM `%s` WHERE shard_id=? ORDER BY device_create_time ASC, id ASC LIMIT ?", tbl), shard%shards, target)
	} else {
		// 按 device_create_time 排序（优先使用注册时间早的设备），如果为 NULL 则按 id 排序
		rows, err = db.QueryContext(ctx, fmt.Sprintf("SELECT account_json FROM `%s` ORDER BY device_create_time ASC, id ASC LIMIT ?", tbl), target)
	}
	if err != nil {
		return nil, fmt.Errorf("mysql query startup accounts: %w", err)
	}
	defer rows.Close()

	out := make([]string, 0, target)
	for rows.Next() {
		var raw string
		if err := rows.Scan(&raw); err != nil {
			continue
		}
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		out = append(out, raw)
		if len(out) >= target {
			break
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("mysql rows: %w", err)
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("startup cookie accounts empty: table=%s", tbl)
	}
	return out, nil
}

// updateCookieFailCountInDB 异步更新数据库中 cookie 的失败计数（不阻塞）
func updateCookieFailCountInDB(cookieID string, failCount int64) {
	if cookieID == "" || cookieID == "default" {
		return
	}
	db, err := getStatsDB()
	if err != nil {
		return // 静默失败，不影响主流程
	}
	tbl := dbCookiePoolTable()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	// 更新 fail_count 字段（如果表中有该字段）
	_, _ = db.ExecContext(ctx, fmt.Sprintf("UPDATE `%s` SET fail_count=? WHERE device_key=? OR JSON_EXTRACT(account_json, '$.device_id')=?", tbl), failCount, cookieID, cookieID)
}

// deleteDeviceFromDB 从数据库中删除设备（账号）
func deleteDeviceFromDB(deviceID string) error {
	if deviceID == "" || deviceID == "default" {
		return nil
	}
	db, err := getStatsDB()
	if err != nil {
		return err
	}
	tbl := dbCookiePoolTable()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// 尝试通过 device_key 删除，或者匹配 JSON 中的 device_id
	// 注意：startup_cookie_accounts 主键通常是 device_key
	query := fmt.Sprintf("DELETE FROM `%s` WHERE device_key=? OR JSON_EXTRACT(account_json, '$.device_id')=?", tbl)
	result, err := db.ExecContext(ctx, query, deviceID, deviceID)
	if err != nil {
		return err
	}
	affected, _ := result.RowsAffected()
	if affected > 0 {
		// optional: log
	}
	return nil
}
