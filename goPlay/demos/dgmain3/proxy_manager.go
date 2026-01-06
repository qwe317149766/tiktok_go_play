package main

import (
	"net/http"
	"net/url"
	"sync"
	"sync/atomic"
	"time"
)

// ProxyStats 代理统计信息
type ProxyStats struct {
	Success             int64
	Failed              int64
	LastUsed            time.Time
	LastError           time.Time
	ConsecutiveFailures int64
}

// ProxyManager 代理管理器 - 智能代理选择和管理
type ProxyManager struct {
	proxies          []string
	stats            map[string]*ProxyStats
	mu               sync.RWMutex
	clients          map[string]*http.Client
	clientMu         sync.RWMutex
	clientUseCount   map[string]int64 // 每个代理连接的使用次数
	clientUseCountMu sync.RWMutex     // 使用次数锁
	maxUsePerIP      int64            // 每个IP最大使用次数（默认100）
}

var globalProxyManager *ProxyManager
var proxyManagerOnce sync.Once

// InitProxyManager 初始化代理管理器
func InitProxyManager(proxies []string) {
	proxyManagerOnce.Do(func() {
		globalProxyManager = &ProxyManager{
			proxies:        proxies,
			stats:          make(map[string]*ProxyStats),
			clients:        make(map[string]*http.Client),
			clientUseCount: make(map[string]int64),
			maxUsePerIP:    2000, // 默认每个IP使用2000次后切换
		}
		// 初始化统计信息
		for _, proxy := range proxies {
			globalProxyManager.stats[proxy] = &ProxyStats{
				LastUsed: time.Now(),
			}
		}
	})
}

// SetMaxUsePerIP 设置每个IP的最大使用次数
func (pm *ProxyManager) SetMaxUsePerIP(maxUse int64) {
	pm.maxUsePerIP = maxUse
}

// GetProxyManager 获取全局代理管理器
func GetProxyManager() *ProxyManager {
	return globalProxyManager
}

// GetBestProxy 获取最佳代理（基于成功率和最近使用时间）
func (pm *ProxyManager) GetBestProxy() string {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	if len(pm.proxies) == 0 {
		return ""
	}

	// 选择策略：优先选择成功率高且最近没有失败的代理
	bestProxy := pm.proxies[0]
	bestScore := float64(-1)

	now := time.Now()
	for _, proxy := range pm.proxies {
		stat := pm.stats[proxy]
		if stat == nil {
			continue
		}

		// 计算分数：成功率 * 时间衰减因子
		total := atomic.LoadInt64(&stat.Success) + atomic.LoadInt64(&stat.Failed)
		successRate := float64(1.0)
		if total > 0 {
			successRate = float64(atomic.LoadInt64(&stat.Success)) / float64(total)
		}

		// 如果最近失败过，降低分数
		timeSinceError := now.Sub(stat.LastError)
		errorPenalty := 1.0
		if timeSinceError < 30*time.Second {
			errorPenalty = 0.3 // 最近30秒内失败过，大幅降低分数
		} else if timeSinceError < 60*time.Second {
			errorPenalty = 0.6
		}

		// 连续失败惩罚
		consecutiveFailures := atomic.LoadInt64(&stat.ConsecutiveFailures)
		if consecutiveFailures > 3 {
			errorPenalty *= 0.1 // 连续失败3次以上，几乎不选
		}

		score := successRate * errorPenalty
		if score > bestScore {
			bestScore = score
			bestProxy = proxy
		}
	}

	return bestProxy
}

// GetNextProxy 获取下一个代理（轮询方式，但跳过最近失败和不健康的）
func (pm *ProxyManager) GetNextProxy() string {
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	if len(pm.proxies) == 0 {
		return ""
	}

	// 简单轮询，但跳过最近失败的代理
	now := time.Now()
	for i := 0; i < len(pm.proxies)*2; i++ {
		proxy := pm.proxies[i%len(pm.proxies)]

		stat := pm.stats[proxy]
		if stat == nil {
			return proxy
		}

		// 如果最近30秒内失败过，跳过
		if now.Sub(stat.LastError) < 30*time.Second {
			continue
		}

		// 如果连续失败超过5次，跳过
		if atomic.LoadInt64(&stat.ConsecutiveFailures) > 5 {
			continue
		}

		return proxy
	}

	// 如果所有代理都最近失败过，返回第一个
	return pm.proxies[0]
}

// RecordSuccess 记录成功
func (pm *ProxyManager) RecordSuccess(proxy string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	stat := pm.stats[proxy]
	if stat != nil {
		atomic.AddInt64(&stat.Success, 1)
		stat.LastUsed = time.Now()
		atomic.StoreInt64(&stat.ConsecutiveFailures, 0) // 重置连续失败计数
	}
}

// RecordFailure 记录失败
func (pm *ProxyManager) RecordFailure(proxy string) {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	stat := pm.stats[proxy]
	if stat != nil {
		atomic.AddInt64(&stat.Failed, 1)
		stat.LastError = time.Now()
		atomic.AddInt64(&stat.ConsecutiveFailures, 1)
	}
}

// GetClient 获取指定代理的HTTP客户端（带连接池优化和IP切换机制）
func (pm *ProxyManager) GetClient(proxyStr string) *http.Client {
	if proxyStr == "" {
		proxyStr = "no_proxy"
	}

	// 检查使用次数，如果达到阈值则强制重建连接
	pm.clientUseCountMu.RLock()
	useCount := pm.clientUseCount[proxyStr]
	pm.clientUseCountMu.RUnlock()

	// 如果使用次数达到阈值，关闭旧连接并重置
	if useCount >= pm.maxUsePerIP {
		pm.clientMu.Lock()
		// 关闭旧连接的Transport（这会关闭所有空闲连接）
		if oldClient, exists := pm.clients[proxyStr]; exists {
			if transport, ok := oldClient.Transport.(*http.Transport); ok {
				transport.CloseIdleConnections() // 关闭所有空闲连接
			}
			delete(pm.clients, proxyStr)
		}
		// 重置使用次数
		pm.clientUseCountMu.Lock()
		pm.clientUseCount[proxyStr] = 0
		pm.clientUseCountMu.Unlock()
		pm.clientMu.Unlock()
	}

	// 快速路径：读锁
	pm.clientMu.RLock()
	client, exists := pm.clients[proxyStr]
	pm.clientMu.RUnlock()

	if exists {
		return client
	}

	// 慢速路径：写锁创建
	pm.clientMu.Lock()
	defer pm.clientMu.Unlock()

	// 双重检查
	if client, exists := pm.clients[proxyStr]; exists {
		return client
	}

	// 解析代理URL
	var proxyURL *url.URL
	if proxyStr != "no_proxy" {
		var err error
		proxyURL, err = url.Parse(proxyStr)
		if err != nil {
			proxyURL = nil
		}
	} else {
		proxyURL, _ = url.Parse("http://127.0.0.1:7777")
	}

	// 优化的Transport配置 - 大幅增加连接池大小
	transport := &http.Transport{
		Proxy: http.ProxyURL(proxyURL),
		// 连接池配置 - 大幅增加以提高性能
		MaxIdleConns:        2000,              // 增加空闲连接数
		MaxIdleConnsPerHost: 200,               // 增加每个主机的空闲连接数
		MaxConnsPerHost:     300,               // 增加每个主机的最大连接数
		IdleConnTimeout:     120 * time.Second, // 增加空闲连接超时时间
		// 超时配置 - 快速失败
		TLSHandshakeTimeout:   5 * time.Second, // 减少TLS握手超时
		ExpectContinueTimeout: 1 * time.Second,
		ResponseHeaderTimeout: 10 * time.Second, // 减少响应头超时
		// 连接复用
		DisableKeepAlives: false,
		// 禁用HTTP/2（代理可能不支持）
		ForceAttemptHTTP2: false,
		// 其他配置
		DisableCompression: false,
	}

	client = &http.Client{
		Timeout:   20 * time.Second, // 总超时时间
		Transport: transport,
	}

	pm.clients[proxyStr] = client
	// 初始化使用次数
	pm.clientUseCountMu.Lock()
	pm.clientUseCount[proxyStr] = 0
	pm.clientUseCountMu.Unlock()
	return client
}

// RecordClientUse 记录客户端使用一次（在每次请求后调用）
func (pm *ProxyManager) RecordClientUse(proxyStr string) {
	if proxyStr == "" {
		proxyStr = "no_proxy"
	}
	pm.clientUseCountMu.Lock()
	pm.clientUseCount[proxyStr]++
	pm.clientUseCountMu.Unlock()
}
