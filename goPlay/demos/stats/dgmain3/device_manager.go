package main

import (
	"os"
	"strconv"
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
var deviceFailThreshold int64 = 10
var devicePlayMax int64 = 0

// InitDeviceManager 初始化设备管理器
func InitDeviceManager() {
	deviceManagerOnce.Do(func() {
		// 连续失败阈值：从 env 读取（默认 10）
		// 优先级：STATS_DEVICE_FAIL_THRESHOLD > DEVICE_FAIL_THRESHOLD > 默认值
		deviceFailThreshold = readEnvInt64("STATS_DEVICE_FAIL_THRESHOLD", readEnvInt64("DEVICE_FAIL_THRESHOLD", 10))
		// 成功播放次数上限：达到后淘汰（默认 0 表示不启用）
		// 优先级：STATS_DEVICE_PLAY_MAX > DEVICE_PLAY_MAX > 默认值
		devicePlayMax = readEnvInt64("STATS_DEVICE_PLAY_MAX", readEnvInt64("DEVICE_PLAY_MAX", 0))
		globalDeviceManager = &DeviceManager{
			stats: make(map[string]*DeviceStats),
		}
	})
}

func readEnvInt64(name string, def int64) int64 {
	v := os.Getenv(name)
	if v == "" {
		return def
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil || n <= 0 {
		return def
	}
	return n
}

func GetDeviceFailThreshold() int64 {
	return deviceFailThreshold
}

func GetDevicePlayMax() int64 {
	return devicePlayMax
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
// - 网络错误不计入连续失败（避免短期网络抖动把设备误判为坏）
func (dm *DeviceManager) RecordFailure(deviceID string, isNetworkError bool) {
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
	if !isNetworkError {
		atomic.AddInt64(&stat.ConsecutiveFailures, 1)
	}
}

// IsHealthy 检查设备是否健康（连续失败次数不超过阈值）
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
	return consecutiveFailures < deviceFailThreshold // 连续失败达到阈值认为不健康
}

// GetDeviceStats 获取设备统计信息
func (dm *DeviceManager) GetDeviceStats(deviceID string) *DeviceStats {
	dm.mu.RLock()
	defer dm.mu.RUnlock()
	return dm.stats[deviceID]
}

