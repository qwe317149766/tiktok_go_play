package main

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/redis/go-redis/v9"
)

type DeviceImportMode string

const (
	DeviceImportOverwrite DeviceImportMode = "overwrite"
	DeviceImportEvict     DeviceImportMode = "evict"
)

type DeviceImportResult struct {
	Mode           DeviceImportMode `json:"mode"`
	Prefix         string           `json:"prefix"` // base prefix (idx=0)
	Shards         int              `json:"shards"`
	MaxDevices     int64            `json:"max_devices"`
	EvictPolicy    string           `json:"evict_policy"`
	InputCount     int              `json:"input_count"`
	AddedCount     int              `json:"added_count"`
	InvalidCount   int              `json:"invalid_count"`
	EvictedIDs     []string         `json:"evicted_ids"`
	Remaining      []string         `json:"remaining_devices"` // 未写入的设备（JSON 行）
	PerShard       []PoolStat       `json:"per_shard"`
	Message        string           `json:"message"`
}

func getDevicePoolPrefix() string {
	p := strings.TrimSpace(getenv("REDIS_DEVICE_POOL_KEY", "tiktok:device_pool"))
	if p == "" {
		return "tiktok:device_pool"
	}
	return p
}

func getDevicePoolShards() int {
	n := envInt("REDIS_DEVICE_POOL_SHARDS", 1)
	if n <= 0 {
		return 1
	}
	return n
}

func devicePoolPrefixByIdx(base string, idx int) string {
	base = strings.TrimSpace(base)
	if idx <= 0 {
		return base
	}
	return fmt.Sprintf("%s:%d", base, idx)
}

func getDeviceIDField() string {
	// env.windows/env.linux 里默认是 cdid
	f := strings.TrimSpace(getenv("REDIS_DEVICE_ID_FIELD", "cdid"))
	if f == "" {
		return "cdid"
	}
	return f
}

func getRedisMaxDevices() int64 {
	v := strings.TrimSpace(getenv("REDIS_MAX_DEVICES", "0"))
	if v == "" {
		return 0
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil || n < 0 {
		return 0
	}
	return n
}

func getRedisEvictPolicy() string {
	p := strings.ToLower(strings.TrimSpace(getenv("REDIS_EVICT_POLICY", "play")))
	switch p {
	case "play", "use", "attempt":
		return p
	default:
		return "play"
	}
}

func (s *Server) redisClient() (*APIKeyCache, error) {
	// 复用现有 redis client（api_keys 永久缓存同一个 redis）
	if s.cache == nil || s.cache.rdb == nil {
		return nil, fmt.Errorf("redis not initialized")
	}
	return s.cache, nil
}

func normalizeJSONLine(line string) (string, map[string]any, error) {
	line = strings.TrimSpace(line)
	if line == "" {
		return "", nil, fmt.Errorf("empty line")
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(line), &m); err != nil {
		return "", nil, err
	}
	// 规范化输出（压缩），方便写入 redis
	b, _ := json.Marshal(m)
	return string(b), m, nil
}

func extractDeviceID(m map[string]any, idField string) (string, bool) {
	if m == nil {
		return "", false
	}
	if v, ok := m[idField]; ok {
		if s, ok2 := v.(string); ok2 && strings.TrimSpace(s) != "" {
			return strings.TrimSpace(s), true
		}
		// 有些字段可能是数字
		switch t := v.(type) {
		case float64:
			return strings.TrimSpace(strconv.FormatInt(int64(t), 10)), true
		}
	}
	// 兜底：常见字段
	for _, k := range []string{"cdid", "device_id", "install_id"} {
		if v, ok := m[k]; ok {
			if s, ok2 := v.(string); ok2 && strings.TrimSpace(s) != "" {
				return strings.TrimSpace(s), true
			}
		}
	}
	return "", false
}

func getInt64FromAny(m map[string]any, key string) int64 {
	if m == nil {
		return 0
	}
	v, ok := m[key]
	if !ok {
		return 0
	}
	switch t := v.(type) {
	case float64:
		return int64(t)
	case int64:
		return t
	case string:
		n, err := strconv.ParseInt(strings.TrimSpace(t), 10, 64)
		if err == nil {
			return n
		}
	}
	return 0
}

func (s *Server) devicePoolKeys(prefix string) (idsKey, dataKey, useKey, failKey, playKey, attemptKey string) {
	return prefix + ":ids", prefix + ":data", prefix + ":use", prefix + ":fail", prefix + ":play", prefix + ":attempt"
}

func (s *Server) clearDevicePool(ctx context.Context, prefix string) error {
	cache, err := s.redisClient()
	if err != nil {
		return err
	}
	idsKey, dataKey, useKey, failKey, playKey, attemptKey := s.devicePoolKeys(prefix)
	return cache.rdb.Del(ctx, idsKey, dataKey, useKey, failKey, playKey, attemptKey).Err()
}

func (s *Server) evictOne(ctx context.Context, prefix string, policy string) (string, bool, error) {
	cache, err := s.redisClient()
	if err != nil {
		return "", false, err
	}
	idsKey, dataKey, useKey, failKey, playKey, attemptKey := s.devicePoolKeys(prefix)

	chooseZ := func(p string) string {
		switch p {
		case "use":
			return useKey
		case "attempt":
			return attemptKey
		case "play":
			return playKey
		default:
			return playKey
		}
	}
	zkey := chooseZ(policy)

	// 分数最高（最“该淘汰”）
	ids, err := cache.rdb.ZRevRange(ctx, zkey, 0, 0).Result()
	if err != nil {
		return "", false, err
	}
	var victim string
	if len(ids) > 0 {
		victim = strings.TrimSpace(ids[0])
	}
	if victim == "" {
		// fallback: 从 set 随便 pop 一个
		v, err := cache.rdb.SPop(ctx, idsKey).Result()
		if err != nil {
			if strings.Contains(err.Error(), "nil") {
				return "", false, nil
			}
			return "", false, err
		}
		victim = strings.TrimSpace(v)
		if victim == "" {
			return "", false, nil
		}
	} else {
		// 确保从 ids set 移除
		_ = cache.rdb.SRem(ctx, idsKey, victim).Err()
	}

	pipe := cache.rdb.Pipeline()
	pipe.HDel(ctx, dataKey, victim)
	pipe.ZRem(ctx, useKey, victim)
	pipe.ZRem(ctx, failKey, victim)
	pipe.ZRem(ctx, playKey, victim)
	pipe.ZRem(ctx, attemptKey, victim)
	_, err = pipe.Exec(ctx)
	if err != nil {
		return "", false, err
	}
	return victim, true, nil
}

func (s *Server) importDevicesToRedis(ctx context.Context, mode DeviceImportMode, lines []string) (*DeviceImportResult, error) {
	prefix := getDevicePoolPrefix()
	idField := getDeviceIDField()
	maxDevices := getRedisMaxDevices()
	policy := getRedisEvictPolicy()

	res := &DeviceImportResult{
		Mode:        mode,
		Prefix:      prefix,
		MaxDevices:  maxDevices,
		EvictPolicy: policy,
		InputCount:  len(lines),
	}

	cache, err := s.redisClient()
	if err != nil {
		return nil, err
	}

	if mode == DeviceImportOverwrite {
		if err := s.clearDevicePool(ctx, prefix); err != nil {
			return nil, err
		}
	}

	idsKey, dataKey, useKey, failKey, playKey, attemptKey := s.devicePoolKeys(prefix)

	addOne := func(deviceJSON string, m map[string]any) error {
		id, ok := extractDeviceID(m, idField)
		if !ok {
			return fmt.Errorf("missing device id field: %s", idField)
		}
		use := getInt64FromAny(m, "use_count")
		fail := getInt64FromAny(m, "fail_count")
		play := getInt64FromAny(m, "play_count")
		attempt := getInt64FromAny(m, "attempt_count")

		pipe := cache.rdb.Pipeline()
		pipe.SAdd(ctx, idsKey, id)
		pipe.HSet(ctx, dataKey, id, deviceJSON)
		pipe.ZAdd(ctx, useKey, redis.Z{Member: id, Score: float64(use)})
		pipe.ZAdd(ctx, failKey, redis.Z{Member: id, Score: float64(fail)})
		pipe.ZAdd(ctx, playKey, redis.Z{Member: id, Score: float64(play)})
		pipe.ZAdd(ctx, attemptKey, redis.Z{Member: id, Score: float64(attempt)})
		_, err := pipe.Exec(ctx)
		return err
	}

	// 容量控制：
	// - overwrite：只写前 maxDevices 条，剩余返回给用户
	// - evict：优先用淘汰挤出旧设备给新设备腾位置；若 batch 本身大于 maxDevices，仍会有剩余返回
	for _, raw := range lines {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		deviceJSON, m, err := normalizeJSONLine(raw)
		if err != nil {
			res.InvalidCount++
			continue
		}

		if maxDevices > 0 {
			// 当前池大小
			n, err := cache.rdb.SCard(ctx, idsKey).Result()
			if err != nil {
				return nil, err
			}

			if mode == DeviceImportOverwrite {
				if n >= maxDevices {
					res.Remaining = append(res.Remaining, deviceJSON)
					continue
				}
			}

			if mode == DeviceImportEvict {
				for n >= maxDevices {
					victim, ok, err := s.evictOne(ctx, prefix, policy)
					if err != nil {
						return nil, err
					}
					if !ok {
						// evict 不出来，说明数据结构异常，避免死循环：直接把本条作为剩余返回
						res.Remaining = append(res.Remaining, deviceJSON)
						goto nextLine
					}
					res.EvictedIDs = append(res.EvictedIDs, victim)
					n--
				}
			}
		}

		if err := addOne(deviceJSON, m); err != nil {
			return nil, err
		}
		res.AddedCount++
	nextLine:
	}

	res.Message = "ok"
	return res, nil
}

// importDevicesToRedisSharded 自动分配到分库池：
// - 0号池：base
// - i号池：base:i
// 选择策略：优先填充“未满且数量最少”的池
func (s *Server) importDevicesToRedisSharded(ctx context.Context, mode DeviceImportMode, lines []string) (*DeviceImportResult, error) {
	base := getDevicePoolPrefix()
	idField := getDeviceIDField()
	maxDevices := getRedisMaxDevices()
	policy := getRedisEvictPolicy()
	shards := getDevicePoolShards()

	res := &DeviceImportResult{
		Mode:        mode,
		Prefix:      base,
		Shards:      shards,
		MaxDevices:  maxDevices,
		EvictPolicy: policy,
		InputCount:  len(lines),
	}

	cache, err := s.redisClient()
	if err != nil {
		return nil, err
	}

	// overwrite：清空所有分库池
	if mode == DeviceImportOverwrite {
		for i := 0; i < shards; i++ {
			p := devicePoolPrefixByIdx(base, i)
			if err := s.clearDevicePool(ctx, p); err != nil {
				return nil, err
			}
		}
	}

	// 初始化每个池的 count
	type pc struct {
		idx    int
		prefix string
		count  int64
	}
	pools := make([]pc, 0, shards)
	for i := 0; i < shards; i++ {
		p := devicePoolPrefixByIdx(base, i)
		n, err := cache.rdb.SCard(ctx, p+":ids").Result()
		if err != nil {
			return nil, err
		}
		pools = append(pools, pc{idx: i, prefix: p, count: n})
	}

	// 去重（按设备id）
	seen := map[string]bool{}

	addToPrefix := func(prefix string, deviceJSON string, m map[string]any) error {
		idsKey, dataKey, useKey, failKey, playKey, attemptKey := s.devicePoolKeys(prefix)
		id, ok := extractDeviceID(m, idField)
		if !ok {
			return fmt.Errorf("missing device id field: %s", idField)
		}
		use := getInt64FromAny(m, "use_count")
		fail := getInt64FromAny(m, "fail_count")
		play := getInt64FromAny(m, "play_count")
		attempt := getInt64FromAny(m, "attempt_count")
		pipe := cache.rdb.Pipeline()
		pipe.SAdd(ctx, idsKey, id)
		pipe.HSet(ctx, dataKey, id, deviceJSON)
		pipe.ZAdd(ctx, useKey, redis.Z{Member: id, Score: float64(use)})
		pipe.ZAdd(ctx, failKey, redis.Z{Member: id, Score: float64(fail)})
		pipe.ZAdd(ctx, playKey, redis.Z{Member: id, Score: float64(play)})
		pipe.ZAdd(ctx, attemptKey, redis.Z{Member: id, Score: float64(attempt)})
		_, err := pipe.Exec(ctx)
		return err
	}

	choosePool := func() *pc {
		// 找未满且最少
		var best *pc
		for i := range pools {
			if maxDevices > 0 && pools[i].count >= maxDevices {
				continue
			}
			if best == nil || pools[i].count < best.count {
				best = &pools[i]
			}
		}
		return best
	}

	for _, raw := range lines {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		deviceJSON, m, err := normalizeJSONLine(raw)
		if err != nil {
			res.InvalidCount++
			continue
		}
		id, ok := extractDeviceID(m, idField)
		if !ok {
			res.InvalidCount++
			continue
		}
		if seen[id] {
			continue
		}
		seen[id] = true

		p := choosePool()
		if p == nil {
			// 全满：evict 模式下尝试在“use_count 最大的池”上淘汰腾位置
			if mode == DeviceImportEvict && maxDevices > 0 {
				// 选择 count 最大的池
				maxIdx := 0
				for i := range pools {
					if pools[i].count > pools[maxIdx].count {
						maxIdx = i
					}
				}
				// 淘汰一个
				victim, ok, err := s.evictOne(ctx, pools[maxIdx].prefix, policy)
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
			res.Remaining = append(res.Remaining, deviceJSON)
			continue
		}

		// 若目标池满且是 evict：淘汰直到有空位
		if mode == DeviceImportEvict && maxDevices > 0 {
			for p.count >= maxDevices {
				victim, ok, err := s.evictOne(ctx, p.prefix, policy)
				if err != nil {
					return nil, err
				}
				if !ok {
					break
				}
				res.EvictedIDs = append(res.EvictedIDs, victim)
				p.count--
			}
			if p.count >= maxDevices {
				res.Remaining = append(res.Remaining, deviceJSON)
				continue
			}
		}

		if err := addToPrefix(p.prefix, deviceJSON, m); err != nil {
			return nil, err
		}
		res.AddedCount++
		p.count++
	}

	// 汇总 per_shard after counts
	res.PerShard = nil
	for _, p := range pools {
		res.PerShard = append(res.PerShard, PoolStat{Idx: p.idx, Prefix: p.prefix, Count: p.count, Max: maxDevices})
	}
	res.Message = "ok"
	return res, nil
}


