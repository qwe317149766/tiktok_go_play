package main

import (
	"net/http"
	"net/url"
	"sync"
	"time"
)

var (
	clientPool   = make(map[string]*http.Client)
	clientPoolMu sync.RWMutex
)

// GetClientForProxy 根据代理获取HTTP客户端（备用方法，当代理管理器不可用时使用）
func GetClientForProxy(proxyStr string) *http.Client {
	if proxyStr == "" {
		proxyStr = "no_proxy"
	}

	// 快速路径：读锁
	clientPoolMu.RLock()
	client, exists := clientPool[proxyStr]
	clientPoolMu.RUnlock()

	if exists {
		return client
	}

	// 慢速路径：写锁创建
	clientPoolMu.Lock()
	defer clientPoolMu.Unlock()

	// 双重检查
	if client, exists := clientPool[proxyStr]; exists {
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

	// 优化的Transport配置
	transport := &http.Transport{
		Proxy:                 http.ProxyURL(proxyURL),
		MaxIdleConns:          1000,
		MaxIdleConnsPerHost:   100,
		MaxConnsPerHost:       200,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   10 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		ResponseHeaderTimeout: 15 * time.Second,
		DisableKeepAlives:     false,
		ForceAttemptHTTP2:     false,
		DisableCompression:    false,
	}

	client = &http.Client{
		Timeout:   30 * time.Second,
		Transport: transport,
	}

	clientPool[proxyStr] = client
	return client
}

