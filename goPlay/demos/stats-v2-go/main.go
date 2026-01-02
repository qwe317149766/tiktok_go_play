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
	"net/url"
	"strings"
	"time"

	"tt_code/headers"
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
	params.Set("device_platform", "android")
	params.Set("os", "android")
	params.Set("ssmix", "a")
	params.Set("_rticket", fmt.Sprintf("%d", rticket))
	params.Set("cdid", device["cdid"].(string))
	params.Set("channel", "googleplay")
	params.Set("aid", "1233")
	params.Set("app_name", "musical_ly")
	params.Set("version_code", "350003")
	params.Set("version_name", "35.0.3")
	params.Set("resolution", resolution)
	params.Set("dpi", fmt.Sprintf("%d", dpi))
	params.Set("device_type", model)
	params.Set("device_brand", brand)
	params.Set("language", "tr")
	params.Set("os_api", "34")
	params.Set("os_version", "14")
	params.Set("ac", "wifi")
	params.Set("is_pad", "1")
	params.Set("current_region", "TR")
	params.Set("app_type", "normal")
	params.Set("sys_region", "TR")
	params.Set("is_foldable", "1")
	params.Set("timezone_name", "Asia/Istanbul")
	params.Set("timezone_offset", "10800")
	params.Set("build_number", "35.0.3")
	params.Set("host_abi", "arm64-v8a")
	params.Set("region", "TR")
	params.Set("ts", fmt.Sprintf("%d", ts))
	params.Set("iid", device["install_id"].(string))
	params.Set("device_id", device["device_id"].(string))
	params.Set("openudid", device["openudid"].(string))
	params.Set("req_id", device["req_id"].(string))

	payload := map[string]interface{}{
		"header": map[string]interface{}{
			"device_model":        model,
			"device_brand":        brand,
			"device_manufacturer": brand,
			"os":                  "Android",
			"os_version":          "14",
			"os_api":              34,
			"resolution":          fmt.Sprintf("%sx%s", resY, resX),
			"density_dpi":         dpi,
			"display_density":     density,
			"display_density_v2":  density,
			"resolution_v2":       fmt.Sprintf("%sx%s", resY, resX),
			"openudid":            device["openudid"],
			"cdid":                device["cdid"],
			"install_id":          device["install_id"],
			"device_id":           device["device_id"],
			"google_aid":          device["gaid"],
			"package":             "com.zhiliaoapp.musically",
			"app_version":         "35.0.3",
			"version_code":        350003,
			"update_version_code": 2023500030,
			"app_name":            "musical_ly",
			"sdk_version":         "2.5.0",
			"sdk_version_code":    2050090,
			"sdk_target_version":  30,
		},
		"magic_tag": "ss_app_log",
		"_gen_time": rticket,
	}

	qs := params.Encode()
	payloadBytes, _ := json.Marshal(payload)
	postDataHex := hex.EncodeToString(payloadBytes)

	// 使用 goPlay/headers 进行签名
	// MakeHeaders(deviceID string, createTime int64, signCount int, reportCount int, settingCount int, appLaunchTime int64, secDeviceToken string, phoneInfo string, seed string, seedEncodeType int, seedEndcodeHex string, algorithmData1 string, hex32 string, queryString string, postData string, appVersion string, sdkVersionStr string, sdkVersion int, callType int, appVersionConstant int)
	// 这里模拟 Python 的 sign 函数
	h := headers.MakeHeaders(
		device["device_id"].(string),
		ts,
		1,  // signCount
		0,  // reportCount
		0,  // settingCount
		ts, // appLaunchTime
		"", // secDeviceToken
		model,
		"", // seed
		0,  // seedEncodeType
		"", // seedEndcodeHex
		"", // algorithmData1
		"", // hex32
		qs,
		postDataHex,
		"35.0.3",
		"v02.05.00-ov-android", // 模拟版本
		0x02050000,
		738,
		0,
	)

	reqUrl := fmt.Sprintf("https://log22-normal-alisg.tiktokv.com/service/2/device_register/?%s", qs)
	req, _ := http.NewRequest("POST", reqUrl, bytes.NewBuffer(payloadBytes))

	req.Header.Set("User-Agent", fmt.Sprintf("com.zhiliaoapp.musically/2023500030 (Linux; Android 14; %s)", model))
	req.Header.Set("Content-Type", "application/json; charset=UTF-8")
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

func sendViewRequest(device map[string]interface{}, itemID string) {
	payloadStr, ts := getDynamicPayload(itemID)

	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	zw.Write([]byte(payloadStr))
	zw.Close()
	payloadGzip := buf.Bytes()

	rticket := ts * 1000

	baseUrl := "https://api31-core-alisg.tiktokv.com/aweme/v1/aweme/stats/"
	commonParams := url.Values{}
	commonParams.Set("ab_version", "35.0.3")
	commonParams.Set("ac", "wifi")
	commonParams.Set("ac2", "wifi")
	commonParams.Set("aid", "1233")
	commonParams.Set("app_language", "tr")
	commonParams.Set("app_name", "musical_ly")
	commonParams.Set("app_type", "normal")
	commonParams.Set("build_number", "35.0.3")
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
	commonParams.Set("manifest_version_code", "2023500030")
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
	commonParams.Set("update_version_code", "2023500030")
	commonParams.Set("version_code", "350003")
	commonParams.Set("version_name", "35.0.3")

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
		0,
		0,
		ts,
		"",
		device["device_type"].(string),
		"",
		0,
		"",
		"",
		"",
		fullParams,
		postDataHex,
		"35.0.3",
		"v35.0.3-ov-android",
		350003,
		738,
		0,
	)

	reqUrl := fmt.Sprintf("%s?%s", baseUrl, urlParams.Encode())
	req, _ := http.NewRequest("POST", reqUrl, bytes.NewBuffer(payloadGzip))

	req.Header.Set("User-Agent", fmt.Sprintf("com.zhiliaoapp.musically/2023500030 (Linux; Android 14; tr_TR; %s; Build/UP1A.231005.007)", device["device_type"]))
	req.Header.Set("Connection", "keep-alive")
	req.Header.Set("Accept", "*/*")
	req.Header.Set("Accept-Encoding", "gzip, deflate, br")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=UTF-8")
	req.Header.Set("x-bd-content-encoding", "gzip")
	req.Header.Set("x-common-params-v2", commonParams.Encode())
	req.Header.Set("x-gorgon", h.XGorgon)
	req.Header.Set("x-khronos", h.XKhronos)
	req.Header.Set("x-ladon", h.XLadon)
	req.Header.Set("x-argus", h.XArgus)

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

	fmt.Print("İzlenme Gönderilecek Video ID'si (video id): ")
	var itemID string
	fmt.Scanln(&itemID)

	fmt.Print("Kaç İzlenme Gönderilsin: ")
	var adet int
	if _, err := fmt.Scanln(&adet); err != nil {
		adet = 1
	}

	for i := 0; i < adet; i++ {
		fmt.Printf("\n📤 Gönderim %d/%d\n", i+1, adet)
		sendViewRequest(device, itemID)
		time.Sleep(1 * time.Second)
	}
}
