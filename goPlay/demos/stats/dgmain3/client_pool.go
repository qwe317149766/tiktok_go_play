package main

import (
	"net/http"
	"net/url"
	"sync"
	"time"
)

// ClientPool HTTP客户端池 - 高性能版本
type ClientPool struct {
	clients map[string]*http.Client
	mu      sync.RWMutex
}

var globalPool = &ClientPool{
	clients: make(map[string]*http.Client),
}

// GetClient 获取或创建指定代理的HTTP客户端
func (p *ClientPool) GetClient(proxyStr string) *http.Client {
	key := proxyStr
	if key == "" {
		key = "no_proxy"
	}

	// 快速路径：读锁
	p.mu.RLock()
	client, exists := p.clients[key]
	p.mu.RUnlock()

	if exists {
		return client
	}

	// 慢速路径：写锁
	p.mu.Lock()
	defer p.mu.Unlock()

	// 双重检查
	if client, exists := p.clients[key]; exists {
		return client
	}

	// 解析代理URL
	var proxyURL *url.URL
	if proxyStr != "" {
		var err error
		proxyURL, err = url.Parse(proxyStr)
		if err != nil {
			proxyURL = nil
		}
	} else {
		proxyURL, _ = url.Parse("http://127.0.0.1:7777")
	}

	// 高性能Transport配置 - 优化以减少连接被关闭
	transport := &http.Transport{
		Proxy: http.ProxyURL(proxyURL),
		// 连接池优化 - 平衡配置
		MaxIdleConns:        300,              // 适中的连接池大小
		MaxIdleConnsPerHost: 30,               // 每个主机适中连接数
		MaxConnsPerHost:     50,               // 限制每个主机并发（避免过载）
		IdleConnTimeout:     60 * time.Second, // 适中的空闲超时
		// 超时配置 - 更短的超时以减少等待
		TLSHandshakeTimeout:   8 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		ResponseHeaderTimeout: 15 * time.Second,
		// 连接复用 - 启用但限制复用时间
		DisableKeepAlives: false,
		// 禁用HTTP/2（某些代理不支持）
		ForceAttemptHTTP2: false,
		// 其他优化
		DisableCompression: false,
		// 连接配置 - 添加连接超时
		DialContext: nil, // 使用默认dialer
	}

	client = &http.Client{
		Timeout:   25 * time.Second, // 稍微缩短超时以提高响应速度
		Transport: transport,
	}

	p.clients[key] = client
	return client
}

// GetClientForProxy 全局函数获取客户端
func GetClientForProxy(proxyStr string) *http.Client {
	return globalPool.GetClient(proxyStr)
}

