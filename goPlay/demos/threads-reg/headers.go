package main

import (
	"fmt"
	"math/rand"
	"net/http"
	"time"
)

// ThreadHeaderManager encapsulates request header parameters and logic
type ThreadHeaderManager struct {
	Headers         map[string]string
	DeviceID        string
	AndroidID       string
	FamilyDeviceID  string
	WaterfallID     string
	BloksVersionID  string
	AppID           string
	FriendlyName    string
	ClientDocID     string
	BloksAppID      string
	PigeonSessionID string
}

// NewThreadHeaderManager creates a new manager with default fixed headers and IDs
func NewThreadHeaderManager() *ThreadHeaderManager {
	rand.Seed(time.Now().UnixNano())
	m := &ThreadHeaderManager{
		Headers:        make(map[string]string),
		BloksVersionID: "1dee137e9f4ca666db6d55081dd85d7316972f7baa521ae48c240d5669c4d686",
		AppID:          "3419628305025917",
		FriendlyName:   "IGBloksAppRootQuery-com.bloks.www.bloks.caa.reg.start.async",
		ClientDocID:    "356548512614739681018024088968",
		BloksAppID:     "com.bloks.www.bloks.caa.reg.start.async",
	}
	lang := "en-US"
	// Initial default headers
	m.Headers["host"] = "i.instagram.com"
	m.Headers["x-ig-capabilities"] = "3brTv10="
	m.Headers["x-graphql-client-library"] = "pando"
	m.Headers["x-ig-validate-null-in-legacy-dict"] = "true"
	m.Headers["x-ig-timezone-offset"] = "14400"
	m.Headers["x-graphql-request-purpose"] = "fetch"
	m.Headers["x-tigon-is-retry"] = "False"
	m.Headers["x-root-field-name"] = "bloks_action"
	m.Headers["accept-language"] = lang + ", en-GB"
	m.Headers["x-ig-is-foldable"] = "false"
	m.Headers["priority"] = "u=3, i"
	m.Headers["content-type"] = "application/x-www-form-urlencoded"
	m.Headers["x-fb-http-engine"] = "Tigon/Liger"
	m.Headers["x-fb-client-ip"] = "True"
	m.Headers["x-fb-server-cluster"] = "True"
	m.Headers["accept-encoding"] = "gzip"
	m.Headers["content-encoding"] = "gzip"

	// New Android simulation headers
	m.Headers["x-ig-connection-type"] = "WIFI"
	m.Headers["x-ig-connection-speed"] = fmt.Sprintf("%dkbps", 1000+rand.Intn(4000))
	m.Headers["x-ig-bandwidth-totalbytes-b"] = fmt.Sprintf("%d", rand.Intn(10000))
	m.Headers["x-ig-bandwidth-totaltime-ms"] = fmt.Sprintf("%d", rand.Intn(1000))
	m.Headers["x-ig-prefetch-request"] = "1"
	m.Headers["x-ig-device-locale"] = lang
	m.Headers["x-ig-app-locale"] = lang
	m.Headers["x-ig-mapped-locale"] = lang
	m.Headers["x-pigeon-rawclienttime"] = fmt.Sprintf("%.3f", float64(time.Now().UnixMilli())/1000.0)
	m.Headers["x-fb-device-group"] = "3614"

	return m
}

// SetIDs 一键设置核心设备相关的 ID
func (m *ThreadHeaderManager) SetIDs(deviceID, androidID, familyDeviceID, waterfallID string, pigeonSessionID ...string) {
	m.DeviceID = deviceID
	m.AndroidID = androidID
	m.FamilyDeviceID = familyDeviceID
	m.WaterfallID = waterfallID
	if len(pigeonSessionID) > 0 && pigeonSessionID[0] != "" {
		m.PigeonSessionID = pigeonSessionID[0]
	}
}

// SetHeader 允许手动设置或覆盖 Header
func (m *ThreadHeaderManager) SetHeader(key, value string) {
	m.Headers[key] = value
}

// DeleteHeader 允许移除 Header
func (m *ThreadHeaderManager) DeleteHeader(key string) {
	delete(m.Headers, key)
}

// DeviceConfig holds details for a specific device model
type DeviceConfig struct {
	Manufacturer string
	Model        string
	Device       string
	Hardware     string
}

// IOSDeviceConfig holds details for a specific iOS device
type IOSDeviceConfig struct {
	Model      string
	Resolution string
	Scale      string
}

// RandomizeUserAgent generates a random Threads/Barcelona User-Agent using Samsung and Google models
func (m *ThreadHeaderManager) RandomizeUserAgent() {
	rand.Seed(time.Now().UnixNano())

	// Fixed version as requested
	ver := "411.0.0.0.75"

	// Android Version and API level (Recent systems + sample)
	osVersions := []string{"29/10", "30/11", "31/12", "32/12", "33/13", "34/14"}
	osVer := osVersions[rand.Intn(len(osVersions))]

	// DPI randomization
	dpis := []string{"320dpi", "400dpi", "440dpi", "480dpi", "560dpi", "600dpi", "640dpi"}
	dpi := dpis[rand.Intn(len(dpis))]

	// Language (Align with accept-language zh_TW)
	locale := "en-US"

	// Specific Device Models for Google and Samsung
	devices := []DeviceConfig{
		// Google Pixel Models
		{"Google", "Pixel 8 Pro", "husky", "husky"},
		{"Google", "Pixel 8", "shiba", "shiba"},
		{"Google", "Pixel 7 Pro", "cheetah", "cheetah"},
		{"Google", "Pixel 7", "panther", "panther"},
		{"Google", "Pixel 6 Pro", "raven", "raven"},
		{"Google", "Pixel 6", "oriole", "oriole"},
		{"Google", "Pixel 6a", "bluejay", "bluejay"},
		{"Google", "Pixel 5", "redfin", "lito"},
		// Samsung Galaxy Models
		{"Samsung", "SM-S928B", "eureka", "s5e9945"}, // S24 Ultra
		{"Samsung", "SM-S921B", "lyq", "s5e9945"},    // S24
		{"Samsung", "SM-S918B", "dm3q", "kalama"},    // S23 Ultra
		{"Samsung", "SM-S911B", "dm1q", "kalama"},    // S23
		{"Samsung", "SM-S908B", "gts8u", "s5e9925"},  // S22 Ultra
		{"Samsung", "SM-S901B", "s22", "s5e9925"},    // S22
		{"Samsung", "SM-G998B", "p3s", "exynos2100"}, // S21 Ultra
		{"Samsung", "SM-G991B", "o1s", "exynos2100"}, // S21
		{"Samsung", "SM-N986B", "c2s", "exynos990"},  // Note 20 Ultra
	}

	dev := devices[rand.Intn(len(devices))]

	// Resolutions randomization
	resolutions := []string{"1080x2029", "1080x2248", "1080x1920", "1080x2160", "1440x2560", "1440x3120", "1080x2400"}
	res := resolutions[rand.Intn(len(resolutions))]

	// Build number randomization
	// buildNum := 341962830 + rand.Intn(1000000)
	buildNum := 846252388

	// User-Agent format: Barcelona [Version] Android ([OS]; [DPI]; [Resolution]; [Manufacturer]; [Model]; [Device]; [Hardware]; [Locale]; [Build Number])
	ua := fmt.Sprintf("Barcelona %s Android (%s; %s; %s; %s; %s; %s; %s; %s; %d)",
		ver, osVer, dpi, res, dev.Manufacturer, dev.Model, dev.Device, dev.Hardware, locale, buildNum)

	m.Headers["user-agent"] = ua
}

// RandomizeIOSUserAgent generates a random Threads/Barcelona User-Agent using iPhone models
func (m *ThreadHeaderManager) RandomizeIOSUserAgent() {
	rand.Seed(time.Now().UnixNano())

	ver := "411.0.0.0.75"

	// iOS versions
	osVersions := []string{"15_6", "16_0", "16_5", "16_6", "17_0", "17_1"}
	osVer := osVersions[rand.Intn(len(osVersions))]

	// iPhone Models
	devices := []IOSDeviceConfig{
		{"iPhone10,1", "750x1334", "2.00"},  // iPhone 8
		{"iPhone10,2", "1242x2208", "3.00"}, // iPhone 8 Plus
		{"iPhone10,3", "1125x2436", "3.00"}, // iPhone X
		{"iPhone11,2", "1125x2436", "3.00"}, // iPhone XS
		{"iPhone11,4", "1242x2688", "3.00"}, // iPhone XS Max
		{"iPhone11,8", "828x1792", "2.00"},  // iPhone XR
		{"iPhone12,1", "828x1792", "2.00"},  // iPhone 11
		{"iPhone12,3", "1125x2436", "3.00"}, // iPhone 11 Pro
		{"iPhone13,2", "1170x2532", "3.00"}, // iPhone 12
		{"iPhone14,5", "1170x2532", "3.00"}, // iPhone 13
		{"iPhone15,3", "1290x2796", "3.00"}, // iPhone 14 Pro Max
	}

	dev := devices[rand.Intn(len(devices))]
	locale := "en_US"
	lang := "en"
	buildNum := 627270390 + rand.Intn(10000)

	// User-Agent format: Barcelona [Version] ([Model]; [OS]; [Locale]; [Lang]; scale=[Scale]; [Resolution]; [Build Number]) AppleWebKit/420+
	ua := fmt.Sprintf("Barcelona %s (%s; iOS %s; %s; %s; scale=%s; %s; %d) AppleWebKit/420+",
		ver, dev.Model, osVer, locale, lang, dev.Scale, dev.Resolution, buildNum)

	m.Headers["user-agent"] = ua
}

// Apply 自动同步内部 ID 到 Header 并注入 Request
func (m *ThreadHeaderManager) Apply(req *http.Request) {
	// 同步内部 ID 到相应的 Header
	m.Headers["x-ig-device-id"] = m.DeviceID
	m.Headers["x-ig-android-id"] = m.AndroidID
	m.Headers["x-ig-family-device-id"] = m.FamilyDeviceID
	m.Headers["x-bloks-version-id"] = m.BloksVersionID
	m.Headers["x-ig-app-id"] = m.AppID
	m.Headers["x-fb-friendly-name"] = m.FriendlyName
	m.Headers["x-client-doc-id"] = m.ClientDocID

	if m.PigeonSessionID != "" {
		m.Headers["x-pigeon-session-id"] = m.PigeonSessionID
	}

	// 生成统计标签
	m.Headers["x-fb-request-analytics-tags"] = fmt.Sprintf(`{"network_tags":{"product":"%s","request_category":"graphql","purpose":"fetch","retry_attempt":"0"}}`, m.AppID)

	for k, v := range m.Headers {
		req.Header.Set(k, v)
	}
}
