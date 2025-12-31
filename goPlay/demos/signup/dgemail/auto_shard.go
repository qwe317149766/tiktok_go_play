package main

import (
	"hash/fnv"
	"os"
	"strings"
)

func autoShardEnabled() bool {
	// poll/显式指定 shard 时：不允许自动分片（避免写乱）
	if getEnvBool("DGEMAIL_POOL_SHARD_LOCKED", false) {
		return false
	}
	// 用户显式配置优先生效
	if strings.TrimSpace(os.Getenv("REDIS_AUTO_SHARD")) != "" {
		return getEnvBool("REDIS_AUTO_SHARD", false)
	}
	// 默认：只有在配置了 cookies 池分库数量>1 时才自动分片（device 池默认不分库）
	return getCookiePoolShards() > 1
}

func shardIndexByKey(key string, shards int) int {
	if shards <= 1 {
		return 0
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return 0
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(key))
	return int(h.Sum32() % uint32(shards))
}


