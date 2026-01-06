package main

import (
	"bytes"
	"compress/gzip"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"strconv"
	"sync"
	"time"

	"tt_code/headers"
)

var (
	randOnce sync.Once
)

func initRand() {
	randOnce.Do(func() {
		rand.Seed(time.Now().UnixNano())
	})
}

// Stats3 发送 TikTok stats 请求 - 高性能版本
func Stats3(awemeID, seed string, seedType int, token string, device map[string]interface{}, signCount int, client *http.Client) (string, error) {
	initRand()

	// 从 device map 中提取参数
	deviceID, _ := device["device_id"].(string)
	installID, _ := device["install_id"].(string)
	ua, _ := device["ua"].(string)
	apkFirstInstallTime, _ := device["apk_first_install_time"].(float64)
	apkLastUpdateTime, _ := device["apk_last_update_time"].(float64)
	privKey, _ := device["priv_key"].(string)
	deviceGuardData0Str, _ := device["device_guard_data0"].(string)

	// 解析 device_guard_data0
	var deviceGuardData0 map[string]interface{}
	if err := json.Unmarshal([]byte(deviceGuardData0Str), &deviceGuardData0); err != nil {
		return "", fmt.Errorf("failed to parse device_guard_data0: %v", err)
	}

	// 构建 guard header - 使用新的BuildGuard接口
	deviceHeaders, err := headers.BuildGuard(deviceGuardData0, nil, "/aweme/v1/aweme/stats/", 0, privKey, false)
	if err != nil {
		return "", fmt.Errorf("failed to build guard: %v", err)
	}

	// 时间戳
	timee := time.Now().Unix()
	utime := timee * 1000
	stime := timee
	lastInstallTime := int64(apkLastUpdateTime) / 1000

	// 构建 query string
	queryString := fmt.Sprintf(
		"os=android&_rticket=%d&is_pad=0&last_install_time=%d&host_abi=arm64-v8a&ts=%d&ab_version=42.4.3&ac=wifi&ac2=wifi&aid=1233&app_language=en&app_name=musical_ly&app_type=normal&build_number=42.4.3&carrier_region=US&carrier_region_v2=310&channel=googleplay&current_region=US&device_brand=google&device_id=%s&device_platform=android&device_type=Pixel%%206&dpi=420&iid=%s&language=en&locale=en&manifest_version_code=2024204030&mcc_mnc=310004&op_region=US&os_api=35&os_version=15&region=US&residence=US&resolution=1080*2209&ssmix=a&sys_region=US&timezone_name=America%%2FNew_York&timezone_offset=-18000&uoo=0&update_version_code=2024204030&version_code=420403&version_name=42.4.3",
		utime, lastInstallTime, stime, deviceID, installID,
	)

	// 构建 URL
	urlStr := fmt.Sprintf(
		"https://aggr16-normal.tiktokv.us/aweme/v1/aweme/stats/?os=android&_rticket=%d&is_pad=0&last_install_time=%d&host_abi=arm64-v8a&ts=%d&",
		utime, lastInstallTime, stime,
	)

	// 构建 POST 数据
	prePlayTime := rand.Intn(900) + 100
	dt := fmt.Sprintf(
		"pre_item_playtime=%d&user_algo_refresh_status=false&first_install_time=%.0f&item_id=%s&is_ad=0&follow_status=0&pre_item_watch_time=%d&sync_origin=false&follower_status=0&action_time=%d&tab_type=22&pre_hot_sentence=&play_delta=1&request_id=&aweme_type=0&order=&pre_item_id=",
		prePlayTime, apkFirstInstallTime, awemeID, utime-int64(prePlayTime), stime,
	)
	post_data := hex.EncodeToString([]byte(dt))

	// Gzip 压缩
	var gzipBuf bytes.Buffer
	gzipWriter := gzip.NewWriter(&gzipBuf)
	gzipWriter.Write([]byte(dt))
	gzipWriter.Close()
	data := gzipBuf.Bytes()

	// 生成 headers
	headersResult := headers.MakeHeaders(
		deviceID,
		stime,
		signCount,
		2,
		4,
		stime-int64(rand.Intn(10)+1),
		token,
		"Pixel 6",
		seed,
		seedType,
		"",
		"",
		"",
		queryString,
		post_data,
		"42.4.3",
		"v05.02.02-ov-android",
		0x05020220,
		738,
		0xC40A800,
	)

	// 构建请求头
	reqHeaders := map[string]string{
		"authority":                                "aggr16-normal.tiktokv.us",
		"x-tt-pba-enable":                          "1",
		"x-bd-kmsv":                                "0",
		"x-tt-dm-status":                           "login=1;ct=1;rt=8",
		"x-ss-req-ticket":                          strconv.FormatInt(utime, 10),
		"sdk-version":                              "2",
		"passport-sdk-version":                     "-1",
		"x-vc-bdturing-sdk-version":                "2.3.17.i18n",
		"rpc-persist-pns-region-1":                 "US|6252001|5332921",
		"rpc-persist-pns-region-2":                 "US|6252001|5332921",
		"rpc-persist-pns-region-3":                 "US|6252001|5332921",
		"oec-vc-sdk-version":                       "3.2.1.i18n",
		"x-tt-request-tag":                         "n=0;nr=111;bg=0;rs=112",
		"x-bd-content-encoding":                    "gzip",
		"content-type":                             "application/x-www-form-urlencoded; charset=UTF-8",
		"x-ss-stub":                                headersResult.XSSStub,
		"rpc-persist-pyxis-policy-state-law-is-ca": "1",
		"rpc-persist-pyxis-policy-v-tnc":           "1",
		"x-tt-ttnet-origin-host":                   "api16-core-useast8.tiktokv.us",
		"x-ss-dp":                                  "1233",
		"user-agent":                               ua,
		"accept-encoding":                          "gzip, deflate, br",
		"x-argus":                                  headersResult.XArgus,
		"x-gorgon":                                 headersResult.XGorgon,
		"x-khronos":                                headersResult.XKhronos,
		"x-ladon":                                  headersResult.XLadon,
		"x-common-params-v2":                       fmt.Sprintf("ab_version=42.4.3&ac=wifi&ac2=wifi&aid=1233&app_language=en&app_name=musical_ly&app_type=normal&build_number=42.4.3&carrier_region=US&carrier_region_v2=310&channel=googleplay&current_region=US&device_brand=google&device_id=%s&device_platform=android&device_type=Pixel%%206&dpi=420&iid=%s&language=en&locale=en&manifest_version_code=2024204030&mcc_mnc=310004&op_region=US&os_api=35&os_version=15&region=US&residence=US&resolution=1080*2209&ssmix=a&sys_region=US&timezone_name=America%%2FNew_York&timezone_offset=-18000&uoo=0&update_version_code=2024204030&version_code=420403&version_name=42.4.3", deviceID, installID),
	}

	// 添加 guard headers
	reqHeaders["tt-device-guard-public-key"] = deviceHeaders["tt-ticket-guard-public-key"]
	reqHeaders["tt-device-guard-client-data"] = deviceHeaders["tt-device-guard-client-data"]

	// 默认 cookies
	// 默认 cookies
defaultCookies := map[string]string{
	"odin_tt":              "fcb67144eaeea8edf0cc23dd04d08453ee4984cf892e7b783df99acb14b781ec3fc3378c78baf1d99d63d6520029ad2f18f363aaf18787df87c4ae2d7201f010c5677df82fc55e6f8ebb02d5593556b9",
	"cmpl_token":           "AgQQAPNSF-RPsLlpSF-uIt038zeL5YZZv4zZYKLtCA",
	"store-idc":            "useast5",
	"store-country-code":   "us",
	"store-country-code-src": "uid",
	"sid_guard":            "bda836ea91cadb89381e4e46a1d859b0%7C1767352085%7C15552000%7CWed%2C+01-Jul-2026+11%3A08%3A05+GMT",
	"uid_tt_ss":            "5dca51b772581a4ae0b4b7bc59d3f7d85ed93a0fbe4648753cfbcc1719b8272f",
	"sid_tt":               "bda836ea91cadb89381e4e46a1d859b0",
	"store-country-sign":   "MEIEDKkuL0FxM8rLq2dllQQg1WNaO6atNycETGn-mOu5Mj-MDNRlKRwJ0OAhSLgc4NIEECfR2Ic1nZZg8DzsYAbaEpE",
	"uid_tt":               "5dca51b772581a4ae0b4b7bc59d3f7d85ed93a0fbe4648753cfbcc1719b8272f",
	"sessionid":            "bda836ea91cadb89381e4e46a1d859b0",
	"sessionid_ss":         "bda836ea91cadb89381e4e46a1d859b0",
	"reg-store-region":     "",
	"install_id":           "7590275174224103223",
	"multi_sids":           "7590719281676944398%3Abda836ea91cadb89381e4e46a1d859b0",
	"tt_session_tlb_tag":   "sttt%7C4%7Cvag26pHK24k4Hk5GodhZsP_________B0z-wSg5hudq4--F_yj7fN0ow7HbtaQqED8RpaA4BUvc%3D",
	"tt-target-idc":        "useast5",
}



	// 从 device 中读取 cookies
	cookies := make(map[string]string)

	// 先使用默认 cookies
	for k, v := range defaultCookies {
		cookies[k] = v
	}

	// 尝试从 device 中读取 cookies
	if cookiesRaw, exists := device["cookies"]; exists {
		switch cookiesVal := cookiesRaw.(type) {
		case map[string]interface{}:
			// 如果是 map[string]interface{}，转换为 map[string]string
			for k, v := range cookiesVal {
				if strVal, ok := v.(string); ok {
					cookies[k] = strVal
				} else {
					cookies[k] = fmt.Sprintf("%v", v)
				}
			}
		case map[string]string:
			// 如果已经是 map[string]string，直接使用
			for k, v := range cookiesVal {
				cookies[k] = v
			}
		case string:
			// 如果是 JSON 字符串，尝试解析
			var cookiesMap map[string]interface{}
			if err := json.Unmarshal([]byte(cookiesVal), &cookiesMap); err == nil {
				for k, v := range cookiesMap {
					if strVal, ok := v.(string); ok {
						cookies[k] = strVal
					} else {
						cookies[k] = fmt.Sprintf("%v", v)
					}
				}
			}
		}
	}

	// 创建请求
	req, err := http.NewRequest("POST", urlStr, bytes.NewReader(data))
	if err != nil {
		return "", fmt.Errorf("failed to create request: %v", err)
	}

	// 设置 headers
	for k, v := range reqHeaders {
		req.Header.Set(k, v)
	}

	// 设置 cookies
	for k, v := range cookies {
		req.AddCookie(&http.Cookie{Name: k, Value: v})
	}

	// 发送请求
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("request failed: %v", err)
	}
	defer resp.Body.Close()

	// 读取响应
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read response: %v", err)
	}

	return string(body), nil
}
