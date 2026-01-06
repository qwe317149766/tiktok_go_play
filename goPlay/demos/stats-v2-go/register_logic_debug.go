package main

import (
	"bytes"
	"compress/gzip"
	"crypto/md5"
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

	"github.com/google/uuid"
)

// 常量定义，与 register_debug.go 一致
const (
	AppVersion     = "40.6.3"
	AppVersionCode = 400603
	ManifestCode   = 2024006030
	UpdateCode     = 2024006030
	BuildNumber    = "40.6.3"
	UserAgent      = "com.zhiliaoapp.musically/2024006030 (Linux; U; Android 10; zh_TW; MI 8; Build/QKQ1.190828.002;tt-ok/3.12.13.20)"
	Domain         = "log-boot.tiktokv.com"
	Region         = "TW"
	Language       = "zh-Hant"
	TZName         = "Asia/Yerevan"
	TZOffset       = 14400
)

type Device struct {
	OpenUDID            string
	CDID                string
	ReqID               string
	ClientUDID          string
	GAID                string
	DeviceID            string
	InstallID           string
	Model               string
	Brand               string
	DeviceManufacturer  string
	Resolution          string
	ResolutionV2        string
	DPI                 int
	ROM                 string
	ROMVersion          string
	ReleaseBuild        string
	RamSize             string
	ScreenHeightDP      int
	ScreenWidthDP       int
	ApkLastUpdateTime   int64
	ApkFirstInstallTime int64
	Proxy               string

	// 用于 DSign
	PrivKey               string
	PubKeyB64             string
	DeviceGuardServerData string
}

func randomHex(n int) string {
	b := make([]byte, n/2)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func randomUUID() string {
	return uuid.New().String()
}

func randomDigits(n int) string {
	res := ""
	for i := 0; i < n; i++ {
		res += fmt.Sprintf("%d", rand.Intn(10))
	}
	return res
}

func getSha256(s string) string {
	h := sha256.New()
	h.Write([]byte(s))
	return hex.EncodeToString(h.Sum(nil))
}

func getMd5(s string) string {
	h := md5.New()
	h.Write([]byte(s))
	return hex.EncodeToString(h.Sum(nil))
}

func newTestDevice() *Device {
	return &Device{
		CDID:                randomUUID(),
		OpenUDID:            randomHex(16),
		ClientUDID:          randomUUID(),
		ReqID:               randomUUID(),
		Model:               "MI 8",
		Brand:               "Xiaomi",
		DeviceManufacturer:  "Xiaomi",
		Resolution:          "2029x1080",
		ResolutionV2:        "2248x1080",
		DPI:                 440,
		ROM:                 "MIUI-V12.5.2.0.QEACNXM",
		ROMVersion:          "miui_V125_V12.5.2.0.QEACNXM",
		ReleaseBuild:        "4ca920e_20250626",
		RamSize:             "6GB",
		ScreenHeightDP:      817,
		ScreenWidthDP:       393,
		ApkLastUpdateTime:   time.Now().UnixNano() / 1e6,
		ApkFirstInstallTime: time.Now().UnixNano() / 1e6,
		GAID:                randomUUID(),
	}
}

func buildQS(d *Device, ts int64, rticket int64, extra map[string]string) string {
	// 按照 capture 的顺序构造 Query String
	resQuery := "1080*2029"

	qsParts := []string{
		fmt.Sprintf("req_id=%s", d.ReqID),
		"device_platform=android",
		"os=android",
		"ssmix=a",
		fmt.Sprintf("_rticket=%d", rticket),
		fmt.Sprintf("cdid=%s", d.CDID),
		"channel=googleplay",
		"aid=1233",
		"app_name=musical_ly",
		fmt.Sprintf("version_code=%d", AppVersionCode),
		fmt.Sprintf("version_name=%s", AppVersion),
		fmt.Sprintf("manifest_version_code=%d", ManifestCode),
		fmt.Sprintf("update_version_code=%d", UpdateCode),
		fmt.Sprintf("ab_version=%s", AppVersion),
		fmt.Sprintf("resolution=%s", resQuery),
		fmt.Sprintf("dpi=%d", d.DPI),
		fmt.Sprintf("device_type=%s", strings.ReplaceAll(d.Model, " ", "%20")),
		fmt.Sprintf("device_brand=%s", d.Brand),
		"language=" + Language,
		"os_api=29",
		"os_version=10",
		"ac=wifi",
		"is_pad=0",
		"app_type=normal",
		"sys_region=" + Region,
		fmt.Sprintf("last_install_time=%d", 1767424023), // 抓包中的值
		"timezone_name=Asia%2FYerevan",
		"app_language=" + Language,
		fmt.Sprintf("timezone_offset=%d", TZOffset),
		"host_abi=arm64-v8a",
		"locale=zh-Hant-TW",
		"ac2=unknown",
		"uoo=1",
		"op_region=" + Region,
		fmt.Sprintf("build_number=%s", BuildNumber),
		"region=" + Region,
		fmt.Sprintf("ts=%d", ts),
		fmt.Sprintf("openudid=%s", d.OpenUDID),
	}

	// 处理额外的参数 (iid, device_id 等)
	for k, v := range extra {
		if v == "" {
			qsParts = append(qsParts, k)
		} else {
			qsParts = append(qsParts, fmt.Sprintf("%s=%s", k, v))
		}
	}

	return strings.Join(qsParts, "&")
}

func makeDidIid(d *Device) error {
	ts := int64(1767424033)
	rticket := int64(1767424033997)

	qs := buildQS(d, ts, rticket, map[string]string{
		"okhttp_version":          "4.2.228.18-tiktok",
		"use_store_region_cookie": "1",
	})
	fmt.Printf("[DEBUG] QS: %s\n", qs)

	payload := map[string]interface{}{
		"header": map[string]interface{}{
			"os":                  "Android",
			"os_version":          "10",
			"os_api":              29,
			"device_model":        d.Model,
			"device_brand":        "Xiaomi", // Capture 中为 Xiaomi
			"device_manufacturer": "Xiaomi",
			"cpu_abi":             "arm64-v8a",
			"density_dpi":         440,
			"display_density":     "mdpi",
			"resolution":          "2029x1080", // Body 中使用 x
			"display_density_v2":  "xxhdpi",
			"resolution_v2":       d.ResolutionV2,
			"access":              "wifi",
			"rom":                 d.ROM,
			"rom_version":         d.ROMVersion,
			"language":            "zh",
			"timezone":            4,
			"region":              Region,
			"tz_name":             TZName,
			"tz_offset":           TZOffset,
			"clientudid":          d.ClientUDID,
			"openudid":            d.OpenUDID,
			"channel":             "googleplay",
			"not_request_sender":  1,
			"aid":                 1233,
			"release_build":       d.ReleaseBuild,
			"ab_version":          AppVersion,
			"gaid_limited":        0,
			"custom": map[string]interface{}{
				"ram_size":                d.RamSize,
				"dark_mode_setting_value": 1,
				"is_foldable":             0,
				"screen_height_dp":        817,
				"apk_last_update_time":    d.ApkLastUpdateTime,
				"filter_warn":             0,
				"priority_region":         Region,
				"user_period":             0,
				"is_kids_mode":            0,
				"web_ua":                  fmt.Sprintf("Dalvik/2.1.0 (Linux; U; Android 10; %s Build/QKQ1.190828.002)", d.Model),
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
			"cdid":                   d.CDID,
			"device_platform":        "android",
			"git_hash":               "5151884",
			"sdk_version_code":       2050990,
			"sdk_target_version":     30,
			"req_id":                 d.ReqID,
			"sdk_version":            "2.5.9",
			"guest_mode":             0,
			"sdk_flavor":             "i18nInner",
			"apk_first_install_time": d.ApkFirstInstallTime,
			"is_system_app":          0,
		},
		"magic_tag": "ss_app_log",
		"_gen_time": rticket,
	}

	payloadBytes, _ := json.Marshal(payload)
	postDataHex := hex.EncodeToString(payloadBytes)
	fmt.Printf("[DEBUG] Body Hex: %s\n", postDataHex)

	h := headers.MakeHeaders(
		"", // DeviceID 为空
		ts,
		1, // SignCount
		0,
		0,
		ts,
		"",
		d.Model,
		"",
		0,
		"",
		"",
		"",
		qs,
		postDataHex,
		AppVersion,
		"v05.02.02-ov-android",
		0x05020220,
		738,
		0xC40A800,
	)

	fmt.Printf("[DEBUG] X-Gorgon: %s\n", h.XGorgon)
	fmt.Printf("[DEBUG] X-SS-Stub: %s\n", h.XSSStub)

	reqUrl := fmt.Sprintf("https://%s/service/2/device_register/?%s", Domain, qs)
	req, _ := http.NewRequest("POST", reqUrl, bytes.NewBuffer(payloadBytes))

	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("X-SS-Stub", h.XSSStub)
	req.Header.Set("X-Khronos", fmt.Sprintf("%d", ts))
	req.Header.Set("X-Ladon", h.XLadon)
	req.Header.Set("X-Argus", h.XArgus)
	req.Header.Set("X-Gorgon", h.XGorgon)
	req.Header.Set("x-ss-req-ticket", fmt.Sprintf("%d", rticket))
	req.Header.Set("x-tt-app-init-region", "carrierregion=;mccmnc=;sysregion=TW;appregion=TW")
	req.Header.Set("x-tt-dm-status", "login=0;ct=0;rt=7")

	client := createClient(d)
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, _ := readResponseBody(resp)

	var respJSON map[string]interface{}
	if err := json.Unmarshal(body, &respJSON); err == nil {
		d.DeviceID = fmt.Sprintf("%v", respJSON["device_id_str"])
		d.InstallID = fmt.Sprintf("%v", respJSON["install_id_str"])
		if d.DeviceID == "" || d.DeviceID == "0" || d.DeviceID == "<nil>" {
			if val, ok := respJSON["device_id"]; ok {
				d.DeviceID = fmt.Sprintf("%v", val)
			}
		}
		if d.InstallID == "" || d.InstallID == "0" || d.InstallID == "<nil>" {
			if val, ok := respJSON["install_id"]; ok {
				d.InstallID = fmt.Sprintf("%v", val)
			}
		}
		if d.DeviceID != "0" && d.DeviceID != "" {
			fmt.Printf("✅ MakeDidIid Success: DID=%s, IID=%s\n", d.DeviceID, d.InstallID)
		} else {
			fmt.Printf("⚠️ MakeDidIid returned 0, Response: %s\n", string(body))
		}
		return nil
	}
	return fmt.Errorf("parse response failed")
}

func alertCheck(d *Device) error {
	ts := time.Now().Unix()
	rticket := time.Now().UnixNano() / 1e6

	// 构造 tt_info (base64)
	ttExtra := map[string]string{
		"iid":            d.InstallID,
		"device_id":      d.DeviceID,
		"current_region": Region,
		"residence":      Region,
		"timezone":       "4.0",
		"custom_bt":      fmt.Sprintf("%d", rticket),
	}
	ttInfoQS := buildQS(d, ts, rticket, ttExtra)
	ttInfoB64 := base64.StdEncoding.EncodeToString([]byte(ttInfoQS))

	extra := map[string]string{
		"iid":       d.InstallID,
		"device_id": d.DeviceID,
		"tt_info":   ttInfoB64,
	}
	qs := buildQS(d, ts, rticket, extra)

	h := headers.MakeHeaders(
		d.DeviceID,
		ts,
		rand.Intn(20)+20,
		2,
		4,
		ts-5,
		"",
		d.Model,
		"",
		0,
		"",
		"",
		"",
		qs,
		"", // GET 请求为空
		AppVersion,
		"v05.02.02-ov-android",
		0x05020220,
		738,
		0xC40A800,
	)

	reqUrl := fmt.Sprintf("https://%s/service/2/app_alert_check/?%s", Domain, qs)
	req, _ := http.NewRequest("GET", reqUrl, nil)

	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("X-SS-Stub", h.XSSStub)
	req.Header.Set("X-Khronos", fmt.Sprintf("%d", ts))
	req.Header.Set("X-Ladon", h.XLadon)
	req.Header.Set("X-Argus", h.XArgus)
	req.Header.Set("X-Gorgon", h.XGorgon)
	req.Header.Set("x-ss-req-ticket", fmt.Sprintf("%d", rticket))
	req.Header.Set("x-tt-app-init-region", "carrierregion=;mccmnc=;sysregion=TW;appregion=TW")
	req.Header.Set("x-tt-dm-status", "login=0;ct=0;rt=7")

	client := createClient(d)
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, _ := readResponseBody(resp)
	fmt.Printf("✅ AlertCheck Response: %s\n", string(body))
	return nil
}

func makeDsSign(d *Device) error {
	ts := time.Now().Unix()
	rticket := time.Now().UnixNano() / 1e6

	kp, _ := headers.GenerateDeltaKeypair()
	d.PrivKey = kp.PrivKeyHex
	d.PubKeyB64 = kp.PubKeyB64

	bodyJSON := fmt.Sprintf(`{"device_id":"%s","install_id":"%s","aid":1233,"app_version":"%s","model":"%s","os":"Android","openudid":"%s","google_aid":"%s","properties_version":"android-1.0","device_properties":{"device_model":"%s","device_manufacturer":"%s","disk_size":"ea489ffb302814b62320c02536989a3962de820f5a481eb5bac1086697d9aa3c","memory_size":"291cf975c42a1e788fdc454e3c7330d641db5f9f7ba06e37f7f388b3448bc374","resolution":"%s","re_time":"0af7de3d5239bb5542f0653e57c7c8b9","indss18":"8725063fe010181646c25d1f993e1589","indc15":"7874453cef13dddd56fcb3c7e8e99c28","indn5":"a9ca935c4885bbc1da2be687f153354c","indmc14":"e678d34e71a6943f1cab0bfa3c7a226b","inda0":"d0eac42291b9a88173d9914972a65d8b","indal2":"d7baecabd462bc9f960eaab4c81a55c5","indm10":"446ae4837d88b3b3988d57b9747e11cd","indsp3":"9861cb1513b66e9aaeb66ef048bfdd18","indsd8":"a15ec37e1115dea871970a39ec0769c4","bl":"a3d41c6f3e8c1892d2cc97469805b1f0","cmf":"5494690cb9b316eb618265ea11dc5146","bc":"1e2b66f4392214037884408109a383df","stz":"e6f9d2069f89b53a8e6f2c65929d2e50","sl":"2389ca43e5adab9de01d2dda7633ac39"}}`,
		d.DeviceID, d.InstallID, AppVersion, d.Model, d.OpenUDID, d.GAID,
		getSha256(d.Model), getSha256(d.Brand), getSha256(d.Resolution))

	postDataHex := hex.EncodeToString([]byte(bodyJSON))

	extra := map[string]string{
		"from":       "normal",
		"from_error": "", // 这种特殊参数在 buildQS 中已处理
		"iid":        d.InstallID,
		"device_id":  d.DeviceID,
	}
	qs := buildQS(d, ts, rticket, extra)

	h := headers.MakeHeaders(
		d.DeviceID,
		ts,
		1, // 对 DSign 来说 Count 可能不重要，但我们保持一致
		2,
		4,
		ts-5,
		"",
		d.Model,
		"",
		0,
		"",
		"",
		"",
		qs,
		postDataHex,
		AppVersion,
		"v05.02.02-ov-android",
		0x05020220,
		738,
		0xC40A800,
	)

	// 使用 log22 域名进行 DSign
	dsignDomain := "log22-normal-alisg.tiktokv.com"
	reqUrl := fmt.Sprintf("https://%s/service/2/dsign/?%s", dsignDomain, qs)
	req, _ := http.NewRequest("POST", reqUrl, bytes.NewBuffer([]byte(bodyJSON)))

	// 同步 register_logic.py 的 Header 逻辑
	cookieString := fmt.Sprintf("install_id=%s; store-idc=alisg; store-country-code=tw; store-country-code-src=did", d.InstallID)
	req.Header.Set("Cookie", cookieString)
	req.Header.Set("x-tt-request-tag", "t=0;n=1")
	req.Header.Set("tt-ticket-guard-public-key", d.PubKeyB64)
	req.Header.Set("sdk-version", "2")
	req.Header.Set("x-tt-dm-status", "login=0;ct=0;rt=1")
	req.Header.Set("x-ss-req-ticket", fmt.Sprintf("%d", rticket))
	req.Header.Set("tt-device-guard-iteration-version", "1")
	req.Header.Set("passport-sdk-version", "-1")
	req.Header.Set("x-vc-bdturing-sdk-version", "2.3.17.i18n")
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	req.Header.Set("X-SS-Stub", h.XSSStub)
	req.Header.Set("rpc-persist-pyxis-policy-state-law-is-ca", "1")
	req.Header.Set("rpc-persist-pyxis-policy-v-tnc", "1")
	req.Header.Set("x-tt-ttnet-origin-host", "log22-normal-alisg.tiktokv.com")
	req.Header.Set("x-ss-dp", "1233")
	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("Accept-Encoding", "gzip, deflate")

	// 注意：根据 Python 经验，这里不设置 X-Gorgon, X-Argus 等头
	// req.Header.Set("X-Khronos", fmt.Sprintf("%d", ts))
	// req.Header.Set("X-Ladon", h.XLadon)
	// req.Header.Set("X-Argus", h.XArgus)
	// req.Header.Set("X-Gorgon", h.XGorgon)

	client := createClient(d)
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	body, _ := readResponseBody(resp)
	d.DeviceGuardServerData = resp.Header.Get("tt-device-guard-server-data")

	fmt.Printf("✅ MakeDsSign Success, ServerData Length: %d\n", len(d.DeviceGuardServerData))
	if d.DeviceGuardServerData == "" {
		fmt.Printf("⚠️ Warning: tt-device-guard-server-data not found in headers, Body: %s\n", string(body))
	}
	return nil
}

func createClient(d *Device) *http.Client {
	transport := &http.Transport{}
	if d.Proxy != "" {
		proxyUrl, _ := url.Parse(d.Proxy)
		transport.Proxy = http.ProxyURL(proxyUrl)
	}
	return &http.Client{
		Timeout:   15 * time.Second,
		Transport: transport,
	}
}

func readResponseBody(resp *http.Response) ([]byte, error) {
	var reader io.ReadCloser
	var err error
	switch resp.Header.Get("Content-Encoding") {
	case "gzip":
		reader, err = gzip.NewReader(resp.Body)
		if err != nil {
			return nil, err
		}
		defer reader.Close()
	default:
		reader = resp.Body
	}
	return io.ReadAll(reader)
}

func main() {
	fmt.Println("🚀 Starting Register Logic Flow Debug (TW Region)...")

	d := newTestDevice()
	// d.Proxy = "http://127.0.0.1:7890" // 如果需要代理，取消注释并设置

	// 1. Device Register
	err := makeDidIid(d)
	if err != nil {
		fmt.Printf("❌ MakeDidIid Failed: %v\n", err)
		return
	}

	if d.DeviceID == "" || d.DeviceID == "0" {
		fmt.Println("❌ Failed to get valid Device ID. Stopping flow.")
		return
	}

	// 2. Alert Check
	err = alertCheck(d)
	if err != nil {
		fmt.Printf("❌ AlertCheck Failed: %v\n", err)
		// 某些情况下 alert_check 失败也能继续，依逻辑而定
	}

	// 3. DSign
	err = makeDsSign(d)
	if err != nil {
		fmt.Printf("❌ MakeDsSign Failed: %v\n", err)
	}

	fmt.Println("\n✨ Final Device Info:")
	fmt.Printf("Device ID:   %s\n", d.DeviceID)
	fmt.Printf("Install ID:  %s\n", d.InstallID)
	fmt.Printf("Guard Data:  %s...\n", func() string {
		if len(d.DeviceGuardServerData) > 20 {
			return d.DeviceGuardServerData[:20]
		}
		return d.DeviceGuardServerData
	}())
}
