package main

import (
	"bytes"
	"compress/gzip"
	"crypto/sha256"
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

// DEVICE_POOL 定义
type DeviceTemplate struct {
	Model      string
	Brand      string
	Resolution string
	DPI        int
}

var devicePool = []DeviceTemplate{
	{"SM-F936B", "samsung", "904*2105", 420},
	{"M2012K11AG", "xiaomi", "904*2105", 440},
	{"RMX3081", "realme", "904*2105", 480},
	{"Pixel 6", "google", "904*2105", 420},
	{"CPH2411", "oppo", "904*2105", 440},
}

func init() {
	rand.Seed(time.Now().UnixNano())
}

func digits(n int) string {
	var sb strings.Builder
	for i := 0; i < n; i++ {
		sb.WriteByte(byte('0' + rand.Intn(10)))
	}
	return sb.String()
}

func hexString(n int) string {
	const charset = "0123456789abcdef"
	var sb strings.Builder
	for i := 0; i < n; i++ {
		sb.WriteByte(charset[rand.Intn(len(charset))])
	}
	return sb.String()
}

func uuid4() string {
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		rand.Uint32(),
		rand.Uint32()&0xffff,
		rand.Uint32()&0xffff,
		rand.Uint32()&0xffff,
		rand.Uint64()&0xffffffffffff)
}

func getDensityClass(dpi int) string {
	if dpi < 400 {
		return "hdpi"
	}
	if dpi < 440 {
		return "xhdpi"
	}
	if dpi < 480 {
		return "xxhdpi"
	}
	return "xxxhdpi"
}

func getSha256(s string) string {
	h := sha256.New()
	h.Write([]byte(s))
	return hex.EncodeToString(h.Sum(nil))
}

func registerDeviceOnce(device map[string]interface{}) {
	ts := time.Now().Unix()
	rticket := ts * 1000
	model := device["device_type"].(string)
	brand := device["device_brand"].(string)
	resolution := device["resolution"].(string)
	dpi := device["dpi"].(int)

	resParts := strings.Split(resolution, "*")
	resX, resY := resParts[0], resParts[1]
	density := getDensityClass(dpi)

	params := url.Values{}
	params.Set("req_id", device["req_id"].(string))
	params.Set("device_platform", "android")
	params.Set("os", "android")
	params.Set("ssmix", "a")
	params.Set("_rticket", fmt.Sprintf("%d", rticket))
	params.Set("cdid", device["cdid"].(string))
	params.Set("channel", "googleplay")
	params.Set("aid", "1233")
	params.Set("app_name", "musical_ly")
	params.Set("version_code", "400603")
	params.Set("version_name", "40.6.3")
	params.Set("manifest_version_code", "2024006030")
	params.Set("update_version_code", "2024006030")
	params.Set("ab_version", "40.6.3")
	params.Set("resolution", resolution)
	params.Set("dpi", fmt.Sprintf("%d", dpi))
	params.Set("device_type", model)
	params.Set("device_brand", brand)
	params.Set("language", "zh-Hant")
	params.Set("os_api", "29")
	params.Set("os_version", "10")
	params.Set("ac", "wifi")
	params.Set("is_pad", "0")
	params.Set("app_type", "normal")
	params.Set("sys_region", "TW")
	params.Set("last_install_time", fmt.Sprintf("%d", ts-10))
	params.Set("timezone_name", "Asia/Yerevan")
	params.Set("app_language", "zh-Hant")
	params.Set("timezone_offset", "14400")
	params.Set("host_abi", "arm64-v8a")
	params.Set("locale", "zh-Hant-TW")
	params.Set("ac2", "unknown")
	params.Set("uoo", "1")
	params.Set("op_region", "TW")
	params.Set("build_number", "40.6.3")
	params.Set("region", "TW")
	params.Set("ts", fmt.Sprintf("%d", ts))
	params.Set("openudid", device["openudid"].(string))
	params.Set("okhttp_version", "4.2.228.18-tiktok")
	params.Set("use_store_region_cookie", "1")

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
			"display_density":     density,
			"resolution":          fmt.Sprintf("%sx%s", resY, resX),
			"display_density_v2":  "xxhdpi",
			"resolution_v2":       fmt.Sprintf("%sx%s", resY, resX),
			"access":              "wifi",
			"rom":                 "MIUI-V12.5.2.0.QEACNXM",
			"rom_version":         "miui_V125_V12.5.2.0.QEACNXM",
			"language":            "zh",
			"timezone":            4,
			"region":              "TW",
			"tz_name":             "Asia/Yerevan",
			"tz_offset":           14400,
			"clientudid":          uuid4(),
			"openudid":            device["openudid"],
			"channel":             "googleplay",
			"not_request_sender":  1,
			"aid":                 1233,
			"release_build":       "4ca920e_20250626",
			"ab_version":          "40.6.3",
			"gaid_limited":        0,
			"custom": map[string]interface{}{
				"ram_size":                "6GB",
				"dark_mode_setting_value": 1,
				"is_foldable":             0,
				"screen_height_dp":        817,
				"apk_last_update_time":    rticket - 600000,
				"filter_warn":             0,
				"priority_region":         "TW",
				"user_period":             0,
				"is_kids_mode":            0,
				"web_ua":                  fmt.Sprintf("Dalvik/2.1.0 (Linux; U; Android 10; %s Build/QKQ1.190828.002)", model),
				"screen_width_dp":         393,
				"user_mode":               -1,
			},
			"package":                "com.zhiliaoapp.musically",
			"app_version":            "40.6.3",
			"app_version_minor":      "",
			"version_code":           400603,
			"update_version_code":    2024006030,
			"manifest_version_code":  2024006030,
			"app_name":               "musical_ly",
			"tweaked_channel":        "googleplay",
			"display_name":           "TikTok",
			"sig_hash":               "194326e82c84a639a52e5c023116f12a",
			"cdid":                   device["cdid"],
			"device_platform":        "android",
			"git_hash":               "5151884",
			"sdk_version_code":       2050990,
			"sdk_target_version":     30,
			"req_id":                 device["req_id"],
			"sdk_version":            "2.5.9",
			"guest_mode":             0,
			"sdk_flavor":             "i18nInner",
			"apk_first_install_time": rticket - 600000,
			"is_system_app":          0,
		},
		"magic_tag": "ss_app_log",
		"_gen_time": rticket,
	}

	qs := params.Encode()
	payloadBytes, _ := json.Marshal(payload)
	postDataHex := hex.EncodeToString(payloadBytes)

	h := headers.MakeHeaders(
		device["device_id"].(string),
		ts,
		1,
		0,
		0,
		ts,
		"",
		model,
		"",
		0,
		"",
		"",
		"",
		qs,
		postDataHex,
		"40.6.3",
		"v05.02.02-ov-android",
		0x05020220,
		738,
		0xC40A800,
	)

	reqUrl := fmt.Sprintf("https://log-boot.tiktokv.com/service/2/device_register/?%s", qs)
	req, _ := http.NewRequest("POST", reqUrl, bytes.NewBuffer(payloadBytes))

	req.Header.Set("User-Agent", fmt.Sprintf("com.zhiliaoapp.musically/2024006030 (Linux; U; Android 10; zh_TW; %s; Build/QKQ1.190828.002;tt-ok/3.12.13.20)", model))
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("x-tt-app-init-region", "carrierregion=;mccmnc=;sysregion=TW;appregion=TW")
	req.Header.Set("x-tt-request-tag", "t=0;n=1")
	req.Header.Set("sdk-version", "2")
	req.Header.Set("x-tt-dm-status", "login=0;ct=0;rt=7")
	req.Header.Set("x-ss-req-ticket", fmt.Sprintf("%d", rticket))
	req.Header.Set("passport-sdk-version", "-1")
	req.Header.Set("x-vc-bdturing-sdk-version", "2.3.13.i18n")
	req.Header.Set("x-ss-stub", h.XSSStub)
	req.Header.Set("x-gorgon", h.XGorgon)
	req.Header.Set("x-khronos", h.XKhronos)
	req.Header.Set("x-argus", h.XArgus)
	req.Header.Set("x-ladon", h.XLadon)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("注册请求失败: %v\n", err)
		return
	}
	defer resp.Body.Close()

	// 提取 Cookies
	cookies := make(map[string]string)
	for _, ck := range resp.Header["Set-Cookie"] {
		parts := strings.Split(strings.Split(ck, ";")[0], "=")
		if len(parts) >= 2 {
			cookies[parts[0]] = parts[1]
		}
	}
	device["cookies"] = cookies

	// 打印Header
	fmt.Println("\nHeader:")
	for name, values := range resp.Header {
		fmt.Printf("%s: %s\n", name, strings.Join(values, ", "))
	}
	body, _ := io.ReadAll(resp.Body)
	var respData map[string]interface{}
	if err := json.Unmarshal(body, &respData); err == nil {
		if newUser, ok := respData["new_user"].(float64); ok && newUser == 0 {
			fmt.Println("\n✅ Cihaz bilgileri (2. kayıt sonrası new_user=0):")
			deviceJSON, _ := json.MarshalIndent(device, "", "  ")
			fmt.Println(string(deviceJSON))
		}
	} else {
		fmt.Println("Cevap parse edilemedi.")
	}

	// 注册成功后执行激活逻辑
	fmt.Println("执行设备激活 (alert_check)...")
	ActivateDevice(device)
}

func ActivateDevice(device map[string]interface{}) {
	ts := time.Now().Unix()
	utime := ts * 1000
	model := device["device_type"].(string)
	deviceID := device["device_id"].(string)
	iid := device["install_id"].(string)

	params := url.Values{}
	params.Set("device_platform", "android")
	params.Set("os", "android")
	params.Set("ssmix", "a")
	params.Set("_rticket", fmt.Sprintf("%d", utime))
	params.Set("cdid", device["cdid"].(string))
	params.Set("channel", "googleplay")
	params.Set("aid", "1233")
	params.Set("app_name", "musical_ly")
	params.Set("version_code", "420403")
	params.Set("version_name", "42.4.3")
	params.Set("manifest_version_code", "2024204030")
	params.Set("update_version_code", "2024204030")
	params.Set("ab_version", "42.4.3")
	params.Set("resolution", device["resolution"].(string))
	params.Set("dpi", fmt.Sprintf("%d", device["dpi"].(int)))
	params.Set("device_type", model)
	params.Set("device_brand", device["device_brand"].(string))
	params.Set("language", "tr")
	params.Set("os_api", "34")
	params.Set("os_version", "14")
	params.Set("ac", "wifi")
	params.Set("is_pad", "1")
	params.Set("current_region", "TR")
	params.Set("app_type", "normal")
	params.Set("sys_region", "TR")
	params.Set("last_install_time", fmt.Sprintf("%d", ts-20000))
	params.Set("timezone_name", "Asia/Istanbul")
	params.Set("residence", "TR")
	params.Set("timezone_offset", "10800")
	params.Set("host_abi", "arm64-v8a")
	params.Set("locale", "tr-TR")
	params.Set("ac2", "wifi")
	params.Set("uoo", "0")
	params.Set("op_region", "TR")
	params.Set("build_number", "42.4.3")
	params.Set("region", "TR")
	params.Set("ts", fmt.Sprintf("%d", ts))
	params.Set("iid", iid)
	params.Set("device_id", deviceID)
	params.Set("openudid", device["openudid"].(string))
	params.Set("req_id", device["req_id"].(string))
	params.Set("google_aid", device["gaid"].(string))

	qs := params.Encode()
	h := headers.MakeHeaders(deviceID, ts, 2, 2, 4, ts-5, "", model, "", 0, "", "", "", qs, "", "42.4.3", "v05.02.02-ov-android", 0x05020220, 738, 0xC40A800)

	reqUrl := fmt.Sprintf("https://log22-normal-alisg.tiktokv.com/service/2/app_alert_check/?%s", qs)
	req, _ := http.NewRequest("GET", reqUrl, nil)

	req.Header.Set("User-Agent", fmt.Sprintf("com.zhiliaoapp.musically/2024204030 (Linux; Android 15; %s)", model))
	req.Header.Set("x-tt-dm-status", "login=0;ct=0;rt=1")
	req.Header.Set("x-ss-req-ticket", fmt.Sprintf("%d", utime))
	req.Header.Set("sdk-version", "2")
	req.Header.Set("passport-sdk-version", "-1")
	req.Header.Set("x-vc-bdturing-sdk-version", "2.3.17.i18n")
	req.Header.Set("oec-vc-sdk-version", "3.2.1.i18n")
	req.Header.Set("x-gorgon", h.XGorgon)
	req.Header.Set("x-khronos", h.XKhronos)
	req.Header.Set("x-argus", h.XArgus)
	req.Header.Set("x-ladon", h.XLadon)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err == nil {
		defer resp.Body.Close()
		body, _ := io.ReadAll(resp.Body)
		fmt.Printf("设备激活结果: %d, 返回值: %s\n", resp.StatusCode, string(body))
	} else {
		fmt.Printf("设备激活失败: %v\n", err)
	}
}

func MakeDsSign(device map[string]interface{}) {
	ts := time.Now().Unix()
	utime := ts * 1000
	model := device["device_type"].(string)
	deviceID := device["device_id"].(string)
	iid := device["install_id"].(string)
	brand := device["device_brand"].(string)

	kp, _ := headers.GenerateDeltaKeypair()
	device["priv_key"] = kp.PrivKeyHex
	device["pub_key_b64"] = kp.PubKeyB64
	bodyJSON := fmt.Sprintf(`{"device_id":"%s","install_id":"%s","aid":1233,"app_version":"42.4.3","model":"%s","os":"Android","openudid":"%s","google_aid":"%s","properties_version":"android-1.0","device_properties":{"device_model":"%s","device_manufacturer":"%s","disk_size":"ea489ffb302814b62320c02536989a3962de820f5a481eb5bac1086697d9aa3c","memory_size":"291cf975c42a1e788fdc454e3c7330d641db5f9f7ba06e37f7f388b3448bc374","resolution":"%s","re_time":"0af7de3d5239bb5542f0653e57c7c8b9","indss18":"8725063fe010181646c25d1f993e1589","indc15":"7874453cef13dddd56fcb3c7e8e99c28","indn5":"a9ca935c4885bbc1da2be687f153354c","indmc14":"e678d34e71a6943f1cab0bfa3c7a226b","inda0":"d0eac42291b9a88173d9914972a65d8b","indal2":"d7baecabd462bc9f960eaab4c81a55c5","indm10":"446ae4837d88b3b3988d57b9747e11cd","indsp3":"9861cb1513b66e9aaeb66ef048bfdd18","indsd8":"a15ec37e1115dea871970a39ec0769c4","bl":"a3d41c6f3e8c1892d2cc97469805b1f0","cmf":"5494690cb9b316eb618265ea11dc5146","bc":"1e2b66f4392214037884408109a383df","stz":"e6f9d2069f89b53a8e6f2c65929d2e50","sl":"2389ca43e5adab9de01d2dda7633ac39"}}`,
		deviceID, iid, model, device["openudid"], device["gaid"],
		getSha256(model), getSha256(brand), getSha256(device["resolution"].(string)))

	postDataHex := hex.EncodeToString([]byte(bodyJSON))
	// 确保 query 参数顺序与 Python 类似
	qs := fmt.Sprintf("from=normal&device_platform=android&os=android&ssmix=a&_rticket=%d&cdid=%s&channel=googleplay&aid=1233&app_name=musical_ly&version_code=420403&version_name=42.4.3&manifest_version_code=2024204030&update_version_code=2024204030&ab_version=42.4.3&resolution=%s&dpi=%d&device_type=%s&device_brand=%s&language=tr&os_api=34&os_version=14&ac=wifi&is_pad=0&app_type=normal&sys_region=TR&last_install_time=%d&mcc_mnc=28601&timezone_name=Asia%%2FIstanbul&carrier_region_v2=286&app_language=tr&carrier_region=TR&ac2=wifi&uoo=0&op_region=TR&timezone_offset=10800&build_number=42.4.3&host_abi=arm64-v8a&locale=tr&region=TR&ts=%d&iid=%s&device_id=%s&openudid=%s",
		utime, device["cdid"], device["resolution"], device["dpi"], model, brand, ts-86400, ts, iid, deviceID, device["openudid"])

	h := headers.MakeHeaders(deviceID, ts, rand.Intn(20)+20, 2, 4, ts-int64(rand.Intn(10)+1), "", model, "", 0, "", "", "", qs, postDataHex, "42.4.3", "v05.02.02-ov-android", 0x05020220, 738, 0xC40A800)

	reqUrl := fmt.Sprintf("https://log22-normal-alisg.tiktokv.com/service/2/dsign/?%s", qs)
	req, _ := http.NewRequest("POST", reqUrl, bytes.NewBuffer([]byte(bodyJSON)))

	// 使用更真实的 UA
	ua := fmt.Sprintf("com.zhiliaoapp.musically/2024204030 (Linux; U; Android 14; tr-TR; %s; Build/TP1A.220624.014; Cronet/TTNetVersion:efce646d 2025-10-16 QuicVersion:c785494a 2025-09-30)", model)
	req.Header.Set("User-Agent", ua)
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("tt-ticket-guard-public-key", kp.PubKeyB64)
	req.Header.Set("x-tt-request-tag", "t=0;n=1")
	req.Header.Set("tt-device-guard-iteration-version", "1")
	req.Header.Set("x-ss-dp", "1233")

	// 合并注册返回的 Cookie 和手动模拟的 Cookie
	cookieStr := fmt.Sprintf("store-idc=alisg; store-country-code=tr; store-country-code-src=did; install_id=%s", iid)
	if regCookies, ok := device["cookies"].(map[string]string); ok {
		for k, v := range regCookies {
			if k != "install_id" && k != "store-idc" && k != "store-country-code" && k != "store-country-code-src" {
				cookieStr += fmt.Sprintf("; %s=%s", k, v)
			}
		}
	}
	req.Header.Set("cookie", cookieStr)

	req.Header.Set("x-tt-dm-status", "login=0;ct=0;rt=1")
	req.Header.Set("x-ss-req-ticket", fmt.Sprintf("%d", utime))
	req.Header.Set("sdk-version", "2")
	req.Header.Set("passport-sdk-version", "-1")
	req.Header.Set("x-vc-bdturing-sdk-version", "2.3.17.i18n")
	req.Header.Set("rpc-persist-pyxis-policy-state-law-is-ca", "1")
	req.Header.Set("rpc-persist-pyxis-policy-v-tnc", "1")
	req.Header.Set("x-tt-ttnet-origin-host", "log22-normal-alisg.tiktokv.com")
	req.Header.Set("x-ss-stub", h.XSSStub)
	req.Header.Set("x-gorgon", h.XGorgon)
	req.Header.Set("x-khronos", h.XKhronos)
	req.Header.Set("x-argus", h.XArgus)
	req.Header.Set("x-ladon", h.XLadon)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err == nil {
		defer resp.Body.Close()
		fmt.Printf("DSign 请求结果: %d\n", resp.StatusCode)
		body, _ := io.ReadAll(resp.Body)
		fmt.Printf("DSign 返回内容: %s\n", string(body))
		var respJSON map[string]interface{}
		json.Unmarshal(body, &respJSON)
		if serverData, ok := respJSON["tt-device-guard-server-data"].(string); ok {
			decoded, _ := base64.StdEncoding.DecodeString(serverData)
			device["device_guard_data0"] = string(decoded)
			fmt.Println("device_guard_data0 获取成功")
		} else {
			fmt.Println("调试: DSign 响应中未找到 tt-device-guard-server-data")
		}
	} else {
		fmt.Printf("DSign 请求失败: %v\n", err)
	}
	fmt.Println("调试: MakeDsSign 执行结束")
}

func registerDeviceFull() map[string]interface{} {
	tpl := devicePool[rand.Intn(len(devicePool))]

	device := map[string]interface{}{
		"device_id":    digits(19),
		"install_id":   digits(19),
		"openudid":     hexString(16),
		"cdid":         uuid4(),
		"req_id":       uuid4(),
		"gaid":         uuid4(),
		"device_type":  tpl.Model,
		"device_brand": tpl.Brand,
		"resolution":   tpl.Resolution,
		"dpi":          tpl.DPI,
	}

	registerDeviceOnce(device)
	time.Sleep(1500 * time.Millisecond)

	fmt.Println("执行 ActivateDevice (alert_check)...")
	ActivateDevice(device)
	time.Sleep(1000 * time.Millisecond)

	fmt.Println("执行 MakeDsSign...")
	MakeDsSign(device)

	return device
}

func getDynamicPayload(itemID string) (string, int64) {
	ts := time.Now().Unix()
	payload := fmt.Sprintf(
		"pre_item_playtime=915&user_algo_refresh_status=false&first_install_time=%d&item_id=%s&is_ad=0&follow_status=0&sync_origin=false&follower_status=0&action_time=%d&tab_type=22&pre_hot_sentence=&play_delta=1&request_id=&aweme_type=0&order=",
		ts-50000, itemID, ts,
	)
	return payload, ts
}

func sendViewRequest(device map[string]interface{}, itemID string, seed string, seedType int, token string) {
	payloadStr, ts := getDynamicPayload(itemID)

	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	zw.Write([]byte(payloadStr))
	zw.Close()
	payloadGzip := buf.Bytes()

	rticket := ts * 1000

	baseUrl := "https://api31-core-alisg.tiktokv.com/aweme/v1/aweme/stats/"
	commonParams := url.Values{}
	commonParams.Set("ab_version", "42.4.3")
	commonParams.Set("ac", "wifi")
	commonParams.Set("ac2", "wifi")
	commonParams.Set("aid", "1233")
	commonParams.Set("app_language", "tr")
	commonParams.Set("app_name", "musical_ly")
	commonParams.Set("app_type", "normal")
	commonParams.Set("build_number", "42.4.3")
	commonParams.Set("cdid", device["cdid"].(string))
	commonParams.Set("channel", "googleplay")
	commonParams.Set("current_region", "TR")
	commonParams.Set("device_brand", device["device_brand"].(string))
	commonParams.Set("device_id", device["device_id"].(string))
	commonParams.Set("device_platform", "android")
	commonParams.Set("device_type", device["device_type"].(string))
	commonParams.Set("dpi", fmt.Sprintf("%d", device["dpi"].(int)))
	commonParams.Set("iid", device["install_id"].(string))
	commonParams.Set("language", "tr")
	commonParams.Set("locale", "tr-TR")
	commonParams.Set("manifest_version_code", "2024204030")
	commonParams.Set("op_region", "TR")
	commonParams.Set("openudid", device["openudid"].(string))
	commonParams.Set("os_api", "34")
	commonParams.Set("os_version", "14")
	commonParams.Set("region", "TR")
	commonParams.Set("residence", "TR")
	commonParams.Set("resolution", device["resolution"].(string))
	commonParams.Set("ssmix", "a")
	commonParams.Set("sys_region", "TR")
	commonParams.Set("timezone_name", "Asia/Istanbul")
	commonParams.Set("timezone_offset", "10800")
	commonParams.Set("uoo", "0")
	commonParams.Set("update_version_code", "2024204030")
	commonParams.Set("version_code", "420403")
	commonParams.Set("version_name", "42.4.3")

	urlParams := url.Values{}
	urlParams.Set("os", "android")
	urlParams.Set("_rticket", fmt.Sprintf("%d", rticket))
	urlParams.Set("is_pad", "1")
	urlParams.Set("last_install_time", fmt.Sprintf("%d", ts-20000))
	urlParams.Set("is_foldable", "1")
	urlParams.Set("host_abi", "arm64-v8a")
	urlParams.Set("ts", fmt.Sprintf("%d", ts))

	fullParams := urlParams.Encode() + "&" + commonParams.Encode()
	postDataHex := hex.EncodeToString([]byte(payloadStr))

	h := headers.MakeHeaders(
		device["device_id"].(string),
		ts,
		1,
		2, // reportCount
		4, // settingCount
		ts,
		token, // secDeviceToken 参与签名
		device["device_type"].(string),
		seed,
		seedType,
		"",
		"",
		"",
		fullParams,
		postDataHex,
		"42.4.3",
		"v05.02.02-ov-android",
		0x05020220,
		738,
		0xC40A800,
	)

	reqUrl := fmt.Sprintf("%s?%s", baseUrl, urlParams.Encode())
	req, _ := http.NewRequest("POST", reqUrl, bytes.NewBuffer(payloadGzip))

	req.Header.Set("User-Agent", fmt.Sprintf("com.zhiliaoapp.musically/2024204030 (Linux; Android 14; tr_TR; %s; Build/UP1A.231005.007)", device["device_type"]))
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Accept-Encoding", "gzip, deflate, br")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=UTF-8")
	req.Header.Set("x-bd-content-encoding", "gzip")
	req.Header.Set("x-common-params-v2", commonParams.Encode())
	req.Header.Set("x-tt-pba-enable", "1")
	req.Header.Set("x-bd-kmsv", "0")
	req.Header.Set("x-ss-req-ticket", fmt.Sprintf("%d", rticket))
	req.Header.Set("sdk-version", "2")
	req.Header.Set("passport-sdk-version", "-1")
	req.Header.Set("x-vc-bdturing-sdk-version", "2.3.17.i18n")
	req.Header.Set("oec-vc-sdk-version", "3.2.1.i18n")
	req.Header.Set("x-ss-stub", h.XSSStub)
	req.Header.Set("x-tt-dm-status", "login=1;ct=1;rt=8")
	req.Header.Set("rpc-persist-pyxis-policy-state-law-is-ca", "1")
	req.Header.Set("rpc-persist-pyxis-policy-v-tnc", "1")
	req.Header.Set("x-gorgon", h.XGorgon)
	req.Header.Set("x-khronos", h.XKhronos)
	req.Header.Set("x-ladon", h.XLadon)
	req.Header.Set("x-argus", h.XArgus)

	// 计算 BuildGuard
	if guardData0Obj, ok := device["device_guard_data0"]; ok {
		fmt.Printf("调试: 找到 device_guard_data0, 类型: %T\n", guardData0Obj)
		guardData0Str, ok := guardData0Obj.(string)
		if !ok {
			fmt.Printf("调试: device_guard_data0 无法转换为 string, 实际类型: %T\n", guardData0Obj)
		} else {
			var guardData0 map[string]interface{}
			if err := json.Unmarshal([]byte(guardData0Str), &guardData0); err == nil {
				fmt.Println("调试: device_guard_data0 解析成功")
				privKey, _ := device["priv_key"].(string)
				// BuildGuard(deviceGuardData0 map[string]interface{}, cookie map[string]string, path string, timestamp int64, privHex string, isTicket bool)
				guardHeaders, err := headers.BuildGuard(guardData0, nil, "/aweme/v1/aweme/stats/", ts, privKey, false)
				//打印guardHeaders
				fmt.Println("guardHeaders:", guardHeaders)
				if err == nil {
					req.Header.Set("tt-device-guard-public-key", guardHeaders["tt-ticket-guard-public-key"])
					req.Header.Set("tt-device-guard-client-data", guardHeaders["tt-device-guard-client-data"])
				} else {
					fmt.Println("调试: BuildGuard 失败:", err)
				}
			} else {
				fmt.Println("调试: device_guard_data0 解析失败:", err)
			}
		}
	} else {
		fmt.Println("调试: 未找到 device_guard_data0")
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("❌ İstek Hatası: %v\n", err)
		return
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode == 200 {
		var respJSON map[string]interface{}
		json.Unmarshal(body, &respJSON)
		fmt.Printf("✅ İzlenme gönderildi. Response: %v\n", respJSON)
	} else {
		fmt.Printf("⚠️ Hata [%d] - %s\n", resp.StatusCode, string(body))
	}
}

func main() {
	device := registerDeviceFull()

	// 构建用于获取 Seed/Token 的参数
	cookieData := make(map[string]string)
	cookieData["device_id"] = device["device_id"].(string)
	cookieData["install_id"] = device["install_id"].(string)
	cookieData["device_type"] = device["device_type"].(string)
	if ckRaw, ok := device["cookies"]; ok {
		ckMap := ckRaw.(map[string]string)
		for k, v := range ckMap {
			cookieData[k] = v
		}
	}
	// 设置默认 UA
	cookieData["ua"] = fmt.Sprintf("com.zhiliaoapp.musically/2024204030 (Linux; Android 14; %s)", device["device_type"].(string))

	fmt.Println("\n获取 Seed...")
	seed, seedType, err := get_seed.GetSeed(cookieData, "")
	if err != nil {
		fmt.Printf("获取 Seed 失败: %v\n", err)
	} else {
		fmt.Printf("Seed 获取成功: %s, Type: %d\n", seed, seedType)
	}

	fmt.Println("获取 Token...")
	token := get_token.GetGetToken(cookieData, "")
	if token == "" {
		fmt.Println("获取 Token 失败")
	} else {
		fmt.Printf("Token 获取成功: %s\n", token)
	}

	itemID := "7569637642548104479" // 视频ID写死用于调试
	adet := 10                      // 调试数量设为10

	for i := 0; i < adet; i++ {
		fmt.Printf("\n📤 Gönderim %d/%d\n", i+1, adet)
		sendViewRequest(device, itemID, seed, seedType, token)
		time.Sleep(1 * time.Second)
	}
}
