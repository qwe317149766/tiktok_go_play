package main

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

type Config struct {
	Addr   string

	DBHost string
	DBPort int
	DBUser string
	DBPass string
	DBName string

	AdminPasswordMD5 string
}

func getenv(key, def string) string {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	return v
}

func getenvInt(key string, def int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

func loadConfig() Config {
	// 按你要求：默认本地库 root/123456 tiktok_play
	return Config{
		Addr:   getenv("API_ADDR", ":8080"),

		DBHost: getenv("DB_HOST", "127.0.0.1"),
		DBPort: getenvInt("DB_PORT", 3306),
		DBUser: getenv("DB_USER", "root"),
		DBPass: getenv("DB_PASSWORD", "123456"),
		DBName: getenv("DB_NAME", "tiktok_play"),

		// 管理后台密码：存放“明文密码的 MD5(hex小写)”
		AdminPasswordMD5: strings.ToLower(getenv("ADMIN_PASSWORD_MD5", "")),
	}
}

func (c Config) MySQLDSN() string {
	// parseTime 用于扫描 TIMESTAMP；utf8mb4 避免字符集问题
	return fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?parseTime=true&charset=utf8mb4&loc=Local",
		c.DBUser, c.DBPass, c.DBHost, c.DBPort, c.DBName,
	)
}


