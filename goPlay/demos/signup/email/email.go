package email

import (
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"strings"
	"time"

	"tt_code/headers"
	"tt_code/mssdk/get_seed"
	"tt_code/mssdk/get_token"
)

// encrypt 加密函数 - 与Python的encrypt函数完全一致
// 将字符串转为hex，每个字节异或5，再转回hex字符串
func encrypt(data string) string {
	dataBytes := []byte(data)
	dataHex := hex.EncodeToString(dataBytes)

	// 将hex字符串转为字节数组
	dataa, _ := hex.DecodeString(dataHex)

	// 每个字节异或5
	for i := range dataa {
		dataa[i] ^= 5
	}

	// 转回hex字符串
	return hex.EncodeToString(dataa)
}

// buildGuard 构建device guard headers - 完全按照Python的build_guard函数实现
func buildGuard(deviceGuardData0 map[string]interface{}, path string) (map[string]string, error) {
	if deviceGuardData0 == nil {
		return nil, fmt.Errorf("device_guard_data0 is nil")
	}

	// 使用新的BuildGuard API
	return headers.BuildGuard(deviceGuardData0, nil, path, 0, "", false)
}

// extractCookiesAndToken 从响应头和JSON中提取cookies和token信息 - 完全按照Python的extract_cookies_and_token函数实现
func extractCookiesAndToken(respHeaders http.Header, respJSON map[string]interface{}) (map[string]string, map[string]string) {
	cookies := make(map[string]string)

	// -------- 1. 先把所有 Set-Cookie 的值收集出来 --------
	setCookieValues := []string{}

	// 情况：使用http.Header的Values方法
	vals := respHeaders.Values("Set-Cookie")
	if len(vals) == 0 {
		vals = respHeaders.Values("set-cookie")
	}
	setCookieValues = append(setCookieValues, vals...)

	// 如果上面都拿不到，就退回items()
	if len(setCookieValues) == 0 {
		for name, values := range respHeaders {
			if strings.ToLower(name) == "set-cookie" {
				for _, value := range values {
					// 有些实现会把多条 Set-Cookie 拼成一坨，这里拆一下换行
					if strings.Contains(value, "\n") {
						for _, line := range strings.Split(value, "\n") {
							line = strings.TrimSpace(line)
							if line != "" {
								setCookieValues = append(setCookieValues, line)
							}
						}
					} else {
						setCookieValues = append(setCookieValues, value)
					}
				}
			}
		}
	}

	// -------- 2. 解析每一条 Set-Cookie，提取 name=value --------
	for _, value := range setCookieValues {
		// 有些库可能会把多个 cookie 写在一行，用逗号/换行拼接，这里只按行处理就行
		for _, part := range strings.Split(value, "\n") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			first := strings.Split(part, ";")[0]
			first = strings.TrimSpace(first)
			if !strings.Contains(first, "=") {
				continue
			}
			parts := strings.SplitN(first, "=", 2)
			if len(parts) == 2 {
				cName := strings.TrimSpace(parts[0])
				cVal := strings.TrimSpace(parts[1])
				cookies[cName] = cVal // 同名 cookie 后面的覆盖前面的
			}
		}
	}

	// -------- 3. 提取 x-tt-token / x-tt-token-sign / x-tt-session-sign --------
	getHeader := func(h http.Header, key string) string {
		// 先走 Get 方法
		if val := h.Get(key); val != "" {
			return val
		}
		// 再试下不同大小写
		if val := h.Get(strings.Title(key)); val != "" {
			return val
		}
		if val := h.Get(strings.ToUpper(key)); val != "" {
			return val
		}

		// 如果没有 get，就遍历
		for k, v := range h {
			if strings.EqualFold(k, key) {
				if len(v) > 0 {
					return v[0]
				}
			}
		}
		return ""
	}

	xTtToken := getHeader(respHeaders, "x-tt-token")
	ttTicketGuardServerData := getHeader(respHeaders, "tt-ticket-guard-server-data")
	secUID := getHeader(respHeaders, "x-tt-store-sec-uid") // 提取sec_uid

	userSession := make(map[string]string)
	if xTtToken != "" { // 有值再塞
		userSession["x-tt-token"] = xTtToken
	} else {
		userSession["x-tt-token"] = ""
	}
	if ttTicketGuardServerData != "" { // 统一叫 ts_sign
		decoded, err := base64.StdEncoding.DecodeString(ttTicketGuardServerData)
		if err == nil {
			var ticketData map[string]interface{}
			if json.Unmarshal(decoded, &ticketData) == nil {
				if tickets, ok := ticketData["tickets"].([]interface{}); ok && len(tickets) > 0 {
					if ticket, ok := tickets[0].(map[string]interface{}); ok {
						if tsSign, ok := ticket["ts_sign"].(string); ok {
							userSession["ts_sign"] = tsSign
						} else {
							userSession["ts_sign"] = ""
						}
					} else {
						userSession["ts_sign"] = ""
					}
				} else {
					userSession["ts_sign"] = ""
				}
			} else {
				userSession["ts_sign"] = ""
			}
		} else {
			userSession["ts_sign"] = ""
		}
	} else {
		userSession["ts_sign"] = ""
	}
	if secUID != "" {
		userSession["sec_uid"] = secUID
	} else {
		userSession["sec_uid"] = ""
	}

	// 从JSON中提取用户信息
	data, ok := respJSON["data"].(map[string]interface{})
	if !ok {
		data = make(map[string]interface{})
	}

	if screenName, ok := data["screen_name"].(string); ok {
		userSession["screen_name"] = screenName
	} else {
		userSession["screen_name"] = ""
	}
	if secUserID, ok := data["sec_user_id"].(string); ok {
		userSession["sec_user_id"] = secUserID
	} else {
		userSession["sec_user_id"] = ""
	}
	if userCreateTime, ok := data["user_create_time"].(string); ok {
		userSession["user_create_time"] = userCreateTime
	} else if userCreateTime, ok := data["user_create_time"].(float64); ok {
		userSession["user_create_time"] = fmt.Sprintf("%.0f", userCreateTime)
	} else {
		userSession["user_create_time"] = ""
	}
	if userIDStr, ok := data["user_id_str"].(string); ok {
		userSession["user_id_str"] = userIDStr
	} else {
		userSession["user_id_str"] = ""
	}
	if username, ok := data["name"].(string); ok {
		userSession["username"] = username
	} else {
		userSession["username"] = ""
	}

	return cookies, userSession
}

// Register 邮箱注册函数 - 完全按照Python的register函数实现
func Register(email, pwd, seed string, seedType int, token string, device map[string]string, proxy string) (map[string]string, map[string]string, error) {
	// 设置代理
	proxies := make(map[string]string)
	if proxy != "" {
		proxies["http"] = proxy
		proxies["https"] = proxy
	}

	t := time.Now()
	stime := int(t.Unix())
	utime := int(t.Unix() * 1000)

	deviceID := device["device_id"]
	if deviceID == "" {
		deviceID = ""
	}
	installID := device["install_id"]
	if installID == "" {
		installID = ""
	}
	ua := device["ua"]
	if ua == "" {
		ua = ""
	}
	openudid := device["openudid"]
	if openudid == "" {
		openudid = ""
	}
	cdid := device["cdid"]
	if cdid == "" {
		cdid = ""
	}

	// 解析device_guard_data0
	deviceGuardData0Str := device["device_guard_data0"]
	var deviceGuardData0 map[string]interface{}
	if deviceGuardData0Str != "" {
		if err := json.Unmarshal([]byte(deviceGuardData0Str), &deviceGuardData0); err != nil {
			return nil, nil, fmt.Errorf("解析device_guard_data0失败: %w", err)
		}
	} else {
		deviceGuardData0 = make(map[string]interface{})
	}

	// 构建query_string1
	queryString1 := fmt.Sprintf("passport-sdk-version=6041990&device_platform=android&os=android&ssmix=a&_rticket=%d&cdid=%s&channel=samsung_store&aid=1233&app_name=musical_ly&version_code=420403&version_name=42.4.3&manifest_version_code=2024204030&update_version_code=2024204030&ab_version=42.4.3&resolution=1080*2209&dpi=420&device_type=Pixel%%206&device_brand=google&language=en&os_api=35&os_version=15&ac=wifi&is_pad=0&app_type=normal&sys_region=US&last_install_time=1766049813&timezone_name=America%%2FNew_York&app_language=en&ac2=wifi&uoo=0&op_region=US&timezone_offset=-18000&build_number=42.4.3&host_abi=arm64-v8a&locale=en&region=US&ts=%d&iid=%s&device_id=%s&openudid=%s&support_webview=1&reg_store_region=us",
		utime, cdid, stime, installID, deviceID, openudid)

	// URL编码query_string - 完全按照Python的逻辑
	queryParts := strings.Split(queryString1, "&")
	var queryPartsEncoded []string
	for _, param := range queryParts {
		if strings.Contains(param, "=") {
			kv := strings.SplitN(param, "=", 2)
			if len(kv) == 2 {
				k := kv[0]
				v := kv[1]
				// quote(v, safe='*').replace('%25', '%')
				encoded := url.QueryEscape(v)
				// 保留*字符（Python的safe='*'）
				encoded = strings.ReplaceAll(encoded, "*", "%2A")
				// 将%25替换为%
				encoded = strings.ReplaceAll(encoded, "%25", "%")
				// 替换+为%20
				encoded = strings.ReplaceAll(encoded, "+", "%20")
				queryPartsEncoded = append(queryPartsEncoded, k+"="+encoded)
			}
		}
	}
	queryString := strings.Join(queryPartsEncoded, "&")

	// 构建URL
	urlStr := fmt.Sprintf("https://api16-normal-useast5.tiktokv.us/passport/email/register/v2/?%s", queryString)

	// 加密email和password
	eEmail := encrypt(email)
	ePwd := encrypt(pwd)

	// 生成随机生日
	rand.Seed(time.Now().UnixNano())
	y := fmt.Sprintf("%d", rand.Intn(2003-1990+1)+1990)
	m := fmt.Sprintf("%02d", rand.Intn(12)+1)
	d := fmt.Sprintf("%02d", rand.Intn(28)+1)
	birthday := fmt.Sprintf("%s-%s-%s", y, m, d)

	// 构建POST数据
	data := fmt.Sprintf("birthday=%s&rules_version=v2&password=%s&fixed_mix_mode=1&account_sdk_source=app&mix_mode=1&multi_login=1&email=%s",
		birthday, ePwd, eEmail)
	postData := hex.EncodeToString([]byte(data))

	// 生成headers - 完全按照Python的make_headers调用
	headersResult := headers.MakeHeaders(
		deviceID,
		int64(stime),
		1079,
		2,
		4,
		int64(stime-rand.Intn(10)-1),
		token,
		"Pixel 6",
		seed,
		seedType,
		"",
		"",
		"",
		queryString,
		postData,
		"42.4.3",
		"v05.02.02-ov-android",
		0x05020220,
		738,
		0xC40A800,
	)

	// 构建请求头
	reqHeaders := map[string]string{
		"Host":                              "api16-normal-useast5.tiktokv.us",
		"tt-ticket-guard-public-key":        "BG2BYwuu9cDJrVAFPzEO+05lxbiI/MNoNrHGNkVs9mGDflI5lO3PIQdZzodHBumb4anImBpsgD4XPTYZHfs/YXE=",
		"sdk-version":                       "2",
		"tt-ticket-guard-iteration-version": "0",
		"x-tt-dm-status":                    "login=0;ct=1;rt=8",
		"x-ss-req-ticket":                   fmt.Sprintf("%d", utime),
		"tt-ticket-guard-version":           "3",
		"passport-sdk-settings":             "x-tt-token",
		"passport-sdk-sign":                 "x-tt-token",
		"passport-sdk-version":              "-1",
		"x-tt-bypass-dp":                    "1",
		"x-vc-bdturing-sdk-version":         "2.3.17.i18n",
		"tt-device-guard-iteration-version": "1",
		"tt-device-guard-client-data":       "eyJkZXZpY2VfdG9rZW4iOiIxfHtcImFpZFwiOjEyMzMsXCJhdlwiOlwiNDIuNC4zXCIsXCJkaWRcIjpcIjc1NTA5Njg4ODUwMzEyOTA0MjNcIixcImlpZFwiOlwiNzU3MjgxMTQzODkxODk2MDk1MVwiLFwiZml0XCI6XCIxNzU4MDk3Mjg1XCIsXCJzXCI6MSxcImlkY1wiOlwidXNlYXN0OFwiLFwidHNcIjpcIjE3NjYwNDk4MTRcIn0iLCJ0aW1lc3RhbXAiOjE3NjYwNDk4ODQsInJlcV9jb250ZW50IjoiZGV2aWNlX3Rva2VuLHBhdGgsdGltZXN0YW1wIiwiZHRva2VuX3NpZ24iOiJ0cy4xLk1FVUNJR3FzMWNHb3puTHFYVlZ6SU9NajRERldyYWlKZ1NGaXBVQ0pvQ01mWEhka0FpRUFzNFA0V21LN0EwZDVtd3c4R3pFR0hXRENuY3JodHI5dXNVaktvTnlQSFwvaz0iLCJkcmVxX3NpZ24iOiJNRVVDSVFEcEpnMytnRmdiMGFsdUFRcktVSzRcL1hVZnJrRXQ1RGpPbHhINnN5TG5yYWdJZ2N1TlNJd2thSUJSQ1NiS2xjY0N0RExpUWFvZ3FVQjVrVFBKSmRRZHluVk09In0=",
		"content-type":                      "application/x-www-form-urlencoded; charset=UTF-8",
		"x-ss-stub":                         headersResult.XSSStub,
		"x-tt-request-tag":                  "s=-1;p=0",
		"user-agent":                        ua,
		"x-argus":                           headersResult.XArgus,
		"x-gorgon":                          headersResult.XGorgon,
		"x-khronos":                         headersResult.XKhronos,
		"x-ladon":                           headersResult.XLadon,
	}

	// 构建device guard headers - 完全按照Python的build_guard调用
	if len(deviceGuardData0) > 0 {
		deviceHeaders, err := buildGuard(deviceGuardData0, "/passport/email/register/v2/")
		if err == nil {
			// 更新headers
			for k, v := range deviceHeaders {
				reqHeaders[k] = v
			}
		}
	}

	// 设置cookies
	cookies := map[string]string{
		"odin_tt":       "32681ed8fcedceb4049e9b86c408dcabcefce511e96e04a1b3aa6c4f206f1b3bbc00b603702ff49571df7c38030db4fcdd5f94f1cac036791d5b00d7c499b2ec1feb1a4f7e6225b0946621db8bba4391",
		"tt-target-idc": "useast5",
		"msToken":       "fyQesJ7rImmN1DtOUPtdAgixfbKWrGVvQFY8GwEJMW7kGDGh39CKeltX8bu1vOOVGpkGLq0R1k2yk1RYHff-WAAnvm9lmOhkxn4RyXDbjqndME-XXG48u5A4FA==",
		"store-idc":     "useast5",
	}

	// 创建HTTP客户端
	var transport *http.Transport
	if proxy != "" {
		if proxyURL, err := url.Parse(proxy); err == nil {
			transport = &http.Transport{
				Proxy: http.ProxyURL(proxyURL),
			}
		}
	}
	client := &http.Client{
		Timeout:   30 * time.Second,
		Transport: transport,
	}

	// 创建请求
	req, err := http.NewRequest("POST", urlStr, strings.NewReader(data))
	if err != nil {
		return nil, nil, fmt.Errorf("创建请求失败: %w", err)
	}

	// 设置请求头
	for k, v := range reqHeaders {
		req.Header.Set(k, v)
	}

	// 设置cookies
	for k, v := range cookies {
		req.AddCookie(&http.Cookie{Name: k, Value: v})
	}

	// 发送请求
	resp, err := client.Do(req)
	if err != nil {
		return nil, nil, fmt.Errorf("请求失败: %w", err)
	}
	defer resp.Body.Close()

	// 读取响应体
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, nil, fmt.Errorf("读取响应失败: %w", err)
	}

	// 打印响应（与Python一致）
	// fmt.Printf("response===> %s\n", string(bodyBytes))

	// 解析响应JSON
	var respJSON map[string]interface{}
	if err := json.Unmarshal(bodyBytes, &respJSON); err != nil {
		// 如果解析失败，仍然尝试提取cookies
		fmt.Printf("解析JSON失败: %v\n", err)
	}
	if msg, ok := respJSON["message"].(string); ok && msg == "error" {
		return nil, nil, fmt.Errorf("register失败: %v", respJSON["data"])
	}

	// 提取cookies和token
	cookiess, userSession := extractCookiesAndToken(resp.Header, respJSON)

	// 添加install_id和device_id到cookiess（从响应中提取的）
	if installID != "" {
		cookiess["install_id"] = installID
	} else {
		cookiess["install_id"] = ""
	}
	// if deviceID != "" {
	// 	cookiess["device_id"] = deviceID
	// } else {
	// 	cookiess["device_id"] = ""
	// }

	// 打印信息（与Python一致）
	// fmt.Printf("\n--- 响应头 ---\n")
	// for k, v := range resp.Header {
	// 	fmt.Printf("%s: %v\n", k, v)
	// }
	fmt.Printf("提取到的cookie===> %v\n", cookiess)
	fmt.Printf("提取到的user_session===> %v\n", userSession)

	// 打印响应内容的前3000字符
	// responseText := string(bodyBytes)
	// if len(responseText) > 3000 {
	// 	fmt.Printf("%s\n", responseText[:3000])
	// } else {
	// 	fmt.Printf("%s\n", responseText)
	// }

	// 返回从响应中提取的所有cookies（包含注册后服务器返回的所有cookies信息）和user_session
	return cookiess, userSession, nil
}

// GetGetSeed 获取seed - 包装mssdk/get_seed的GetSeed函数
func GetGetSeed(device map[string]string, proxy string) (string, int, error) {
	return get_seed.GetSeed(device, proxy)
}

// GetGetToken 获取token - 包装mssdk/get_token的GetGetToken函数
func GetGetToken(device map[string]string, proxy string) string {
	return get_token.GetGetToken(device, proxy)
}
