package main

import (
	"bufio"
	"fmt"
	"os"
	"strings"
	"sync"
)

// DeviceCache 设备缓存管理器
type DeviceCache struct {
	cache map[string]*DeviceInfo
	mu    sync.RWMutex
	file  string
}

// DeviceInfo 设备信息
type DeviceInfo struct {
	Seed     string
	SeedType int
	Token    string
}

var globalCache *DeviceCache
var cacheOnce sync.Once

// InitDeviceCache 初始化设备缓存
func InitDeviceCache(filename string) {
	cacheOnce.Do(func() {
		globalCache = &DeviceCache{
			cache: make(map[string]*DeviceInfo),
			file:  filename,
		}
		globalCache.Load()
	})
}

// GetDeviceCache 获取全局缓存实例
func GetDeviceCache() *DeviceCache {
	return globalCache
}

// Load 从文件加载缓存
func (dc *DeviceCache) Load() {
	dc.mu.Lock()
	defer dc.mu.Unlock()

	file, err := os.Open(dc.file)
	if err != nil {
		// 文件不存在，创建空缓存
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// 解析格式: device_id:seed,seed_type,token
		parts := strings.SplitN(line, ":", 2)
		if len(parts) != 2 {
			continue
		}

		deviceID := strings.TrimSpace(parts[0])
		values := strings.Split(parts[1], ",")
		if len(values) != 3 {
			continue
		}

		seed := strings.TrimSpace(values[0])
		var seedType int
		fmt.Sscanf(strings.TrimSpace(values[1]), "%d", &seedType)
		token := strings.TrimSpace(values[2])

		if deviceID != "" && seed != "" && token != "" {
			dc.cache[deviceID] = &DeviceInfo{
				Seed:     seed,
				SeedType: seedType,
				Token:    token,
			}
		}
	}
}

// Get 获取设备信息
func (dc *DeviceCache) Get(deviceID string) (*DeviceInfo, bool) {
	dc.mu.RLock()
	defer dc.mu.RUnlock()
	info, exists := dc.cache[deviceID]
	return info, exists
}

// Set 设置设备信息并保存到文件（如果不存在才写入）
func (dc *DeviceCache) Set(deviceID string, seed string, seedType int, token string) error {
	dc.mu.Lock()
	defer dc.mu.Unlock()

	// 如果已存在，不重复写入
	if _, exists := dc.cache[deviceID]; exists {
		return nil
	}

	dc.cache[deviceID] = &DeviceInfo{
		Seed:     seed,
		SeedType: seedType,
		Token:    token,
	}

	// 追加写入文件
	file, err := os.OpenFile(dc.file, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer file.Close()

	line := fmt.Sprintf("%s:%s,%d,%s\n", deviceID, seed, seedType, token)
	file.WriteString(line)

	return nil
}

// Has 检查设备是否在缓存中
func (dc *DeviceCache) Has(deviceID string) bool {
	dc.mu.RLock()
	defer dc.mu.RUnlock()
	_, exists := dc.cache[deviceID]
	return exists
}

