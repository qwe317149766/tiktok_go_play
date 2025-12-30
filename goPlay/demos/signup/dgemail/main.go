package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"math/rand"
	"os"
	"path/filepath"
	"strconv"
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

	// 轮询补齐模式：检查 Redis cookies 池缺口，自动补齐（Linux 默认开启；Windows 可用 DGEMAIL_POLL_MODE=1 调试）
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
	if shouldLoadDevicesFromRedis() {
		limit := getEnvInt("DEVICES_LIMIT", getEnvInt("MAX_GENERATE", 0))
		devices, err = loadDevicesFromRedis(limit)
		if err != nil {
			log.Fatalf("从Redis读取设备失败: %v", err)
		}
		fmt.Printf("已从Redis加载 %d 个设备\n", len(devices))
	} else {
		devices, err = loadDevices("data/devices.txt")
		if err != nil {
			log.Fatalf("读取设备列表失败: %v", err)
		}
		fmt.Printf("已从文件加载 %d 个设备\n", len(devices))
	}
	if err != nil {
		log.Fatalf("读取设备列表失败: %v", err)
	}

	// 如果账号列表为空，根据设备数量生成随机账号
	if len(accounts) == 0 {
		// 注册数量从配置读取（STARTUP_REGISTER_COUNT 优先，否则回退 MAX_GENERATE）
		target := getEnvInt("STARTUP_REGISTER_COUNT", getEnvInt("MAX_GENERATE", len(devices)))
		if target <= 0 {
			target = len(devices)
		}
		if target > len(devices) {
			log.Fatalf("STARTUP_REGISTER_COUNT=%d 大于设备数量=%d，请增加设备或降低注册数量", target, len(devices))
		}
		accounts = generateRandomAccounts(target)
		fmt.Printf("已生成 %d 个随机账号（来自 STARTUP_REGISTER_COUNT/MAX_GENERATE）\n", len(accounts))
	} else {
		fmt.Printf("已加载 %d 个账号\n", len(accounts))
	}

	// 3. 读取代理列表
	proxyPath := findTopmostFileUpwards("proxies.txt", 8)
	if proxyPath == "" {
		// 兼容旧目录结构
		proxyPath = "data/proxies.txt"
	}
	proxies, err := loadProxies(proxyPath)
	if err != nil {
		log.Fatalf("读取代理列表失败: %v", err)
	}
	fmt.Printf("已加载 %d 个代理\n", len(proxies))

	// 4. 设置并发数（降低并发以提高成功率）
	maxConcurrency := 50
	fmt.Printf("并发数: %d (已优化以提高成功率)\n", maxConcurrency)
	fmt.Println()

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
	saveDevicesWithCookies("res/devices1221/devices12_21_3.txt", devices)

	// 8. （可选）将 startUp 注册成功的 cookies 写入 Redis，供 stats 项目读取
	if n, err := saveStartupCookiesToRedis(results, 0); err != nil {
		log.Fatalf("写入startUp cookies到Redis失败: %v", err)
	} else if n > 0 {
		fmt.Printf("已写入 %d 份 startUp cookies 到 Redis\n", n)
	}
}

// generateRandomAccounts 生成随机账号
func generateRandomAccounts(count int) []AccountInfo {
	rand.Seed(time.Now().UnixNano())
	accounts := make([]AccountInfo, count)

	for i := 0; i < count; i++ {
		// 生成随机字符串作为邮箱用户名（8-12位）
		usernameLength := rand.Intn(5) + 8
		username := generateRandomString(usernameLength)
		email := fmt.Sprintf("wazss%s@gmail.com", username)

		accounts[i] = AccountInfo{
			Email:    email,
			Password: "qw123456789!", // 固定密码
		}
	}

	return accounts
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
			} else {
				atomic.AddInt64(&failedCount, 1)
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
	result := RegisterResult{
		Email:    account.Email,
		DeviceID: device["device_id"],
		Proxy:    proxy,
	}

	fmt.Printf("[%d/%d] 开始注册: %s (设备: %s, 代理: %s)\n", current, total, account.Email, device["device_id"], proxy)

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

// saveDevicesWithCookies 保存设备信息（提取指定字段）并填充cookies
// 格式：每行一个设备的JSON字符串，追加到文件
// 按照用户要求的字段顺序和类型保存
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

	// 遍历所有成功注册的结果，提取对应设备的字段并保存
	for _, result := range results {
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

		// 构建新的设备字典，按照用户要求的顺序
		newDevice := make(map[string]interface{})

		// 提取指定字段，保持原始类型（数字保持为数字）
		if val, ok := deviceRaw["create_time"]; ok {
			newDevice["create_time"] = val
		}
		if val, ok := deviceRaw["device_id"]; ok {
			newDevice["device_id"] = val
		}
		if val, ok := deviceRaw["install_id"]; ok {
			newDevice["install_id"] = val
		}
		if val, ok := deviceRaw["clientudid"]; ok {
			newDevice["clientudid"] = val
		}
		if val, ok := deviceRaw["google_aid"]; ok {
			newDevice["google_aid"] = val
		}
		// 数字字段保持为数字类型（int64）
		if val, ok := deviceRaw["apk_last_update_time"]; ok {
			var intVal int64
			switch v := val.(type) {
			case float64:
				// JSON数字会被解析为float64
				intVal = int64(v)
			case int64:
				intVal = v
			case int:
				intVal = int64(v)
			case string:
				// 如果是字符串，尝试转换为数字
				if parsed, err := strconv.ParseInt(v, 10, 64); err == nil {
					intVal = parsed
				} else {
					// 转换失败，保持原值
					newDevice["apk_last_update_time"] = val
					intVal = -1 // 标记为无效
				}
			default:
				// 其他类型保持原值
				newDevice["apk_last_update_time"] = val
				intVal = -1 // 标记为无效
			}
			if intVal >= 0 {
				newDevice["apk_last_update_time"] = intVal
			}
		}
		if val, ok := deviceRaw["apk_first_install_time"]; ok {
			var intVal int64
			switch v := val.(type) {
			case float64:
				// JSON数字会被解析为float64
				intVal = int64(v)
			case int64:
				intVal = v
			case int:
				intVal = int64(v)
			case string:
				// 如果是字符串，尝试转换为数字
				if parsed, err := strconv.ParseInt(v, 10, 64); err == nil {
					intVal = parsed
				} else {
					// 转换失败，保持原值
					newDevice["apk_first_install_time"] = val
					intVal = -1 // 标记为无效
				}
			default:
				// 其他类型保持原值
				newDevice["apk_first_install_time"] = val
				intVal = -1 // 标记为无效
			}
			if intVal >= 0 {
				newDevice["apk_first_install_time"] = intVal
			}
		}
		if val, ok := deviceRaw["cdid"]; ok {
			newDevice["cdid"] = val
		}
		if val, ok := deviceRaw["openudid"]; ok {
			newDevice["openudid"] = val
		}
		if val, ok := deviceRaw["device_guard_data0"]; ok {
			newDevice["device_guard_data0"] = val
		}
		if val, ok := deviceRaw["tt_ticket_guard_public_key"]; ok {
			newDevice["tt_ticket_guard_public_key"] = val
		}
		if val, ok := deviceRaw["priv_key"]; ok {
			newDevice["priv_key"] = val
		}

		// 填充 cookies（注册成功的结果）
		if result.Cookies != nil {
			// 将 cookies map 转换为 Python 字典格式的字符串
			cookiesStr := convertCookiesToPythonDict(result.Cookies)
			newDevice["cookies"] = cookiesStr
		} else {
			newDevice["cookies"] = ""
		}

		// 转换为 JSON 并写入文件
		jsonBytes, err := json.Marshal(newDevice)
		if err != nil {
			log.Printf("序列化设备失败: %v", err)
			continue
		}

		writer.WriteString(string(jsonBytes) + "\n")
	}

	fmt.Printf("设备信息已保存到: %s\n", filename)
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
