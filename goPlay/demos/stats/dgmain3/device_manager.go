package main

import (
	"sync"
	"sync/atomic"
)

// DeviceStats 设备统计信息
type DeviceStats struct {
	ConsecutiveFailures int64 // 连续失败次数
	TotalSuccess        int64 // 总成功次数
	TotalFailed         int64 // 总失败次数
}

// DeviceManager 设备管理器
type DeviceManager struct {
	stats map[string]*DeviceStats
	mu    sync.RWMutex
}

var globalDeviceManager *DeviceManager
var deviceManagerOnce sync.Once

// InitDeviceManager 初始化设备管理器
func InitDeviceManager() {
	deviceManagerOnce.Do(func() {
		globalDeviceManager = &DeviceManager{
			stats: make(map[string]*DeviceStats),
		}
	})
}

// GetDeviceManager 获取全局设备管理器
func GetDeviceManager() *DeviceManager {
	return globalDeviceManager
}

// RecordSuccess 记录设备成功
func (dm *DeviceManager) RecordSuccess(deviceID string) {
	if deviceID == "" || deviceID == "unknown" {
		return
	}
	
	dm.mu.Lock()
	defer dm.mu.Unlock()
	
	stat, exists := dm.stats[deviceID]
	if !exists {
		stat = &DeviceStats{}
		dm.stats[deviceID] = stat
	}
	
	atomic.AddInt64(&stat.TotalSuccess, 1)
	atomic.StoreInt64(&stat.ConsecutiveFailures, 0) // 重置连续失败计数
}

// RecordFailure 记录设备失败
func (dm *DeviceManager) RecordFailure(deviceID string) {
	if deviceID == "" || deviceID == "unknown" {
		return
	}
	
	dm.mu.Lock()
	defer dm.mu.Unlock()
	
	stat, exists := dm.stats[deviceID]
	if !exists {
		stat = &DeviceStats{}
		dm.stats[deviceID] = stat
	}
	
	atomic.AddInt64(&stat.TotalFailed, 1)
	atomic.AddInt64(&stat.ConsecutiveFailures, 1)
}

// IsHealthy 检查设备是否健康（连续失败次数不超过10次）
func (dm *DeviceManager) IsHealthy(deviceID string) bool {
	if deviceID == "" || deviceID == "unknown" {
		return true // 未知设备默认认为健康
	}
	
	dm.mu.RLock()
	defer dm.mu.RUnlock()
	
	stat, exists := dm.stats[deviceID]
	if !exists {
		return true // 新设备默认认为健康
	}
	
	consecutiveFailures := atomic.LoadInt64(&stat.ConsecutiveFailures)
	return consecutiveFailures < 10 // 连续失败10次以上认为不健康
}

// GetDeviceStats 获取设备统计信息
func (dm *DeviceManager) GetDeviceStats(deviceID string) *DeviceStats {
	dm.mu.RLock()
	defer dm.mu.RUnlock()
	return dm.stats[deviceID]
}

