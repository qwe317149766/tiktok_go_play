package main

import (
	"context"
	"net/http"
	"net/url"
	"sync"
	"sync/atomic"
	"time"
)

// ProxyStats 代理统计信息
type ProxyStats struct {
	Success    int64
	Failed     int64
	LastUsed   time.Time
	LastError  time.Time
	ConsecutiveFailures int64
}

// ProxyManager 代理管理器 - 智能代理选择和管理
type ProxyManager struct {
	proxies     []string
	stats       map[string]*ProxyStats
	mu          sync.RWMutex
	clients     map[string]*http.Client
	clientMu    sync.RWMutex
	healthCheck map[string]bool // 代理健康状态
	healthMu   sync.RWMutex
	stopHealth  chan struct{}   // 停止健康检查信号
}

var globalProxyManager *ProxyManager
var proxyManagerOnce sync.Once

// InitProxyManager 初始化代理管理器
func InitProxyManager(proxies []string) {
	proxyManagerOnce.Do(func() {
		globalProxyManager = &ProxyManager{
			proxies:     proxies,
			stats:       make(map[string]*ProxyStats),
			clients:     make(map[string]*http.Client),
			healthCheck: make(map[string]bool),
			stopHealth:  make(chan struct{}),
		}
		// 初始化统计信息
		for _, proxy := range proxies {
			globalProxyManager.stats[proxy] = &ProxyStats{
				LastUsed: time.Now(),
			}
			globalProxyManager.healthCheck[proxy] = true // 默认认为健康
		}
		// 启动健康检查
		go globalProxyManager.startHealthCheck()
	})
}

// startHealthCheck 启动代理健康检查
func (pm *ProxyManager) startHealthCheck() {
	ticker := time.NewTicker(30 * time.Second) // 每30秒检查一次
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			pm.checkAllProxies()
		case <-pm.stopHealth:
			return
		}
	}
}

// checkAllProxies 检查所有代理的健康状态
func (pm *ProxyManager) checkAllProxies() {
	pm.mu.RLock()
	proxies := make([]string, len(pm.proxies))
	copy(proxies, pm.proxies)
	pm.mu.RUnlock()

	for _, proxy := range proxies {
		go pm.checkProxyHealth(proxy)
	}
}

// checkProxyHealth 检查单个代理的健康状态
func (pm *ProxyManager) checkProxyHealth(proxy string) {
	client := pm.GetClient(proxy)
	
	// 使用一个简单的HTTP请求测试代理
	testReq, _ := http.NewRequest("GET", "https://www.google.com", nil)
	testReq.Header.Set("User-Agent", "Mozilla/5.0")
	
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	testReq = testReq.WithContext(ctx)
	
	resp, err := client.Do(testReq)
	healthy := err == nil && resp != nil && resp.StatusCode < 500
	
	if resp != nil {
		resp.Body.Close()
	}
	
	pm.healthMu.Lock()
	pm.healthCheck[proxy] = healthy
	pm.healthMu.Unlock()
	
	// 健康检查结果静默处理，不打印日志
}

// IsHealthy 检查代理是否健康
func (pm *ProxyManager) IsHealthy(proxy string) bool {
	pm.healthMu.RLock()
	defer pm.healthMu.RUnlock()
	return pm.healthCheck[proxy]
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

	// 简单轮询，但跳过最近失败的代理和不健康的代理
	now := time.Now()
	for i := 0; i < len(pm.proxies)*2; i++ {
		proxy := pm.proxies[i%len(pm.proxies)]
		
		// 检查健康状态
		if !pm.IsHealthy(proxy) {
			continue
		}
		
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

	// 如果所有代理都最近失败过，返回第一个健康的
	for _, proxy := range pm.proxies {
		if pm.IsHealthy(proxy) {
			return proxy
		}
	}
	
	// 如果所有代理都不健康，返回第一个
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

// GetClient 获取指定代理的HTTP客户端（带连接池优化）
func (pm *ProxyManager) GetClient(proxyStr string) *http.Client {
	if proxyStr == "" {
		proxyStr = "no_proxy"
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
		MaxIdleConns:        2000,  // 增加空闲连接数
		MaxIdleConnsPerHost: 200,  // 增加每个主机的空闲连接数
		MaxConnsPerHost:     300,  // 增加每个主机的最大连接数
		IdleConnTimeout:     120 * time.Second, // 增加空闲连接超时时间
		// 超时配置 - 快速失败
		TLSHandshakeTimeout:   5 * time.Second,   // 减少TLS握手超时
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
	return client
}

