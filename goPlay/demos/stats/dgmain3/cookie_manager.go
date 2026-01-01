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
// 如果连续失败达到阈值，会从内存中删除并异步删除数据库记录
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
		newFailures := atomic.AddInt64(&stat.ConsecutiveFailures, 1)
		// 如果连续失败达到阈值，从内存中删除并从数据库彻底删除
		if newFailures >= cookieFailThreshold {
			// 从内存中删除
			removeCookieFromPool(cookieID)
			// 彻底从数据库删除（不只是更新计数），防止下次重启再被加载
			go func() {
				// 调用 db_pool.go 中的删除函数
				_ = deleteDeviceFromDB(cookieID)
			}()
		}
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
