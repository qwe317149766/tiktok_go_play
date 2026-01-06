package main

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
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
)

// 针对 device_regitser.txt 中的 40.6.3 版本设置
const (
	AppVersion     = "40.6.3"
	AppVersionCode = 400603
	BuildNumber    = "40.6.3"
	UpdateCode     = 2024006030
	ManifestCode   = 2024006030
	UserAgent      = "com.zhiliaoapp.musically/2024006030 (Linux; U; Android 10; zh_TW; MI 8; Build/QKQ1.190828.002;tt-ok/3.12.13.20)"
	BdturingSDK    = "2.3.13.i18n"
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

// 随机生成工具
func randomHex(n int) string {
	charset := "0123456789abcdef"
	b := make([]byte, n)
	for i := range b {
		b[i] = charset[rand.Intn(len(charset))]
	}
	return string(b)
}

func randomUUID() string {
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		rand.Uint32(), rand.Uint32()&0xffff, rand.Uint32()&0xffff, rand.Uint32()&0xffff, rand.Uint64()&0xffffffffffff)
}

func randomDigits(n int) string {
	var sb strings.Builder
	for i := 0; i < n; i++ {
		sb.WriteByte(byte('0' + rand.Intn(10)))
	}
	return sb.String()
}

func main() {
	// 1. 初始化设备数据 (模拟抓包中的环境)
	openUDID := randomHex(16)
	cdid := randomUUID()
	reqID := randomUUID()
	ts := time.Now().Unix()
	rticket := time.Now().UnixNano() / 1e6

	// 设备基础信息
	model := "MI 8"
	brand := "Xiaomi"
	res := "1080*2029"
	dpi := 440

	fmt.Printf("🚀 开始设备注册调试...\n")
	fmt.Printf("OpenUDID: %s\nCDID: %s\n", openUDID, cdid)

	// 2. 构造 Query String (严格对应抓包中的顺序)
	qsParts := []string{
		fmt.Sprintf("req_id=%s", reqID),
		"device_platform=android",
		"os=android",
		"ssmix=a",
		fmt.Sprintf("_rticket=%d", rticket),
		fmt.Sprintf("cdid=%s", cdid),
		"channel=googleplay",
		"aid=1233",
		"app_name=musical_ly",
		fmt.Sprintf("version_code=%d", AppVersionCode),
		fmt.Sprintf("version_name=%s", AppVersion),
		fmt.Sprintf("manifest_version_code=%d", ManifestCode),
		fmt.Sprintf("update_version_code=%d", UpdateCode),
		fmt.Sprintf("ab_version=%s", AppVersion),
		fmt.Sprintf("resolution=%s", res),
		fmt.Sprintf("dpi=%d", dpi),
		fmt.Sprintf("device_type=%s", strings.ReplaceAll(model, " ", "%20")),
		fmt.Sprintf("device_brand=%s", brand),
		"language=zh-Hant",
		"os_api=29",
		"os_version=10",
		"ac=wifi",
		"is_pad=0",
		"app_type=normal",
		"sys_region=TW",
		fmt.Sprintf("last_install_time=%d", ts-15),
		"timezone_name=Asia%2FYerevan",
		"app_language=zh-Hant",
		"timezone_offset=14400",
		"host_abi=arm64-v8a",
		"locale=zh-Hant-TW",
		"ac2=unknown",
		"uoo=1",
		"op_region=TW",
		fmt.Sprintf("build_number=%s", BuildNumber),
		"region=TW",
		fmt.Sprintf("ts=%d", ts),
		fmt.Sprintf("openudid=%s", openUDID),
		"okhttp_version=4.2.228.18-tiktok",
		"use_store_region_cookie=1",
	}
	qs := strings.Join(qsParts, "&")

	// 3. 构造 JSON Payload (header 部分)
	payload := map[string]interface{}{
		"header": map[string]interface{}{
			"os":                  "Android",
			"os_version":          "10",
			"os_api":              29,
			"device_model":        model,
			"device_brand":        brand,
			"device_manufacturer": brand,
			"cpu_abi":             "arm64-v8a",
			"density_dpi":         dpi,
			"display_density":     "mdpi",
			"resolution":          "2029x1080",
			"display_density_v2":  "xxhdpi",
			"resolution_v2":       "2248x1080",
			"access":              "wifi",
			"rom":                 "MIUI-V12.5.2.0.QEACNXM",
			"rom_version":         "miui_V125_V12.5.2.0.QEACNXM",
			"language":            "zh",
			"timezone":            4,
			"region":              "TW",
			"tz_name":             "Asia/Yerevan",
			"tz_offset":           14400,
			"clientudid":          randomUUID(),
			"openudid":            openUDID,
			"channel":             "googleplay",
			"not_request_sender":  1,
			"aid":                 1233,
			"release_build":       "4ca920e_20250626",
			"ab_version":          AppVersion,
			"gaid_limited":        0,
			"custom": map[string]interface{}{
				"ram_size":                "6GB",
				"dark_mode_setting_value": 1,
				"is_foldable":             0,
				"screen_height_dp":        817,
				"apk_last_update_time":    rticket - 60000,
				"filter_warn":             0,
				"priority_region":         "TW",
				"user_period":             0,
				"is_kids_mode":            0,
				"web_ua":                  fmt.Sprintf("Dalvik/2.1.0 (Linux; U; Android 10; %s MIUI/V12.5.2.0.QEACNXM)", model),
				"screen_width_dp":         393,
				"user_mode":               -1,
			},
			"package":                "com.zhiliaoapp.musically",
			"app_version":            AppVersion,
			"app_version_minor":      "",
			"version_code":           AppVersionCode,
			"update_version_code":    UpdateCode,
			"manifest_version_code":  ManifestCode,
			"app_name":               "musical_ly",
			"tweaked_channel":        "googleplay",
			"display_name":           "TikTok",
			"sig_hash":               "194326e82c84a639a52e5c023116f12a",
			"cdid":                   cdid,
			"device_platform":        "android",
			"git_hash":               "5151884",
			"sdk_version_code":       2050990,
			"sdk_target_version":     30,
			"req_id":                 reqID,
			"sdk_version":            "2.5.9",
			"guest_mode":             0,
			"sdk_flavor":             "i18nInner",
			"apk_first_install_time": rticket - 60000,
			"is_system_app":          0,
		},
		"magic_tag": "ss_app_log",
		"_gen_time": rticket,
	}

	payloadBytes, _ := json.Marshal(payload)
	postDataHex := hex.EncodeToString(payloadBytes)

	// 4. 调用加密算法 (使用与 main-1.go 相同的 headers 包)
	// MakeHeaders 内部处理 Gorgon, Khronos, Argus, Ladon 等
	h := headers.MakeHeaders(
		"0", // 初始注册 deviceID 为 "0"
		ts,
		1,  // signCount
		0,  // reportCount
		0,  // settingCount
		ts, // launchTime
		"", // secDeviceToken
		model,
		"", // seed
		0,  // seedType
		"", // seedHex
		"", // algorithmData
		"", // hex32
		qs,
		postDataHex,
		AppVersion,
		"v05.02.02-ov-android",
		0x05020220,
		738,
		0xC40A800,
	)

	// 5. 发送请求
	apiUrl := fmt.Sprintf("https://log-boot.tiktokv.com/service/2/device_register/?%s", qs)
	req, _ := http.NewRequest("POST", apiUrl, bytes.NewBuffer(payloadBytes))

	// 设置 Headers (严格对应抓包数据)
	req.Header.Set("Host", "log-boot.tiktokv.com")
	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("X-SS-Stub", h.XSSStub) // 对应抓包中的 x-ss-stub
	req.Header.Set("Accept-Encoding", "gzip")
	req.Header.Set("X-TT-App-Init-Region", "carrierregion=;mccmnc=;sysregion=TW;appregion=TW")
	req.Header.Set("X-TT-Request-Tag", "t=0;n=1")
	req.Header.Set("SDK-Version", "2")
	req.Header.Set("X-TT-DM-Status", "login=0;ct=0;rt=7")
	req.Header.Set("X-SS-Req-Ticket", fmt.Sprintf("%d", rticket))
	req.Header.Set("Passport-SDK-Version", "-1")
	req.Header.Set("X-VC-Bdturing-SDK-Version", BdturingSDK)
	req.Header.Set("X-Ladon", h.XLadon)
	req.Header.Set("X-Khronos", h.XKhronos)
	req.Header.Set("X-Argus", h.XArgus)
	req.Header.Set("X-Gorgon", h.XGorgon)

	// 6. 配置 Client (支持代理调试)
	transport := &http.Transport{}
	// 如果需要使用代理，取消下面注释并设置正确的代理地址
	proxyUrl, _ := url.Parse("http://127.0.0.1:7890")
	transport.Proxy = http.ProxyURL(proxyUrl)

	client := &http.Client{
		Timeout:   15 * time.Second,
		Transport: transport,
	}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("❌ 请求失败: %v\n", err)
		return
	}
	var reader io.ReadCloser
	if resp.Header.Get("Content-Encoding") == "gzip" {
		reader, err = gzip.NewReader(resp.Body)
		if err != nil {
			fmt.Printf("❌ Gzip 解压失败: %v\n", err)
			return
		}
		defer reader.Close()
	} else {
		reader = resp.Body
	}

	body, _ := io.ReadAll(reader)
	fmt.Printf("\nHTTP 状态码: %d\n", resp.StatusCode)

	// 解析响应
	var respJSON map[string]interface{}
	decoder := json.NewDecoder(bytes.NewReader(body))
	decoder.UseNumber() // 防止大数字丢失精度
	if err := decoder.Decode(&respJSON); err == nil {
		fmt.Println("✅ 注册成功！提取关键信息:")

		didStr, _ := respJSON["device_id_str"].(string)
		iidStr, _ := respJSON["install_id_str"].(string)

		if didStr == "" {
			// 尝试从非 string 字段获取
			if val, ok := respJSON["device_id"]; ok {
				didStr = fmt.Sprintf("%v", val)
			}
		}
		if iidStr == "" {
			if val, ok := respJSON["install_id"]; ok {
				iidStr = fmt.Sprintf("%v", val)
			}
		}

		fmt.Printf("Device ID:  %s\n", didStr)
		fmt.Printf("Install ID: %s\n", iidStr)

		if didStr == "" || didStr == "0" {
			fmt.Println("⚠️ 警告: 未获取到有效的 Device ID，可能是请求被拦截或参数有误。")
			fmt.Printf("完整响应预览 (前 500 字符): %s\n", string(body)[:500])
		}
	} else {
		fmt.Printf("❌ 响应解析失败: %v\n", err)
		// 如果解析失败，打印前 500 个字符
		limit := len(body)
		if limit > 500 {
			limit = 500
		}
		fmt.Printf("原始响应内容: %s\n", string(body[:limit]))
	}
}

// 辅助函数: 计算 SHA256 (用于 payload 模拟)
func getSha256(s string) string {
	h := sha256.New()
	h.Write([]byte(s))
	return hex.EncodeToString(h.Sum(nil))
}
