package main

import (
	"log"
	"os"
	"strconv"
	"strings"
)

func getDevicePoolShards() int {
	n := getEnvInt("REDIS_DEVICE_POOL_SHARDS", 1)
	if n <= 0 {
		return 1
	}
	return n
}

// splitPoolShardSuffix tries to split "prefix:<idx>" into ("prefix", idx, true).
// If it doesn't look like a shard suffix, returns ("", 0, false).
func splitPoolShardSuffix(prefix string) (string, int, bool) {
	prefix = strings.TrimSpace(prefix)
	if prefix == "" {
		return "", 0, false
	}
	parts := strings.Split(prefix, ":")
	if len(parts) < 2 {
		return "", 0, false
	}
	last := strings.TrimSpace(parts[len(parts)-1])
	if last == "" {
		return "", 0, false
	}
	idx, err := strconv.Atoi(last)
	if err != nil || idx < 0 {
		return "", 0, false
	}
	base := strings.Join(parts[:len(parts)-1], ":")
	base = strings.TrimSpace(base)
	if base == "" {
		return "", 0, false
	}
	return base, idx, true
}

func normalizePoolBase(prefix string) string {
	if base, _, ok := splitPoolShardSuffix(prefix); ok {
		return base
	}
	return strings.TrimSpace(prefix)
}

func poolPrefixForShard(base string, idx int) string {
	base = strings.TrimSpace(base)
	if base == "" {
		return ""
	}
	if idx <= 0 {
		return base
	}
	return base + ":" + strconv.Itoa(idx)
}

// applyRedisPoolShardFromArgs: dgemail 支持通过启动参数切换设备池/ cookies 池分库：
// - go run . 1      => device_pool:1 + startup_cookie_pool:1
// - go run . 1 0    => device_pool:1 + startup_cookie_pool:0
// - 不传参          => 保持 env 默认（通常为 0 号，不加后缀）
func applyRedisPoolShardFromArgs(args []string) {
	deviceIdx := -1
	cookieIdx := -1

	if len(args) >= 2 {
		if n, err := strconv.Atoi(strings.TrimSpace(args[1])); err == nil && n >= 0 {
			deviceIdx = n
		}
	}
	if len(args) >= 3 {
		if n, err := strconv.Atoi(strings.TrimSpace(args[2])); err == nil && n >= 0 {
			cookieIdx = n
		}
	}
	// 只传一个 idx：默认 device/cookie 同 shard（方便一组实例成对跑）
	if deviceIdx >= 0 && cookieIdx < 0 {
		cookieIdx = deviceIdx
	}
	if deviceIdx < 0 && cookieIdx < 0 {
		return
	}
	// 一旦通过启动参数指定分库，就认为“锁定分库”，后续写入不再自动分片
	_ = os.Setenv("DGEMAIL_POOL_SHARD_LOCKED", "1")
	if deviceIdx < 0 {
		deviceIdx = 0
	}
	if cookieIdx < 0 {
		cookieIdx = 0
	}

	devShards := getDevicePoolShards()
	ckShards := getCookiePoolShards()
	if devShards <= 0 {
		devShards = 1
	}
	if ckShards <= 0 {
		ckShards = 1
	}
	if deviceIdx >= devShards {
		log.Printf("[pool] devicePoolIdx=%d 超出 REDIS_DEVICE_POOL_SHARDS=%d，自动回退为 0", deviceIdx, devShards)
		deviceIdx = 0
	}
	if cookieIdx >= ckShards {
		log.Printf("[pool] cookiePoolIdx=%d 超出 REDIS_COOKIE_POOL_SHARDS=%d，自动回退为 0", cookieIdx, ckShards)
		cookieIdx = 0
	}

	baseDev := normalizePoolBase(getEnvStr("REDIS_DEVICE_POOL_KEY", "tiktok:device_pool"))
	baseCk := normalizePoolBase(getEnvStr("REDIS_STARTUP_COOKIE_POOL_KEY", "tiktok:startup_cookie_pool"))

	// device 池可配置为 1：此时 idx 会被回退为 0，相当于不分库
	if devShards <= 1 {
		deviceIdx = 0
	}
	devKey := poolPrefixForShard(baseDev, deviceIdx)
	ckKey := poolPrefixForShard(baseCk, cookieIdx)

	_ = os.Setenv("REDIS_DEVICE_POOL_KEY", devKey)
	_ = os.Setenv("REDIS_STARTUP_COOKIE_POOL_KEY", ckKey)

	log.Printf("[pool] selected devicePoolIdx=%d/%d key=%s | cookiePoolIdx=%d/%d key=%s",
		deviceIdx, devShards, devKey, cookieIdx, ckShards, ckKey)
}


