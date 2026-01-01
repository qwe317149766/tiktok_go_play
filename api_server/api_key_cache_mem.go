package main

import (
	"context"
	"sync"
	"time"
)

// APIKeyCache: 全 DB 模式下使用进程内 TTL 缓存（跨进程不共享）。
type APIKeyCache struct {
	mu   sync.RWMutex
	ttl  time.Duration
	data map[string]cacheItem
}

type cacheItem struct {
	row    *APIKeyRow
	expire time.Time
}

func newAPIKeyCache() (*APIKeyCache, error) {
	ttlSec := getenvInt("API_KEY_CACHE_TTL_SEC", 30)
	if ttlSec <= 0 {
		ttlSec = 30
	}
	return &APIKeyCache{
		ttl:  time.Duration(ttlSec) * time.Second,
		data: map[string]cacheItem{},
	}, nil
}

func (c *APIKeyCache) Close() error { return nil }

func (c *APIKeyCache) Get(ctx context.Context, key string) (*APIKeyRow, bool, error) {
	_ = ctx
	if c == nil {
		return nil, false, nil
	}
	c.mu.RLock()
	it, ok := c.data[key]
	c.mu.RUnlock()
	if !ok || it.row == nil {
		return nil, false, nil
	}
	if !it.expire.IsZero() && time.Now().After(it.expire) {
		// lazy delete
		c.mu.Lock()
		delete(c.data, key)
		c.mu.Unlock()
		return nil, false, nil
	}
	return it.row, true, nil
}

func (c *APIKeyCache) Set(ctx context.Context, row *APIKeyRow) error {
	_ = ctx
	if c == nil || row == nil {
		return nil
	}
	c.mu.Lock()
	c.data[row.Key] = cacheItem{row: row, expire: time.Now().Add(c.ttl)}
	c.mu.Unlock()
	return nil
}


