package main

import (
	"net/http"
	"net/url"
	"sync"
	"time"
)

// ProxyManager 代理管理器，用于复用HTTP客户端
type ProxyManager struct {
	clients map[string]*http.Client
	clientMu sync.RWMutex
}

var globalProxyManager *ProxyManager
var once sync.Once

// GetProxyManager 获取全局代理管理器（单例）
func GetProxyManager() *ProxyManager {
	once.Do(func() {
		globalProxyManager = &ProxyManager{
			clients: make(map[string]*http.Client),
		}
	})
	return globalProxyManager
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

	// 优化的Transport配置 - 禁用连接复用以避免连接被强制关闭
	transport := &http.Transport{
		Proxy: http.ProxyURL(proxyURL),
		// 连接池配置 - 禁用连接复用以避免代理服务器强制关闭连接
		MaxIdleConns:        0,  // 禁用连接池
		MaxIdleConnsPerHost: 0, // 禁用每个主机的连接池
		MaxConnsPerHost:     0,  // 不限制连接数
		IdleConnTimeout:     0,  // 立即关闭空闲连接
		// 超时配置 - 增加超时时间以提高成功率
		TLSHandshakeTimeout:   15 * time.Second,   // TLS握手超时（增加）
		ExpectContinueTimeout: 2 * time.Second,     // Expect Continue超时
		ResponseHeaderTimeout: 30 * time.Second,   // 响应头超时（增加以应对代理延迟）
		// 连接复用 - 禁用以避免连接被强制关闭
		DisableKeepAlives: true, // 每次请求都新建连接，避免连接复用导致的问题
		// 禁用HTTP/2（代理可能不支持）
		ForceAttemptHTTP2: false,
		// 其他配置
		DisableCompression: false,
	}

	client = &http.Client{
		Timeout:   45 * time.Second, // 总超时时间（增加以提高成功率）
		Transport: transport,
	}

	pm.clients[proxyStr] = client
	return client
}

