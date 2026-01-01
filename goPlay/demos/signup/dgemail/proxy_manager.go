package main

import (
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

// ProxyStats 代理统计信息
type ProxyStats struct {
	UseCount int64 // 使用次数
	MaxUses  int64 // 最大使用次数（0=不限制）
}

// ProxyTemplate 代理模板（用于生成多个连接）
type ProxyTemplate struct {
	OriginalURL string   // 原始代理URL
	Username    string   // 用户名（不含连接ID部分）
	Password    string   // 密码
	Host        string   // 主机地址
	Port        string   // 端口
	Scheme      string   // 协议（socks5h, http等）
	MaxUses     int64    // 每个生成的代理最大使用次数（0=不限制）
	ConnCounter int64    // 连接计数器（原子操作）
	ProxyIndex  int64    // 轮询索引（用于在已生成的代理之间轮询）
}

// ProxyManager 代理管理器，用于复用HTTP客户端和代理生成
type ProxyManager struct {
	proxies      []string              // 原始代理列表（普通代理）
	templates    []*ProxyTemplate      // 代理模板列表
	generated    map[string]string     // 生成的代理映射（key=生成的代理URL, value=模板key）
	generatedMu  sync.RWMutex
	stats        map[string]*ProxyStats // 代理使用统计
	statsMu      sync.RWMutex
	clients      map[string]*http.Client
	clientMu     sync.RWMutex
	proxyIndex   int64 // 轮询索引
}

var globalProxyManager *ProxyManager
var once sync.Once

// parseProxyTemplate 解析代理URL，提取模板信息
func parseProxyTemplate(proxyURL string) (*ProxyTemplate, error) {
	u, err := url.Parse(proxyURL)
	if err != nil {
		return nil, fmt.Errorf("invalid proxy URL: %w", err)
	}

	template := &ProxyTemplate{
		OriginalURL: proxyURL,
		Scheme:      u.Scheme,
		Host:        u.Hostname(),
		Port:        u.Port(),
		MaxUses:     0, // 默认不限制
	}

	// 解析用户名和密码
	if u.User != nil {
		template.Username = u.User.Username()
		template.Password, _ = u.User.Password()
	}

	// 默认所有代理都生成连接ID（除非明确禁用）
	// 可以通过环境变量 PROXY_NO_GENERATE_CONN_ID=1 禁用自动生成
	shouldGenerateConnID := !getEnvBool("PROXY_NO_GENERATE_CONN_ID", false) && !getEnvBool("SIGNUP_PROXY_NO_GENERATE_CONN_ID", false)

	if shouldGenerateConnID {
		// 提取基础用户名（去掉可能的连接ID后缀）
		parts := strings.Split(template.Username, "-conn-")
		template.Username = parts[0] // 基础用户名
	}

	return template, nil
}

// generateProxy 从模板生成带连接ID的代理URL
func (pt *ProxyTemplate) generateProxy(connID int64) string {
	username := fmt.Sprintf("%s-conn-%d", pt.Username, connID)
	proxyURL := fmt.Sprintf("%s://%s:%s@%s:%s", pt.Scheme, username, pt.Password, pt.Host, pt.Port)
	return proxyURL
}

// InitProxyManager 初始化代理管理器（signup 项目专用）
func InitProxyManager(proxies []string) {
	once.Do(func() {
		// signup 项目专用配置：SIGNUP_PROXY_MAX_USES，如果没有则使用通用配置 PROXY_MAX_USES
		maxUses := int64(getEnvInt("SIGNUP_PROXY_MAX_USES", getEnvInt("PROXY_MAX_USES", 0))) // 每个代理最大使用次数（0=不限制）

		globalProxyManager = &ProxyManager{
			proxies:   make([]string, 0),
			templates: make([]*ProxyTemplate, 0),
			generated: make(map[string]string),
			stats:     make(map[string]*ProxyStats),
			clients:   make(map[string]*http.Client),
		}

		// 解析代理，生成模板
		for _, proxy := range proxies {
			template, err := parseProxyTemplate(proxy)
			if err != nil {
				// 解析失败，作为普通代理处理
				globalProxyManager.proxies = append(globalProxyManager.proxies, proxy)
				globalProxyManager.stats[proxy] = &ProxyStats{
					MaxUses: maxUses,
				}
				continue
			}

			// 默认所有代理都生成连接ID（除非明确禁用）
			// 可以通过环境变量 PROXY_NO_GENERATE_CONN_ID=1 禁用自动生成
			shouldGenerateConnID := !getEnvBool("PROXY_NO_GENERATE_CONN_ID", false) && !getEnvBool("SIGNUP_PROXY_NO_GENERATE_CONN_ID", false)

			if shouldGenerateConnID {
				// 使用模板模式，生成连接ID
				template.MaxUses = maxUses
				globalProxyManager.templates = append(globalProxyManager.templates, template)
			} else {
				// 普通代理，直接使用（不生成连接ID）
				globalProxyManager.proxies = append(globalProxyManager.proxies, proxy)
				globalProxyManager.stats[proxy] = &ProxyStats{
					MaxUses: maxUses,
				}
			}
		}

		// 预生成代理链接（根据并发数提前生成）
		PreGenerateProxies()
	})
}

// getOrGenerateProxy 从模板获取或生成新的代理（轮询使用不同的连接ID）
func (pm *ProxyManager) getOrGenerateProxy(template *ProxyTemplate) string {
	pm.statsMu.Lock()
	defer pm.statsMu.Unlock()

	// 优先收集未使用过的代理（UseCount == 0）
	unusedProxies := make([]string, 0)
	// 其次收集已使用但未达上限的代理
	usedProxies := make([]string, 0)

	for genProxy, templateKey := range pm.generated {
		if templateKey != template.OriginalURL {
			continue
		}
		stat := pm.stats[genProxy]
		if stat != nil {
			useCount := atomic.LoadInt64(&stat.UseCount)
			maxUses := atomic.LoadInt64(&stat.MaxUses)
			if maxUses == 0 || useCount < maxUses {
				if useCount == 0 {
					// 未使用过的代理
					unusedProxies = append(unusedProxies, genProxy)
				} else {
					// 已使用但未达上限的代理
					usedProxies = append(usedProxies, genProxy)
				}
			}
		}
	}

	// 优先使用未使用过的代理（轮询选择）
	if len(unusedProxies) > 0 {
		proxyIdx := int(atomic.AddInt64(&template.ProxyIndex, 1)-1) % len(unusedProxies)
		selectedProxy := unusedProxies[proxyIdx]
		// 立即增加使用次数，避免并发时多个 goroutine 选择同一个代理
		if stat := pm.stats[selectedProxy]; stat != nil {
			atomic.AddInt64(&stat.UseCount, 1)
		}
		return selectedProxy
	}

	// 如果没有未使用的代理，使用已使用但未达上限的代理（轮询选择）
	if len(usedProxies) > 0 {
		proxyIdx := int(atomic.AddInt64(&template.ProxyIndex, 1)-1) % len(usedProxies)
		selectedProxy := usedProxies[proxyIdx]
		// 立即增加使用次数
		if stat := pm.stats[selectedProxy]; stat != nil {
			atomic.AddInt64(&stat.UseCount, 1)
		}
		return selectedProxy
	}

	// 没有可用代理，生成新的
	connID := atomic.AddInt64(&template.ConnCounter, 1)
	genProxy := template.generateProxy(connID)

	// 初始化统计信息
	pm.stats[genProxy] = &ProxyStats{
		MaxUses:  template.MaxUses,
		UseCount: 0,
	}

	// 记录生成的代理
	pm.generated[genProxy] = template.OriginalURL

	return genProxy
}

// GetNextProxy 获取下一个代理（轮询方式，支持模板代理生成）
func (pm *ProxyManager) GetNextProxy() string {
	// 优先从模板生成代理
	if len(pm.templates) > 0 {
		// 轮询模板
		templateIdx := int(atomic.AddInt64(&pm.proxyIndex, 1)-1) % len(pm.templates)
		template := pm.templates[templateIdx]
		genProxy := pm.getOrGenerateProxy(template)
		if genProxy != "" {
			// 检查使用次数
			pm.statsMu.RLock()
			stat := pm.stats[genProxy]
			pm.statsMu.RUnlock()
			if stat != nil {
				useCount := atomic.LoadInt64(&stat.UseCount)
				maxUses := atomic.LoadInt64(&stat.MaxUses)
				if maxUses == 0 || useCount < maxUses {
					return genProxy
				}
			}
		}
	}

	// 降级到普通代理
	if len(pm.proxies) > 0 {
		pm.statsMu.RLock()
		defer pm.statsMu.RUnlock()

		for i := 0; i < len(pm.proxies)*2; i++ {
			proxyIdx := int(atomic.AddInt64(&pm.proxyIndex, 1)-1) % len(pm.proxies)
			proxy := pm.proxies[proxyIdx]

			stat := pm.stats[proxy]
			if stat == nil {
				return proxy
			}

			// 检查使用次数
			useCount := atomic.LoadInt64(&stat.UseCount)
			maxUses := atomic.LoadInt64(&stat.MaxUses)
			if maxUses > 0 && useCount >= maxUses {
				continue
			}

			return proxy
		}

		// 如果所有代理都达到上限，返回第一个
		if len(pm.proxies) > 0 {
			return pm.proxies[0]
		}
	}

	return ""
}

// RecordProxyUse 记录代理使用（成功或失败都算使用）
func (pm *ProxyManager) RecordProxyUse(proxy string) {
	if proxy == "" {
		return
	}
	pm.statsMu.RLock()
	stat := pm.stats[proxy]
	pm.statsMu.RUnlock()
	if stat != nil {
		atomic.AddInt64(&stat.UseCount, 1)
	}
}

// PreGenerateProxies 预生成代理链接（根据并发数提前生成，例如1000并发生成2000个代理链接）
func PreGenerateProxies() {
	if globalProxyManager == nil {
		return
	}

	// 获取并发数（从环境变量读取）
	maxConcurrency := getEnvInt("SIGNUP_CONCURRENCY", 50)
	if maxConcurrency <= 0 {
		maxConcurrency = 50
	}

	// 计算需要生成的代理数量：并发数 * 2（例如1000并发生成2000个）
	// 可以通过环境变量 SIGNUP_PROXY_PREGEN_MULTIPLIER 自定义倍数，默认为2
	multiplier := getEnvInt("SIGNUP_PROXY_PREGEN_MULTIPLIER", 2)
	if multiplier <= 0 {
		multiplier = 2
	}
	targetCount := maxConcurrency * multiplier

	// 如果没有模板代理，无法预生成
	if len(globalProxyManager.templates) == 0 {
		return
	}

	// 计算每个模板需要生成的代理数量
	proxiesPerTemplate := targetCount / len(globalProxyManager.templates)
	if proxiesPerTemplate <= 0 {
		proxiesPerTemplate = 1
	}

	// 为每个模板预生成代理链接
	globalProxyManager.statsMu.Lock()
	defer globalProxyManager.statsMu.Unlock()

	totalGenerated := 0
	for _, template := range globalProxyManager.templates {
		for i := int64(0); i < int64(proxiesPerTemplate); i++ {
			connID := atomic.AddInt64(&template.ConnCounter, 1)
			genProxy := template.generateProxy(connID)

			// 初始化统计信息
			globalProxyManager.stats[genProxy] = &ProxyStats{
				MaxUses:  template.MaxUses,
				UseCount: 0,
			}

			// 记录生成的代理
			globalProxyManager.generated[genProxy] = template.OriginalURL
			totalGenerated++
		}
	}

	// 如果还有余数，再为前几个模板各生成一个
	remainder := targetCount % len(globalProxyManager.templates)
	for i := 0; i < remainder && i < len(globalProxyManager.templates); i++ {
		template := globalProxyManager.templates[i]
		connID := atomic.AddInt64(&template.ConnCounter, 1)
		genProxy := template.generateProxy(connID)

		// 初始化统计信息
		globalProxyManager.stats[genProxy] = &ProxyStats{
			MaxUses:  template.MaxUses,
			UseCount: 0,
		}

		// 记录生成的代理
		globalProxyManager.generated[genProxy] = template.OriginalURL
		totalGenerated++
	}

	fmt.Printf("已预生成 %d 个代理链接（并发数: %d, 倍数: %d, 模板数: %d）\n", totalGenerated, maxConcurrency, multiplier, len(globalProxyManager.templates))
}

// GetProxyManager 获取全局代理管理器（单例）
func GetProxyManager() *ProxyManager {
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

