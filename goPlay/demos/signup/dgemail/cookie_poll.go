package main

import (
	"context"
	"fmt"
	"log"
	"runtime"
	"strings"
	"sync/atomic"
	"time"
)

func dgemailPollEnabled() bool {
	// 默认：Linux 开启轮询补齐；其它系统不开启（可用 DGEMAIL_POLL_MODE=1 强制开启调试）
	def := runtime.GOOS == "linux"
	return getEnvBool("DGEMAIL_POLL_MODE", def)
}

func dgemailPollIntervalSec() int {
	v := getEnvInt("DGEMAIL_POLL_INTERVAL_SEC", 10)
	if v <= 0 {
		return 10
	}
	return v
}

func dgemailPollBatchMax() int {
	v := getEnvInt("DGEMAIL_POLL_BATCH_MAX", 2000)
	if v <= 0 {
		return 2000
	}
	return v
}

func dgemailTargetCookies() int {
	// 目标 cookies 池大小优先级：
	// REDIS_TARGET_COOKIES > REDIS_MAX_COOKIES > STARTUP_REGISTER_COUNT > MAX_GENERATE
	if v := getEnvInt("REDIS_TARGET_COOKIES", 0); v > 0 {
		return v
	}
	if v := getEnvInt("REDIS_MAX_COOKIES", 0); v > 0 {
		return v
	}
	if v := getEnvInt("STARTUP_REGISTER_COUNT", 0); v > 0 {
		return v
	}
	if v := getEnvInt("MAX_GENERATE", 0); v > 0 {
		return v
	}
	return 0
}

func getStartupCookiePoolCount() (int, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	rdb, err := newRedisClient()
	if err != nil {
		return 0, fmt.Errorf("redis init: %w", err)
	}
	defer rdb.Close()
	if err := rdb.Ping(ctx).Err(); err != nil {
		return 0, fmt.Errorf("redis ping: %w", err)
	}
	prefix := getEnvStr("REDIS_STARTUP_COOKIE_POOL_KEY", "tiktok:startup_cookie_pool")
	idsKey := strings.TrimSpace(prefix) + ":ids"
	n, err := rdb.SCard(ctx, idsKey).Result()
	if err != nil {
		return 0, fmt.Errorf("redis scard: %w", err)
	}
	return int(n), nil
}

func runCookiePollLoop() {
	if !getEnvBool("SAVE_STARTUP_COOKIES_TO_REDIS", false) {
		log.Fatalf("[poll] DGEMAIL_POLL_MODE 打开时必须同时打开 SAVE_STARTUP_COOKIES_TO_REDIS=1（否则补齐没有意义）")
	}

	target := dgemailTargetCookies()
	if target <= 0 {
		log.Fatalf("[poll] 目标 cookies 数量无效：请配置 REDIS_TARGET_COOKIES 或 REDIS_MAX_COOKIES 或 STARTUP_REGISTER_COUNT/MAX_GENERATE")
	}
	interval := dgemailPollIntervalSec()
	batchMax := dgemailPollBatchMax()
	log.Printf("[poll] 启动：interval=%ds target=%d batch_max=%d", interval, target, batchMax)

	// 预加载设备与代理（减少每轮开销）
	var devices []map[string]interface{}
	var err error
	if shouldLoadDevicesFromRedis() {
		limit := getEnvInt("DEVICES_LIMIT", getEnvInt("MAX_GENERATE", 0))
		devices, err = loadDevicesFromRedis(limit)
		if err != nil {
			log.Fatalf("[poll] 从Redis读取设备失败: %v", err)
		}
		log.Printf("[poll] 已从Redis加载 %d 个设备", len(devices))
	} else {
		devices, err = loadDevices("data/devices.txt")
		if err != nil {
			log.Fatalf("[poll] 读取设备列表失败: %v", err)
		}
		log.Printf("[poll] 已从文件加载 %d 个设备", len(devices))
	}
	if len(devices) == 0 {
		log.Fatalf("[poll] 设备列表为空，无法补齐 cookies")
	}

	proxyPath := findTopmostFileUpwards("proxies.txt", 8)
	if proxyPath == "" {
		proxyPath = "data/proxies.txt"
	}
	proxies, err := loadProxies(proxyPath)
	if err != nil || len(proxies) == 0 {
		log.Fatalf("[poll] 读取代理列表失败: %v", err)
	}
	log.Printf("[poll] 已加载 %d 个代理", len(proxies))

	maxConcurrency := getEnvInt("SIGNUP_CONCURRENCY", 50)
	if maxConcurrency <= 0 {
		maxConcurrency = 50
	}

	for {
		cur, err := getStartupCookiePoolCount()
		if err != nil {
			log.Fatalf("[poll] Redis 读取 cookies 池数量失败（致命）：%v", err)
		}
		missing := target - cur
		if missing <= 0 {
			log.Printf("[poll] cookies 池充足 cur=%d target=%d，sleep %ds", cur, target, interval)
			time.Sleep(time.Duration(interval) * time.Second)
			continue
		}

		fill := missing
		if fill > batchMax {
			fill = batchMax
		}
		log.Printf("[poll] cookies 池不足 cur=%d target=%d missing=%d -> 本轮补齐 %d", cur, target, missing, fill)

		// 每轮都用随机账号（避免复用 accounts.txt 造成重复/耗尽）
		accounts := generateRandomAccounts(fill)

		// 重置全局结果，避免轮询模式下内存增长
		atomicStoreZero()
		clearResults()

		startTime := time.Now()
		registerAccounts(accounts, devices, proxies, maxConcurrency)
		duration := time.Since(startTime)

		// 本轮写入 Redis（强制按 fill 写入上限）
		if n, err := saveStartupCookiesToRedis(results, fill); err != nil {
			log.Fatalf("[poll] 写入startUp cookies到Redis失败（致命）: %v", err)
		} else {
			log.Printf("[poll] 本轮写入 cookies=%d（目标补齐=%d）耗时=%v 成功=%d 失败=%d", n, fill, duration, atomicLoadSuccess(), atomicLoadFailed())
		}
		// 立即进入下一轮检查（不额外 sleep）
	}
}

func atomicStoreZero() {
	atomic.StoreInt64(&totalCount, 0)
	atomic.StoreInt64(&successCount, 0)
	atomic.StoreInt64(&failedCount, 0)
}

func atomicLoadSuccess() int64 { return atomic.LoadInt64(&successCount) }
func atomicLoadFailed() int64  { return atomic.LoadInt64(&failedCount) }

func clearResults() {
	resultMutex.Lock()
	results = nil
	resultMutex.Unlock()
}


