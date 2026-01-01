package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"tt_code/demos/signup/email"
)

// AccountInfo 账号信息
type AccountInfo struct {
	Email    string
	Password string
}

// RegisterResult 注册结果
type RegisterResult struct {
	Email       string
	Success     bool
	Cookies     map[string]string
	UserSession map[string]string
	Error       error
	DeviceID    string
	Proxy       string
}

// 全局统计
var (
	totalCount   int64
	successCount int64
	failedCount  int64
	resultMutex  sync.Mutex
	results      []RegisterResult
)

func main() {
	fmt.Println("=== TikTok 邮箱批量注册工具 ===")
	fmt.Println()

	// 载入 env（优先读取 dgemail 目录或仓库根目录的 env.windows/env.linux）
	loadEnvForDemo()

	// 轮询补齐模式：检查 DB cookies 池缺口，自动补齐（建议 run-once + cron 调度）
	if dgemailPollEnabled() {
		runCookiePollLoop()
		return
	}

	// 确保目录存在
	os.MkdirAll("data", 0755)
	os.MkdirAll("res", 0755)

	// 1. 读取账号列表
	accounts, err := loadAccounts("data/accounts.txt")
	if err != nil {
		log.Fatalf("读取账号列表失败: %v", err)
	}

	// 如果账号列表为空，则根据设备数量自动生成随机账号
	if len(accounts) == 0 {
		fmt.Println("账号列表为空，将自动生成随机账号")
	}

	// 2. 读取设备列表
	var devices []map[string]interface{}
	// 全 DB 模式：signup 设备固定从 MySQL 设备池读取
	if shouldLoadDevicesFromDB() {
		limit := getEnvInt("DEVICES_LIMIT", getEnvInt("MAX_GENERATE", 0))
		devices, err = loadDevicesFromDB(limit)
		if err != nil {
			log.Fatalf("从MySQL读取设备失败: %v", err)
		}
		fmt.Printf("已从MySQL加载 %d 个设备\n", len(devices))
	} else {
		log.Fatalf("SIGNUP_DEVICES_SOURCE 未配置为 db/mysql：当前版本已移除 file/redis 设备来源，请设置 SIGNUP_DEVICES_SOURCE=db")
	}
	if err != nil {
		log.Fatalf("读取设备列表失败: %v", err)
	}

	// DB 模式：在 loadDevicesFromDB 内已做筛选；这里补一句可观测日志
	if shouldLoadDevicesFromDB() {
		if h := getSignupDeviceMinAgeHours(); h > 0 {
			fmt.Printf("设备已按 create_time 早于 %d 小时筛选（db 模式）\n", h)
		}
	}

	// 如果账号列表为空，根据设备数量生成随机账号
	if len(accounts) == 0 {
		// 注册数量从配置读取（STARTUP_REGISTER_COUNT 优先，否则回退 MAX_GENERATE）
		target := getEnvInt("STARTUP_REGISTER_COUNT", getEnvInt("MAX_GENERATE", len(devices)))
		if target <= 0 {
			target = len(devices)
		}
		// 允许设备复用：设备会在并发注册中按索引取模轮询使用
		// 注意：设备复用可能增加风控/封禁概率，这是预期行为
		if target > len(devices) && len(devices) > 0 {
			fmt.Printf("⚠️  STARTUP_REGISTER_COUNT=%d 大于设备数量=%d，将循环复用设备进行注册\n", target, len(devices))
		}
		accounts = generateUniqueAccounts(target)
		fmt.Printf("已生成 %d 个账号（不重复，来自 STARTUP_REGISTER_COUNT/MAX_GENERATE）\n", len(accounts))
	} else {
		fmt.Printf("已加载 %d 个账号\n", len(accounts))
	}

	// 3. 读取代理列表
	// 代理文件优先级（按你的需求：优先读取 dgemail 自己目录下的代理，而不是仓库根目录）
	// - PROXIES_FILE（统一配置，推荐）
	// - SIGNUP_PROXIES_FILE（signup 专用，兼容）
	// - 当前目录 proxies.txt
	// - 当前目录 data/proxies.txt（旧默认）
	// - 最后兜底：向上查找仓库根目录 proxies.txt（兼容旧项目统一代理）
	proxyPath := strings.TrimSpace(getEnvStr("PROXIES_FILE", ""))
	if proxyPath == "" {
		proxyPath = strings.TrimSpace(getEnvStr("SIGNUP_PROXIES_FILE", ""))
	}
	if proxyPath == "" && fileExists("proxies.txt") {
		proxyPath = "proxies.txt"
	}
	if proxyPath == "" && fileExists("data/proxies.txt") {
		proxyPath = "data/proxies.txt"
	}
	if proxyPath == "" {
		proxyPath = findTopmostFileUpwards("proxies.txt", 8)
	}
	if proxyPath == "" {
		// 最后再回退旧默认（理论上前面 fileExists 已覆盖）
		proxyPath = "data/proxies.txt"
	}
	proxies, err := loadProxies(proxyPath)
	if err != nil {
		log.Fatalf("读取代理列表失败: %v", err)
	}
	fmt.Printf("已加载 %d 个代理\n", len(proxies))

	// 4. 设置并发数（降低并发以提高成功率）
	maxConcurrency := getEnvInt("SIGNUP_CONCURRENCY", 50)
	if maxConcurrency <= 0 {
		maxConcurrency = 50
	}
	fmt.Printf("并发数: %d (来自 SIGNUP_CONCURRENCY)\n", maxConcurrency)
	fmt.Println()

	// DB 模式：注册成功会直接写入 MySQL cookies 池（startup_cookie_accounts）

	// 5. 开始并发注册
	startTime := time.Now()
	registerAccounts(accounts, devices, proxies, maxConcurrency)

	// 6. 输出统计结果
	duration := time.Since(startTime)
	fmt.Println("\n=== 注册完成 ===")
	fmt.Printf("总账号数: %d\n", atomic.LoadInt64(&totalCount))
	fmt.Printf("成功: %d\n", atomic.LoadInt64(&successCount))
	fmt.Printf("失败: %d\n", atomic.LoadInt64(&failedCount))
	fmt.Printf("耗时: %v\n", duration)
	fmt.Printf("平均速度: %.2f 账号/秒\n", float64(atomic.LoadInt64(&totalCount))/duration.Seconds())

	// 7. 保存结果
	saveResults("res/register_results.json")
	saveSuccessAccounts("res/success_accounts.txt")
	saveFailedAccounts("res/failed_accounts.txt")
	saveDevicesWithCookies(startupDevicesWithCookiesOutPath(), devices)
	// 7.1 固定目录 JSONL 日志（例如 results_w01_part0002.jsonl）
	saveResultsJSONLFixed()

	// 8. 组装“注册成功账号数据”（设备字段 + cookies 字段）
	// 注意：你要求 stats 直接从账号池读取设备+cookies，因此这里的账号数据会写入 MySQL cookies 池表（startup_cookie_accounts）。
	startupDevs := buildStartupDevicesWithCookies(devices, results)
	// DB 模式：账号池写入在注册成功时已完成；这里保留 startupDevs 仅用于文件输出/对账。
	_ = startupDevs
}

// startupDevicesWithCookiesOutPath 生成“类似 devices12_20.txt 的文件名”，用于保存 signup 成功账号 JSON（每行一个）
// - 可通过 env 覆盖：DGEMAIL_STARTUP_DEVICES_FILE
// - 默认：goPlay/demos/signup/dgemail/res/devicesMM_DD.txt（与你现有命名一致）
func startupDevicesWithCookiesOutPath() string {
	if p := strings.TrimSpace(getEnvStr("DGEMAIL_STARTUP_DEVICES_FILE", "")); p != "" {
		return p
	}
	name := time.Now().Format("devices01_02.txt") // 例如 devices12_20.txt
	return filepath.Join("res", name)
}

// generateRandomAccounts 生成随机账号
func generateRandomAccounts(count int) []AccountInfo {
	// 兼容旧代码：保留函数名，但内部改为“尽量不重复”生成（优先用 Redis 序号）
	return generateUniqueAccounts(count)
}

// generateRandomString 生成随机字符串
func generateRandomString(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[rand.Intn(len(charset))]
	}
	return string(b)
}

// loadAccounts 从文件加载账号列表
// 格式：每行一个账号，格式为 email:password 或 email,password
// 如果文件不存在或为空，返回空列表
func loadAccounts(filename string) ([]AccountInfo, error) {
	file, err := os.Open(filename)
	if err != nil {
		// 文件不存在时返回空列表，不报错
		return []AccountInfo{}, nil
	}
	defer file.Close()

	var accounts []AccountInfo
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		var email, password string
		if strings.Contains(line, ":") {
			parts := strings.SplitN(line, ":", 2)
			email = strings.TrimSpace(parts[0])
			password = strings.TrimSpace(parts[1])
		} else if strings.Contains(line, ",") {
			parts := strings.SplitN(line, ",", 2)
			email = strings.TrimSpace(parts[0])
			password = strings.TrimSpace(parts[1])
		} else {
			continue
		}

		if email != "" && password != "" {
			accounts = append(accounts, AccountInfo{
				Email:    email,
				Password: password,
			})
		}
	}

	return accounts, scanner.Err()
}

// DeviceInfo 设备信息结构（保留原始类型）
type DeviceInfo struct {
	RawData map[string]interface{} // 原始数据
}

// loadDevices 从文件加载设备列表
// 格式：每行一个设备的JSON字符串
// 返回原始数据，保留数字类型
func loadDevices(filename string) ([]map[string]interface{}, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var devices []map[string]interface{}
	scanner := bufio.NewScanner(file)
	lineNum := 0
	for scanner.Scan() {
		lineNum++
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// 解析为 map[string]interface{}，保留原始类型
		var deviceRaw map[string]interface{}
		if err := json.Unmarshal([]byte(line), &deviceRaw); err != nil {
			log.Printf("第 %d 行JSON解析失败: %v", lineNum, err)
			continue
		}

		devices = append(devices, deviceRaw)
	}

	return devices, scanner.Err()
}

// convertDeviceToStringMap 将设备信息转换为 map[string]string（用于注册函数）
func convertDeviceToStringMap(device map[string]interface{}) map[string]string {
	result := make(map[string]string)
	for k, v := range device {
		switch val := v.(type) {
		case string:
			result[k] = val
		case float64:
			// JSON数字会被解析为float64
			result[k] = fmt.Sprintf("%.0f", val)
		case int:
			result[k] = fmt.Sprintf("%d", val)
		case int64:
			result[k] = fmt.Sprintf("%d", val)
		case bool:
			result[k] = fmt.Sprintf("%t", val)
		default:
			// 其他类型转为字符串
			result[k] = fmt.Sprintf("%v", val)
		}
	}
	return result
}

// loadProxies 从文件加载代理列表
// 格式：每行一个代理地址
func loadProxies(filename string) ([]string, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var proxies []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		proxies = append(proxies, line)
	}

	return proxies, scanner.Err()
}

// findTopmostFileUpwards 从当前目录开始向上查找文件，返回“最顶层”的那个路径（更接近仓库根目录）。
// 例如：希望所有项目统一使用仓库根目录的 proxies.txt。
func findTopmostFileUpwards(name string, maxUp int) string {
	start, err := os.Getwd()
	if err != nil || start == "" {
		return ""
	}
	start, _ = filepath.Abs(start)

	found := ""
	dir := start
	for i := 0; i <= maxUp; i++ {
		p := filepath.Join(dir, name)
		if _, err := os.Stat(p); err == nil {
			found = p
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	return found
}

// registerAccounts 并发注册账号
func registerAccounts(accounts []AccountInfo, devices []map[string]interface{}, proxies []string, maxConcurrency int) {
	// 创建信号量控制并发数
	semaphore := make(chan struct{}, maxConcurrency)
	var wg sync.WaitGroup

	deviceIndex := int64(0)
	proxyIndex := int64(0)

	for i, account := range accounts {
		wg.Add(1)
		semaphore <- struct{}{} // 获取信号量

		// 增加请求间隔，避免所有请求同时发起（随机延迟0-200ms）
		if i > 0 {
			delay := time.Duration(rand.Intn(200)) * time.Millisecond
			time.Sleep(delay)
		}

		go func(idx int, acc AccountInfo) {
			defer wg.Done()
			defer func() { <-semaphore }() // 释放信号量

			// 选择设备和代理（轮询方式）
			deviceIdx := int(atomic.AddInt64(&deviceIndex, 1)-1) % len(devices)
			proxyIdx := int(atomic.AddInt64(&proxyIndex, 1)-1) % len(proxies)

			deviceRaw := devices[deviceIdx]
			device := convertDeviceToStringMap(deviceRaw) // 转换为字符串map用于注册
			proxy := proxies[proxyIdx]

			// 执行注册
			result := registerSingleAccount(acc, device, proxy, idx+1, len(accounts))

			// 更新统计
			atomic.AddInt64(&totalCount, 1)
			if result.Success {
				atomic.AddInt64(&successCount, 1)
				// ✅ 注册成功立刻构造“账号 JSON（完整设备字段+cookies+create_time）”
				// 要求：写入 DB/文件 的 JSON 必须一致
				if len(result.Cookies) > 0 {
					accJSON := buildStartupAccountJSON(deviceRaw, result.Cookies)
					// 1) 写入 MySQL cookies 池（全 DB 模式）
					if shouldWriteStartupAccountsToDB() {
						if err := writeStartupAccountToDB(accJSON); err != nil {
							log.Printf("[db] 写入 startup_cookie_accounts 失败: %v", err)
						}
					}
					// 2) 实时写入 Log/ startup_accounts_*.jsonl（与 DB 完全一致）
					appendStartupAccountJSONLFixed(accJSON)
				}
				// ✅ signup 设备淘汰策略：成功次数 >3 删设备；连续失败 >10 也删设备
				signupDeviceUsageUpdate(result.DeviceID, true)
			} else {
				atomic.AddInt64(&failedCount, 1)
				// ✅ 连续失败计数
				signupDeviceUsageUpdate(result.DeviceID, false)
			}

			// 保存结果（同时保存原始设备数据用于后续保存）
			resultMutex.Lock()
			results = append(results, result)
			resultMutex.Unlock()
		}(i, account)
	}

	wg.Wait()
}

// registerSingleAccount 注册单个账号（带重试机制）
func registerSingleAccount(account AccountInfo, device map[string]string, proxy string, current, total int) RegisterResult {
	// device key：优先 device_id；缺失时回退 cdid（用于 DB 设备池删除/计数一致性）
	deviceKey := strings.TrimSpace(device["device_id"])
	if deviceKey == "" {
		deviceKey = strings.TrimSpace(device["cdid"])
	}
	result := RegisterResult{
		Email:    account.Email,
		DeviceID: deviceKey,
		Proxy:    proxy,
	}

	fmt.Printf("[%d/%d] 开始注册: %s (设备: %s, 代理: %s)\n", current, total, account.Email, deviceKey, proxy)

	// 获取 seed 和 seedType（带重试，最多10次，优先成功率）
	var seed string
	var seedType int
	var err error
	maxRetries := 10
	for retry := 0; retry < maxRetries; retry++ {
		seed, seedType, err = GetSeed(device, proxy)
		if err == nil && seed != "" {
			if retry > 0 {
				fmt.Printf("[%d/%d] ⚠️  seed 重试成功 (第%d次重试): %s\n", current, total, retry, account.Email)
			}
			break
		}
		if retry < maxRetries-1 {
			// 指数退避：500ms, 1s, 2s, 3s, 5s, 5s...（最大5秒）
			baseDelay := 500 * time.Millisecond
			backoffDelay := time.Duration(1<<uint(retry)) * baseDelay
			if backoffDelay > 5*time.Second {
				backoffDelay = 5 * time.Second
			}
			// 额外增加随机延迟（0-1秒），避免所有请求同时重试
			randomDelay := time.Duration(rand.Intn(1000)) * time.Millisecond
			if retry < 3 {
				// 前3次重试显示日志
				fmt.Printf("[%d/%d] ⏳ seed 重试中 (第%d/%d次): %s - %v\n", current, total, retry+1, maxRetries, account.Email, err)
			}
			time.Sleep(backoffDelay + randomDelay)
			continue
		}
	}
	if err != nil || seed == "" {
		result.Error = fmt.Errorf("获取 seed 失败: %v", err)
		result.Success = false
		fmt.Printf("[%d/%d] ❌ 失败: %s - %v\n", current, total, account.Email, result.Error)
		return result
	}

	// 获取 token（带重试，最多10次，优先成功率）
	var token string
	maxTokenRetries := 10
	for retry := 0; retry < maxTokenRetries; retry++ {
		token = GetToken(device, proxy)
		if token != "" {
			if retry > 0 {
				fmt.Printf("[%d/%d] ⚠️  token 重试成功 (第%d次重试): %s\n", current, total, retry, account.Email)
			}
			break
		}
		if retry < maxTokenRetries-1 {
			// 指数退避：500ms, 1s, 2s, 3s, 5s, 5s...（最大5秒）
			baseDelay := 500 * time.Millisecond
			backoffDelay := time.Duration(1<<uint(retry)) * baseDelay
			if backoffDelay > 5*time.Second {
				backoffDelay = 5 * time.Second
			}
			// 额外增加随机延迟（0-1秒），避免所有请求同时重试
			randomDelay := time.Duration(rand.Intn(1000)) * time.Millisecond
			if retry < 3 {
				// 前3次重试显示日志
				fmt.Printf("[%d/%d] ⏳ token 重试中 (第%d/%d次): %s\n", current, total, retry+1, maxTokenRetries, account.Email)
			}
			time.Sleep(backoffDelay + randomDelay)
			continue
		}
	}
	if token == "" {
		result.Error = fmt.Errorf("获取 token 失败")
		result.Success = false
		fmt.Printf("[%d/%d] ❌ 失败: %s - %v\n", current, total, account.Email, result.Error)
		return result
	}

	// 调用 Register 函数（带重试，最多10次，优先成功率）
	var cookies map[string]string
	var userSession map[string]string
	maxRegisterRetries := 10
	for retry := 0; retry < maxRegisterRetries; retry++ {
		cookies, userSession, err = email.Register(account.Email, account.Password, seed, seedType, token, device, proxy)
		if err == nil {
			break
		}
		// 判断是否是网络错误
		errStr := err.Error()
		isNetworkError := strings.Contains(errStr, "connect") ||
			strings.Contains(errStr, "timeout") ||
			strings.Contains(errStr, "connection") ||
			strings.Contains(errStr, "wsarecv") ||
			strings.Contains(errStr, "EOF") ||
			strings.Contains(errStr, "forcibly closed") ||
			strings.Contains(errStr, "network") ||
			strings.Contains(errStr, "dial")
		// 只有网络错误才重试
		if !isNetworkError || retry >= maxRegisterRetries-1 {
			break
		}
		// 指数退避：500ms, 1s, 2s, 3s, 5s, 5s...（最大5秒）
		baseDelay := 500 * time.Millisecond
		backoffDelay := time.Duration(1<<uint(retry)) * baseDelay
		if backoffDelay > 5*time.Second {
			backoffDelay = 5 * time.Second
		}
		// 额外增加随机延迟（0-1秒），避免所有请求同时重试
		randomDelay := time.Duration(rand.Intn(1000)) * time.Millisecond
		if retry < 3 {
			// 前3次重试显示日志
			fmt.Printf("[%d/%d] ⏳ 注册重试中 (第%d/%d次): %s - %v\n", current, total, retry+1, maxRegisterRetries, account.Email, err)
		}
		time.Sleep(backoffDelay + randomDelay)
	}
	if err != nil {
		result.Error = err
		result.Success = false
		fmt.Printf("[%d/%d] ❌ 失败: %s - %v\n", current, total, account.Email, err)
		return result
	}

	result.Cookies = cookies
	result.UserSession = userSession
	result.Success = true
	fmt.Printf("[%d/%d] ✅ 成功: %s (用户名: %s)\n", current, total, account.Email, userSession["username"])

	return result
}

// saveResults 保存所有结果到JSON文件
func saveResults(filename string) {
	resultMutex.Lock()
	defer resultMutex.Unlock()

	file, err := os.Create(filename)
	if err != nil {
		log.Printf("创建结果文件失败: %v", err)
		return
	}
	defer file.Close()

	encoder := json.NewEncoder(file)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(results); err != nil {
		log.Printf("保存结果失败: %v", err)
		return
	}

	fmt.Printf("\n结果已保存到: %s\n", filename)
}

// saveSuccessAccounts 保存成功账号到文件
func saveSuccessAccounts(filename string) {
	resultMutex.Lock()
	defer resultMutex.Unlock()

	file, err := os.Create(filename)
	if err != nil {
		log.Printf("创建成功账号文件失败: %v", err)
		return
	}
	defer file.Close()

	writer := bufio.NewWriter(file)
	defer writer.Flush()

	for _, result := range results {
		if result.Success {
			line := fmt.Sprintf("%s:%s\n", result.Email, result.UserSession["username"])
			writer.WriteString(line)
		}
	}

	fmt.Printf("成功账号已保存到: %s\n", filename)
}

// saveFailedAccounts 保存失败账号到文件
func saveFailedAccounts(filename string) {
	resultMutex.Lock()
	defer resultMutex.Unlock()

	file, err := os.Create(filename)
	if err != nil {
		log.Printf("创建失败账号文件失败: %v", err)
		return
	}
	defer file.Close()

	writer := bufio.NewWriter(file)
	defer writer.Flush()

	for _, result := range results {
		if !result.Success {
			errorMsg := ""
			if result.Error != nil {
				errorMsg = result.Error.Error()
			}
			line := fmt.Sprintf("%s - %s\n", result.Email, errorMsg)
			writer.WriteString(line)
		}
	}

	fmt.Printf("失败账号已保存到: %s\n", filename)
}

// saveDevicesWithCookies 保存 startUp 注册成功的设备信息（保留原始字段）并填充 cookies
// 格式：每行一个设备的JSON字符串，追加到文件
func saveDevicesWithCookies(filename string, devices []map[string]interface{}) {
	resultMutex.Lock()
	defer resultMutex.Unlock()

	file, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		log.Printf("创建设备文件失败: %v", err)
		return
	}
	defer file.Close()

	writer := bufio.NewWriter(file)
	defer writer.Flush()

	for _, newDevice := range buildStartupDevicesWithCookies(devices, results) {
		jsonBytes, err := json.Marshal(newDevice)
		if err != nil {
			log.Printf("序列化设备失败: %v", err)
			continue
		}
		writer.WriteString(string(jsonBytes) + "\n")
	}

	fmt.Printf("设备信息已保存到: %s\n", filename)
}

// buildStartupDevicesWithCookies 将“注册成功结果”映射回 devices 原始数据，并组装为 startUp 设备行：
// - 保留 devices 原始 JSON 的全部字段（例如 ua/resolution/dpi/device_type/...）
// - 追加/覆盖 cookies 字段（来自注册成功结果）
// - 确保 create_time 字段存在（优先用原始设备的 create_time；若缺失则写入当前时间）
func buildStartupDevicesWithCookies(devices []map[string]interface{}, regs []RegisterResult) []map[string]interface{} {
	out := make([]map[string]interface{}, 0, len(regs))
	for _, result := range regs {
		if !result.Success || result.DeviceID == "" {
			continue
		}

		// 找到对应的设备（使用原始数据）
		var deviceRaw map[string]interface{}
		deviceIDStr := result.DeviceID
		for _, d := range devices {
			// 获取device_id进行比较
			var did string
			if val, ok := d["device_id"]; ok {
				switch v := val.(type) {
				case string:
					did = v
				case float64:
					did = fmt.Sprintf("%.0f", v)
				default:
					did = fmt.Sprintf("%v", v)
				}
			}
			if did == deviceIDStr {
				deviceRaw = d
				break
			}
		}
		if deviceRaw == nil {
			continue
		}
		out = append(out, buildStartupAccountJSON(deviceRaw, result.Cookies))
	}
	return out
}

// buildStartupAccountJSON 统一构造“账号池 account JSON”，确保：
// - Redis 写入与文件写入完全一致
// - 字段：保留原始 device JSON 全部字段 + cookies + create_time
func buildStartupAccountJSON(deviceRaw map[string]interface{}, cookies map[string]string) map[string]interface{} {
	if deviceRaw == nil {
		return nil
	}
	acc := make(map[string]interface{}, len(deviceRaw)+2)
	for k, v := range deviceRaw {
		acc[k] = v
	}
	// 确保 create_time 存在
	if _, ok := acc["create_time"]; !ok {
		acc["create_time"] = time.Now().Format("2006-01-02 15:04:05")
	}
	// cookies 统一为 python dict string（与原有 Redis/文件兼容）
	if cookies != nil {
		acc["cookies"] = convertCookiesToPythonDict(cookies)
	} else {
		acc["cookies"] = "{}"
	}
	return acc
}

// convertCookiesToPythonDict 将 cookies map 转换为 Python 字典格式的字符串
func convertCookiesToPythonDict(cookies map[string]string) string {
	if len(cookies) == 0 {
		return "{}"
	}

	var parts []string
	for k, v := range cookies {
		// 转义单引号
		key := strings.ReplaceAll(k, "'", "\\'")
		value := strings.ReplaceAll(v, "'", "\\'")
		parts = append(parts, fmt.Sprintf("'%s': '%s'", key, value))
	}

	return "{" + strings.Join(parts, ", ") + "}"
}
