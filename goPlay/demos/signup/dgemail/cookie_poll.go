package main

import (
	"context"
	"fmt"
	"log"
	"os"
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
	// 目标 cookies 池大小（每个池的“最终数量”）优先级：
	// REDIS_MAX_COOKIES > STARTUP_REGISTER_COUNT > MAX_GENERATE
	//
	// 兼容旧参数：REDIS_TARGET_COOKIES（已废弃，如配置会被忽略并提示）
	if legacy := getEnvInt("REDIS_TARGET_COOKIES", 0); legacy > 0 {
		log.Printf("[poll] 检测到已废弃配置 REDIS_TARGET_COOKIES=%d，将忽略并优先使用 REDIS_MAX_COOKIES", legacy)
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

func getCookiePoolShards() int {
	n := getEnvInt("REDIS_COOKIE_POOL_SHARDS", 1)
	if n <= 0 {
		return 1
	}
	return n
}

func getStartupCookiePoolCount(prefix string) (int, error) {
	// 轮询模式在高并发/Redis 负载高时，SCARD 也可能变慢；统一用更大的 load timeout
	ctx, cancel := context.WithTimeout(context.Background(), redisLoadTimeout())
	defer cancel()
	rdb, err := newRedisClient()
	if err != nil {
		return 0, fmt.Errorf("redis init: %w", err)
	}
	defer rdb.Close()
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
	shards := getCookiePoolShards()
	baseCookiePrefix := normalizePoolBase(getEnvStr("REDIS_STARTUP_COOKIE_POOL_KEY", "tiktok:startup_cookie_pool"))
	if baseCookiePrefix == "" {
		baseCookiePrefix = "tiktok:startup_cookie_pool"
	}
	baseDevPrefix := normalizePoolBase(getEnvStr("REDIS_DEVICE_POOL_KEY", "tiktok:device_pool"))
	if baseDevPrefix == "" {
		baseDevPrefix = "tiktok:device_pool"
	}
	log.Printf("[poll] 启动：interval=%ds target=%d batch_max=%d shards=%d cookie_base=%s device_base=%s",
		interval, target, batchMax, shards, baseCookiePrefix, baseDevPrefix)

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
		// 设备文件路径（文件模式）对齐 main.go：
		// 优先级：
		// - DEVICES_FILE（统一配置，推荐）
		// - SIGNUP_DEVICES_FILE（signup 专用，兼容）
		// - 向上查找仓库根目录的 devices.txt
		// - 旧默认：data/devices.txt
		devPath := strings.TrimSpace(getEnvStr("DEVICES_FILE", ""))
		if devPath == "" {
			devPath = strings.TrimSpace(getEnvStr("SIGNUP_DEVICES_FILE", ""))
		}
		if devPath == "" {
			devPath = findTopmostFileUpwards("devices.txt", 8)
		}
		if devPath == "" {
			devPath = "data/devices.txt"
		}
		devices, err = loadDevices(devPath)
		if err != nil {
			log.Fatalf("[poll] 读取设备列表失败: %v", err)
		}
		// 设备最小年龄筛选（create_time 早于 N 小时）
		if h := getSignupDeviceMinAgeHours(); h > 0 {
			devices = filterDevicesByMinAge(devices, h)
			log.Printf("[poll] 已从文件加载 %d 个设备（create_time 早于 %d 小时）", len(devices), h)
		} else {
			log.Printf("[poll] 已从文件加载 %d 个设备", len(devices))
		}
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
		// 扫描每个 cookies 池的数量，选择“未满且数量最少”的池补货
		type poolInfo struct {
			cur    int
			idx    int
			prefix string
		}
		var pools []poolInfo
		scanOK := true
		for i := 0; i < shards; i++ {
			prefix := baseCookiePrefix
			if i > 0 {
				prefix = fmt.Sprintf("%s:%d", baseCookiePrefix, i)
			}
			cur, err := getStartupCookiePoolCount(prefix)
			if err != nil {
				// 轮询模式不应因临时 Redis 抖动直接退出：记录后 sleep，下一轮重试
				log.Printf("[poll] Redis 读取 cookies 池数量失败 prefix=%s err=%v；sleep %ds 后重试", prefix, err, interval)
				scanOK = false
				break
			}
			pools = append(pools, poolInfo{cur: cur, idx: i, prefix: prefix})
		}
		if !scanOK {
			time.Sleep(time.Duration(interval) * time.Second)
			continue
		}
		// 找最少且未满
		chosen := poolInfo{cur: 0, idx: -1, prefix: ""}
		for _, p := range pools {
			if p.cur >= target {
				continue
			}
			if chosen.idx == -1 || p.cur < chosen.cur {
				chosen = p
			}
		}
		if chosen.idx == -1 {
			log.Printf("[poll] 所有 cookies 池已满（每池 target=%d）sleep %ds", target, interval)
			time.Sleep(time.Duration(interval) * time.Second)
			continue
		}

		missing := target - chosen.cur
		fill := missing
		if fill > batchMax {
			fill = batchMax
		}
		log.Printf("[poll] 选择池 idx=%d prefix=%s cur=%d target=%d missing=%d -> 本轮补齐 %d", chosen.idx, chosen.prefix, chosen.cur, target, missing, fill)

		// 本轮写入到选中的 cookies 池
		_ = os.Setenv("REDIS_STARTUP_COOKIE_POOL_KEY", chosen.prefix)
	// device 池默认不分库：保持不变，只按 cookies 分库补齐
	log.Printf("[poll] 本轮写入 cookie_shard=%d -> cookie_prefix=%s | device_prefix=%s(不分库)",
		chosen.idx, chosen.prefix, baseDevPrefix)

		// 每轮都用不重复账号（避免复用/重复导致注册失败）
		accounts := generateUniqueAccounts(fill)

		// 重置全局结果，避免轮询模式下内存增长
		atomicStoreZero()
		clearResults()

		startTime := time.Now()
		registerAccounts(accounts, devices, proxies, maxConcurrency)
		duration := time.Since(startTime)

		// 本轮写入 Redis（强制按 fill 写入上限）
		// 写“账号池数据”（完整设备字段 + cookies 字段）
		startupAccounts := buildStartupDevicesWithCookies(devices, results)
		if n, err := saveStartupCookiesToRedis(startupAccounts, fill); err != nil {
			log.Printf("[poll] 写入startUp cookies到Redis失败: %v；sleep %ds 后重试", err, interval)
			time.Sleep(time.Duration(interval) * time.Second)
			continue
		} else {
			log.Printf("[poll] 本轮写入 cookies=%d（目标补齐=%d）耗时=%v 成功=%d 失败=%d pool_idx=%d", n, fill, duration, atomicLoadSuccess(), atomicLoadFailed(), chosen.idx)
		}

		// 同步把本轮“注册成功的设备”也写入 Redis 设备池（供 stats 从 Redis 取设备）
		startupDevs := startupAccounts
		if dn, derr := saveStartupDevicesToRedis(startupDevs); derr != nil {
			log.Printf("[poll] 写入startUp devices到Redis失败: %v；本轮 cookies 已写入，继续下一轮", derr)
		} else if dn > 0 {
			log.Printf("[poll] 本轮写入 devices=%d 到 Redis 设备池(%s)", dn, getEnvStr("REDIS_DEVICE_POOL_KEY", "tiktok:device_pool"))
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


