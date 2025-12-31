package main

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
)

type PoolStat struct {
	Idx    int    `json:"idx"`
	Prefix string `json:"prefix"`
	Count  int64  `json:"count"`
	Max    int64  `json:"max"`
}

type PoolsStatsResp struct {
	DevicePools []PoolStat `json:"device_pools"`
	CookiePools []PoolStat `json:"cookie_pools"`
}

func envInt64(name string, def int64) int64 {
	v := strings.TrimSpace(getenv(name, ""))
	if v == "" {
		return def
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return def
	}
	return n
}

func envInt(name string, def int) int {
	v := strings.TrimSpace(getenv(name, ""))
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

func shardPrefixes(base string, shards int) []string {
	base = strings.TrimSpace(base)
	if base == "" {
		return []string{}
	}
	if shards <= 1 {
		return []string{base}
	}
	out := make([]string, 0, shards)
	for i := 0; i < shards; i++ {
		if i == 0 {
			out = append(out, base)
		} else {
			out = append(out, fmt.Sprintf("%s:%d", base, i))
		}
	}
	return out
}

func (s *Server) getPoolsStats(ctx context.Context) (*PoolsStatsResp, error) {
	cache, err := s.redisClient()
	if err != nil {
		return nil, err
	}

	devBase := strings.TrimSpace(getenv("REDIS_DEVICE_POOL_KEY", "tiktok:device_pool"))
	ckBase := strings.TrimSpace(getenv("REDIS_STARTUP_COOKIE_POOL_KEY", "tiktok:startup_cookie_pool"))
	devShards := envInt("REDIS_DEVICE_POOL_SHARDS", 1)
	ckShards := envInt("REDIS_COOKIE_POOL_SHARDS", 1)
	if devShards <= 0 {
		devShards = 1
	}
	if ckShards <= 0 {
		ckShards = 1
	}

	devMax := envInt64("REDIS_MAX_DEVICES", 0)
	ckMax := envInt64("REDIS_MAX_COOKIES", 0)

	var devicePools []PoolStat
	for idx, p := range shardPrefixes(devBase, devShards) {
		n, err := cache.rdb.SCard(ctx, p+":ids").Result()
		if err != nil {
			return nil, err
		}
		devicePools = append(devicePools, PoolStat{Idx: idx, Prefix: p, Count: n, Max: devMax})
	}
	var cookiePools []PoolStat
	for idx, p := range shardPrefixes(ckBase, ckShards) {
		n, err := cache.rdb.SCard(ctx, p+":ids").Result()
		if err != nil {
			return nil, err
		}
		cookiePools = append(cookiePools, PoolStat{Idx: idx, Prefix: p, Count: n, Max: ckMax})
	}
	sort.Slice(devicePools, func(i, j int) bool { return devicePools[i].Idx < devicePools[j].Idx })
	sort.Slice(cookiePools, func(i, j int) bool { return cookiePools[i].Idx < cookiePools[j].Idx })
	return &PoolsStatsResp{DevicePools: devicePools, CookiePools: cookiePools}, nil
}


