package registration

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

// NewInstagramHeaderManager creates a new manager with Instagram-specific defaults
func NewInstagramHeaderManager() *ThreadHeaderManager {
	rand.Seed(time.Now().UnixNano())
	m := &ThreadHeaderManager{
		Headers:        make(map[string]string),
		BloksVersionID: "b7737193b91c3a2f4050bdfc9d9ae0f578a93b4181fd43efe549daacba5c7db9",
		AppID:          "567067343352427",
		FriendlyName:   "IgApi: bloks/async_action/com.bloks.www.fx.settings.security.two_factor.totp.generate_key/",
		ClientDocID:    "",
		BloksAppID:     "com.bloks.www.fx.settings.security.two_factor.totp.generate_key",
	}
	lang := "zh_TW"
	// Initial default headers for Instagram
	m.Headers["host"] = "i.instagram.com"
	m.Headers["x-ig-capabilities"] = "3brTv10="
	m.Headers["x-ig-validate-null-in-legacy-dict"] = "true"
	m.Headers["x-ig-timezone-offset"] = "14400"
	m.Headers["x-tigon-is-retry"] = "False"
	m.Headers["accept-language"] = "zh-TW, en-US"
	m.Headers["x-ig-is-foldable"] = "false"
	m.Headers["priority"] = "u=3"
	m.Headers["content-type"] = "application/x-www-form-urlencoded; charset=UTF-8"
	m.Headers["x-fb-http-engine"] = "Tigon/MNS/TCP"
	m.Headers["x-fb-client-ip"] = "True"
	m.Headers["x-fb-connection-type"] = "WIFI"
	m.Headers["x-fb-network-properties"] = "Wifi;VPN;Validated;"
	m.Headers["x-fb-server-cluster"] = "True"
	m.Headers["accept-encoding"] = "gzip, deflate, br"

	m.Headers["x-ig-connection-type"] = "WIFI"

	m.Headers["x-ig-bandwidth-speed-kbps"] = "909.000"
	m.Headers["x-ig-bandwidth-totalbytes-b"] = "7670967"
	m.Headers["x-ig-bandwidth-totaltime-ms"] = "52654"

	m.Headers["x-ig-device-locale"] = lang
	m.Headers["x-ig-app-locale"] = lang
	m.Headers["x-ig-mapped-locale"] = lang
	m.Headers["x-ig-device-languages"] = "{\"system_languages\":\"zh-TW\"}"

	return m
}

func (m *ThreadHeaderManager) SetIDs(deviceID, androidID, familyDeviceID, waterfallID string, pigeonSessionID ...string) {
	m.DeviceID = deviceID
	m.AndroidID = androidID
	m.FamilyDeviceID = familyDeviceID
	m.WaterfallID = waterfallID
	if len(pigeonSessionID) > 0 && pigeonSessionID[0] != "" {
		m.PigeonSessionID = pigeonSessionID[0]
	}
}

func (m *ThreadHeaderManager) SetHeader(key, value string) {
	m.Headers[key] = value
}

func (m *ThreadHeaderManager) DeleteHeader(key string) {
	delete(m.Headers, key)
}

type DeviceConfig struct {
	Manufacturer string
	Model        string
	Device       string
	Hardware     string
}

type IOSDeviceConfig struct {
	Model      string
	Resolution string
	Scale      string
}

func (m *ThreadHeaderManager) RandomizeWithConfig(enableIOS, enableAnomalous bool) {
	if enableAnomalous {
		m.RandomizeAnomalousUserAgent()
	} else if enableIOS {
		if rand.Intn(2) == 0 {
			m.RandomizeIOSUserAgent()
		} else {
			m.RandomizeUserAgent()
		}
	} else {
		m.RandomizeUserAgent()
	}
}

func (m *ThreadHeaderManager) RandomizeAnomalousUserAgent() {
	ver := "411.0.0.0.75"
	osVersions := []string{"29/10", "30/11", "31/12", "32/12", "33/13", "34/14"}
	osVer := osVersions[rand.Intn(len(osVersions))]
	dpis := []string{"320dpi", "400dpi", "440dpi", "480dpi", "560dpi", "600dpi", "640dpi"}
	dpi := dpis[rand.Intn(len(dpis))]
	locale := "en-US"

	manufacturers := []string{"Samsung", "Google", "OnePlus", "Xiaomi", "Oppo"}
	manufacturer := manufacturers[rand.Intn(len(manufacturers))]

	randomString := func(n int) string {
		const letters = "abcdefghijklmnopqrstuvwxyz"
		b := make([]byte, n)
		for i := range b {
			b[i] = letters[rand.Intn(len(letters))]
		}
		return string(b)
	}

	model := randomString(6)
	device := randomString(6)
	hardware := randomString(6)

	resolutions := []string{"1080x2029", "1080x2248", "1080x1920", "1080x2160", "1440x2560", "1440x3120", "1080x2400"}
	res := resolutions[rand.Intn(len(resolutions))]
	buildNum := 846252388

	ua := fmt.Sprintf("Barcelona %s Android (%s; %s; %s; %s; %s; %s; %s; %s; %d)",
		ver, osVer, dpi, res, manufacturer, model, device, hardware, locale, buildNum)

	m.Headers["user-agent"] = ua
}

func (m *ThreadHeaderManager) RandomizeUserAgent() {
	ver := "411.0.0.0.75"
	osVersions := []string{"29/10", "30/11", "31/12", "32/12", "33/13", "34/14"}
	osVer := osVersions[rand.Intn(len(osVersions))]
	dpis := []string{"320dpi", "400dpi", "440dpi", "480dpi", "560dpi", "600dpi", "640dpi"}
	dpi := dpis[rand.Intn(len(dpis))]
	locale := "en-US"

	devices := []DeviceConfig{
		{"Google", "Pixel 8 Pro", "husky", "husky"},
		{"Google", "Pixel 8", "shiba", "shiba"},
		{"Google", "Pixel 7 Pro", "cheetah", "cheetah"},
		{"Google", "Pixel 7", "panther", "panther"},
		{"Google", "Pixel 6 Pro", "raven", "raven"},
		{"Google", "Pixel 6", "oriole", "oriole"},
		{"Google", "Pixel 6a", "bluejay", "bluejay"},
		{"Google", "Pixel 5", "redfin", "lito"},
		{"Samsung", "SM-S928B", "eureka", "s5e9945"},
		{"Samsung", "SM-S921B", "lyq", "s5e9945"},
		{"Samsung", "SM-S918B", "dm3q", "kalama"},
		{"Samsung", "SM-S911B", "dm1q", "kalama"},
		{"Samsung", "SM-S908B", "gts8u", "s5e9925"},
		{"Samsung", "SM-S901B", "s22", "s5e9925"},
		{"Samsung", "SM-G998B", "p3s", "exynos2100"},
		{"Samsung", "SM-G991B", "o1s", "exynos2100"},
		{"Samsung", "SM-N986B", "c2s", "exynos990"},
	}

	dev := devices[rand.Intn(len(devices))]
	resolutions := []string{"1080x2029", "1080x2248", "1080x1920", "1080x2160", "1440x2560", "1440x3120", "1080x2400"}
	res := resolutions[rand.Intn(len(resolutions))]
	buildNum := 846252388

	ua := fmt.Sprintf("Barcelona %s Android (%s; %s; %s; %s; %s; %s; %s; %s; %d)",
		ver, osVer, dpi, res, dev.Manufacturer, dev.Model, dev.Device, dev.Hardware, locale, buildNum)

	m.Headers["user-agent"] = ua
}

func (m *ThreadHeaderManager) RandomizeIOSUserAgent() {
	ver := "411.0.0.0.75"
	osVersions := []string{"15_6", "16_0", "16_5", "16_6", "17_0", "17_1"}
	osVer := osVersions[rand.Intn(len(osVersions))]

	devices := []IOSDeviceConfig{
		{"iPhone10,1", "750x1334", "2.00"},
		{"iPhone10,2", "1242x2208", "3.00"},
		{"iPhone10,3", "1125x2436", "3.00"},
		{"iPhone11,2", "1125x2436", "3.00"},
		{"iPhone11,4", "1242x2688", "3.00"},
		{"iPhone11,8", "828x1792", "2.00"},
		{"iPhone12,1", "828x1792", "2.00"},
		{"iPhone12,3", "1125x2436", "3.00"},
		{"iPhone13,2", "1170x2532", "3.00"},
		{"iPhone14,5", "1170x2532", "3.00"},
		{"iPhone15,3", "1290x2796", "3.00"},
	}

	dev := devices[rand.Intn(len(devices))]
	locale := "en_US"
	lang := "en"
	buildNum := 627270390 + rand.Intn(10000)

	ua := fmt.Sprintf("Barcelona %s (%s; iOS %s; %s; %s; scale=%s; %s; %d) AppleWebKit/420+",
		ver, dev.Model, osVer, locale, lang, dev.Scale, dev.Resolution, buildNum)

	m.Headers["user-agent"] = ua
}

func (m *ThreadHeaderManager) Apply(req *http.Request) {
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

	m.Headers["x-fb-request-analytics-tags"] = fmt.Sprintf(`{"network_tags":{"product":"%s","request_category":"graphql","purpose":"fetch","retry_attempt":"0"}}`, m.AppID)

	for k, v := range m.Headers {
		req.Header.Set(k, v)
	}
}
