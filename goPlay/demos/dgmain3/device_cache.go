package main

import (
	"bufio"
	"encoding/json"
	"os"
	"sync"
	"time"
)

// CacheInfo 缓存的设备信息
type CacheInfo struct {
	Seed      string    `json:"seed"`
	SeedType  int       `json:"seed_type"`
	Token     string    `json:"token"`
	CreatedAt time.Time `json:"created_at"`
}

// DeviceCache 设备缓存
type DeviceCache struct {
	cache    map[string]*CacheInfo
	mu       sync.RWMutex
	filePath string
	ttl      time.Duration // 缓存过期时间
}

var (
	globalDeviceCache *DeviceCache
	deviceCacheOnce   sync.Once
)

// InitDeviceCache 初始化设备缓存
func InitDeviceCache(filePath string) {
	deviceCacheOnce.Do(func() {
		globalDeviceCache = &DeviceCache{
			cache:    make(map[string]*CacheInfo),
			filePath: filePath,
			ttl:      30 * time.Minute, // 默认30分钟过期
		}
		// 从文件加载缓存
		globalDeviceCache.loadFromFile()
	})
}

// GetDeviceCache 获取全局设备缓存
func GetDeviceCache() *DeviceCache {
	return globalDeviceCache
}

// Get 获取缓存
func (dc *DeviceCache) Get(deviceID string) (*CacheInfo, bool) {
	if dc == nil {
		return nil, false
	}

	dc.mu.RLock()
	defer dc.mu.RUnlock()

	info, exists := dc.cache[deviceID]
	if !exists {
		return nil, false
	}

	// 检查是否过期
	if time.Since(info.CreatedAt) > dc.ttl {
		return nil, false
	}

	return info, true
}

// Set 设置缓存
func (dc *DeviceCache) Set(deviceID, seed string, seedType int, token string) {
	if dc == nil {
		return
	}

	dc.mu.Lock()
	defer dc.mu.Unlock()

	dc.cache[deviceID] = &CacheInfo{
		Seed:      seed,
		SeedType:  seedType,
		Token:     token,
		CreatedAt: time.Now(),
	}

	// 异步保存到文件
	go dc.saveToFile()
}

// loadFromFile 从文件加载缓存
func (dc *DeviceCache) loadFromFile() {
	if dc.filePath == "" {
		return
	}

	file, err := os.Open(dc.filePath)
	if err != nil {
		return
	}
	defer file.Close()

	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		var entry struct {
			DeviceID string    `json:"device_id"`
			Seed     string    `json:"seed"`
			SeedType int       `json:"seed_type"`
			Token    string    `json:"token"`
			Time     time.Time `json:"time"`
		}

		if err := json.Unmarshal([]byte(line), &entry); err != nil {
			continue
		}

		// 只加载未过期的缓存
		if time.Since(entry.Time) <= dc.ttl {
			dc.cache[entry.DeviceID] = &CacheInfo{
				Seed:      entry.Seed,
				SeedType:  entry.SeedType,
				Token:     entry.Token,
				CreatedAt: entry.Time,
			}
		}
	}
}

// saveToFile 保存缓存到文件
func (dc *DeviceCache) saveToFile() {
	if dc.filePath == "" {
		return
	}

	dc.mu.RLock()
	defer dc.mu.RUnlock()

	file, err := os.Create(dc.filePath)
	if err != nil {
		return
	}
	defer file.Close()

	for deviceID, info := range dc.cache {
		entry := struct {
			DeviceID string    `json:"device_id"`
			Seed     string    `json:"seed"`
			SeedType int       `json:"seed_type"`
			Token    string    `json:"token"`
			Time     time.Time `json:"time"`
		}{
			DeviceID: deviceID,
			Seed:     info.Seed,
			SeedType: info.SeedType,
			Token:    info.Token,
			Time:     info.CreatedAt,
		}

		data, err := json.Marshal(entry)
		if err != nil {
			continue
		}
		file.WriteString(string(data) + "\n")
	}
}

// Clear 清空缓存
func (dc *DeviceCache) Clear() {
	if dc == nil {
		return
	}

	dc.mu.Lock()
	defer dc.mu.Unlock()

	dc.cache = make(map[string]*CacheInfo)
}

// SetTTL 设置缓存过期时间
func (dc *DeviceCache) SetTTL(ttl time.Duration) {
	if dc == nil {
		return
	}

	dc.mu.Lock()
	defer dc.mu.Unlock()

	dc.ttl = ttl
}

