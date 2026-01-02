package main

import (
	"bufio"
	"bytes"
	"encoding/hex"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"sync"
	"time"

	"tt_code/headers"
	"tt_code/mssdk/endecode"
	"tt_code/tt_protobuf"
)

// === CK_CHECKER V2 SPECIFIC STRUCTS ===

// Config 存储命令行参数
type Config struct {
	InputFile   string
	Concurrency int
	Proxy       string
}

// AccountData 存储账号信息
type AccountData struct {
	DeviceID  string `json:"device_id"`
	InstallID string `json:"install_id"`
	UA        string `json:"ua"`
	Cookies   string `json:"cookies"`              // 可能是cookie字符串或包含在json中
	XToken    string `json:"x_tt_token,omitempty"` // 可选，如果输入已经是token形式
	Raw       string `json:"-"`                    // 原始输入行
}

// CheckResult 检查结果
type CheckResult struct {
	Account AccountData
	Valid   bool
	Error   string
	Profile string // 用户名或UID等简要信息
}

// === MAIN FUNCTION ===

func main() {
	config := parseFlags()

	if config.InputFile == "" {
		fmt.Println("Usage: ck_checker_v2 -i <input_file> [-c <concurrency>] [-p <proxy>]")
		return
	}

	accounts, err := loadAccounts(config.InputFile)
	if err != nil {
		fmt.Printf("Error loading accounts: %v\n", err)
		return
	}

	// Create output directory
	err = os.MkdirAll("demos/stats/cookiesCheck", 0755)
	if err != nil {
		fmt.Printf("Error creating output directory: %v\n", err)
		return
	}

	// Open output files
	validFile, err := os.OpenFile("demos/stats/cookiesCheck/valid.txt", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Printf("Error opening valid file: %v\n", err)
		return
	}
	defer validFile.Close()

	invalidFile, err := os.OpenFile("demos/stats/cookiesCheck/invalid.txt", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Printf("Error opening invalid file: %v\n", err)
		return
	}
	defer invalidFile.Close()

	fmt.Printf("Loaded %d accounts. Starting check with concurrency %d...\n", len(accounts), config.Concurrency)

	results := make(chan CheckResult, len(accounts))
	var wg sync.WaitGroup
	sem := make(chan struct{}, config.Concurrency)

	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	// Normalize proxy scheme (e.g., socks5h -> socks5)
	proxyStr := config.Proxy
	if strings.HasPrefix(strings.ToLower(proxyStr), "socks5h://") {
		proxyStr = "socks5://" + proxyStr[10:]
	}

	// 如果设置了代理，配置Transport
	if proxyStr != "" {
		proxyURL, err := url.Parse(proxyStr)
		if err != nil {
			fmt.Printf("Invalid proxy URL: %v. Proceeding without proxy.\n", err)
		} else {
			client.Transport = &http.Transport{
				Proxy: http.ProxyURL(proxyURL),
			}
			fmt.Printf("Using proxy: %s\n", proxyStr)
		}
	}

	for _, acc := range accounts {
		wg.Add(1)
		go func(a AccountData) {
			defer wg.Done()
			sem <- struct{}{} // Acquire token

			// Call GetUserProfile (Modified)
			// Construct cookieData map
			cookieData := map[string]string{
				"device_id":   a.DeviceID,
				"install_id":  a.InstallID,
				"ua":          a.UA,
				"device_type": "Pixel 6",
			}

			// Note: user.go uses hardcoded IDs in main, but GetUserProfile accepts args.
			// We pass file args here.
			// Also, user.go GetUserProfile has NO return. We need to modify it below to return bool.
			valid, msg := GetUserProfile(client, a.DeviceID, a.InstallID, a.UA, cookieData, a.Cookies, a.XToken)

			result := CheckResult{
				Account: a,
				Valid:   valid,
			}
			if valid {
				result.Profile = msg
			} else {
				result.Error = msg
			}

			results <- result
			<-sem // Release token
		}(acc)
	}

	go func() {
		wg.Wait()
		close(results)
	}()

	validCount := 0
	for res := range results {
		if res.Valid {
			validCount++
			fmt.Printf("[VALID] %s - %s\n", MaskString(res.Account.Cookies, 20), res.Profile)
			if _, err := validFile.WriteString(res.Account.Raw + "\n"); err != nil {
				fmt.Printf("Error writing to valid file: %v\n", err)
			}
		} else {
			fmt.Printf("[INVALID] %s - Error: %s\n", MaskString(res.Account.Cookies, 20), res.Error)
			if _, err := invalidFile.WriteString(res.Account.Raw + "\n"); err != nil {
				fmt.Printf("Error writing to invalid file: %v\n", err)
			}
		}
	}
	fmt.Printf("Check complete. Valid: %d/%d\n", validCount, len(accounts))
}

func parseFlags() Config {
	var config Config
	flag.StringVar(&config.InputFile, "i", "", "Input file path containing cookies/accounts")
	flag.IntVar(&config.Concurrency, "c", 5, "Concurrency level")
	flag.StringVar(&config.Proxy, "p", "", "Proxy URL (e.g., http://user:pass@host:port)")
	flag.Parse()
	return config
}

func loadAccounts(filePath string) ([]AccountData, error) {
	file, err := os.Open(filePath)
	if err != nil {
		return nil, err
	}
	defer file.Close()

	var accounts []AccountData
	scanner := bufio.NewScanner(file)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		acc := parseLine(line)
		acc.Raw = line
		// 默认补充一些必要信息如果是纯cookie格式
		if acc.DeviceID == "" {
			// Using logic from user.go main function hardcoded values just in case,
			// but ideally we should parse or generate.
			// user.go hardcoded: deviceID := "7550968885031290423", installID := "7572811438918960951"
			// To mimic user.go exactly for the test case provided by user:
			acc.DeviceID = "7550968885031290423"
			acc.InstallID = "7572811438918960951"
			// acc.DeviceID = generateRandomDeviceID() // Was causing 455 if mismatched
		}
		if acc.InstallID == "" {
			acc.InstallID = "7572811438918960951"
			// acc.InstallID = generateRandomInstallID()
		}
		if acc.UA == "" {
			acc.UA = "com.zhiliaoapp.musically/2024204030 (Linux; U; Android 15; en; Pixel 6; Build/BP1A.250505.005; Cronet/TTNetVersion:efce646d 2025-10-16 QuicVersion:c785494a 2025-09-30)"
		}

		accounts = append(accounts, acc)
	}

	return accounts, scanner.Err()
}

func parseLine(line string) AccountData {
	var acc AccountData
	acc.Raw = line
	// Try parsing as struct first
	if err := json.Unmarshal([]byte(line), &acc); err == nil {
		if acc.Cookies != "" {
			acc.Cookies = cleanCookies(acc.Cookies)
			return acc
		}
	}

	// Try parsing as map to gather all fields as cookies if they are strings
	var m map[string]interface{}
	if err := json.Unmarshal([]byte(line), &m); err == nil {
		var cookies []string
		for k, v := range m {
			if vs, ok := v.(string); ok && vs != "" {
				// Normalize key
				kl := strings.ToLower(k)

				// Priority fields
				switch kl {
				case "cookies":
					acc.Cookies = vs
					continue
				case "device_id", "did":
					acc.DeviceID = vs
					continue
				case "install_id", "iid":
					acc.InstallID = vs
					continue
				case "ua", "user_agent":
					acc.UA = vs
					continue
				case "x-tt-token", "x_tt_token", "xtoken":
					acc.XToken = vs
					continue
				}

				// Skip meta fields or URLs
				if strings.HasPrefix(vs, "http") {
					continue
				}

				// Whitelist common cookie keys
				isCookie := false
				cookieWhitelist := []string{
					"sessionid", "sessionid_ss", "sid_tt", "sid_tt_ss", "sid_guard",
					"uid_tt", "uid_tt_ss", "tt_token", "msToken", "odin_tt",
					"multi_sids", "cmpl_token", "store-idc", "store-country-code",
					"tt-target-idc", "user_oec_info", "d_ticket", "passport_csrf_token",
					"passport_csrf_token_ss", "tt_session_tlb_tag", "sid_tt_ss",
				}
				for _, w := range cookieWhitelist {
					if kl == w {
						isCookie = true
						break
					}
				}

				if isCookie {
					cookies = append(cookies, fmt.Sprintf("%s=%s", k, vs))
				}
			}
		}
		if acc.Cookies == "" && len(cookies) > 0 {
			acc.Cookies = strings.Join(cookies, "; ")
		} else if acc.Cookies != "" {
			acc.Cookies = cleanCookies(acc.Cookies)
		}
		return acc
	}

	// Not JSON, handle as raw cookie string
	if strings.Contains(line, "=") {
		acc.Cookies = line
		return acc
	}
	acc.Cookies = line
	return acc
}

func cleanCookies(s string) string {
	s = strings.TrimSpace(s)
	// If it looks like a Python dict string: {'key': 'value'}
	if strings.HasPrefix(s, "{") && strings.HasSuffix(s, "}") && strings.Contains(s, "'") {
		content := s[1 : len(s)-1]
		var result []string
		// Simplified Python dict-like parser
		parts := strings.Split(content, ",")
		for _, part := range parts {
			kv := strings.SplitN(part, ":", 2)
			if len(kv) == 2 {
				k := strings.Trim(strings.TrimSpace(kv[0]), "'\"")
				v := strings.Trim(strings.TrimSpace(kv[1]), "'\"")
				if k != "" {
					result = append(result, fmt.Sprintf("%s=%s", k, v))
				}
			}
		}
		if len(result) > 0 {
			return strings.Join(result, "; ")
		}
	}
	return s
}

func MaskString(s string, l int) string {
	if len(s) <= l {
		return s
	}
	return s[:l] + "..."
}

// === COPIED FROM USER.GO (Modified to return bool) ===

func GetUserProfile(client *http.Client, deviceID, installID, ua string, cookieData map[string]string, cookies string, xToken string) (bool, string) {
	// Dynamic Data Acquisition
	// fmt.Println("Getting Token...")

	// 1. Mandatory MSSDK Token for signing (secDeviceToken)
	mssdkToken := strings.Trim(GetGetToken(cookieData, client), " []\"")
	if mssdkToken == "" {
		// Log or handle error if needed, but it's required for MakeHeaders
	}

	// 2. X-Tt-Token from JSON (if missing, pass as empty)
	xTtTokenForHeader := strings.Trim(xToken, " []\"")

	// fmt.Printf("DEBUG: mssdkToken (for sign): %s\n", MaskString(mssdkToken, 10))
	// fmt.Printf("DEBUG: x-tt-token (for header): %s\n", MaskString(xTtTokenForHeader, 10))

	// fmt.Println("Getting Seed...")
	seed, seedType, err := GetSeed(cookieData, client)
	if err != nil {
		// fmt.Printf("Failed to get seed: %v\n", err)
		return false, fmt.Sprintf("Failed to get seed: %v", err)
	}
	// fmt.Printf("Got Seed: %s, SeedType: %d\n", seed, seedType)

	// Determine Host based on store-country-code
	apiHost := "aggr16-normal.tiktokv.us"
	countryCode := strings.ToLower(cookieData["store-country-code"])
	if countryCode == "" {
		// Try parsing from raw cookies string if not in map
		if strings.Contains(cookies, "store-country-code=us") {
			countryCode = "us"
		} else if strings.Contains(cookies, "store-country-code=") {
			// Basic extraction for non-us
			countryCode = "other"
		}
	}

	if countryCode != "" && countryCode != "us" {
		apiHost = "api22-normal-c-alisg.tiktokv.com"
	}

	// URL Construction
	originalURL := fmt.Sprintf("https://%s/aweme/v1/user/profile/self/?is_after_login=0&scene_id=3&device_platform=android&os=android&ssmix=a&_rticket=1767263078752&channel=samsung_store&aid=1233&app_name=musical_ly&version_code=420403&version_name=42.4.3&manifest_version_code=2024204030&update_version_code=2024204030&ab_version=42.4.3&resolution=1080*2400&dpi=420&device_type=Pixel%206&device_brand=google&language=en&os_api=35&os_version=15&ac=wifi&is_pad=0&current_region=US&app_type=normal&sys_region=US&last_install_time=1767115711&timezone_name=America%2FNew_York&residence=US&app_language=en&ac2=wifi&uoo=0&op_region=US&timezone_offset=-18000&build_number=42.4.3&host_abi=arm64-v8a&locale=en&region=US&ts=1767263052&iid=7572811438918960951&device_id=7550968885031290423", apiHost)

	u, err := url.Parse(originalURL)
	if err != nil {
		return false, "Error parsing URL"
	}

	q := u.Query()

	// Dynamic time
	timee := time.Now().Unix()
	utime := timee * 1000

	// Update time-based params
	q.Set("_rticket", strconv.FormatInt(utime, 10))
	q.Set("ts", strconv.FormatInt(timee, 10))
	// ensure iid/did matches
	q.Set("iid", installID)
	q.Set("device_id", deviceID)

	u.RawQuery = q.Encode()
	newFullURL := u.String()
	queryString := u.RawQuery

	req, err := http.NewRequest("GET", newFullURL, nil)
	if err != nil {
		return false, fmt.Sprintf("Error creating request: %v", err)
	}

	// Generate Headers
	headersResult := headers.MakeHeaders(
		deviceID,
		timee,
		1,          // signCount
		2,          // reportCount
		4,          // settingCount
		timee-10,   // appLaunchTime
		mssdkToken, // This corresponds to secDeviceToken, used for signing
		"Pixel 6",
		seed,     // seed
		seedType, // seedEncodeType
		"",       // seedEndcodeHex
		"",       // algorithmData1
		"",       // hex32
		queryString,
		"", // postData (empty for GET)
		"42.4.3",
		"v05.02.02-ov-android",
		0x05020220,
		738,
		0xC40A800,
	)

	req.Header.Set("Host", apiHost)
	req.Header.Set("Cookie", cookies)
	req.Header.Set("x-tt-pba-enable", "1")
	req.Header.Set("x-bd-kmsv", "0")
	req.Header.Set("x-tt-dm-status", "login=1;ct=1;rt=8")
	req.Header.Set("x-ss-req-ticket", strconv.FormatInt(utime, 10))
	req.Header.Set("sdk-version", "2")
	// Use Set to handle capitalization and formatting properly
	// If xTtTokenForHeader is empty, it will be set as empty string in header (as per user request: "如果没有就传空")
	req.Header.Set("x-tt-token", xTtTokenForHeader)

	req.Header.Set("passport-sdk-version", "-1")
	req.Header.Set("rpc-persist-pns-region-1", "US|6252001|5549030")
	req.Header.Set("rpc-persist-pns-region-2", "US|6252001|5549030")
	req.Header.Set("rpc-persist-pns-region-3", "US|6252001|5549030")
	req.Header.Set("x-vc-bdturing-sdk-version", "2.3.17.i18n")
	req.Header.Set("x-tt-request-tag", "n=0;nr=111;bg=0;rs=112")
	req.Header.Set("rpc-persist-pyxis-policy-state-law-is-ca", "0")
	req.Header.Set("rpc-persist-pyxis-policy-v-tnc", "1")
	req.Header.Set("x-tt-ttnet-origin-host", "api16-normal-useast5.tiktokv.us")
	req.Header.Set("x-ss-dp", "1233")
	req.Header.Set("User-Agent", ua)

	// Replaced Headers
	req.Header.Set("x-argus", headersResult.XArgus)
	req.Header.Set("x-gorgon", headersResult.XGorgon)
	req.Header.Set("x-khronos", headersResult.XKhronos)
	req.Header.Set("x-ladon", headersResult.XLadon)
	req.Header.Set("x-ss-stub", headersResult.XSSStub)

	// Debug Prints
	fmt.Printf("\n--- Debug Request Info ---\n")
	fmt.Printf("URL: %s\n", newFullURL)
	fmt.Printf("Final Token Used (Signing): %s\n", mssdkToken)
	fmt.Printf("Final Token Used (Header): %s\n", xTtTokenForHeader)
	fmt.Printf("Final Seed Used: %s\n", seed)
	for k, v := range req.Header {
		fmt.Printf("Header: %s: %v\n", k, v)
	}
	fmt.Printf("--------------------------\n\n")

	resp, err := client.Do(req)
	if err != nil {
		return false, fmt.Sprintf("Error sending request: %v", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return false, fmt.Sprintf("Error reading response: %v", err)
	}

	fmt.Printf("--- Debug Response Info ---\n")
	fmt.Printf("Status: %s\n", resp.Status)
	fmt.Printf("Body: %s\n", string(body))
	fmt.Printf("---------------------------\n\n")

	// fmt.Printf("Response Status: %s\n", resp.Status)
	// fmt.Printf("Response Body Length: %d\n", len(body))

	// Debug print only if invalid
	if resp.StatusCode != 200 {
		return false, fmt.Sprintf("Status %s", resp.Status)
	}

	// Parse User Info
	if len(body) == 0 {
		return false, "Empty response body (Cookie Invalid)"
	}

	var respData struct {
		StatusCode int `json:"status_code"`
		User       struct {
			Nickname       string `json:"nickname"`
			UniqueID       string `json:"unique_id"`
			Uid            string `json:"uid"`
			FollowerCount  int    `json:"follower_count"`
			FollowingCount int    `json:"following_count"`
		} `json:"user"`
	}

	if err := json.Unmarshal(body, &respData); err != nil {
		// If it's valid 200 but not JSON, it's likely a captcha or expired session
		return false, "Invalid JSON response (Cookie likely invalid)"
	}

	if respData.StatusCode != 0 {
		return false, fmt.Sprintf("API status error: %d", respData.StatusCode)
	}

	info := fmt.Sprintf("UID:%s | %s (@%s) | Fans:%d | Follow:%d", respData.User.Uid, respData.User.Nickname, respData.User.UniqueID, respData.User.FollowerCount, respData.User.FollowingCount)
	return true, info
}

// GetGetToken implementation (Copied from user.go)
func GetGetToken(cookieData map[string]string, client *http.Client) string {
	ua := cookieData["ua"]
	if ua == "" {
		ua = cookieData["User-Agent"]
	}
	iid := cookieData["install_id"]
	deviceID := cookieData["device_id"]

	queryString := fmt.Sprintf("lc_id=2142840551&platform=android&device_platform=android&sdk_ver=v05.02.02-alpha.12-ov-android&sdk_ver_code=84017696&app_ver=42.4.3&version_code=2024204030&aid=1233&sdkid&subaid&iid=%s&did=%s&bd_did&client_type=inhouse&region_type=ov&mode=2", iid, deviceID)
	urlStr := fmt.Sprintf("https://mssdk16-normal-useast5.tiktokv.us/sdi/get_token?%s", queryString)

	timee := time.Now().Unix()
	utime := timee * 1000
	stime := timee

	tem, err := tt_protobuf.MakeTokenEncryptHex(stime, deviceID)
	if err != nil {
		// fmt.Printf("MakeTokenEncryptHex error: %v\n", err)
		return ""
	}

	tokenEncrypt, err := endecode.MssdkEncrypt(tem, false, 1274)
	if err != nil {
		// fmt.Printf("MssdkEncrypt error: %v\n", err)
		return ""
	}

	postData, err := tt_protobuf.MakeTokenRequest(tokenEncrypt, utime)
	if err != nil {
		// fmt.Printf("MakeTokenRequest error: %v\n", err)
		return ""
	}

	_ = headers.MakeHeaders(
		deviceID, stime, 53, 2, 4, stime-6,
		"", "Pixel 6", "", 0, "", "", "",
		queryString, postData,
		"42.4.3", "v05.02.02-alpha.12-ov-android", 0x05020220, 738, 0,
	)

	if client == nil {
		return ""
	}

	postDataBytes, _ := hex.DecodeString(postData)
	req, err := http.NewRequest("POST", urlStr, bytes.NewReader(postDataBytes))
	if err != nil {
		// fmt.Printf("NewRequest error: %v\n", err)
		return ""
	}

	req.Header.Set("rpc-persist-pyxis-policy-v-tnc", "1")
	req.Header.Set("rpc-persist-pyxis-policy-state-law-is-ca", "1")
	req.Header.Set("Accept-Encoding", "gzip")
	req.Header.Set("x-tt-request-tag", "n=0;nr=111;bg=0;t=0")
	req.Header.Set("x-tt-pba-enable", "1")
	req.Header.Set("Accept", "*/*")
	req.Header.Set("x-bd-kmsv", "0")
	req.Header.Set("X-SS-REQ-TICKET", fmt.Sprintf("%d", utime))
	req.Header.Set("x-vc-bdturing-sdk-version", "2.3.13.i18n")
	req.Header.Set("oec-vc-sdk-version", "3.0.12.i18n")
	req.Header.Set("sdk-version", "2")
	req.Header.Set("x-tt-dm-status", "login=1;ct=1;rt=1")
	req.Header.Set("passport-sdk-version", "-1")
	req.Header.Set("x-tt-store-region", "us")
	req.Header.Set("x-tt-store-region-src", "uid")
	req.Header.Set("User-Agent", ua)
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("Host", "mssdk16-normal-useast5.tiktokv.us")
	req.Header.Set("Connection", "Keep-Alive")

	resp, err := client.Do(req)
	if err != nil {
		// fmt.Printf("client.Do error: %v\n", err)
		return ""
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		// fmt.Printf("ReadAll error: %v\n", err)
		return ""
	}

	resHex := hex.EncodeToString(body)
	resPb, err := tt_protobuf.MakeTokenResponse(resHex)
	if err != nil {
		// fmt.Printf("MakeTokenResponse error: %v\n", err)
		return ""
	}

	tokenDecrypt := resPb.TokenDecrypt
	if tokenDecrypt == "" {
		// fmt.Println("TokenDecrypt is empty")
		return ""
	}

	tokenDecryptRes, err := endecode.MssdkDecrypt(tokenDecrypt, false, false)
	if err != nil {
		// fmt.Printf("MssdkDecrypt error: %v\n", err)
		return ""
	}

	afterDecryptToken, err := tt_protobuf.MakeTokenDecrypt(tokenDecryptRes)
	if err != nil {
		// fmt.Printf("MakeTokenDecrypt error: %v\n", err)
		return ""
	}

	return afterDecryptToken.Token
}

// GetSeed implementation (Copied from user.go)
func GetSeed(cookieData map[string]string, client *http.Client) (string, int, error) {
	deviceID := strings.TrimSpace(cookieData["device_id"])
	if deviceID == "" {
		return "", 0, errors.New("device_id is required")
	}

	iid := strings.TrimSpace(cookieData["install_id"])
	if iid == "" {
		return "", 0, errors.New("install_id is required")
	}

	deviceType := cookieData["device_type"]
	if deviceType == "" {
		deviceType = "Pixel 6"
	}

	ua := cookieData["ua"]
	if ua == "" {
		ua = cookieData["User-Agent"]
	}
	if ua == "" {
		return "", 0, errors.New("ua is required")
	}

	queryString := fmt.Sprintf("lc_id=2142840551&platform=android&device_platform=android&sdk_ver=v05.02.02-alpha.12-ov-android&sdk_ver_code=84017696&app_ver=42.4.3&version_code=2024204030&aid=1233&sdkid&subaid&iid=%s&did=%s&bd_did&client_type=inhouse&region_type=ov&mode=2", iid, deviceID)
	requestURL := fmt.Sprintf("https://mssdk16-normal-useast5.tiktokv.us/ms/get_seed?%s", queryString)

	sessionID := generateUUID()

	seedEncryptHex, err := tt_protobuf.MakeSeedEncrypt(sessionID, deviceID, "android", "v05.02.02")
	if err != nil {
		return "", 0, fmt.Errorf("make seed encrypt: %w", err)
	}

	encryptedPayload, err := endecode.MssdkEncrypt(seedEncryptHex, false, 170)
	if err != nil {
		return "", 0, fmt.Errorf("mssdk encrypt: %w", err)
	}

	now := time.Now().Unix()
	utime := now * 1000

	postData, err := tt_protobuf.MakeSeedRequest(encryptedPayload, utime)
	if err != nil {
		return "", 0, fmt.Errorf("make seed request: %w", err)
	}

	headersResult := headers.MakeHeaders(
		deviceID, now, 52, 2, 4, now-6,
		"", deviceType, "", 0, "", "", "",
		queryString, postData,
		"42.4.3", "v05.02.02-alpha.12-ov-android", 0x05020220, 738, 0,
	)

	if client == nil {
		return "", 0, errors.New("http client is required")
	}

	postBytes, _ := hex.DecodeString(postData)
	req, err := http.NewRequest("POST", requestURL, bytes.NewReader(postBytes))
	if err != nil {
		return "", 0, fmt.Errorf("new request: %w", err)
	}

	req.Header.Set("rpc-persist-pyxis-policy-v-tnc", "1")
	req.Header.Set("rpc-persist-pyxis-policy-state-law-is-ca", "1")
	req.Header.Set("X-SS-STUB", headersResult.XSSStub)
	req.Header.Set("Accept-Encoding", "gzip")
	req.Header.Set("x-tt-request-tag", "n=0;nr=111;bg=0;t=0")
	req.Header.Set("x-tt-pba-enable", "1")
	req.Header.Set("Accept", "*/*")
	req.Header.Set("x-bd-kmsv", "0")
	req.Header.Set("X-SS-REQ-TICKET", fmt.Sprintf("%d", utime))
	req.Header.Set("x-vc-bdturing-sdk-version", "2.3.13.i18n")
	req.Header.Set("oec-vc-sdk-version", "3.0.12.i18n")
	req.Header.Set("sdk-version", "2")
	req.Header.Set("x-tt-dm-status", "login=1;ct=1;rt=1")
	req.Header.Set("passport-sdk-version", "-1")
	req.Header.Set("x-tt-store-region", "us")
	req.Header.Set("x-tt-store-region-src", "uid")
	req.Header.Set("User-Agent", ua)
	req.Header.Set("X-Ladon", headersResult.XLadon)
	req.Header.Set("X-Khronos", headersResult.XKhronos)
	req.Header.Set("X-Argus", headersResult.XArgus)
	req.Header.Set("X-Gorgon", headersResult.XGorgon)
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("Host", "mssdk16-normal-useast5.tiktokv.us")
	req.Header.Set("Connection", "Keep-Alive")

	resp, err := client.Do(req)
	if err != nil {
		return "", 0, fmt.Errorf("request seed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", 0, fmt.Errorf("read body: %w", err)
	}

	resHex := hex.EncodeToString(body)
	respPb, err := tt_protobuf.MakeSeedResponse(resHex)
	if err != nil {
		return "", 0, fmt.Errorf("decode response: %w", err)
	}
	if respPb.SeedDecrypt == "" {
		return "", 0, errors.New("empty seed_decrypt payload")
	}

	decryptedHex, err := endecode.MssdkDecrypt(respPb.SeedDecrypt, false, false)
	if err != nil {
		return "", 0, fmt.Errorf("mssdk decrypt: %w", err)
	}

	decryptedSeed, err := tt_protobuf.MakeSeedDecrypt(decryptedHex)
	if err != nil {
		return "", 0, fmt.Errorf("parse seed decrypt: %w", err)
	}

	seedValue := decryptedSeed.Seed
	var algorithm string
	if decryptedSeed.ExtraInfo != nil {
		algorithm = decryptedSeed.ExtraInfo.Algorithm
	}
	if seedValue == "" || algorithm == "" {
		return "", 0, errors.New("invalid seed response")
	}

	algBytes := []byte(algorithm)
	algInt, err := strconv.ParseInt(hex.EncodeToString(algBytes), 16, 64)
	if err != nil {
		return "", 0, fmt.Errorf("parse algorithm: %w", err)
	}
	return seedValue, int(algInt / 2), nil
}

func generateUUID() string {
	b := make([]byte, 16)
	rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x%x%x%x%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}
