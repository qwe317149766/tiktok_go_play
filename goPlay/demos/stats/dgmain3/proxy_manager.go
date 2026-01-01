package main

import (
	"context"
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
	Success    int64
	Failed     int64
	LastUsed   time.Time
	LastError  time.Time
	ConsecutiveFailures int64
	UseCount   int64 // 使用次数
	MaxUses    int64 // 最大使用次数（0=不限制）
}

// ProxyTemplate 代理模板（用于生成多个连接）
type ProxyTemplate struct {
	OriginalURL string // 原始代理URL
	BaseURL     string // 基础URL（不含用户名）
	Username    string // 用户名（不含连接ID部分）
	Password    string // 密码
	Host        string // 主机地址
	Port        string // 端口
	Scheme      string // 协议（socks5h, http等）
	MaxUses     int64  // 每个生成的代理最大使用次数（0=不限制）
	ConnCounter int64  // 连接计数器（原子操作）
	ProxyIndex  int64  // 轮询索引（用于在已生成的代理之间轮询）
}

// ProxyManager 代理管理器 - 智能代理选择和管理
type ProxyManager struct {
	proxies      []string              // 原始代理列表
	templates    []*ProxyTemplate      // 代理模板列表
	generated    map[string]string     // 生成的代理映射（key=生成的代理URL, value=模板key）
	generatedMu  sync.RWMutex
	stats        map[string]*ProxyStats
	mu           sync.RWMutex
	clients      map[string]*http.Client
	clientMu     sync.RWMutex
	healthCheck  map[string]bool // 代理健康状态
	healthMu     sync.RWMutex
	stopHealth   chan struct{}   // 停止健康检查信号
	maxUsesPerProxy int64        // 每个代理最大使用次数（默认值，可通过环境变量配置）
}

var globalProxyManager *ProxyManager
var proxyManagerOnce sync.Once

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
	shouldGenerateConnID := !envBool("PROXY_NO_GENERATE_CONN_ID", false) && !envBool("STATS_PROXY_NO_GENERATE_CONN_ID", false)

	if shouldGenerateConnID {
		// 提取基础用户名（去掉可能的连接ID后缀）
		parts := strings.Split(template.Username, "-conn-")
		template.Username = parts[0] // 基础用户名
	}

	return template, nil
}

// generateProxyFromTemplate 从模板生成带连接ID的代理URL
func (pt *ProxyTemplate) generateProxy(connID int64) string {
	username := fmt.Sprintf("%s-conn-%d", pt.Username, connID)
	proxyURL := fmt.Sprintf("%s://%s:%s@%s:%s", pt.Scheme, username, pt.Password, pt.Host, pt.Port)
	return proxyURL
}

// InitProxyManager 初始化代理管理器
func InitProxyManager(proxies []string) {
	proxyManagerOnce.Do(func() {
		// stats 项目专用配置：STATS_PROXY_MAX_USES，如果没有则使用通用配置 PROXY_MAX_USES
		maxUses := int64(envInt("STATS_PROXY_MAX_USES", envInt("PROXY_MAX_USES", 0))) // 每个代理最大使用次数（0=不限制）
		
		globalProxyManager = &ProxyManager{
			proxies:         proxies,
			templates:       make([]*ProxyTemplate, 0),
			generated:       make(map[string]string),
			stats:           make(map[string]*ProxyStats),
			clients:         make(map[string]*http.Client),
			healthCheck:     make(map[string]bool),
			stopHealth:      make(chan struct{}),
			maxUsesPerProxy: maxUses,
		}

		// 解析代理，生成模板
		normalProxies := make([]string, 0)
		for _, proxy := range proxies {
			template, err := parseProxyTemplate(proxy)
			if err != nil {
				// 解析失败，作为普通代理处理
				normalProxies = append(normalProxies, proxy)
				globalProxyManager.stats[proxy] = &ProxyStats{
					LastUsed: time.Now(),
					MaxUses:  maxUses,
				}
				globalProxyManager.healthCheck[proxy] = true
				continue
			}

			// 默认所有代理都生成连接ID（除非明确禁用）
			// 可以通过环境变量 PROXY_NO_GENERATE_CONN_ID=1 禁用自动生成
			shouldGenerateConnID := !envBool("PROXY_NO_GENERATE_CONN_ID", false) && !envBool("STATS_PROXY_NO_GENERATE_CONN_ID", false)

			if shouldGenerateConnID {
				// 使用模板模式，生成连接ID
				template.MaxUses = maxUses
				globalProxyManager.templates = append(globalProxyManager.templates, template)
			} else {
				// 普通代理，直接使用（不生成连接ID）
				normalProxies = append(normalProxies, proxy)
				globalProxyManager.stats[proxy] = &ProxyStats{
					LastUsed: time.Now(),
					MaxUses:  maxUses,
				}
				globalProxyManager.healthCheck[proxy] = true
			}
		}
		// 更新普通代理列表（不包含模板代理）
		globalProxyManager.proxies = normalProxies

		// 预生成代理链接（根据并发数提前生成）
		PreGenerateProxies()

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

// getOrGenerateProxy 从模板获取或生成新的代理（轮询使用不同的连接ID）
func (pm *ProxyManager) getOrGenerateProxy(template *ProxyTemplate) string {
	pm.mu.Lock()
	defer pm.mu.Unlock()

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
		LastUsed: time.Now(),
		MaxUses:  template.MaxUses,
		UseCount: 0,
	}
	pm.healthCheck[genProxy] = true

	// 记录生成的代理
	pm.generated[genProxy] = template.OriginalURL

	return genProxy
}

// GetNextProxy 获取下一个代理（轮询方式，但跳过最近失败和不健康的）
func (pm *ProxyManager) GetNextProxy() string {
	// 优先从模板生成代理
	pm.mu.RLock()
	templates := make([]*ProxyTemplate, len(pm.templates))
	copy(templates, pm.templates)
	pm.mu.RUnlock()

	if len(templates) > 0 {
		// 轮询模板，找到可用的生成代理
		for _, template := range templates {
			genProxy := pm.getOrGenerateProxy(template)
			if genProxy != "" {
				// 检查健康状态和使用次数
				if pm.IsHealthy(genProxy) {
					pm.mu.RLock()
					stat := pm.stats[genProxy]
					pm.mu.RUnlock()
					if stat != nil {
						useCount := atomic.LoadInt64(&stat.UseCount)
						maxUses := atomic.LoadInt64(&stat.MaxUses)
						if maxUses == 0 || useCount < maxUses {
							return genProxy
						}
					}
				}
			}
		}
	}

	// 降级到普通代理
	pm.mu.RLock()
	defer pm.mu.RUnlock()

	if len(pm.proxies) == 0 {
		return ""
	}

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

		// 检查使用次数
		useCount := atomic.LoadInt64(&stat.UseCount)
		maxUses := atomic.LoadInt64(&stat.MaxUses)
		if maxUses > 0 && useCount >= maxUses {
			continue
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
		atomic.AddInt64(&stat.UseCount, 1) // 增加使用次数
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
		atomic.AddInt64(&stat.UseCount, 1) // 失败也算使用次数
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

// PreGenerateProxies 预生成代理链接（根据并发数提前生成，例如1000并发生成2000个代理链接）
func PreGenerateProxies() {
	if globalProxyManager == nil {
		return
	}

	// 获取并发数（从环境变量读取，优先级：STATS_CONCURRENCY > GEN_CONCURRENCY > 默认值）
	maxConcurrency := envInt("STATS_CONCURRENCY", 0)
	if maxConcurrency <= 0 {
		maxConcurrency = envInt("GEN_CONCURRENCY", 0)
	}
	if maxConcurrency <= 0 {
		maxConcurrency = 500 // stats 项目默认并发数
	}

	// 计算需要生成的代理数量：并发数 * 2（例如1000并发生成2000个）
	// 可以通过环境变量 STATS_PROXY_PREGEN_MULTIPLIER 自定义倍数，默认为2
	multiplier := envInt("STATS_PROXY_PREGEN_MULTIPLIER", 2)
	if multiplier <= 0 {
		multiplier = 2
	}
	targetCount := maxConcurrency * multiplier

	// 如果没有模板代理，无法预生成
	globalProxyManager.mu.RLock()
	templateCount := len(globalProxyManager.templates)
	globalProxyManager.mu.RUnlock()

	if templateCount == 0 {
		return
	}

	// 计算每个模板需要生成的代理数量
	proxiesPerTemplate := targetCount / templateCount
	if proxiesPerTemplate <= 0 {
		proxiesPerTemplate = 1
	}

	// 为每个模板预生成代理链接
	globalProxyManager.mu.Lock()
	defer globalProxyManager.mu.Unlock()

	totalGenerated := 0
	for _, template := range globalProxyManager.templates {
		for i := int64(0); i < int64(proxiesPerTemplate); i++ {
			connID := atomic.AddInt64(&template.ConnCounter, 1)
			genProxy := template.generateProxy(connID)

			// 初始化统计信息
			globalProxyManager.stats[genProxy] = &ProxyStats{
				LastUsed: time.Now(),
				MaxUses:  template.MaxUses,
				UseCount: 0,
			}
			globalProxyManager.healthCheck[genProxy] = true

			// 记录生成的代理
			globalProxyManager.generated[genProxy] = template.OriginalURL
			totalGenerated++
		}
	}

	// 如果还有余数，再为前几个模板各生成一个
	remainder := targetCount % templateCount
	for i := 0; i < remainder && i < len(globalProxyManager.templates); i++ {
		template := globalProxyManager.templates[i]
		connID := atomic.AddInt64(&template.ConnCounter, 1)
		genProxy := template.generateProxy(connID)

		// 初始化统计信息
		globalProxyManager.stats[genProxy] = &ProxyStats{
			LastUsed: time.Now(),
			MaxUses:  template.MaxUses,
			UseCount: 0,
		}
		globalProxyManager.healthCheck[genProxy] = true

		// 记录生成的代理
		globalProxyManager.generated[genProxy] = template.OriginalURL
		totalGenerated++
	}

	fmt.Printf("已预生成 %d 个代理链接（并发数: %d, 倍数: %d, 模板数: %d）\n", totalGenerated, maxConcurrency, multiplier, templateCount)
}

