package registration

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/google/uuid"

	crand "crypto/rand"
	"encoding/base64"
)

// RegConfig defines configuration for registration worker
type RegConfig struct {
	PollTimeoutSec        int
	SMSWaitTimeoutSec     int
	EnableHeaderRotation  bool
	EnableAnomalousUA     bool
	EnableAuto2FA         bool
	FinalizeRetries       int
	EnableIOS             bool
	HTTPRequestTimeoutSec int
}

// ExecuteMobileConfig executes the sessionless mobileconfig request
func ExecuteMobileConfig(ctx context.Context, client *http.Client, hm *ThreadHeaderManager, deviceID, sessionID string, workerID int) (string, http.Header, error) {
	apiUrl := "https://b.i.instagram.com/api/v1/launcher/mobileconfig/"

	hm.Headers["host"] = "b.i.instagram.com"
	hm.FriendlyName = "IgApi: launcher/mobileconfig/sessionless"

	data := map[string]any{
		"bool_opt_policy":         "0",
		"mobileconfigsessionless": "",
		"api_version":             "10",
		"unit_type":               "1",
		"use_case":                "STANDARD",
		"query_hash":              "00206ccbc326cc23a5645dd4e586f1eaa7be8b123fe5508ae6f384caac8406dd",
		"tier":                    "-1",
		"device_id":               deviceID,
		"fetch_mode":              "CONFIG_SYNC_ONLY",
		"fetch_type":              "SYNC_FULL",
		"family_device_id":        "EMPTY_FAMILY_DEVICE_ID",
		"unauthenticated_id":      deviceID,
		"capabilities":            "3brTv10=",
	}

	jsonData, _ := json.Marshal(data)
	payloadStr := "signed_body=SIGNATURE." + string(jsonData)

	req, err := http.NewRequestWithContext(ctx, "POST", apiUrl, strings.NewReader(payloadStr))
	if err != nil {
		return "", nil, err
	}

	hm.Apply(req)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=UTF-8")
	req.Header.Set("x-ig-app-id", "3419628305025917")

	if client == nil {
		return "", nil, fmt.Errorf("http client is nil")
	}
	res, err := client.Do(req)
	if err != nil {
		return "", nil, err
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return "", nil, err
	}

	return string(body), res.Header, nil
}

// ExecuteCreateAndroidKeystore executes the create_android_keystore request
func ExecuteCreateAndroidKeystore(ctx context.Context, client *http.Client, hm *ThreadHeaderManager, deviceID string, workerID int) (string, http.Header, error) {
	apiUrl := "https://i.instagram.com/api/v1/attestation/create_android_keystore/"

	hm.Headers["host"] = "i.instagram.com"
	hm.FriendlyName = "IgApi: attestation/create_android_keystore/"

	formData := url.Values{}
	formData.Set("app_scoped_device_id", deviceID)
	formData.Set("key_hash", "")
	payloadStr := formData.Encode()

	req, err := http.NewRequestWithContext(ctx, "POST", apiUrl, strings.NewReader(payloadStr))
	if err != nil {
		return "", nil, err
	}

	hm.Apply(req)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=UTF-8")
	req.Header.Set("x-ig-app-id", "3419628305025917")

	if client == nil {
		return "", nil, fmt.Errorf("http client is nil")
	}
	res, err := client.Do(req)
	if err != nil {
		return "", nil, err
	}
	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return "", nil, err
	}

	return string(body), res.Header, nil
}

func JSONStringify(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

func (e *RegistrationEngine) ProcessRegistration(ctx context.Context, client *http.Client, phoneNum string, workerID int, proxyURL string, smsMgr *SMSManager, pm *ProxyManager, config RegConfig) (bool, string) {
	log := func(msg string) {
		if e.Logger != nil {
			// Retrieve current stored count securely
			count := smsMgr.GetSuccessCount(phoneNum)
			// Format: [Count: N] msg
			formattedMsg := fmt.Sprintf("[%d] %s", count, msg)
			e.Logger(workerID, phoneNum, "REG", formattedMsg)
		}
	}

	log("Fetching GeoIP...")
	var geoInfo *GeoInfo

	// Safety check
	if config.PollTimeoutSec <= 0 {
		config.PollTimeoutSec = 300
	}
	log(fmt.Sprintf("DEBUG: PollTimeoutSec=%d", config.PollTimeoutSec))

	// Global deadline for the whole process
	deadline := time.Now().Add(time.Duration(config.PollTimeoutSec) * time.Second)

	for {
		select {
		case <-ctx.Done():
			return false, "STOPPED"
		default:
		}

		if time.Now().After(deadline) {
			return false, "GEO_TIMEOUT"
		}
		// ...
		info, err := GetIPAndTimezone(ctx, proxyURL, func(s string) {
			// e.Logger(workerID, phoneNum, "GEO", s) // too verbose
		})
		if err == nil {
			geoInfo = info
			log(fmt.Sprintf("Geo: %s (%s)", info.IP, info.Timezone))
			break
		}
		log("Geo failed, rotating...")
		proxyURL = e.RotateClientProxy(client, pm, workerID)
		if err := Sleep(ctx, 2*time.Second); err != nil {
			return false, "STOPPED"
		}
	}

	for attempt := 1; attempt <= 3; attempt++ {
		if attempt > 1 {
			log(fmt.Sprintf("Full Reset (Attempt %d/3)...", attempt))
			proxyURL = e.RotateClientProxy(client, pm, workerID)
		}

		hm := NewThreadHeaderManager()
		hm.RandomizeWithConfig(config.EnableIOS, config.EnableAnomalousUA)

		DeviceID := uuid.New().String()
		AndroidID := GenerateRandomAndroidID()
		FamilyDeviceID := uuid.New().String()
		WaterfallID := uuid.New().String()

		// Step 0: Mobile Config
		log("Step 0: MobileConfig")
		var mid string
		// var pubKey string
		// var keyIdInt int
		mcSuccess := false

		for {
			if time.Now().After(deadline) {
				return false, "MC_TIMEOUT"
			}
			sessionID_mc := "UFS-" + uuid.New().String() + "-0"
			hm.SetIDs(DeviceID, AndroidID, FamilyDeviceID, WaterfallID, sessionID_mc)

			_, mcHeaders, err := ExecuteMobileConfig(ctx, client, hm, DeviceID, sessionID_mc, workerID)
			if err != nil {
				proxyURL = e.RotateClientProxy(client, pm, workerID)
				if err := Sleep(ctx, 2*time.Second); err != nil {
					continue // loop will catch timeout/ctx or break
				}
				continue
			}

			mid = mcHeaders.Get("Ig-Set-X-Mid")
			// pubKey = mcHeaders.Get("ig-set-password-encryption-pub-key")
			// keyIdStr := mcHeaders.Get("ig-set-password-encryption-key-id")

			// if pubKey == "" {
			// 	pubKey = mcHeaders.Get("Ig-Set-X-Pub-Key")
			// 	keyIdStr = mcHeaders.Get("Ig-Set-X-Key-Id")
			// }
			// keyIdInt, _ = strconv.Atoi(keyIdStr)

			if mid != "" {
				mcSuccess = true
				break
			}
			proxyURL = e.RotateClientProxy(client, pm, workerID)
		}
		if !mcSuccess {
			continue
		}

		// Step 0.5: Keystore
		log("Step 0.5: Keystore")
		var challengeNonce string
		ksResp, _, err := ExecuteCreateAndroidKeystore(ctx, client, hm, AndroidID, workerID)
		if err == nil {
			var ksMap map[string]any
			if json.Unmarshal([]byte(ksResp), &ksMap) == nil {
				if nonce, ok := ksMap["challenge_nonce"].(string); ok {
					challengeNonce = nonce
				}
			}
		}

		// Step 1
		log("Starting Step 1...")
		step1Config := ThreadsRequestConfig{
			DeviceID: DeviceID, AndroidID: AndroidID, FamilyDeviceID: FamilyDeviceID,
			WaterfallID: WaterfallID, BloksAppID: "com.bloks.www.bloks.caa.reg.start.async",
			FriendlyName: "IGBloksAppRootQuery-com.bloks.www.bloks.caa.reg.start.async",
			ProxyURL:     proxyURL, HeaderManager: hm, HTTPClient: client, PhoneNumber: phoneNum, WorkerID: workerID,
		}
		jsonParams := map[string]any{
			"family_device_id": step1Config.FamilyDeviceID, "qe_device_id": step1Config.DeviceID,
			"device_id": step1Config.AndroidID, "waterfall_id": step1Config.WaterfallID,
			"reg_flow_source": "threads_ig_account_creation", "skip_welcome": false, "is_from_spc": false,
		}
		step1Config.InnerParams = JSONStringify(jsonParams)

		paramsStep1, err := e.PollUntilParamSuccess(ctx, "com.bloks.www.bloks.caa.reg.async.contactpoint_phone.async", step1Config, 1, 2, 5, pm, config.PollTimeoutSec)
		if err != nil {
			if ctx.Err() != nil {
				return false, "STOPPED"
			}
			log(fmt.Sprintf("Step 1 failed: %v", err))
			continue
		}

		// Step 2
		log("Starting Step 2...")
		jsonParamsStep1, _ := json.Marshal(paramsStep1)
		jsonParamsStep2 := map[string]any{
			"params": JSONStringify(map[string]any{
				"client_input_params": JSONStringify(map[string]any{
					"aac": "", "device_id": step1Config.AndroidID, "was_headers_prefill_available": 0,
					"login_upsell_phone_list": []any{}, "whatsapp_installed_on_client": 0, "zero_balance_state": "",
					"network_bssid": nil, "msg_previous_cp": "", "switch_cp_first_time_loading": 1,
					"accounts_list": []any{}, "confirmed_cp_and_code": map[string]any{}, "country_code": "",
					"family_device_id": step1Config.FamilyDeviceID, "block_store_machine_id": "", "fb_ig_device_id": []any{},
					"phone": phoneNum, "lois_settings": map[string]any{"lois_token": ""},
					"cloud_trust_token": nil, "was_headers_prefill_used": 0, "headers_infra_flow_id": "",
					"build_type": "release", "encrypted_msisdn": "", "switch_cp_have_seen_suma": 0,
				}),
				"server_params": string(jsonParamsStep1),
			}),
		}
		step1Config.BloksAppID = "com.bloks.www.bloks.caa.reg.async.contactpoint_phone.async"
		step1Config.FriendlyName = "IGBloksAppRootQuery-com.bloks.www.bloks.caa.reg.async.contactpoint_phone.async"
		step1Config.InnerParams = JSONStringify(jsonParamsStep2)

		paramsStep2, err := e.PollUntilParamSuccess(ctx, "com.bloks.www.bloks.caa.reg.confirmation.async", step1Config, 1, 2, 0, pm, config.PollTimeoutSec)
		if err != nil {
			if ctx.Err() != nil {
				return false, "STOPPED"
			}
			return false, "STEP2_FAILED"
		}

		// Step 3: SMS
		log("Starting Step 3...")
		smsTimeout := time.Duration(config.SMSWaitTimeoutSec) * time.Second
		code, rawMsg, err := smsMgr.PollCode(ctx, phoneNum, smsTimeout, workerID, log)
		if err != nil {
			if ctx.Err() != nil {
				return false, "STOPPED"
			}
			log(fmt.Sprintf("SMS Error: %v", err))
			return false, "SMS_TIMEOUT"
		}
		log(fmt.Sprintf("SMS Received: %s (%s)", code, rawMsg))

		jsonParamsStep3 := map[string]any{
			"params": JSONStringify(map[string]any{
				"client_input_params": JSONStringify(map[string]any{
					"confirmed_cp_and_code": map[string]any{}, "aac": "", "block_store_machine_id": "",
					"code": code, "fb_ig_device_id": []any{}, "device_id": step1Config.AndroidID,
					"lois_settings": map[string]any{"lois_token": ""}, "cloud_trust_token": nil, "network_bssid": nil,
				}),
				"server_params": JSONStringify(paramsStep2),
			}),
		}
		step1Config.BloksAppID = "com.bloks.www.bloks.caa.reg.confirmation.async"
		step1Config.FriendlyName = "IGBloksAppRootQuery-com.bloks.www.bloks.caa.reg.confirmation.async"
		step1Config.InnerParams = JSONStringify(jsonParamsStep3)
		paramsStep3, err := e.PollUntilParamSuccess(ctx, "com.bloks.www.bloks.caa.reg.password.async", step1Config, 1, 2, 0, pm, config.PollTimeoutSec)
		if err != nil {
			if ctx.Err() != nil {
				return false, "STOPPED"
			}
			return false, "STEP3_FAILED"
		}

		// Step 4: Password
		log("Starting Step 4...")
		passwordLen := rand.Intn(5) + 8
		password := GenerateRandomPassword(passwordLen)
		timestamp := time.Now().Unix()

		tokenBytes := make([]byte, 24)
		crand.Read(tokenBytes)
		tokenData := fmt.Sprintf("%s|%d|", phoneNum, timestamp)
		safetynet_token := base64.StdEncoding.EncodeToString(append([]byte(tokenData), tokenBytes...))
		safetynet_response := "API_ERROR:+class+com.google.android.gms.common.api.ApiException:7:+"

		// passwordEnd, _, _ := EncryptPassword4NodeCompatible(pubKey, keyIdInt, password)
		// Match main.go: Force use of PWD_INSTAGRAM:0 format
		passwordEnd := fmt.Sprintf("#PWD_INSTAGRAM:0:%d:%s", timestamp, password)

		jsonParamsPassword := map[string]any{
			"params": JSONStringify(map[string]any{
				"client_input_params": JSONStringify(map[string]any{
					"safetynet_response":                    safetynet_response,
					"caa_play_integrity_attestation_result": "", "aac": "", "safetynet_token": safetynet_token,
					"whatsapp_installed_on_client": 0, "zero_balance_state": "", "network_bssid": nil,
					"machine_id": mid, "headers_last_infra_flow_id_safetynet": "", "email_oauth_token_map": map[string]any{},
					"block_store_machine_id": "", "fb_ig_device_id": []any{}, "encrypted_msisdn_for_safetynet": "",
					"lois_settings": map[string]any{"lois_token": ""}, "cloud_trust_token": nil,
					"client_known_key_hash": "", "encrypted_password": passwordEnd,
				}),
				"server_params": JSONStringify(paramsStep3),
			}),
		}
		step1Config.BloksAppID = "com.bloks.www.bloks.caa.reg.password.async"
		step1Config.FriendlyName = "IGBloksAppRootQuery-com.bloks.www.bloks.caa.reg.password.async"
		step1Config.InnerParams = JSONStringify(jsonParamsPassword)
		paramsStep4, err := e.PollUntilParamSuccess(ctx, "com.bloks.www.bloks.caa.reg.birthday.async", step1Config, 1, 2, 0, pm, config.PollTimeoutSec)
		if err != nil {
			if ctx.Err() != nil {
				return false, "STOPPED"
			}
			return false, "STEP4_FAILED"
		}

		// Step 5: Birthday
		log("Starting Step 5...")
		day, month, year := GenerateRandomBirthday(18, 40)
		birthdayStr := fmt.Sprintf("%02d-%02d-%d", day, month, year)
		birthDayTimestamp, err := GetTimestampByBirthdayAndTZ(birthdayStr, geoInfo.Timezone)
		if err != nil {
			birthDayTimestamp = time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.Local).Unix()
		}

		jsonParamsStep5 := map[string]any{
			"params": JSONStringify(map[string]any{
				"client_input_params": JSONStringify(map[string]any{
					"client_timezone": geoInfo.Timezone, "aac": "", "birthday_or_current_date_string": birthdayStr,
					"os_age_range": "", "birthday_timestamp": birthDayTimestamp,
					"lois_settings": map[string]any{"lois_token": ""}, "zero_balance_state": "", "network_bssid": nil,
					"should_skip_youth_tos": 0, "is_youth_regulation_flow_complete": 0,
				}),
				"server_params": JSONStringify(paramsStep4),
			}),
		}
		step1Config.BloksAppID = "com.bloks.www.bloks.caa.reg.birthday.async"
		step1Config.FriendlyName = "IGBloksAppRootQuery-com.bloks.www.bloks.caa.reg.birthday.async"
		step1Config.InnerParams = JSONStringify(jsonParamsStep5)
		paramsStep5, err := e.PollUntilParamSuccess(ctx, "com.bloks.www.bloks.caa.reg.name_ig_and_soap.async", step1Config, 1, 2, 0, pm, config.PollTimeoutSec)
		if err != nil {
			if ctx.Err() != nil {
				return false, "STOPPED"
			}
			return false, "STEP5_FAILED"
		}

		// Step 6: Name & Username
		log("Starting Step 6...")
		jsonParamsStep6 := map[string]any{
			"params": JSONStringify(map[string]any{
				"client_input_params": JSONStringify(map[string]any{
					"accounts_list": []any{}, "aac": "", "lois_settings": map[string]any{"lois_token": ""},
					"zero_balance_state": "", "network_bssid": nil, "name": "",
				}),
				"server_params": JSONStringify(paramsStep5),
			}),
		}
		step1Config.BloksAppID = "com.bloks.www.bloks.caa.reg.name_ig_and_soap.async"
		step1Config.FriendlyName = "IGBloksAppRootQuery-com.bloks.www.bloks.caa.reg.name_ig_and_soap.async"
		step1Config.InnerParams = JSONStringify(jsonParamsStep6)
		paramsStep6, err := e.PollUntilParamSuccess(ctx, "com.bloks.www.bloks.caa.reg.username.async", step1Config, 1, 2, 0, pm, config.PollTimeoutSec)
		if err != nil {
			if ctx.Err() != nil {
				return false, "STOPPED"
			}
			return false, "STEP6_FAILED"
		}

		regInfo := paramsStep6["reg_info"]
		var regInfoMap map[string]any
		json.Unmarshal([]byte(regInfo), &regInfoMap)
		usernameArg, _ := regInfoMap["username_prefill"].(string)

		// Step 7 confirmation
		jsonParamsStep7 := map[string]any{
			"params": JSONStringify(map[string]any{
				"client_input_params": JSONStringify(map[string]any{
					"validation_text": usernameArg, "aac": "", "family_device_id": step1Config.FamilyDeviceID,
					"device_id": step1Config.AndroidID, "lois_settings": map[string]any{"lois_token": ""},
					"zero_balance_state": "", "network_bssid": nil, "qe_device_id": step1Config.DeviceID,
				}),
				"server_params": JSONStringify(paramsStep6),
			}),
		}
		step1Config.BloksAppID = "com.bloks.www.bloks.caa.reg.username.async"
		step1Config.FriendlyName = "IGBloksAppRootQuery-com.bloks.www.bloks.caa.reg.username.async"
		step1Config.InnerParams = JSONStringify(jsonParamsStep7)
		paramsStep7, err := e.PollUntilParamSuccess(ctx, "com.bloks.www.bloks.caa.reg.create.account.async", step1Config, 1, 2, 0, pm, config.PollTimeoutSec)
		if err != nil {
			if ctx.Err() != nil {
				return false, "STOPPED"
			}
			return false, "STEP7_FAILED"
		}

		// Finalize
		log("Starting Step 7...")
		jsonParamsStep8 := map[string]any{
			"params": JSONStringify(map[string]any{
				"client_input_params": JSONStringify(map[string]any{
					"ck_error": "", "aac": "", "device_id": step1Config.AndroidID, "waterfall_id": step1Config.WaterfallID,
					"zero_balance_state": "", "network_bssid": nil, "failed_birthday_year_count": "",
					"headers_last_infra_flow_id": "", "ig_partially_created_account_nonce_expiry": 0, "machine_id": mid,
					"should_ignore_existing_login": 0, "reached_from_tos_screen": 1, "ig_partially_created_account_nonce": "",
					"ck_nonce": "", "force_sessionless_nux_experience": 0, "lois_settings": map[string]any{"lois_token": ""},
					"ig_partially_created_account_user_id": 0, "ck_id": "", "no_contact_perm_email_oauth_token": "", "encrypted_msisdn": "",
				}),
				"server_params": JSONStringify(paramsStep7),
			}),
		}
		step1Config.BloksAppID = "com.bloks.www.bloks.caa.reg.create.account.async"
		step1Config.FriendlyName = "IGBloksAppRootQuery-com.bloks.www.bloks.caa.reg.create.account.async"
		step1Config.InnerParams = JSONStringify(jsonParamsStep8)
		if challengeNonce != "" {
			step1Config.HeaderOverrides = map[string]string{
				"x-ig-attest-params": fmt.Sprintf(`{"attestation":[{"version":2,"type":"keystore","errors":[-1014],"challenge_nonce":"%s","signed_nonce":"","key_hash":""}]}`, challengeNonce),
			}
		}

		retries := config.FinalizeRetries
		if retries <= 0 {
			retries = 5
		}

		for i := 1; i <= retries; i++ {
			// Removed deadline check for finalization to rely on retries
			log(fmt.Sprintf("Finalizing %d...", i))
			paramsStep8, err := e.PollUntilParamSuccess(ctx, "", step1Config, 1, 2, 1, pm, 36000)
			if err != nil {
				if ctx.Err() != nil {
					return false, "STOPPED"
				}
				if config.EnableHeaderRotation {
					hm.RandomizeWithConfig(config.EnableIOS, config.EnableAnomalousUA)
				}
				continue
			}

			if paramsStep8 != nil {
				body := paramsStep8["full_response"]
				finalUsername, finalToken, finalPKID, finalSessionID, _, fbidV2 := ExtractTokenAndUsername(body)

				if strings.Contains(finalUsername, "Instagram User") {
					return false, "INSTAGRAM_USER"
				}
				if strings.Contains(body, "Please try again") || finalUsername == "" || finalToken == "" {
					proxyURL = e.RotateClientProxy(client, pm, workerID)
					if config.EnableHeaderRotation {
						hm.RandomizeWithConfig(config.EnableIOS, config.EnableAnomalousUA)
					}
					// Refresh GeoIP for logs if needed (optional, skipping for perf)
					if err := Sleep(ctx, 3*time.Second); err != nil {
						continue
					}
					continue
				}

				// Success
				ua := hm.Headers["user-agent"]
				ids := fmt.Sprintf("%s;%s;%s;%s", AndroidID, DeviceID, FamilyDeviceID, WaterfallID)
				authString := fmt.Sprintf("X-MID=%s;sessionid=%s;IG-U-DS-USER-ID=%s;Authorization=%s;fbid_v2=%s;", mid, finalSessionID, finalPKID, finalToken, fbidV2)

				resultLine := fmt.Sprintf("%s:%s|%s|%s|%s;done:%d|||", finalUsername, password, ua, ids, authString, i)

				// 2FA
				has2FA := false
				twoFactorLine := ""
				if config.EnableAuto2FA {
					log("Enabling 2FA...")
					hm_ig := NewInstagramHeaderManager()
					api_ig := NewInstagramApi(client, hm_ig)
					if err := api_ig.InitSession(resultLine); err == nil {
						if secret, err := api_ig.Automate2FA(); err == nil {
							// User requested specific format: 账号----密码----2fa
							twoFactorLine = fmt.Sprintf("%s----%s----%s", finalUsername, password, secret)
							has2FA = true
						} else {
							log(fmt.Sprintf("2FA Failed: %v", err))
						}
					}
				}

				// Using logger hack to pass success string back
				if has2FA {
					// Return BOTH lines separated by "@@@@" so app.go can split them
					// Format: SUCCESS_2FA:CookiesLine@@@@2FALine
					return true, "SUCCESS_2FA:" + resultLine + "@@@@" + twoFactorLine
				}
				return true, "SUCCESS:" + resultLine
			}
		}
	}

	return false, "ATTEMPTS_EXHAUSTED"
}
