package main

import (
	"sync"
	"sync/atomic"
)

// CookieStats cookies 统计信息
type CookieStats struct {
	ConsecutiveFailures int64 // 连续失败次数（排除网络错误）
	TotalSuccess        int64 // 总成功次数
	TotalFailed         int64 // 总失败次数
}

// CookieManager cookies 管理器（用于连续失败阈值触发替换）
type CookieManager struct {
	stats map[string]*CookieStats
	mu    sync.RWMutex
}

var globalCookieManager *CookieManager
var cookieManagerOnce sync.Once
var cookieFailThreshold int64 = 10

func InitCookieManager() {
	cookieManagerOnce.Do(func() {
		// 连续失败阈值：从 env 读取（默认 10）
		// 优先级：STATS_COOKIE_FAIL_THRESHOLD > COOKIE_FAIL_THRESHOLD > 默认值
		cookieFailThreshold = readEnvInt64("STATS_COOKIE_FAIL_THRESHOLD", readEnvInt64("COOKIE_FAIL_THRESHOLD", 10))
		globalCookieManager = &CookieManager{
			stats: make(map[string]*CookieStats),
		}
	})
}

func GetCookieFailThreshold() int64 {
	return cookieFailThreshold
}

func GetCookieManager() *CookieManager {
	return globalCookieManager
}

func (cm *CookieManager) RecordSuccess(cookieID string) {
	if cookieID == "" || cookieID == "default" {
		return
	}
	cm.mu.Lock()
	defer cm.mu.Unlock()
	stat, ok := cm.stats[cookieID]
	if !ok {
		stat = &CookieStats{}
		cm.stats[cookieID] = stat
	}
	atomic.AddInt64(&stat.TotalSuccess, 1)
	atomic.StoreInt64(&stat.ConsecutiveFailures, 0)
}

// RecordFailure - 网络错误不计入连续失败（避免短期网络抖动误判）
func (cm *CookieManager) RecordFailure(cookieID string, isNetworkError bool) {
	if cookieID == "" || cookieID == "default" {
		return
	}
	cm.mu.Lock()
	defer cm.mu.Unlock()
	stat, ok := cm.stats[cookieID]
	if !ok {
		stat = &CookieStats{}
		cm.stats[cookieID] = stat
	}
	atomic.AddInt64(&stat.TotalFailed, 1)
	if !isNetworkError {
		atomic.AddInt64(&stat.ConsecutiveFailures, 1)
	}
}

func (cm *CookieManager) IsHealthy(cookieID string) bool {
	if cookieID == "" || cookieID == "default" {
		return true
	}
	// 阈值<=0 视为不启用
	if cookieFailThreshold <= 0 {
		return true
	}
	cm.mu.RLock()
	defer cm.mu.RUnlock()
	stat, ok := cm.stats[cookieID]
	if !ok {
		return true
	}
	return atomic.LoadInt64(&stat.ConsecutiveFailures) < cookieFailThreshold
}


