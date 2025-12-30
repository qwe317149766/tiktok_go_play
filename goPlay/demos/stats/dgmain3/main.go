package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"math/rand"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// Config 配置
type Config struct {
	MaxConcurrency int
	TargetSuccess  int64
	MaxRequests    int64
	Proxies        []string
	Devices        []string
	AwemeID        string
	ResultFile     string
	ErrorFile      string
}

var (
	config = Config{
		MaxConcurrency: 500, // 进一步提高并发数以优化速度
		TargetSuccess:  10000,
		MaxRequests:    19000,
		AwemeID:        "7569635953183100191",
		ResultFile:     "results.jsonl",
		ErrorFile:      "error.log",
	}
	cacheFile = "device_cache.txt" // 设备缓存文件
)

// TaskResult 任务结果
type TaskResult struct {
	TaskID  int
	Success bool
	Extra   map[string]interface{}
	Time    string
}

// StatsResult stats请求结果
type StatsResult struct {
	Res string
	Err error
}

// ResultWriter 结果写入器 - 高性能版本
type ResultWriter struct {
	file  *os.File
	queue chan TaskResult
	wg    sync.WaitGroup
	done  chan struct{}
	mu    sync.Mutex
	batch []TaskResult
}

func NewResultWriter(filename string) (*ResultWriter, error) {
	file, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, err
	}

	rw := &ResultWriter{
		file:  file,
		queue: make(chan TaskResult, 10000), // 更大的缓冲区
		done:  make(chan struct{}),
		batch: make([]TaskResult, 0, 100), // 批量写入
	}

	rw.wg.Add(1)
	go rw.writer()

	return rw, nil
}

func (rw *ResultWriter) Write(result TaskResult) {
	select {
	case rw.queue <- result:
	case <-rw.done:
	}
}

func (rw *ResultWriter) writer() {
	defer rw.wg.Done()
	ticker := time.NewTicker(500 * time.Millisecond) // 更频繁的写入
	defer ticker.Stop()

	for {
		select {
		case result := <-rw.queue:
			rw.mu.Lock()
			rw.batch = append(rw.batch, result)
			if len(rw.batch) >= 100 { // 批量写入
				rw.flush()
			}
			rw.mu.Unlock()
		case <-ticker.C:
			rw.mu.Lock()
			if len(rw.batch) > 0 {
				rw.flush()
			}
			rw.mu.Unlock()
		case <-rw.done:
			rw.mu.Lock()
			rw.flush()
			rw.mu.Unlock()
			return
		}
	}
}

func (rw *ResultWriter) flush() {
	if len(rw.batch) == 0 {
		return
	}

	for _, result := range rw.batch {
		data, _ := json.Marshal(result)
		rw.file.WriteString(string(data) + "\n")
	}
	rw.batch = rw.batch[:0]
}

func (rw *ResultWriter) Close() {
	close(rw.done)
	rw.wg.Wait()
	rw.file.Close()
}

// ErrorWriter 错误日志写入器
type ErrorWriter struct {
	file  *os.File
	queue chan string
	wg    sync.WaitGroup
	done  chan struct{}
	mu    sync.Mutex
	batch []string
}

func NewErrorWriter(filename string) (*ErrorWriter, error) {
	file, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return nil, err
	}

	ew := &ErrorWriter{
		file:  file,
		queue: make(chan string, 1000),
		done:  make(chan struct{}),
		batch: make([]string, 0, 50),
	}

	ew.wg.Add(1)
	go ew.writer()

	return ew, nil
}

func (ew *ErrorWriter) Write(msg string) {
	select {
	case ew.queue <- msg:
	case <-ew.done:
	}
}

func (ew *ErrorWriter) writer() {
	defer ew.wg.Done()
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case msg := <-ew.queue:
			ew.mu.Lock()
			ew.batch = append(ew.batch, msg)
			if len(ew.batch) >= 50 {
				ew.flush()
			}
			ew.mu.Unlock()
		case <-ticker.C:
			ew.mu.Lock()
			if len(ew.batch) > 0 {
				ew.flush()
			}
			ew.mu.Unlock()
		case <-ew.done:
			ew.mu.Lock()
			ew.flush()
			ew.mu.Unlock()
			return
		}
	}
}

func (ew *ErrorWriter) flush() {
	if len(ew.batch) == 0 {
		return
	}
	for _, msg := range ew.batch {
		ew.file.WriteString(msg + "\n")
	}
	ew.batch = ew.batch[:0]
}

func (ew *ErrorWriter) Close() {
	close(ew.done)
	ew.wg.Wait()
	ew.file.Close()
}

// ErrorStats 错误统计
type ErrorStats struct {
	SeedErrors    int64 // seed获取失败
	TokenErrors   int64 // token获取失败
	StatsErrors   int64 // stats请求失败
	NetworkErrors int64 // 网络连接错误
	ParseErrors   int64 // 解析错误
	OtherErrors   int64 // 其他错误
}

// Engine 高性能引擎
type Engine struct {
	proxyIndex  int64
	deviceIndex int64
	proxyMutex  sync.Mutex
	deviceMutex sync.Mutex
	// 设备替换：被淘汰的 poolID 不再回补（本次运行内）
	bannedDeviceMu sync.RWMutex
	bannedPoolIDs  map[string]bool
	// 设备淘汰统计
	evictedTotal int64
	evictedFail  int64
	evictedPlay  int64

	// Linux 抢单模式：成功回调（每次成功播放触发一次，用于更新 Redis/DB 进度）
	onPlaySuccess func()

	writer        *ResultWriter
	errorWriter   *ErrorWriter
	sem           chan struct{}
	proxyManager  *ProxyManager
	deviceManager *DeviceManager

	success    int64
	failed     int64
	total      int64
	errorStats ErrorStats

	// 动态并发调整
	currentConcurrency int64
	concurrencyMu      sync.RWMutex
	minConcurrency     int
	maxConcurrency     int

	// 退出信号
	stopChan chan struct{}
	stopOnce sync.Once // 确保只关闭一次
}

func NewEngine() (*Engine, error) {
	writer, err := NewResultWriter(config.ResultFile)
	if err != nil {
		return nil, err
	}

	errorWriter, err := NewErrorWriter(config.ErrorFile)
	if err != nil {
		writer.Close()
		return nil, err
	}

	// 初始化代理管理器
	InitProxyManager(config.Proxies)

	// 初始化设备管理器（用于连续失败阈值触发替换）
	InitDeviceManager()

	// 初始化设备缓存
	InitDeviceCache(cacheFile)

	return &Engine{
		sem:                make(chan struct{}, config.MaxConcurrency),
		writer:             writer,
		errorWriter:        errorWriter,
		proxyManager:       GetProxyManager(),
		deviceManager:      GetDeviceManager(),
		bannedPoolIDs:      make(map[string]bool),
		currentConcurrency: int64(config.MaxConcurrency),
		minConcurrency:     50,                        // 最小并发数
		maxConcurrency:     config.MaxConcurrency * 2, // 最大并发数（2倍初始值）
		stopChan:           make(chan struct{}),       // 初始化stopChan
	}, nil
}

// adjustConcurrency 根据成功率动态调整并发数
func (e *Engine) adjustConcurrency() {
	total := atomic.LoadInt64(&e.total)
	if total < 100 { // 至少需要100个样本才开始调整
		return
	}

	success := atomic.LoadInt64(&e.success)
	successRate := float64(success) / float64(total)

	e.concurrencyMu.Lock()
	defer e.concurrencyMu.Unlock()

	current := int(atomic.LoadInt64(&e.currentConcurrency))
	newConcurrency := current

	// 根据成功率调整
	if successRate > 0.8 && current < e.maxConcurrency {
		// 成功率高，增加并发
		newConcurrency = current + 10
		if newConcurrency > e.maxConcurrency {
			newConcurrency = e.maxConcurrency
		}
	} else if successRate < 0.5 && current > e.minConcurrency {
		// 成功率低，减少并发
		newConcurrency = current - 10
		if newConcurrency < e.minConcurrency {
			newConcurrency = e.minConcurrency
		}
	}

	if newConcurrency != current {
		atomic.StoreInt64(&e.currentConcurrency, int64(newConcurrency))
		// 动态调整并发数（静默处理，不打印日志）
	}
}

func (e *Engine) nextProxy() string {
	// 使用智能代理选择
	if e.proxyManager != nil {
		return e.proxyManager.GetNextProxy()
	}
	// 降级到简单轮询
	e.proxyMutex.Lock()
	defer e.proxyMutex.Unlock()
	if len(config.Proxies) == 0 {
		return ""
	}
	idx := atomic.AddInt64(&e.proxyIndex, 1) - 1
	return config.Proxies[int(idx)%len(config.Proxies)]
}

func (e *Engine) nextDevice() (int, string) {
	e.deviceMutex.Lock()
	defer e.deviceMutex.Unlock()
	if len(config.Devices) == 0 {
		return 0, ""
	}

	// 简化版本：直接轮询，健康检查在失败时进行
	idx := atomic.AddInt64(&e.deviceIndex, 1) - 1
	slot := int(idx) % len(config.Devices)
	deviceJSON := config.Devices[slot]

	// 快速提取device_id（只解析一次，不检查健康状态）
	// 健康检查在taskWrapper失败时进行，避免每次选择都解析JSON
	return slot, deviceJSON
}

func extractPoolIDFromDeviceJSON(deviceJSON string) string {
	var device map[string]interface{}
	if err := json.Unmarshal([]byte(deviceJSON), &device); err != nil {
		return ""
	}
	return devicePoolIDFromDevice(device)
}

func (e *Engine) snapshotActivePoolIDsLocked() map[string]bool {
	out := make(map[string]bool, len(config.Devices))
	for _, dj := range config.Devices {
		pid := extractPoolIDFromDeviceJSON(dj)
		if strings.TrimSpace(pid) != "" {
			out[pid] = true
		}
	}
	return out
}

// replaceBadDeviceIfNeeded：当某个 poolID 连续失败超过阈值时，从 Redis 设备池补一个新设备替换该 slot。
func (e *Engine) replaceBadDeviceIfNeeded(slot int, deviceJSON string, poolID string) {
	// 仅 Redis 设备来源才支持动态补位
	if !shouldLoadDevicesFromRedis() {
		return
	}
	if e.deviceManager == nil {
		return
	}
	if strings.TrimSpace(poolID) == "" {
		return
	}
	// 连续失败阈值：沿用 DeviceManager 的规则（阈值可配置）
	if e.deviceManager.IsHealthy(poolID) {
		return
	}

	e.replaceDevice(slot, deviceJSON, poolID, "consecutive_fail")
}

func (e *Engine) replaceDevice(slot int, deviceJSON string, poolID string, reason string) {
	// 加锁替换，保证与 nextDevice 互斥
	e.deviceMutex.Lock()
	defer e.deviceMutex.Unlock()
	// slot 可能越界（理论上不会），防御一下
	if slot < 0 || slot >= len(config.Devices) {
		return
	}
	// 若 slot 已被其它协程替换过，则不重复操作
	if config.Devices[slot] != deviceJSON {
		return
	}

	// 组装 exclude：当前活跃 + banned
	exclude := e.snapshotActivePoolIDsLocked()
	e.bannedDeviceMu.RLock()
	for k := range e.bannedPoolIDs {
		exclude[k] = true
	}
	e.bannedDeviceMu.RUnlock()

	newPoolID, newJSON, err := pickOneDeviceFromRedis(exclude)
	if err != nil || strings.TrimSpace(newJSON) == "" || strings.TrimSpace(newPoolID) == "" {
		return
	}

	// 替换 slot
	config.Devices[slot] = newJSON
	// ban 老的 poolID，避免后续再次被补回来
	e.bannedDeviceMu.Lock()
	e.bannedPoolIDs[poolID] = true
	e.bannedDeviceMu.Unlock()

	// 统计：淘汰次数
	atomic.AddInt64(&e.evictedTotal, 1)
	switch reason {
	case "consecutive_fail":
		atomic.AddInt64(&e.evictedFail, 1)
	case "play_max":
		atomic.AddInt64(&e.evictedPlay, 1)
	}
}

// executeTask 执行单个任务 - 优化版本
func executeTask(taskID int, awemeID, deviceJSON, proxy string) (bool, map[string]interface{}) {
	var device map[string]interface{}
	if err := json.Unmarshal([]byte(deviceJSON), &device); err != nil {
		return false, map[string]interface{}{
			"stage":   "parse",
			"reason":  err.Error(),
			"task_id": taskID,
			"proxy":   proxy,
			"device":  "parse_error",
		}
	}

	// 获取设备ID用于日志
	deviceID := "unknown"
	if id, ok := device["device_id"].(string); ok {
		deviceID = id
	} else if id, ok := device["device_id"].(float64); ok {
		deviceID = fmt.Sprintf("%.0f", id)
	}

	// Redis 设备池 ID（与 Python 写入的 key 保持一致）
	poolID := devicePoolIDFromDevice(device)

	// 转换device
	deviceMap := make(map[string]string)
	for k, v := range device {
		switch val := v.(type) {
		case string:
			deviceMap[k] = val
		case float64:
			deviceMap[k] = fmt.Sprintf("%.0f", val)
		case int:
			deviceMap[k] = fmt.Sprintf("%d", val)
		case int64:
			deviceMap[k] = fmt.Sprintf("%d", val)
		}
	}

	// 定义变量
	var seed string
	var seedType int
	var token string

	// 检查缓存（如果设备来自 Redis，则直接读/写 Redis，确保“缓存更新到 Python 注册设备信息”）
	if shouldLoadDevicesFromRedis() {
		if s, st, t, ok := getSeedTokenFromRedis(poolID); ok {
			seed, seedType, token = s, st, t
		}
	} else {
		cache := GetDeviceCache()
		if cacheInfo, exists := cache.Get(deviceID); exists {
			seed = cacheInfo.Seed
			seedType = cacheInfo.SeedType
			token = cacheInfo.Token
		}
	}

	if seed == "" || token == "" || seedType == 0 {
		// 缓存不存在，需要请求
		// 获取HTTP客户端（使用代理管理器）
		var client *http.Client
		if proxyManager := GetProxyManager(); proxyManager != nil {
			client = proxyManager.GetClient(proxy)
		} else {
			client = GetClientForProxy(proxy)
		}

		// 异步并行获取seed和token（同时发起，提高效率）
		seedChan := GetSeedAsync(deviceMap, client)
		tokenChan := GetTokenAsync(deviceMap, client)

		// 等待seed结果（带重试逻辑，最多3次）
		var err error
		seedRetry := 0
		maxSeedRetries := 3

		for seedRetry < maxSeedRetries {
			select {
			case seedResult := <-seedChan:
				if seedResult.Err == nil && seedResult.Seed != "" {
					seed = seedResult.Seed
					seedType = seedResult.SeedType
					break // 成功，退出循环
				} else {
					if seedRetry < maxSeedRetries-1 {
						// 重试：使用指数退避策略 (2^retry * baseDelay)
						baseDelay := 200 * time.Millisecond
						backoffDelay := time.Duration(1<<uint(seedRetry)) * baseDelay
						if backoffDelay > 5*time.Second {
							backoffDelay = 5 * time.Second // 最大延迟5秒
						}
						time.Sleep(backoffDelay)
						seedChan = GetSeedAsync(deviceMap, client)
						seedRetry++
						continue
					} else {
						err = seedResult.Err
						if err == nil {
							err = fmt.Errorf("empty seed")
						}
						seedRetry = maxSeedRetries
					}
				}
			case <-time.After(20 * time.Second):
				// 超时，重试（使用指数退避）
				if seedRetry < maxSeedRetries-1 {
					baseDelay := 200 * time.Millisecond
					backoffDelay := time.Duration(1<<uint(seedRetry)) * baseDelay
					if backoffDelay > 5*time.Second {
						backoffDelay = 5 * time.Second
					}
					time.Sleep(backoffDelay)
					seedChan = GetSeedAsync(deviceMap, client)
					seedRetry++
					continue
				} else {
					err = fmt.Errorf("seed request timeout")
					seedRetry = maxSeedRetries
				}
			}
			if seed != "" {
				break // 成功获取seed，退出循环
			}
		}

		if err != nil || seed == "" {
			reason := "empty seed"
			if err != nil {
				reason = fmt.Sprintf("empty seed: %v", err)
			}
			// 判断是否是网络错误
			errStr := ""
			if err != nil {
				errStr = err.Error()
			}
			isNetworkError := strings.Contains(errStr, "connect") ||
				strings.Contains(errStr, "timeout") ||
				strings.Contains(errStr, "connection") ||
				strings.Contains(errStr, "wsarecv") ||
				strings.Contains(errStr, "panic")
			return false, map[string]interface{}{
				"stage":         "seed",
				"reason":        reason,
				"task_id":       taskID,
				"proxy":         proxy,
				"device_id":     deviceID,
				"network_error": isNetworkError,
				"error_detail":  errStr,
			}
		}

		// 等待token结果（带重试逻辑，最多3次）
		tokenRetry := 0
		maxTokenRetries := 3
		for tokenRetry < maxTokenRetries {
			select {
			case tokenResult := <-tokenChan:
				if tokenResult.Token != "" {
					token = tokenResult.Token
					break
				} else {
					if tokenRetry < maxTokenRetries-1 {
						// 使用指数退避策略
						baseDelay := 200 * time.Millisecond
						backoffDelay := time.Duration(1<<uint(tokenRetry)) * baseDelay
						if backoffDelay > 5*time.Second {
							backoffDelay = 5 * time.Second
						}
						time.Sleep(backoffDelay)
						tokenChan = GetTokenAsync(deviceMap, client)
						tokenRetry++
						continue
					} else {
						tokenRetry = maxTokenRetries
					}
				}
			case <-time.After(20 * time.Second):
				if tokenRetry < maxTokenRetries-1 {
					// 使用指数退避策略
					baseDelay := 200 * time.Millisecond
					backoffDelay := time.Duration(1<<uint(tokenRetry)) * baseDelay
					if backoffDelay > 5*time.Second {
						backoffDelay = 5 * time.Second
					}
					time.Sleep(backoffDelay)
					tokenChan = GetTokenAsync(deviceMap, client)
					tokenRetry++
					continue
				} else {
					tokenRetry = maxTokenRetries
				}
			}
			if token != "" {
				break
			}
		}

		if token == "" {
			// token 获取失败也算一次失败使用
			if shouldLoadDevicesFromRedis() {
				_ = incrDeviceFail(poolID, 1)
			}
			return false, map[string]interface{}{
				"stage":     "token",
				"reason":    "empty token after retries",
				"task_id":   taskID,
				"proxy":     proxy,
				"device_id": deviceID,
				"pool_id":   poolID,
			}
		}

		// 保存到缓存（Redis 模式：写回 Python 注册设备信息；文件模式：沿用 device_cache.txt）
		if shouldLoadDevicesFromRedis() {
			if err := setSeedTokenToRedis(poolID, seed, seedType, token); err != nil {
				return false, map[string]interface{}{
					"stage":     "cache",
					"reason":    fmt.Sprintf("write seed/token to redis failed: %v", err),
					"task_id":   taskID,
					"proxy":     proxy,
					"device_id": deviceID,
					"pool_id":   poolID,
				}
			}
		} else {
			cache := GetDeviceCache()
			cache.Set(deviceID, seed, seedType, token)
		}
	}

	// 执行stats请求 - 添加快速重试（最多2次）
	// 获取HTTP客户端（如果缓存中没有，client已经在上面获取了）
	var client *http.Client
	if proxyManager := GetProxyManager(); proxyManager != nil {
		client = proxyManager.GetClient(proxy)
	} else {
		client = GetClientForProxy(proxy)
	}

	signCount := 212
	var res string
	var err error
	// 执行stats请求 - 添加快速重试（最多2次）
	for retry := 0; retry < 2; retry++ {
		// 尝试次数：每次发起 stats 请求即 +1
		if shouldLoadDevicesFromRedis() {
			_ = incrDeviceAttempt(poolID, 1)
		}
		// 使用defer recover来捕获panic
		func() {
			defer func() {
				if r := recover(); r != nil {
					err = fmt.Errorf("panic in Stats3: %v", r)
				}
			}()
			res, err = Stats3(awemeID, seed, seedType, token, device, getCookiesForTask(taskID), signCount, client)
		}()
		if err == nil {
			break
		}
		if retry < 1 {
			time.Sleep(100 * time.Millisecond) // 短暂延迟后重试
		}
	}
	if err != nil {
		if shouldLoadDevicesFromRedis() {
			_ = incrDeviceFail(poolID, 1)
		}
		// 判断是否是网络错误
		errStr := err.Error()
		isNetworkError := strings.Contains(errStr, "connect") ||
			strings.Contains(errStr, "timeout") ||
			strings.Contains(errStr, "connection") ||
			strings.Contains(errStr, "wsarecv")
		return false, map[string]interface{}{
			"stage":         "stats",
			"reason":        errStr,
			"task_id":       taskID,
			"proxy":         proxy,
			"device_id":     deviceID,
			"pool_id":       poolID,
			"network_error": isNetworkError,
		}
	}

	success := res != ""
	// 播放次数：只在成功时 +1
	if success && shouldLoadDevicesFromRedis() {
		// 记录 play_count，并返回当前值用于阈值淘汰
		if pc, err := incrDevicePlayGet(poolID, 1); err == nil {
			// 返回给上层用于淘汰判断
			//（注意：map 的 int64 在 JSON 序列化时会变成 number，不影响）
			result := map[string]interface{}{
				"stage":      "stats",
				"raw":        "",
				"pool_id":    poolID,
				"device_id":  deviceID,
				"play_count": pc,
			}
			if len(res) > 2000 {
				result["raw"] = res[:2000]
			} else {
				result["raw"] = res
			}
			return true, result
		} else {
			_ = incrDevicePlay(poolID, 1) // 降级：尽量不影响主流程
		}
	}
	result := map[string]interface{}{
		"stage":     "stats",
		"raw":       "",
		"pool_id":   poolID,
		"device_id": deviceID,
	}
	if len(res) > 2000 {
		result["raw"] = res[:2000]
	} else {
		result["raw"] = res
	}

	return success, result
}

func (e *Engine) taskWrapper(taskID int) {
	// 添加panic恢复，防止程序崩溃
	defer func() {
		if r := recover(); r != nil {
			// 静默处理panic，不打印日志
			atomic.AddInt64(&e.failed, 1)
			atomic.AddInt64(&e.total, 1)
		}
	}()

	// 获取信号量（阻塞方式，因为worker数量=信号量大小，所以不会死锁）
	e.sem <- struct{}{}
	defer func() { <-e.sem }()

	slot, deviceJSON := e.nextDevice()
	proxy := e.nextProxy()

	ok, extra := executeTask(taskID, config.AwemeID, deviceJSON, proxy)

	atomic.AddInt64(&e.total, 1)
	deviceID, _ := extra["device_id"].(string)
	// 用 poolID 做“设备健康/替换”的主键（与 Redis 设备池一致）
	poolID := extractPoolIDFromDeviceJSON(deviceJSON)

	if ok {
		atomic.AddInt64(&e.success, 1)
		// 记录代理成功
		if e.proxyManager != nil {
			e.proxyManager.RecordSuccess(proxy)
		}
		// 记录设备成功
		if e.deviceManager != nil {
			e.deviceManager.RecordSuccess(poolID)
		}
		if e.onPlaySuccess != nil {
			e.onPlaySuccess()
		}
		// 方式A（维度2）：成功播放达到阈值就淘汰并补位
		if shouldLoadDevicesFromRedis() && GetDevicePlayMax() > 0 {
			if v, ok2 := extra["play_count"]; ok2 {
				switch t := v.(type) {
				case int64:
					if t >= GetDevicePlayMax() {
						e.replaceDevice(slot, deviceJSON, poolID, "play_max")
					}
				case float64:
					if int64(t) >= GetDevicePlayMax() {
						e.replaceDevice(slot, deviceJSON, poolID, "play_max")
					}
				}
			}
		}
		// 成功，不打印日志
	} else {
		atomic.AddInt64(&e.failed, 1)
		// 记录代理失败
		if e.proxyManager != nil {
			e.proxyManager.RecordFailure(proxy)
		}
		// 记录设备失败
		if e.deviceManager != nil {
			// 注意：连续失败需要排除网络错误（network_error=true 不累加 ConsecutiveFailures）
			isNetworkError, _ := extra["network_error"].(bool)
			e.deviceManager.RecordFailure(poolID, isNetworkError)
			// 方式A：仅在“非网络错误导致的连续失败”达到阈值后动态补位
			if !isNetworkError {
				e.replaceBadDeviceIfNeeded(slot, deviceJSON, poolID)
			}
		}

		// 分类统计错误
		stage, _ := extra["stage"].(string)
		isNetworkError, _ := extra["network_error"].(bool)
		reason, _ := extra["reason"].(string)

		switch stage {
		case "seed":
			atomic.AddInt64(&e.errorStats.SeedErrors, 1)
			if isNetworkError {
				atomic.AddInt64(&e.errorStats.NetworkErrors, 1)
			}
		case "token":
			atomic.AddInt64(&e.errorStats.TokenErrors, 1)
		case "stats":
			atomic.AddInt64(&e.errorStats.StatsErrors, 1)
			if isNetworkError {
				atomic.AddInt64(&e.errorStats.NetworkErrors, 1)
			}
		case "parse":
			atomic.AddInt64(&e.errorStats.ParseErrors, 1)
		default:
			atomic.AddInt64(&e.errorStats.OtherErrors, 1)
		}

		// 不打印错误日志，只写入错误文件
		errorMsg := fmt.Sprintf("[%s] task=%d, stage=%s, reason=%s, proxy=%s, device=%s, network_error=%v",
			time.Now().Format(time.RFC3339), taskID, stage, reason, proxy, deviceID, isNetworkError)
		if e.errorWriter != nil {
			e.errorWriter.Write(errorMsg)
		}
	}

	// 异步写入结果，不阻塞
	e.writer.Write(TaskResult{
		TaskID:  taskID,
		Success: ok,
		Extra:   extra,
		Time:    time.Now().Format(time.RFC3339),
	})
}

func (e *Engine) Run() {
	defer e.writer.Close()
	if e.errorWriter != nil {
		defer e.errorWriter.Close()
	}

	startTime := time.Now()
	// 不打印开始日志，只打印进度日志

	var wg sync.WaitGroup
	taskID := int64(0)

	// 启动定期日志输出
	stopLog := make(chan struct{})

	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(3 * time.Second) // 更频繁的进度更新
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				success := atomic.LoadInt64(&e.success)
				failed := atomic.LoadInt64(&e.failed)
				total := atomic.LoadInt64(&e.total)
				rate := 0.0
				if total > 0 {
					rate = float64(success) / float64(total) * 100
				}
				// 详细错误统计
				seedErr := atomic.LoadInt64(&e.errorStats.SeedErrors)
				tokenErr := atomic.LoadInt64(&e.errorStats.TokenErrors)
				statsErr := atomic.LoadInt64(&e.errorStats.StatsErrors)
				networkErr := atomic.LoadInt64(&e.errorStats.NetworkErrors)
				parseErr := atomic.LoadInt64(&e.errorStats.ParseErrors)
				otherErr := atomic.LoadInt64(&e.errorStats.OtherErrors)
				// 设备淘汰统计（方式A）
				evAll := atomic.LoadInt64(&e.evictedTotal)
				evFail := atomic.LoadInt64(&e.evictedFail)
				evPlay := atomic.LoadInt64(&e.evictedPlay)
				e.bannedDeviceMu.RLock()
				bannedN := len(e.bannedPoolIDs)
				e.bannedDeviceMu.RUnlock()

				log.Printf("[进度] 成功=%d, 失败=%d, 总数=%d, 成功率=%.2f%% | 错误分类: seed=%d, token=%d, stats=%d, network=%d, parse=%d, other=%d | 设备淘汰: total=%d (fail=%d, play=%d) banned=%d",
					success, failed, total, rate, seedErr, tokenErr, statsErr, networkErr, parseErr, otherErr,
					evAll, evFail, evPlay, bannedN)
				// 动态调整并发数
				e.adjustConcurrency()

				// 检查是否达到目标
				if success >= config.TargetSuccess || total >= config.MaxRequests {
					// 安全关闭stopChan（只关闭一次）
					e.stopOnce.Do(func() {
						close(e.stopChan)
					})
					return
				}
			case <-stopLog:
				return
			case <-e.stopChan:
				return
			}
		}
	}()

	// 使用worker pool模式
	for i := 0; i < config.MaxConcurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				// 检查退出信号
				select {
				case <-e.stopChan:
					return
				default:
				}

				success := atomic.LoadInt64(&e.success)
				total := atomic.LoadInt64(&e.total)

				if success >= config.TargetSuccess || total >= config.MaxRequests {
					e.stopOnce.Do(func() {
						if e.stopChan != nil {
							close(e.stopChan)
						}
					})
					return
				}

				id := int(atomic.AddInt64(&taskID, 1))
				e.taskWrapper(id)
			}
		}()
	}

	wg.Wait()
	close(stopLog)

	// 最终详细统计
	elapsed := time.Since(startTime)
	success := atomic.LoadInt64(&e.success)
	failed := atomic.LoadInt64(&e.failed)
	total := atomic.LoadInt64(&e.total)
	seedErr := atomic.LoadInt64(&e.errorStats.SeedErrors)
	tokenErr := atomic.LoadInt64(&e.errorStats.TokenErrors)
	statsErr := atomic.LoadInt64(&e.errorStats.StatsErrors)
	networkErr := atomic.LoadInt64(&e.errorStats.NetworkErrors)
	parseErr := atomic.LoadInt64(&e.errorStats.ParseErrors)
	otherErr := atomic.LoadInt64(&e.errorStats.OtherErrors)

	successRate := 0.0
	if total > 0 {
		successRate = float64(success) / float64(total) * 100
	}

	fmt.Printf("\n========== 最终统计 ==========\n")
	fmt.Printf("总耗时: %.2f秒\n", elapsed.Seconds())
	fmt.Printf("成功: %d\n", success)
	fmt.Printf("失败: %d\n", failed)
	fmt.Printf("总数: %d\n", total)
	fmt.Printf("成功率: %.2f%%\n", successRate)
	fmt.Printf("错误分类统计：seed=%d, token=%d, stats=%d, network=%d, parse=%d, other=%d\n",
		seedErr, tokenErr, statsErr, networkErr, parseErr, otherErr)
	fmt.Printf("=============================\n")
}

func loadLines(filename string) ([]string, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var lines []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line != "" {
			lines = append(lines, line)
		}
	}
	return lines, scanner.Err()
}

func main() {
	rand.Seed(time.Now().UnixNano())
	loadEnvForDemo()

	// 并发数：从 env 读取（统一配置）
	// 优先级：STATS_CONCURRENCY > GEN_CONCURRENCY > 代码默认值
	if v := envInt("STATS_CONCURRENCY", 0); v > 0 {
		config.MaxConcurrency = v
	} else if v := envInt("GEN_CONCURRENCY", 0); v > 0 {
		config.MaxConcurrency = v
	}

	proxiesPath := findTopmostFileUpwards("proxies.txt", 8)
	if proxiesPath == "" {
		proxiesPath = "proxies.txt"
	}
	devicesPath := "devices.txt"

	// 加载代理
	if data, err := ioutil.ReadFile(proxiesPath); err == nil {
		scanner := bufio.NewScanner(strings.NewReader(string(data)))
		for scanner.Scan() {
			line := strings.TrimSpace(scanner.Text())
			if line != "" {
				config.Proxies = append(config.Proxies, line)
			}
		}
		fmt.Printf("已加载 %d 个代理\n", len(config.Proxies))
	} else {
		fmt.Printf("缺少 proxies.txt（请在仓库根目录放 proxies.txt）: %v\n", err)
		os.Exit(1)
	}

	// 加载设备：优先从 Python 注册成功写入的 Redis 设备池读取
	if shouldLoadDevicesFromRedis() {
		// Redis 模式：用多少取多少
		// 默认按并发数取设备（例如并发 1000 就先取 1000 个），设备淘汰时再从 Redis 补位。
		need := envInt("DEVICES_LIMIT", 0)
		if need <= 0 {
			need = config.MaxConcurrency
		}
		if need <= 0 {
			need = 1
		}
		devs, err := loadDevicesFromRedisN(need)
		if err != nil {
			fmt.Printf("从Redis读取设备失败: %v\n", err)
			os.Exit(1)
		}
		config.Devices = append(config.Devices, devs...)
		fmt.Printf("已从Redis加载 %d 个设备（按需加载，目标=%d）\n", len(config.Devices), need)
	} else {
		// 兼容旧逻辑：读本地文件（如果不存在则自动生成）
		if data, err := ioutil.ReadFile(devicesPath); err == nil {
			scanner := bufio.NewScanner(strings.NewReader(string(data)))
			for scanner.Scan() {
				line := strings.TrimSpace(scanner.Text())
				if line != "" {
					config.Devices = append(config.Devices, line)
				}
			}
		} else {
			// 文件不存在，自动生成1000个设备
			if err := GenerateDevicesFile(devicesPath, 1000); err != nil {
				fmt.Printf("生成设备文件失败: %v\n", err)
				os.Exit(1)
			}
			// 重新加载
			if data, err := ioutil.ReadFile(devicesPath); err == nil {
				scanner := bufio.NewScanner(strings.NewReader(string(data)))
				for scanner.Scan() {
					line := strings.TrimSpace(scanner.Text())
					if line != "" {
						config.Devices = append(config.Devices, line)
					}
				}
			} else {
				fmt.Printf("加载生成的设备文件失败: %v\n", err)
				os.Exit(1)
			}
		}
		fmt.Printf("已从文件加载 %d 个设备\n", len(config.Devices))
	}

	if len(config.Proxies) == 0 || len(config.Devices) == 0 {
		fmt.Println("代理或设备列表为空")
		os.Exit(1)
	}

	// 加载 cookies：必须来自 Go startUp 注册写入的 Redis cookie 池
	if shouldLoadCookiesFromRedis() {
		limit := envInt("COOKIES_LIMIT", 0)
		cookies, err := loadStartupCookiesFromRedis(limit)
		if err != nil {
			fmt.Printf("从Redis读取startUp cookies失败: %v\n", err)
			os.Exit(1)
		}
		// 存到全局变量（供 Stats3 按 task 轮询使用）
		globalCookiePool = cookies
		fmt.Printf("已从Redis加载 %d 份 startUp cookies\n", len(globalCookiePool))
	} else {
		fmt.Printf("未启用 COOKIES_SOURCE=redis（COOKIES_FROM_REDIS 为旧兼容写法），将继续使用 stats.go 的空 cookies（通常会失败）\n")
	}

	// Linux 抢单模式：从数据库抢未完成订单，按订单 aweme_id 执行播放，并实时写 Redis/回写数据库
	if shouldRunOrderMode() {
		runOrderMode()
		return
	}

	// Windows/非抢单模式：视频 ID 从配置文件读取
	if aweme := strings.TrimSpace(envStr("AWEME_ID", "")); aweme != "" {
		config.AwemeID = aweme
	}

	engine, err := NewEngine()
	if err != nil {
		log.Fatal(err)
	}
	engine.Run()
	// 总耗时已在Run()方法中打印
}

// findTopmostFileUpwards 从当前目录开始向上查找文件，返回“最顶层”的那个路径（更接近仓库根目录）。
func findTopmostFileUpwards(name string, maxUp int) string {
	start, err := os.Getwd()
	if err != nil || start == "" {
		return ""
	}
	start, _ = filepath.Abs(start)

	found := ""
	dir := start
	for i := 0; i <= maxUp; i++ {
		p := filepath.Join(dir, name)
		if _, err := os.Stat(p); err == nil {
			found = p
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return found
}
