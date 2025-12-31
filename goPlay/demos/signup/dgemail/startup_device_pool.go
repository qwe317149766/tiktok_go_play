package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/redis/go-redis/v9"
)

// saveStartupDevicesToRedis 将 startUp 注册成功的“设备信息（含cookies字段）”写入 Redis 设备池（供 stats 使用）。
// 写入的 key 结构与 Python mwzzzh_spider 的设备池一致：
// - {prefix}:ids (SET)
// - {prefix}:data (HASH) id -> device_json
// - {prefix}:use/:fail/:play/:attempt (ZSET) 统计
// - {prefix}:seq (STRING/INT) 自增序号
// - {prefix}:in  (ZSET) FIFO 入队顺序（score=seq）
//
// 注意：这里的“设备id”优先用 REDIS_DEVICE_ID_FIELD（默认 cdid），没有则回退 device_id/install_id。
func saveStartupDevicesToRedis(devs []map[string]interface{}) (int, error) {
	// 默认：如果开启了写 cookies 到 Redis，也默认写设备到 Redis（除非显式关闭）
	if !getEnvBool("SAVE_STARTUP_DEVICES_TO_REDIS", getEnvBool("SAVE_STARTUP_COOKIES_TO_REDIS", false)) {
		return 0, nil
	}
	if len(devs) == 0 {
		return 0, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), redisLoadTimeout())
	defer cancel()

	rdb, err := newRedisClient()
	if err != nil {
		return 0, fmt.Errorf("redis init: %w", err)
	}
	defer rdb.Close()

	// 默认写入 REDIS_DEVICE_POOL_KEY；如果你希望 stats “只读 startup 成功设备”，可以单独配置 REDIS_STARTUP_DEVICE_POOL_KEY
	basePrefix := normalizePoolBase(getEnvStr("REDIS_STARTUP_DEVICE_POOL_KEY", getEnvStr("REDIS_DEVICE_POOL_KEY", "tiktok:device_pool")))
	if basePrefix == "" {
		basePrefix = "tiktok:device_pool"
	}
	shards := getDevicePoolShards()
	if shards <= 0 {
		shards = 1
	}
	// 如果当前 prefix 已经是明确的 :idx（例如 poll 模式选择 shard），则视为“锁定分库”，不做自动分片
	if _, _, ok := splitPoolShardSuffix(strings.TrimSpace(os.Getenv("REDIS_DEVICE_POOL_KEY"))); ok {
		_ = os.Setenv("DGEMAIL_POOL_SHARD_LOCKED", "1")
	}
	// device 池默认不分库：只有显式配置 REDIS_DEVICE_POOL_SHARDS>1 且允许 auto shard 时才分片写入
	useAuto := autoShardEnabled() && shards > 1

	idField := strings.TrimSpace(getEnvStr("REDIS_DEVICE_ID_FIELD", "cdid"))
	if idField == "" {
		idField = "cdid"
	}

	// Lua：INCR seqKey 并 ZADD NX 到 FIFO 队列
	const luaEnqueue = `
local seq = redis.call('INCR', KEYS[1])
redis.call('ZADD', KEYS[2], 'NX', seq, ARGV[1])
return seq
`

	wrote := 0
	for _, d := range devs {
		if d == nil {
			continue
		}
		id := extractDeviceIDForRedis(d, idField)
		if id == "" {
			continue
		}
		b, err := json.Marshal(d)
		if err != nil || len(b) == 0 {
			continue
		}
		deviceJSON := string(b)

		shardIdx := 0
		if useAuto {
			// 设备与 cookies 用同一个 shardKey：优先 device_id（与 RegisterResult.DeviceID 一致），否则 fallback redis-id
			shardKey := ""
			if v, ok := d["device_id"]; ok {
				if s, ok2 := v.(string); ok2 {
					shardKey = strings.TrimSpace(s)
				} else {
					shardKey = strings.TrimSpace(fmt.Sprintf("%v", v))
				}
			}
			if shardKey == "" {
				shardKey = id
			}
			shardIdx = shardIndexByKey(shardKey, shards)
		}
		prefix := poolPrefixForShard(basePrefix, shardIdx)
		idsKey := prefix + ":ids"
		dataKey := prefix + ":data"
		useKey := prefix + ":use"
		failKey := prefix + ":fail"
		playKey := prefix + ":play"
		attemptKey := prefix + ":attempt"
		seqKey := prefix + ":seq"
		inKey := prefix + ":in"

		pipe := rdb.Pipeline()
		pipe.SAdd(ctx, idsKey, id)
		pipe.HSet(ctx, dataKey, id, deviceJSON)
		pipe.ZAddNX(ctx, useKey, redis.Z{Member: id, Score: 0})
		pipe.ZAddNX(ctx, failKey, redis.Z{Member: id, Score: 0})
		pipe.ZAddNX(ctx, playKey, redis.Z{Member: id, Score: 0})
		pipe.ZAddNX(ctx, attemptKey, redis.Z{Member: id, Score: 0})
		pipe.Eval(ctx, luaEnqueue, []string{seqKey, inKey}, id)
		if _, err := pipe.Exec(ctx); err != nil {
			return wrote, fmt.Errorf("redis write startup device: %w", err)
		}
		wrote++
	}
	return wrote, nil
}

func extractDeviceIDForRedis(m map[string]interface{}, idField string) string {
	if m == nil {
		return ""
	}
	// 先按配置字段取
	if v, ok := m[idField]; ok {
		switch t := v.(type) {
		case string:
			if strings.TrimSpace(t) != "" {
				return strings.TrimSpace(t)
			}
		case float64:
			// JSON number
			return strings.TrimSpace(fmt.Sprintf("%.0f", t))
		}
	}
	// fallback：常见字段
	for _, k := range []string{"cdid", "device_id", "install_id"} {
		if v, ok := m[k]; ok {
			switch t := v.(type) {
			case string:
				if strings.TrimSpace(t) != "" {
					return strings.TrimSpace(t)
				}
			case float64:
				return strings.TrimSpace(fmt.Sprintf("%.0f", t))
			}
		}
	}
	return ""
}


