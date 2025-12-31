package main

import (
	"context"
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/redis/go-redis/v9"
)

type CookieImportMode string

const (
	CookieImportAppend    CookieImportMode = "append"
	CookieImportOverwrite CookieImportMode = "overwrite"
	CookieImportEvict     CookieImportMode = "evict"
)

type CookieImportResult struct {
	Mode         CookieImportMode `json:"mode"`
	Prefix       string           `json:"prefix"` // base prefix (idx=0)
	Shards       int              `json:"shards"`
	InputCount   int              `json:"input_count"`
	Imported     int              `json:"imported"`
	Invalid      int              `json:"invalid"`
	TotalNow     int64            `json:"total_now"`
	Remaining    []string         `json:"remaining_cookies"` // 未写入的 cookies（原始行）
	EvictedIDs   []string         `json:"evicted_ids"`
	MaxCookies   int64            `json:"max_cookies"`
	PerShard     []PoolStat       `json:"per_shard"`
	Message      string           `json:"message"`
}

func getStartupCookiePoolPrefix() string {
	p := strings.TrimSpace(getenv("REDIS_STARTUP_COOKIE_POOL_KEY", "tiktok:startup_cookie_pool"))
	if p == "" {
		return "tiktok:startup_cookie_pool"
	}
	return p
}

func getCookiePoolShards() int {
	n := envInt("REDIS_COOKIE_POOL_SHARDS", 1)
	if n <= 0 {
		return 1
	}
	return n
}

func cookiePoolPrefixByIdx(base string, idx int) string {
	base = strings.TrimSpace(base)
	if idx <= 0 {
		return base
	}
	return fmt.Sprintf("%s:%d", base, idx)
}

func getRedisMaxCookies() int64 {
	v := strings.TrimSpace(getenv("REDIS_MAX_COOKIES", "0"))
	if v == "" {
		return 0
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil || n < 0 {
		return 0
	}
	return n
}

func cookieIDFromMapForPool(cookies map[string]string) string {
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

func (s *Server) clearStartupCookiePool(ctx context.Context, prefix string) error {
	cache, err := s.redisClient()
	if err != nil {
		return err
	}
	idsKey := prefix + ":ids"
	dataKey := prefix + ":data"
	useKey := prefix + ":use"
	return cache.rdb.Del(ctx, idsKey, dataKey, useKey).Err()
}

func (s *Server) evictOneCookieByUse(ctx context.Context, prefix string) (string, bool, error) {
	cache, err := s.redisClient()
	if err != nil {
		return "", false, err
	}
	idsKey := prefix + ":ids"
	dataKey := prefix + ":data"
	useKey := prefix + ":use"

	ids, err := cache.rdb.ZRevRange(ctx, useKey, 0, 0).Result()
	if err != nil {
		return "", false, err
	}
	var victim string
	if len(ids) > 0 {
		victim = strings.TrimSpace(ids[0])
	}
	if victim == "" {
		v, err := cache.rdb.SPop(ctx, idsKey).Result()
		if err != nil {
			return "", false, err
		}
		victim = strings.TrimSpace(v)
		if victim == "" {
			return "", false, nil
		}
	} else {
		_ = cache.rdb.SRem(ctx, idsKey, victim).Err()
	}

	pipe := cache.rdb.Pipeline()
	pipe.HDel(ctx, dataKey, victim)
	pipe.ZRem(ctx, useKey, victim)
	_, err = pipe.Exec(ctx)
	if err != nil {
		return "", false, err
	}
	return victim, true, nil
}

func (s *Server) importCookiesToRedis(ctx context.Context, mode CookieImportMode, lines []string) (*CookieImportResult, error) {
	prefix := getStartupCookiePoolPrefix()
	maxCookies := getRedisMaxCookies()

	res := &CookieImportResult{
		Mode:       mode,
		Prefix:     prefix,
		Shards:     1,
		InputCount: len(lines),
		MaxCookies: maxCookies,
	}

	cache, err := s.redisClient()
	if err != nil {
		return nil, err
	}

	idsKey := prefix + ":ids"
	dataKey := prefix + ":data"
	useKey := prefix + ":use"

	if mode == CookieImportOverwrite {
		if err := s.clearStartupCookiePool(ctx, prefix); err != nil {
			return nil, err
		}
	}

	for _, raw := range lines {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		skipped := false
		m, ok := parseCookieLine(raw)
		if !ok || len(m) == 0 {
			res.Invalid++
			continue
		}

		if maxCookies > 0 {
			n, err := cache.rdb.SCard(ctx, idsKey).Result()
			if err != nil {
				return nil, err
			}
			if n >= maxCookies {
				if mode == CookieImportEvict {
					// 淘汰策略：优先淘汰 use_count 最大的 cookies
					for n >= maxCookies {
						victim, ok, err := s.evictOneCookieByUse(ctx, prefix)
						if err != nil {
							return nil, err
						}
						if !ok {
							res.Remaining = append(res.Remaining, raw)
							skipped = true
							break
						}
						res.EvictedIDs = append(res.EvictedIDs, victim)
						n--
					}
					if skipped {
						continue
					}
				} else {
					res.Remaining = append(res.Remaining, raw)
					continue
				}
			}
		}

		id := cookieIDFromMapForPool(m)
		val, _ := json.Marshal(m)
		pipe := cache.rdb.Pipeline()
		pipe.SAdd(ctx, idsKey, id)
		pipe.HSet(ctx, dataKey, id, string(val))
		// 初始化使用次数（不覆盖已有统计）
		pipe.ZAddNX(ctx, useKey, redis.Z{Member: id, Score: 0})
		if _, err := pipe.Exec(ctx); err != nil {
			return nil, fmt.Errorf("redis write cookie: %w", err)
		}
		res.Imported++
	}

	total, _ := cache.rdb.SCard(ctx, idsKey).Result()
	res.TotalNow = total
	res.Message = "ok"
	return res, nil
}

func (s *Server) importCookiesToRedisSharded(ctx context.Context, mode CookieImportMode, lines []string) (*CookieImportResult, error) {
	base := getStartupCookiePoolPrefix()
	maxCookies := getRedisMaxCookies()
	shards := getCookiePoolShards()

	res := &CookieImportResult{
		Mode:       mode,
		Prefix:     base,
		Shards:     shards,
		InputCount: len(lines),
		MaxCookies: maxCookies,
	}

	cache, err := s.redisClient()
	if err != nil {
		return nil, err
	}

	// overwrite：清空所有分库池
	if mode == CookieImportOverwrite {
		for i := 0; i < shards; i++ {
			p := cookiePoolPrefixByIdx(base, i)
			if err := s.clearStartupCookiePool(ctx, p); err != nil {
				return nil, err
			}
		}
	}

	type pc struct {
		idx    int
		prefix string
		count  int64
	}
	pools := make([]pc, 0, shards)
	for i := 0; i < shards; i++ {
		p := cookiePoolPrefixByIdx(base, i)
		n, err := cache.rdb.SCard(ctx, p+":ids").Result()
		if err != nil {
			return nil, err
		}
		pools = append(pools, pc{idx: i, prefix: p, count: n})
	}

	choosePool := func() *pc {
		var best *pc
		for i := range pools {
			if maxCookies > 0 && pools[i].count >= maxCookies {
				continue
			}
			if best == nil || pools[i].count < best.count {
				best = &pools[i]
			}
		}
		return best
	}

	addToPrefix := func(prefix string, m map[string]string) error {
		idsKey := prefix + ":ids"
		dataKey := prefix + ":data"
		useKey := prefix + ":use"

		id := cookieIDFromMapForPool(m)
		val, _ := json.Marshal(m)
		pipe := cache.rdb.Pipeline()
		pipe.SAdd(ctx, idsKey, id)
		pipe.HSet(ctx, dataKey, id, string(val))
		pipe.ZAddNX(ctx, useKey, redis.Z{Member: id, Score: 0})
		_, err := pipe.Exec(ctx)
		return err
	}

	for _, raw := range lines {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		m, ok := parseCookieLine(raw)
		if !ok || len(m) == 0 {
			res.Invalid++
			continue
		}

		p := choosePool()
		if p == nil {
			// 全满：evict 模式下尝试在“use_count 最大的池”上淘汰腾位置
			if mode == CookieImportEvict && maxCookies > 0 {
				maxIdx := 0
				for i := range pools {
					if pools[i].count > pools[maxIdx].count {
						maxIdx = i
					}
				}
				victim, ok, err := s.evictOneCookieByUse(ctx, pools[maxIdx].prefix)
				if err != nil {
					return nil, err
				}
				if ok {
					res.EvictedIDs = append(res.EvictedIDs, victim)
					pools[maxIdx].count--
					p = &pools[maxIdx]
				}
			}
		}
		if p == nil {
			res.Remaining = append(res.Remaining, raw)
			continue
		}

		if mode == CookieImportEvict && maxCookies > 0 {
			for p.count >= maxCookies {
				victim, ok, err := s.evictOneCookieByUse(ctx, p.prefix)
				if err != nil {
					return nil, err
				}
				if !ok {
					break
				}
				res.EvictedIDs = append(res.EvictedIDs, victim)
				p.count--
			}
			if p.count >= maxCookies {
				res.Remaining = append(res.Remaining, raw)
				continue
			}
		}

		if err := addToPrefix(p.prefix, m); err != nil {
			return nil, fmt.Errorf("redis write cookie: %w", err)
		}
		res.Imported++
		p.count++
	}

	// total now（全分库汇总）
	var total int64
	res.PerShard = nil
	for _, p := range pools {
		total += p.count
		res.PerShard = append(res.PerShard, PoolStat{Idx: p.idx, Prefix: p.prefix, Count: p.count, Max: maxCookies})
	}
	res.TotalNow = total
	res.Message = "ok"
	return res, nil
}


