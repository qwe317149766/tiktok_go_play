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

type APIKeyCache struct {
	rdb    *redis.Client
	hashKey string // e.g. tiktok:api_keys
}

func newRedisClientFromEnv() (*redis.Client, error) {
	if url := strings.TrimSpace(os.Getenv("REDIS_URL")); url != "" {
		opt, err := redis.ParseURL(url)
		if err != nil {
			return nil, err
		}
		return redis.NewClient(opt), nil
	}

	host := strings.TrimSpace(os.Getenv("REDIS_HOST"))
	if host == "" {
		host = "127.0.0.1"
	}
	port := 6379
	if v := strings.TrimSpace(os.Getenv("REDIS_PORT")); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			port = n
		}
	}
	db := 0
	if v := strings.TrimSpace(os.Getenv("REDIS_DB")); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			db = n
		}
	}
	user := strings.TrimSpace(os.Getenv("REDIS_USERNAME"))
	pass := strings.TrimSpace(os.Getenv("REDIS_PASSWORD"))
	useTLS := strings.TrimSpace(os.Getenv("REDIS_SSL"))

	opt := &redis.Options{
		Addr:     fmt.Sprintf("%s:%d", host, port),
		DB:       db,
		Username: user,
		Password: pass,
	}
	if useTLS == "1" || strings.EqualFold(useTLS, "true") {
		opt.TLSConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	}
	return redis.NewClient(opt), nil
}

func newAPIKeyCache() (*APIKeyCache, error) {
	rdb, err := newRedisClientFromEnv()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := rdb.Ping(ctx).Err(); err != nil {
		_ = rdb.Close()
		return nil, err
	}

	hashKey := strings.TrimSpace(os.Getenv("REDIS_API_KEYS_KEY"))
	if hashKey == "" {
		hashKey = "tiktok:api_keys"
	}
	return &APIKeyCache{rdb: rdb, hashKey: hashKey}, nil
}

func (c *APIKeyCache) Close() error {
	if c == nil || c.rdb == nil {
		return nil
	}
	return c.rdb.Close()
}

func (c *APIKeyCache) Get(ctx context.Context, apiKey string) (*APIKeyRow, bool, error) {
	if c == nil || c.rdb == nil {
		return nil, false, fmt.Errorf("redis not initialized")
	}
	raw, err := c.rdb.HGet(ctx, c.hashKey, apiKey).Result()
	if err == redis.Nil {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, err
	}
	var row APIKeyRow
	if err := json.Unmarshal([]byte(raw), &row); err != nil {
		// 反序列化失败：当成 miss，避免影响服务
		return nil, false, nil
	}
	return &row, true, nil
}

func (c *APIKeyCache) Set(ctx context.Context, row *APIKeyRow) error {
	if c == nil || c.rdb == nil {
		return fmt.Errorf("redis not initialized")
	}
	if row == nil || strings.TrimSpace(row.Key) == "" {
		return fmt.Errorf("invalid row")
	}
	b, _ := json.Marshal(row)
	// 永久缓存：不设置过期
	return c.rdb.HSet(ctx, c.hashKey, row.Key, string(b)).Err()
}


