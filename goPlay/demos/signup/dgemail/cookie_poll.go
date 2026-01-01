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
	// 默认：Linux 开启补齐（但采用 run-once，不常驻）；其它系统不开启（可用 DGEMAIL_POLL_MODE=1 调试）
	def := runtime.GOOS == "linux"
	return getEnvBool("DGEMAIL_POLL_MODE", def)
}

func dgemailPollOnce() bool {
	// 持续轮询模式：默认 false（一直轮询，不退出）
	// 设置为 true 时，只跑一轮就退出（适合 cron 调度）
	return getEnvBool("DGEMAIL_POLL_ONCE", false)
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
	// 目标 cookies 池大小（每个 shard 的最终数量）优先级：
	// DB_MAX_COOKIES > STARTUP_REGISTER_COUNT > MAX_GENERATE
	if v := getEnvInt("DB_MAX_COOKIES", 0); v > 0 {
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

func getStartupCookiePoolCountDB(ctx context.Context, shard int) (int, error) {
	db, err := getSignupDB()
	if err != nil {
		return 0, fmt.Errorf("mysql init: %w", err)
	}
	tbl := cookiePoolTable()
	var n int
	if err := db.QueryRowContext(ctx, fmt.Sprintf("SELECT COUNT(*) FROM `%s` WHERE shard_id=?", tbl), shard).Scan(&n); err != nil {
		return 0, err
	}
	return n, nil
}

func runCookiePollLoop() {
	// 全 DB：必须写入 MySQL cookies 池
	if !shouldWriteStartupAccountsToDB() {
		log.Fatalf("[poll] DGEMAIL_POLL_MODE 打开时必须同时打开 SAVE_STARTUP_COOKIES_TO_DB=1 或 COOKIES_SINK=db（否则补齐没有意义）")
	}
	// 全 DB：必须从 MySQL 读设备池
	if !shouldLoadDevicesFromDB() {
		log.Fatalf("[poll] 全 DB 模式下必须配置 SIGNUP_DEVICES_SOURCE=db")
	}

	target := dgemailTargetCookies()
	if target <= 0 {
		log.Fatalf("[poll] 目标 cookies 数量无效：请配置 DB_MAX_COOKIES 或 STARTUP_REGISTER_COUNT/MAX_GENERATE")
	}

	interval := dgemailPollIntervalSec()
	batchMax := dgemailPollBatchMax()
	shards := dbCookieShards()
	runOnce := dgemailPollOnce()

	log.Printf("[poll] 启动(DB)：once=%v interval=%ds target(per_shard)=%d batch_max=%d shards=%d cookie_table=%s device_table=%s",
		runOnce, interval, target, batchMax, shards, cookiePoolTable(), devicePoolTable())

	// 预加载设备与代理（减少每轮开销）
	limit := getEnvInt("DEVICES_LIMIT", getEnvInt("MAX_GENERATE", 0))
	devices, err := loadDevicesFromDB(limit)
	if err != nil {
		log.Fatalf("[poll] 从MySQL读取设备失败: %v", err)
	}
	if len(devices) == 0 {
		log.Fatalf("[poll] 设备列表为空，无法补齐 cookies")
	}
	log.Printf("[poll] 已从MySQL加载 %d 个设备", len(devices))

	// 代理
	proxyPath := strings.TrimSpace(getEnvStr("PROXIES_FILE", ""))
	if proxyPath == "" {
		proxyPath = strings.TrimSpace(getEnvStr("SIGNUP_PROXIES_FILE", ""))
	}
	if proxyPath == "" && fileExists("proxies.txt") {
		proxyPath = "proxies.txt"
	}
	if proxyPath == "" && fileExists("data/proxies.txt") {
		proxyPath = "data/proxies.txt"
	}
	if proxyPath == "" {
		proxyPath = findTopmostFileUpwards("proxies.txt", 8)
	}
	if proxyPath == "" {
		proxyPath = "data/proxies.txt"
	}
	proxies, err := loadProxies(proxyPath)
	if err != nil || len(proxies) == 0 {
		log.Fatalf("[poll] 读取代理列表失败: %v", err)
	}
	log.Printf("[poll] 已加载 %d 个代理", len(proxies))

	// 初始化代理管理器（支持代理生成和使用次数限制）
	InitProxyManager(proxies)

	maxConcurrency := getEnvInt("SIGNUP_CONCURRENCY", 50)
	if maxConcurrency <= 0 {
		maxConcurrency = 50
	}

	for round := 0; ; round++ {
		// 统计每个 shard 缺口
		type poolInfo struct {
			cur     int
			idx     int
			missing int
		}
		pools := make([]poolInfo, 0, shards)
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		for i := 0; i < shards; i++ {
			cur, err := getStartupCookiePoolCountDB(ctx, i)
			if err != nil {
				cancel()
				log.Printf("[poll] DB 读取 cookies 池数量失败 shard=%d err=%v；sleep %ds 后重试", i, err, interval)
				time.Sleep(time.Duration(interval) * time.Second)
				continue
			}
			missing := target - cur
			if missing < 0 {
				missing = 0
			}
			log.Printf("[poll] shard=%d cur=%d target=%d missing=%d", i, cur, target, missing)
			pools = append(pools, poolInfo{cur: cur, idx: i, missing: missing})
		}
		cancel()

		// 选缺口最大的 shard（优先补最缺的）
		chosen := poolInfo{idx: -1, cur: 0, missing: 0}
		for _, p := range pools {
			if p.missing <= 0 {
				continue
			}
			if chosen.idx == -1 || p.missing > chosen.missing {
				chosen = p
			}
		}
		if chosen.idx == -1 {
			// 所有 shard 已满足，休眠后继续轮询（不退出）
			log.Printf("[poll] 所有 cookies shard 已满足 target=%d（per_shard），休眠 %ds 后继续检查", target, interval)
			if runOnce {
				log.Printf("[poll] run-once 模式：退出")
				return
			}
			time.Sleep(time.Duration(interval) * time.Second)
			continue
		}

		fill := chosen.missing
		if fill > batchMax {
			fill = batchMax
		}

		// 强制写入到选中的 shard
		_ = os.Setenv("DGEMAIL_COOKIE_SHARD", fmt.Sprintf("%d", chosen.idx))
		log.Printf("[poll] round=%d choose_shard=%d cur=%d missing=%d -> fill=%d (DGEMAIL_COOKIE_SHARD=%d)",
			round, chosen.idx, chosen.cur, chosen.missing, fill, chosen.idx)

		accounts := generateUniqueAccounts(fill)
		if len(accounts) == 0 {
			log.Printf("[poll] round=%d 警告：generateUniqueAccounts 返回空列表（fill=%d），休眠 %ds 后重试", round, fill, interval)
			if runOnce {
				return
			}
			time.Sleep(time.Duration(interval) * time.Second)
			continue
		}
		if len(devices) == 0 {
			log.Printf("[poll] round=%d 错误：设备列表为空，无法注册，休眠 %ds 后重试", round, interval)
			if runOnce {
				return
			}
			time.Sleep(time.Duration(interval) * time.Second)
			continue
		}
		if len(proxies) == 0 {
			log.Printf("[poll] round=%d 错误：代理列表为空，无法注册，休眠 %ds 后重试", round, interval)
			if runOnce {
				return
			}
			time.Sleep(time.Duration(interval) * time.Second)
			continue
		}

		log.Printf("[poll] round=%d 开始注册：accounts=%d devices=%d proxies=%d concurrency=%d",
			round, len(accounts), len(devices), len(proxies), maxConcurrency)

		// 重置全局结果（避免轮询模式内存增长）
		atomic.StoreInt64(&totalCount, 0)
		atomic.StoreInt64(&successCount, 0)
		atomic.StoreInt64(&failedCount, 0)
		resultMutex.Lock()
		results = nil
		resultMutex.Unlock()

		startTime := time.Now()
		registerAccounts(accounts, devices, proxies, maxConcurrency)
		duration := time.Since(startTime)
		success := atomic.LoadInt64(&successCount)
		failed := atomic.LoadInt64(&failedCount)
		log.Printf("[poll] round=%d done: took=%v success=%d failed=%d (target_fill=%d shard=%d)",
			round, duration, success, failed, fill, chosen.idx)

		// run-once：只跑一轮就退出（交给 cron 再跑）
		if runOnce {
			log.Printf("[poll] run-once 模式：退出")
			return
		}
		// 持续轮询模式：处理完毕后立即重新检查（不 sleep，直接进入下一轮循环）
		// 只有在"没有数据要处理"时才会 sleep（见上面的 continue 逻辑）
		log.Printf("[poll] round=%d 处理完成，立即重新检查 cookies 池状态", round)
		// 不 sleep，直接 continue 进入下一轮循环
	}
}


