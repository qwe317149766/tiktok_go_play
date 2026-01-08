package registration

import (
	"bytes"
	"compress/gzip"
	"crypto/hmac"
	"crypto/sha1"
	"encoding/base32"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// GenerateTOTPCode generates a 6-digit TOTP code using the provided base32 secret
func GenerateTOTPCode(secret string) (string, error) {
	secret = strings.ReplaceAll(secret, " ", "")
	key, err := base32.StdEncoding.WithPadding(base32.NoPadding).DecodeString(strings.ToUpper(secret))
	if err != nil {
		key, err = base32.StdEncoding.DecodeString(strings.ToUpper(secret))
		if err != nil {
			return "", fmt.Errorf("failed to decode secret: %v", err)
		}
	}

	counter := time.Now().Unix() / 30
	buf := make([]byte, 8)
	binary.BigEndian.PutUint64(buf, uint64(counter))

	mac := hmac.New(sha1.New, key)
	mac.Write(buf)
	sum := mac.Sum(nil)

	offset := sum[len(sum)-1] & 0xf
	binCode := binary.BigEndian.Uint32(sum[offset:offset+4]) & 0x7fffffff

	code := binCode % 1000000
	return fmt.Sprintf("%06d", code), nil
}

type InstagramRequestConfig struct {
	DeviceID        string
	AndroidID       string
	FamilyDeviceID  string
	MachineID       string
	Uuid            string
	BloksAppID      string
	Url             string
	Params          map[string]any
	LimitOne        bool
	HeaderOverrides map[string]string
	FormOverrides   map[string]string
	HeaderManager   *ThreadHeaderManager
	HTTPClient      *http.Client
	WorkerID        int
}

type InstagramApi struct {
	Client        *http.Client
	HeaderManager *ThreadHeaderManager
	Semaphore     chan struct{}

	AndroidID      string
	Uuid           string
	FamilyDeviceID string
	MachineID      string
	DsUserID       string
	AuthHeader     string
	UserAgent      string
	FbidV2         string
	Rur            string
	WwwClaim       string

	CommonHeaders map[string]string
}

func NewInstagramApi(client *http.Client, hm *ThreadHeaderManager) *InstagramApi {
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	if hm == nil {
		hm = NewInstagramHeaderManager()
	}
	return &InstagramApi{
		Client:        client,
		HeaderManager: hm,
		CommonHeaders: make(map[string]string),
	}
}

func (api *InstagramApi) InitSession(cookiesStr string) error {
	parts := strings.Split(cookiesStr, "|")
	if len(parts) < 3 {
		return fmt.Errorf("invalid cookie string format")
	}

	if len(parts) > 1 {
		api.UserAgent = parts[1]
	}
	if api.UserAgent == "" {
		api.UserAgent = "Instagram 410.1.0.63.71 Android (29/10; 440dpi; 1080x2029; Xiaomi; MI 8; dipper; qcom; zh_TW; 846519343)"
	}

	api.AndroidID = "android-" + GenerateRandomString(16)
	if len(parts) >= 3 {
		idPart := parts[2]
		ids := strings.Split(idPart, ";")
		if len(ids) > 0 && strings.HasPrefix(ids[0], "android-") {
			api.AndroidID = ids[0]
		}
		if len(ids) > 1 {
			api.Uuid = ids[1]
		}
		if len(ids) > 2 {
			api.FamilyDeviceID = ids[2]
		}
	}
	if api.Uuid == "" {
		api.Uuid = GenerateRandomString(36)
	}

	cookieMap := make(map[string]string)
	if len(parts) >= 4 {
		cookiePart := parts[3]
		rawItems := strings.Split(cookiePart, ";")
		for _, item := range rawItems {
			item = strings.TrimSpace(item)
			if item == "" {
				continue
			}
			if idx := strings.Index(item, "="); idx != -1 {
				k := strings.TrimSpace(item[:idx])
				v := strings.TrimSpace(item[idx+1:])
				cookieMap[k] = v
			}
		}
	}

	api.MachineID = cookieMap["X-MID"]
	api.DsUserID = cookieMap["IG-U-DS-USER-ID"]
	api.AuthHeader = cookieMap["Authorization"]
	api.FbidV2 = cookieMap["fbid_v2"]
	api.Rur = cookieMap["ig-u-rur"]
	api.WwwClaim = cookieMap["x-ig-www-claim"]

	api.initCommonHeaders()
	return nil
}

func (api *InstagramApi) initCommonHeaders() {
	lang := "en_US"
	api.CommonHeaders = map[string]string{
		"host":                                          "i.instagram.com",
		"accept-language":                               lang + ", en-GB",
		"authorization":                                 api.AuthHeader,
		"priority":                                      "u=3",
		"x-bloks-is-layout-rtl":                         "false",
		"x-bloks-prism-button-version":                  "INDIGO_PRIMARY_BORDERED_SECONDARY",
		"x-bloks-prism-colors-enabled":                  "true",
		"x-bloks-prism-extended-palette-gray":           "false",
		"x-bloks-prism-extended-palette-indigo":         "true",
		"x-bloks-prism-extended-palette-polish-enabled": "false",
		"x-bloks-prism-extended-palette-red":            "true",
		"x-bloks-prism-extended-palette-rest-of-colors": "true",
		"x-bloks-prism-font-enabled":                    "true",
		"x-bloks-prism-indigo-link-version":             "1",
		"x-bloks-version-id":                            "b7737193b91c3a2f4050bdfc9d9ae0f578a93b4181fd43efe549daacba5c7db9",
		"x-fb-client-ip":                                "True",
		"x-fb-connection-type":                          "WIFI",
		"x-fb-network-properties":                       "Wifi",
		"x-fb-server-cluster":                           "True",
		"x-ig-android-id":                               api.AndroidID,
		"x-ig-app-id":                                   "567067343352427",
		"x-ig-app-locale":                               lang,
		"x-ig-device-id":                                api.Uuid,
		"x-ig-device-languages":                         "{\"system_languages\":\"en_US\"}",
		"x-ig-device-locale":                            lang,
		"x-ig-family-device-id":                         api.FamilyDeviceID,
		"x-ig-is-foldable":                              "false",
		"x-ig-mapped-locale":                            lang,
		"x-ig-timezone-offset":                          "14400",
		"x-ig-www-claim":                                api.WwwClaim,
		"x-mid":                                         api.MachineID,
		"user-agent":                                    api.UserAgent,
		"x-fb-http-engine":                              "Tigon/MNS/TCP",
		"x-fb-rmd":                                      "state=URL_ELIGIBLE",
		"ig-u-ds-user-id":                               api.DsUserID,
		"ig-u-fbid":                                     api.FbidV2,
		"ig-u-rur":                                      api.Rur,
	}
}

func (api *InstagramApi) ExecuteInstagramAPI(cfg InstagramRequestConfig) (string, http.Header, error) {
	hm := cfg.HeaderManager
	if hm == nil {
		hm = api.HeaderManager
	}

	if cfg.Uuid != "" {
		hm.DeviceID = cfg.Uuid
	}
	if cfg.DeviceID != "" {
		hm.AndroidID = cfg.DeviceID
	}

	paramsJson, _ := json.Marshal(cfg.Params)

	bkClientContext := fmt.Sprintf(`{"bloks_version":%q,"styles_id":"instagram"}`, hm.BloksVersionID)

	payloadParts := []string{
		fmt.Sprintf("params=%s", url.QueryEscape(string(paramsJson))),
		fmt.Sprintf("_uuid=%s", url.QueryEscape(cfg.Uuid)),
		fmt.Sprintf("bk_client_context=%s", url.QueryEscape(bkClientContext)),
		fmt.Sprintf("bloks_versioning_id=%s", url.QueryEscape(hm.BloksVersionID)),
	}

	for k, v := range cfg.FormOverrides {
		payloadParts = append(payloadParts, fmt.Sprintf("%s=%s", url.QueryEscape(k), url.QueryEscape(v)))
	}

	payloadStr := strings.Join(payloadParts, "&")

	var payload io.Reader
	if hm.Headers["content-encoding"] == "gzip" {
		var buf bytes.Buffer
		gz := gzip.NewWriter(&buf)
		gz.Write([]byte(payloadStr))
		gz.Close()
		payload = bytes.NewReader(buf.Bytes())
	} else {
		payload = strings.NewReader(payloadStr)
	}

	req, err := http.NewRequest("POST", cfg.Url, payload)
	if err != nil {
		return "", nil, err
	}

	hm.Apply(req)

	for k, v := range api.CommonHeaders {
		req.Header.Set(k, v)
	}

	req.Header.Set("x-pigeon-rawclienttime", fmt.Sprintf("%.3f", float64(time.Now().UnixNano())/1e9))

	for k, v := range cfg.HeaderOverrides {
		req.Header.Set(k, v)
	}

	if req.Header.Get("Host") == "" {
		req.Header.Set("Host", "i.instagram.com")
	}

	if api.Semaphore != nil {
		api.Semaphore <- struct{}{}
		defer func() { <-api.Semaphore }()
	}

	client := cfg.HTTPClient
	if client == nil {
		client = api.Client
	}

	res, err := client.Do(req)
	if err != nil {
		return "", nil, err
	}
	defer res.Body.Close()

	var reader io.ReadCloser
	var errBody error
	switch res.Header.Get("Content-Encoding") {
	case "gzip":
		reader, errBody = gzip.NewReader(res.Body)
		if errBody != nil {
			return "", nil, errBody
		}
		defer reader.Close()
	default:
		reader = res.Body
	}

	body, err := io.ReadAll(reader)
	if err != nil {
		return "", nil, err
	}

	return string(body), res.Header, nil
}

func (api *InstagramApi) TwoFactor() (string, error) {
	if api.AuthHeader == "" {
		return "", fmt.Errorf("session not initialized: missing Authorization header")
	}
	if api.HeaderManager == nil {
		api.HeaderManager = NewInstagramHeaderManager()
	}

	if api.FbidV2 == "" {
		if api.DsUserID == "" {
			return "", fmt.Errorf("SESSION_INCOMPLETE_MISSING_USER_ID")
		}
		userInfo, err := api.GetUserInfo(api.DsUserID)
		if err != nil {
			return "", err
		}

		if strings.Contains(userInfo, "Something went wrong") {
			return "", fmt.Errorf("GetUserInfo: Something went wrong")
		}

		var resp struct {
			User struct {
				Pk     json.Number `json:"pk"`
				FbidV2 json.Number `json:"fbid_v2"`
			} `json:"user"`
		}
		if err := json.Unmarshal([]byte(userInfo), &resp); err != nil {
			return "", fmt.Errorf("GET_USER_INFO_FORMAT_ERROR")
		}

		fbidStr := resp.User.FbidV2.String()
		if fbidStr == "" || fbidStr == "0" {
			return "", fmt.Errorf("GET_USER_INFO_FBID_MISSED")
		}

		api.FbidV2 = fbidStr
		api.initCommonHeaders()
	}

	urlStr := "https://i.instagram.com/api/v1/bloks/async_action/com.bloks.www.fx.settings.security.two_factor.totp.generate_key/"

	clientInputParams := map[string]any{
		"family_device_id": api.FamilyDeviceID,
		"device_id":        api.AndroidID,
		"machine_id":       api.MachineID,
	}

	qplMarkerId := 36700000 + rand.Intn(100000)
	qplInstanceId := float64(100000000000000 + rand.Int63n(900000000000000))

	serverParams := map[string]any{
		"requested_screen_component_type":   nil,
		"account_type":                      1,
		"machine_id":                        nil,
		"INTERNAL__latency_qpl_marker_id":   qplMarkerId,
		"INTERNAL__latency_qpl_instance_id": qplInstanceId,
		"account_id":                        api.FbidV2,
	}

	params := map[string]any{
		"client_input_params": clientInputParams,
		"server_params":       serverParams,
	}

	headers := map[string]string{
		"ig-intended-user-id":  api.DsUserID,
		"ig-u-ds-user-id":      api.DsUserID,
		"priority":             "u=3",
		"x-fb-friendly-name":   "IgApi: bloks/async_action/com.bloks.www.fx.settings.security.two_factor.totp.generate_key/",
		"x-ig-client-endpoint": "IgCdsScreenNavigationLoggerModule:com.bloks.www.fx.settings.security.two_factor.select_method",
		"x-ig-nav-chain":       "SelfFragment:self_profile:3:main_profile:1767770615.35:::1767770615.35,ProfileMediaTabFragment:self_profile:4:button:1767770616.541:::1767770616.541,SettingsScreenFragment:main_settings_screen:5:button:1767770621.773:::1767770629.338,com.bloks.www.fxcal.settings.FXAccountsCenterHomeScreenQuery:com.bloks.www.fxcal.settings.FXAccountsCenterHomeScreenQuery:7:button:1767770634.682:::1767770634.682,IgCdsScreenNavigationLoggerModule:com.bloks.www.fxcal.settings.navigation:8:button:1767770636.894:::1767770636.894,IgCdsScreenNavigationLoggerModule:com.bloks.www.fxcal.settings.identity_selection:9:button:1767770640.359:::1767770640.359,IgCdsScreenNavigationLoggerModule:com.bloks.www.fx.settings.security.two_factor.select_method:10:button:1767770641.523:::1767770641.523,IgCdsScreenNavigationLoggerModule:com.bloks.www.fxcal.settings.identity_selection:11:button:1767770659.341:::1767770659.341,IgCdsScreenNavigationLoggerModule:com.bloks.www.fx.settings.security.two_factor.select_method:12:button:1767770660.205:::1767770660.205",
		"x-ig-salt-ids":        "241963416,332016044,332020615",
		"x-meta-usdid":         "b72d30b2-629c-4039-8d60-7f284be7ec20.1767774211.MEUCIA-20dLIvTmGeNvdFrVU3CeVwJ1AMm6ORDBrXHuNamfeAiEA7l3Hks0s-GV3hKuMA92OxGxq44JAqnBgGEkYf3Qc1rk",
	}

	config := InstagramRequestConfig{
		DeviceID:        api.AndroidID,
		Uuid:            api.Uuid,
		Url:             urlStr,
		Params:          params,
		HeaderOverrides: headers,
		HeaderManager:   api.HeaderManager,
	}

	body, _, err := api.ExecuteInstagramAPI(config)
	if err != nil {
		return "", err
	}

	extractedJson, extractErr := ExtractTwoFactorParams(body)
	if extractErr != nil {
		return body, nil
	}

	return extractedJson, nil
}

func (api *InstagramApi) TwoFactorTotpEnable(verificationCode string, accountID int64) (string, error) {
	if api.AuthHeader == "" {
		return "", fmt.Errorf("session not initialized: missing Authorization header")
	}

	urlStr := "https://i.instagram.com/api/v1/bloks/async_action/com.bloks.www.fx.settings.security.two_factor.totp.enable/"

	clientInputParams := map[string]any{
		"family_device_id":  api.FamilyDeviceID,
		"device_id":         api.AndroidID,
		"machine_id":        api.MachineID,
		"verification_code": verificationCode,
	}

	qplMarkerId := 36700000 + rand.Intn(100000)
	qplInstanceId := float64(100000000000000 + rand.Int63n(900000000000000))

	serverParams := map[string]any{
		"account_type":                      1,
		"INTERNAL__latency_qpl_marker_id":   qplMarkerId,
		"INTERNAL__latency_qpl_instance_id": qplInstanceId,
		"account_id":                        accountID,
	}

	params := map[string]any{
		"client_input_params": clientInputParams,
		"server_params":       serverParams,
	}

	timestamp := fmt.Sprintf("%.3f", float64(time.Now().UnixNano())/1e9)
	headers := map[string]string{
		"content-type":         "application/x-www-form-urlencoded; charset=UTF-8",
		"ig-intended-user-id":  api.DsUserID,
		"ig-u-ds-user-id":      api.DsUserID,
		"priority":             "u=3",
		"x-fb-friendly-name":   "IgApi: bloks/async_action/com.bloks.www.fx.settings.security.two_factor.totp.enable/",
		"x-ig-client-endpoint": "IgCdsScreenNavigationLoggerModule:com.bloks.www.fx.settings.security.two_factor.totp.code",
		"x-ig-nav-chain":       fmt.Sprintf("SelfFragment:self_profile:2:main_profile:%s:::%s", timestamp, timestamp),
		"x-ig-salt-ids":        "241963416,332016044,332020615",
		"x-meta-usdid":         "b72d30b2-629c-4039-8d60-7f284be7ec20.1767774211.MEUCIA-20dLIvTmGeNvdFrVU3CeVwJ1AMm6ORDBrXHuNamfeAiEA7l3Hks0s-GV3hKuMA92OxGxq44JAqnBgGEkYf3Qc1rk",
	}

	config := InstagramRequestConfig{
		DeviceID:        api.AndroidID,
		Uuid:            api.Uuid,
		Url:             urlStr,
		Params:          params,
		HeaderOverrides: headers,
		HeaderManager:   api.HeaderManager,
	}

	body, _, err := api.ExecuteInstagramAPI(config)
	return body, err
}

func (api *InstagramApi) TwoFactorTotpCompletion(accountID int64) (string, error) {
	if api.AuthHeader == "" {
		return "", fmt.Errorf("session not initialized: missing Authorization header")
	}

	urlStr := "https://i.instagram.com/api/v1/bloks/apps/com.bloks.www.fx.settings.security.two_factor.totp.completion/"

	clientInputParams := map[string]any{
		"machine_id": api.MachineID,
	}

	serverParams := map[string]any{
		"account_id":               accountID,
		"INTERNAL_INFRA_screen_id": "w84hx9:17",
	}

	params := map[string]any{
		"client_input_params": clientInputParams,
		"server_params":       serverParams,
	}

	headers := map[string]string{
		"content-type":         "application/x-www-form-urlencoded; charset=UTF-8",
		"ig-intended-user-id":  api.DsUserID,
		"ig-u-ds-user-id":      api.DsUserID,
		"priority":             "u=3",
		"x-fb-friendly-name":   "IgApi: bloks/apps/com.bloks.www.fx.settings.security.two_factor.totp.completion/",
		"x-ig-client-endpoint": "IgCdsScreenNavigationLoggerModule:com.bloks.www.fx.settings.security.two_factor.totp.completion",
		"x-ig-nav-chain":       "SelfFragment:self_profile:2:main_profile:176782033.223:::176782033.223,ProfileMediaTabFragment:self_profile:3:button:176782037.383:::176782037.383,SettingsScreenFragment:main_settings_screen:4:button:176782085.661:::176782399.832,TRUNCATEDx1,IgCdsScreenNavigationLoggerModule:com.bloks.www.fx.settings.security.two_factor.totp.key:26:button:176782562.622:::176782562.622,IgCdsScreenNavigationLoggerModule:com.bloks.www.fx.settings.security.two_factor.select_method:27:button:176782569.684:::176782569.684,IgCdsScreenNavigationLoggerModule:com.bloks.www.fx.settings.security.two_factor.totp.key:28:button:176782572.152:::176782572.152,IgCdsScreenNavigationLoggerModule:com.bloks.www.fx.settings.security.two_factor.totp.code:29:button:176782758.142:::176782758.142,IgCdsScreenNavigationLoggerModule:com.bloks.www.fx.settings.security.two_factor.totp.key:30:button:176782763.875:::176782763.875,IgCdsScreenNavigationLoggerModule:com.bloks.www.fx.settings.security.two_factor.totp.code:31:button:176782774.662:::176782774.662,IgCdsScreenNavigationLoggerModule:com.bloks.www.fx.settings.security.two_factor.totp.completion:32:button:176782784.220:::176782784.220",
		"x-ig-salt-ids":        "241963416,332016044,332020615",
		"x-meta-usdid":         "b72d30b2-629c-4039-8d60-7f284be7ec20.1767774211.MEUCIA-20dLIvTmGeNvdFrVU3CeVwJ1AMm6ORDBrXHuNamfeAiEA7l3Hks0s-GV3hKuMA92OxGxq44JAqnBgGEkYf3Qc1rk",
	}

	config := InstagramRequestConfig{
		DeviceID:        api.AndroidID,
		Uuid:            api.Uuid,
		Url:             urlStr,
		Params:          params,
		HeaderOverrides: headers,
		HeaderManager:   api.HeaderManager,
	}

	body, _, err := api.ExecuteInstagramAPI(config)
	return body, err
}

func (api *InstagramApi) Automate2FA() (string, error) {
	resp, err := api.TwoFactor()
	if err != nil {
		return "", fmt.Errorf("TwoFactor failed: %v", err)
	}

	var tfResp struct {
		KeyText string `json:"key_text"`
	}
	if err := json.Unmarshal([]byte(resp), &tfResp); err != nil {
		return "", fmt.Errorf("JSON parse failed: %v", err)
	}
	if tfResp.KeyText == "" {
		return "", fmt.Errorf("key_text is empty")
	}

	code, _ := GenerateTOTPCode(tfResp.KeyText)
	accID, _ := strconv.ParseInt(api.FbidV2, 10, 64)
	enResp, err := api.TwoFactorTotpEnable(code, accID)
	if err != nil {
		return "", fmt.Errorf("TotpEnable failed: %v", err)
	}
	if !strings.Contains(enResp, `"status":"ok"`) {
		return "", fmt.Errorf("TotpEnable status not ok: %s", enResp)
	}

	cpResp, err := api.TwoFactorTotpCompletion(accID)
	if err != nil {
		return "", fmt.Errorf("TotpCompletion failed: %v", err)
	}
	if !strings.Contains(cpResp, `"status":"ok"`) {
		return "", fmt.Errorf("TotpCompletion status not ok: %s", cpResp)
	}

	return strings.ReplaceAll(tfResp.KeyText, " ", ""), nil
}

func GenerateRandomString(n int) string {
	return fmt.Sprintf("%d", time.Now().UnixNano())
}

func ExtractTwoFactorParams(content string) (string, error) {
	content = strings.ReplaceAll(content, "\\", "")
	blockRe := regexp.MustCompile(`\(f4i\s*\(dkc\s*"account_id"([\s\S]*?)\)\)`)
	match := blockRe.FindStringSubmatch(content)

	if len(match) == 0 {
		return "", fmt.Errorf("no matching parameter block found")
	}
	innerParams := match[0]

	strRe := regexp.MustCompile(`"([^"]*)"`)
	matches := strRe.FindAllStringSubmatch(innerParams, -1)

	var params []string
	for _, m := range matches {
		if len(m) > 1 {
			params = append(params, m[1])
		}
	}

	result := make(map[string]string)
	if len(params) >= 9 {
		result[params[1]] = params[5]
		result[params[2]] = params[6]
		result[params[3]] = params[7]
		result[params[4]] = params[8]

		jsonBytes, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return "", err
		}
		return string(jsonBytes), nil
	}

	return "", fmt.Errorf("insufficient parameters extracted (found %d)", len(params))
}

func (api *InstagramApi) GetUserInfo(targetUserID string) (string, error) {
	if api.AuthHeader == "" {
		return "", fmt.Errorf("session not initialized: missing Authorization header")
	}

	urlStr := fmt.Sprintf("https://i.instagram.com/api/v1/users/%s/info/", targetUserID)

	req, err := http.NewRequest("GET", urlStr, nil)
	if err != nil {
		return "", err
	}

	for k, v := range api.CommonHeaders {
		req.Header.Set(k, v)
	}

	req.Header.Add("ig-intended-user-id", targetUserID)
	req.Header.Add("ig-u-rur", "HIL,79955719929,1799319604:01fe080ff2ea7265883c39c6b1f44f24a90905178a4898194f19e40b68e77d7c6d702c9e")
	req.Header.Add("x-fb-friendly-name", fmt.Sprintf("IgApi: users/%s/info/", targetUserID))
	req.Header.Add("x-fb-request-analytics-tags", `{"network_tags":{"product":"567067343352427","surface":"undefined","request_category":"api","purpose":"fetch","retry_attempt":"0"}}`)

	req.Header.Add("x-ig-bandwidth-speed-kbps", "502.000")
	req.Header.Add("x-ig-bandwidth-totalbytes-b", "0")
	req.Header.Add("x-ig-bandwidth-totaltime-ms", "0")
	req.Header.Add("x-ig-client-endpoint", "MainFeedFragment:feed_timeline")
	req.Header.Add("x-ig-capabilities", "3brTv10=")
	req.Header.Add("x-ig-connection-type", "WIFI")

	timestamp := fmt.Sprintf("%.3f", float64(time.Now().UnixNano())/1e9)
	req.Header.Add("x-ig-nav-chain", fmt.Sprintf("MainFeedFragment:feed_timeline:1:cold_start:%s:::%s", timestamp, timestamp))

	req.Header.Add("x-pigeon-rawclienttime", timestamp)
	req.Header.Add("x-pigeon-session-id", "UFS-"+GenerateRandomString(12))
	req.Header.Add("x-tigon-is-retry", "False")
	req.Header.Add("x-fb-conn-uuid-client", GenerateRandomString(32))

	resp, err := api.Client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	var reader io.ReadCloser
	switch resp.Header.Get("Content-Encoding") {
	case "gzip":
		reader, err = gzip.NewReader(resp.Body)
		if err != nil {
			return "", err
		}
		defer reader.Close()
	default:
		reader = resp.Body
	}

	body, err := io.ReadAll(reader)
	if err != nil {
		return "", err
	}

	return string(body), nil
}
