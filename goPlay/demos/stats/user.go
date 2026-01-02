package main

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"tt_code/headers"
	"tt_code/mssdk/endecode"
	"tt_code/tt_protobuf"
)

func main() {
	// Setup HTTP Client
	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	// Constants extraction
	deviceID := "7550968885031290423"
	installID := "7572811438918960951"
	ua := "com.zhiliaoapp.musically/2024204030 (Linux; U; Android 15; en; Pixel 6; Build/BP1A.250505.005; Cronet/TTNetVersion:efce646d 2025-10-16 QuicVersion:c785494a 2025-09-30)"

	cookieData := map[string]string{
		"device_id":   deviceID,
		"install_id":  installID,
		"ua":          ua,
		"device_type": "Pixel 6",
	}

	// Example: Allow customization via variable or later via args
	customCookies := "multi_sids=7586669408203555853%3A5a655a0686add6853d986f475726ae09; cmpl_token=AgQQAPNSF-RPsLl3rQYIeR008qv3AhnOf7XZYKL-bA; sid_guard=5a655a0686add6853d986f475726ae09%7C1767115724%7C15552000%7CSun%2C+28-Jun-2026+17%3A28%3A44+GMT; uid_tt=c61fc3719b7fef86d6f9ea93cd4b2964a10cd06abff7f0794beaeb3645e0b93a; uid_tt_ss=c61fc3719b7fef86d6f9ea93cd4b2964a10cd06abff7f0794beaeb3645e0b93a; sid_tt=5a655a0686add6853d986f475726ae09; sessionid=5a655a0686add6853d986f475726ae09; sessionid_ss=5a655a0686add6853d986f475726ae09; tt_session_tlb_tag=sttt%7C5%7CWmVaBoat1oU9mG9HVyauCf________-oLyOD2K8dMPRfAcBvCJpbphh1AZyX-xPy0NA2T_vXNsA%3D; store-idc=useast5; store-country-code=us; store-country-code-src=uid; tt-target-idc=useast5; user_oec_info=0a535bde6d6029df466c2899a85479b21e5bd339b9885cc12056f450a3e857a2d80555cb4f04157f96fd54411949d45b92a5724c7fbb74c1c5d001355deabe529d866d652229fa5127e4d990b710990601254f7f8e1a490a3c000000000000000000004fe4e1cf40957e44763820061bec8a4f46b2fc22de16504d046463361337d357800d3232db02d99bf1677df775398950314c10f6cb850e1886d2f6f20d220104a6618b9f; msToken=tH1NDCTZkH1fr05Hi0ONUZYsFlqs0zxA76Kyfi_FXpKZUS02p8QTR_eZgGkazgQ6ZrH6vp3VIcxhqK1vej3gBqZyWz4C--dCTkJADTJ58nCV8IKe6H0kVT7prQ==; store-country-sign=MEIEDFnh6atGmpse-69o3wQgd3CSQgZlvRw3pR_qXFQdwpmtohhQPjQDPz_oFAhWFqkEEEB_l4ibaQmgUyNc3qH2igU; odin_tt=033dd7b267fb2cd33d44eff7f865a8c6679db4d6bd3fb87961889a011a0e1b50871f516ce0c546f6f3638d435852d69c05bcfea38a8c29e97230f482500d7b7a40b445841e4b63d854cb3e83570d055b"
	// Optional: if non-empty, use this token specifically (though GetGetToken also gets one)
	customToken := ""

	GetUserProfile(client, deviceID, installID, ua, cookieData, customCookies, customToken)
}

func GetUserProfile(client *http.Client, deviceID, installID, ua string, cookieData map[string]string, cookies string, xToken string) {
	// Dynamic Data Acquisition
	fmt.Println("Getting Token...")

	// User clarification: x-tt-token is NOT seed. It is the token used in headers.
	// We use 'token' variable to hold this x-tt-token value.
	seedToken := GetGetToken(cookieData, client)
	if seedToken == "" {
		fmt.Println("Failed to get token")
		return
	}
	fmt.Printf("Got Token: %s\n", seedToken)

	fmt.Println("Getting Seed...")
	seed, seedType, err := GetSeed(cookieData, client)
	if err != nil {
		fmt.Printf("Failed to get seed: %v\n", err)
		return
	}
	fmt.Printf("Got Seed: %s, SeedType: %d\n", seed, seedType)

	// URL Construction
	originalURL := "https://aggr16-normal.tiktokv.us/aweme/v1/user/profile/self/?is_after_login=0&scene_id=3&device_platform=android&os=android&ssmix=a&_rticket=1767263078752&channel=samsung_store&aid=1233&app_name=musical_ly&version_code=420403&version_name=42.4.3&manifest_version_code=2024204030&update_version_code=2024204030&ab_version=42.4.3&resolution=1080*2400&dpi=420&device_type=Pixel%206&device_brand=google&language=en&os_api=35&os_version=15&ac=wifi&is_pad=0&current_region=US&app_type=normal&sys_region=US&last_install_time=1767115711&timezone_name=America%2FNew_York&residence=US&app_language=en&ac2=wifi&uoo=0&op_region=US&timezone_offset=-18000&build_number=42.4.3&host_abi=arm64-v8a&locale=en&region=US&ts=1767263052&iid=7572811438918960951&device_id=7550968885031290423"

	u, err := url.Parse(originalURL)
	if err != nil {
		fmt.Printf("Error parsing URL: %v\n", err)
		return
	}

	q := u.Query()

	// Dynamic time
	timee := time.Now().Unix()
	utime := timee * 1000

	// Update time-based params
	q.Set("_rticket", strconv.FormatInt(utime, 10))
	q.Set("ts", strconv.FormatInt(timee, 10))
	// ensure iid/did matches
	q.Set("iid", installID)
	q.Set("device_id", deviceID)

	u.RawQuery = q.Encode()
	newFullURL := u.String()
	queryString := u.RawQuery

	req, err := http.NewRequest("GET", newFullURL, nil)
	if err != nil {
		fmt.Printf("Error creating request: %v\n", err)
		return
	}

	// Generate Headers
	headersResult := headers.MakeHeaders(
		deviceID,
		timee,
		1,         // signCount
		2,         // reportCount
		4,         // settingCount
		timee-10,  // appLaunchTime
		seedToken, // This corresponds to secDeviceToken, which is the x-tt-token
		"Pixel 6",
		seed,     // seed
		seedType, // seedEncodeType
		"",       // seedEndcodeHex
		"",       // algorithmData1
		"",       // hex32
		queryString,
		"", // postData (empty for GET)
		"42.4.3",
		"v05.02.02-ov-android",
		0x05020220,
		738,
		0xC40A800,
	)

	req.Header.Set("Host", "aggr16-normal.tiktokv.us")
	req.Header.Set("Cookie", cookies)
	req.Header.Set("x-tt-pba-enable", "1")
	req.Header.Set("x-bd-kmsv", "0")
	req.Header.Set("x-tt-dm-status", "login=1;ct=1;rt=8")
	req.Header.Set("x-ss-req-ticket", strconv.FormatInt(utime, 10))
	req.Header.Set("sdk-version", "2")
	// Make sure x-tt-token is set in header
	if xToken != "" {
		req.Header.Set("x-tt-token", xToken)
	}
	req.Header.Set("passport-sdk-version", "-1")
	req.Header.Set("rpc-persist-pns-region-1", "US|6252001|5549030")
	req.Header.Set("rpc-persist-pns-region-2", "US|6252001|5549030")
	req.Header.Set("rpc-persist-pns-region-3", "US|6252001|5549030")
	req.Header.Set("x-vc-bdturing-sdk-version", "2.3.17.i18n")
	req.Header.Set("x-tt-request-tag", "n=0;nr=111;bg=0;rs=112")
	req.Header.Set("rpc-persist-pyxis-policy-state-law-is-ca", "0")
	req.Header.Set("rpc-persist-pyxis-policy-v-tnc", "1")
	req.Header.Set("x-tt-ttnet-origin-host", "api16-normal-useast5.tiktokv.us")
	req.Header.Set("x-ss-dp", "1233")
	req.Header.Set("User-Agent", ua)

	// Replaced Headers
	req.Header.Set("x-argus", headersResult.XArgus)
	req.Header.Set("x-gorgon", headersResult.XGorgon)
	req.Header.Set("x-khronos", headersResult.XKhronos)
	req.Header.Set("x-ladon", headersResult.XLadon)
	req.Header.Set("x-ss-stub", headersResult.XSSStub)

	fmt.Printf("URL: %s\n", newFullURL)
	fmt.Printf("X-Gorgon: %s\n", headersResult.XGorgon)
	fmt.Printf("X-Khronos: %s\n", headersResult.XKhronos)

	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("Error sending request: %v\n", err)
		return
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Printf("Error reading response: %v\n", err)
		return
	}

	fmt.Printf("Response Status: %s\n", resp.Status)
	fmt.Printf("Response Body Length: %d\n", len(body))

	for k, v := range resp.Header {
		fmt.Printf("Response Header: %s: %s\n", k, v)
	}

	bodyStr := string(body)
	if len(bodyStr) > 500 {
		fmt.Printf("Response Body (first 500 chars): %s...\n", bodyStr)
	} else {
		fmt.Printf("Response Body: %s\n", bodyStr)
	}

	if strings.Contains(bodyStr, "\"status_code\":0") {
		fmt.Println("Success!")
	} else {
		fmt.Println("Request might have failed logic check.")
	}
}

// GetGetToken implementation
func GetGetToken(cookieData map[string]string, client *http.Client) string {
	ua := cookieData["ua"]
	if ua == "" {
		ua = cookieData["User-Agent"]
	}
	iid := cookieData["install_id"]
	deviceID := cookieData["device_id"]

	queryString := fmt.Sprintf("lc_id=2142840551&platform=android&device_platform=android&sdk_ver=v05.02.02-alpha.12-ov-android&sdk_ver_code=84017696&app_ver=42.4.3&version_code=2024204030&aid=1233&sdkid&subaid&iid=%s&did=%s&bd_did&client_type=inhouse&region_type=ov&mode=2", iid, deviceID)
	urlStr := fmt.Sprintf("https://mssdk16-normal-useast5.tiktokv.us/sdi/get_token?%s", queryString)

	timee := time.Now().Unix()
	utime := timee * 1000
	stime := timee

	tem, err := tt_protobuf.MakeTokenEncryptHex(stime, deviceID)
	if err != nil {
		fmt.Printf("MakeTokenEncryptHex error: %v\n", err)
		return ""
	}

	tokenEncrypt, err := endecode.MssdkEncrypt(tem, false, 1274)
	if err != nil {
		fmt.Printf("MssdkEncrypt error: %v\n", err)
		return ""
	}

	postData, err := tt_protobuf.MakeTokenRequest(tokenEncrypt, utime)
	if err != nil {
		fmt.Printf("MakeTokenRequest error: %v\n", err)
		return ""
	}

	_ = headers.MakeHeaders(
		deviceID, stime, 53, 2, 4, stime-6,
		"", "Pixel 6", "", 0, "", "", "",
		queryString, postData,
		"42.4.3", "v05.02.02-alpha.12-ov-android", 0x05020220, 738, 0,
	)

	if client == nil {
		return ""
	}

	postDataBytes, _ := hex.DecodeString(postData)
	req, err := http.NewRequest("POST", urlStr, bytes.NewReader(postDataBytes))
	if err != nil {
		fmt.Printf("NewRequest error: %v\n", err)
		return ""
	}

	req.Header.Set("rpc-persist-pyxis-policy-v-tnc", "1")
	req.Header.Set("rpc-persist-pyxis-policy-state-law-is-ca", "1")
	req.Header.Set("Accept-Encoding", "gzip")
	req.Header.Set("x-tt-request-tag", "n=0;nr=111;bg=0;t=0")
	req.Header.Set("x-tt-pba-enable", "1")
	req.Header.Set("Accept", "*/*")
	req.Header.Set("x-bd-kmsv", "0")
	req.Header.Set("X-SS-REQ-TICKET", fmt.Sprintf("%d", utime))
	req.Header.Set("x-vc-bdturing-sdk-version", "2.3.13.i18n")
	req.Header.Set("oec-vc-sdk-version", "3.0.12.i18n")
	req.Header.Set("sdk-version", "2")
	req.Header.Set("x-tt-dm-status", "login=1;ct=1;rt=1")
	req.Header.Set("passport-sdk-version", "-1")
	req.Header.Set("x-tt-store-region", "us")
	req.Header.Set("x-tt-store-region-src", "uid")
	req.Header.Set("User-Agent", ua)
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("Host", "mssdk16-normal-useast5.tiktokv.us")
	req.Header.Set("Connection", "Keep-Alive")

	resp, err := client.Do(req)
	if err != nil {
		fmt.Printf("client.Do error: %v\n", err)
		return ""
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		fmt.Printf("ReadAll error: %v\n", err)
		return ""
	}

	resHex := hex.EncodeToString(body)
	resPb, err := tt_protobuf.MakeTokenResponse(resHex)
	if err != nil {
		fmt.Printf("MakeTokenResponse error: %v\n", err)
		return ""
	}

	tokenDecrypt := resPb.TokenDecrypt
	if tokenDecrypt == "" {
		fmt.Println("TokenDecrypt is empty")
		return ""
	}

	tokenDecryptRes, err := endecode.MssdkDecrypt(tokenDecrypt, false, false)
	if err != nil {
		fmt.Printf("MssdkDecrypt error: %v\n", err)
		return ""
	}

	afterDecryptToken, err := tt_protobuf.MakeTokenDecrypt(tokenDecryptRes)
	if err != nil {
		fmt.Printf("MakeTokenDecrypt error: %v\n", err)
		return ""
	}

	return afterDecryptToken.Token
}

// GetSeed implementation
func GetSeed(cookieData map[string]string, client *http.Client) (string, int, error) {
	deviceID := strings.TrimSpace(cookieData["device_id"])
	if deviceID == "" {
		return "", 0, errors.New("device_id is required")
	}

	iid := strings.TrimSpace(cookieData["install_id"])
	if iid == "" {
		return "", 0, errors.New("install_id is required")
	}

	deviceType := cookieData["device_type"]
	if deviceType == "" {
		deviceType = "Pixel 6"
	}

	ua := cookieData["ua"]
	if ua == "" {
		ua = cookieData["User-Agent"]
	}
	if ua == "" {
		return "", 0, errors.New("ua is required")
	}

	queryString := fmt.Sprintf("lc_id=2142840551&platform=android&device_platform=android&sdk_ver=v05.02.02-alpha.12-ov-android&sdk_ver_code=84017696&app_ver=42.4.3&version_code=2024204030&aid=1233&sdkid&subaid&iid=%s&did=%s&bd_did&client_type=inhouse&region_type=ov&mode=2", iid, deviceID)
	requestURL := fmt.Sprintf("https://mssdk16-normal-useast5.tiktokv.us/ms/get_seed?%s", queryString)

	sessionID := generateUUID()

	seedEncryptHex, err := tt_protobuf.MakeSeedEncrypt(sessionID, deviceID, "android", "v05.02.02")
	if err != nil {
		return "", 0, fmt.Errorf("make seed encrypt: %w", err)
	}

	encryptedPayload, err := endecode.MssdkEncrypt(seedEncryptHex, false, 170)
	if err != nil {
		return "", 0, fmt.Errorf("mssdk encrypt: %w", err)
	}

	now := time.Now().Unix()
	utime := now * 1000

	postData, err := tt_protobuf.MakeSeedRequest(encryptedPayload, utime)
	if err != nil {
		return "", 0, fmt.Errorf("make seed request: %w", err)
	}

	headersResult := headers.MakeHeaders(
		deviceID, now, 52, 2, 4, now-6,
		"", deviceType, "", 0, "", "", "",
		queryString, postData,
		"42.4.3", "v05.02.02-alpha.12-ov-android", 0x05020220, 738, 0,
	)

	if client == nil {
		return "", 0, errors.New("http client is required")
	}

	postBytes, _ := hex.DecodeString(postData)
	req, err := http.NewRequest("POST", requestURL, bytes.NewReader(postBytes))
	if err != nil {
		return "", 0, fmt.Errorf("new request: %w", err)
	}

	req.Header.Set("rpc-persist-pyxis-policy-v-tnc", "1")
	req.Header.Set("rpc-persist-pyxis-policy-state-law-is-ca", "1")
	req.Header.Set("X-SS-STUB", headersResult.XSSStub)
	req.Header.Set("Accept-Encoding", "gzip")
	req.Header.Set("x-tt-request-tag", "n=0;nr=111;bg=0;t=0")
	req.Header.Set("x-tt-pba-enable", "1")
	req.Header.Set("Accept", "*/*")
	req.Header.Set("x-bd-kmsv", "0")
	req.Header.Set("X-SS-REQ-TICKET", fmt.Sprintf("%d", utime))
	req.Header.Set("x-vc-bdturing-sdk-version", "2.3.13.i18n")
	req.Header.Set("oec-vc-sdk-version", "3.0.12.i18n")
	req.Header.Set("sdk-version", "2")
	req.Header.Set("x-tt-dm-status", "login=1;ct=1;rt=1")
	req.Header.Set("passport-sdk-version", "-1")
	req.Header.Set("x-tt-store-region", "us")
	req.Header.Set("x-tt-store-region-src", "uid")
	req.Header.Set("User-Agent", ua)
	req.Header.Set("X-Ladon", headersResult.XLadon)
	req.Header.Set("X-Khronos", headersResult.XKhronos)
	req.Header.Set("X-Argus", headersResult.XArgus)
	req.Header.Set("X-Gorgon", headersResult.XGorgon)
	req.Header.Set("Content-Type", "application/octet-stream")
	req.Header.Set("Host", "mssdk16-normal-useast5.tiktokv.us")
	req.Header.Set("Connection", "Keep-Alive")

	resp, err := client.Do(req)
	if err != nil {
		return "", 0, fmt.Errorf("request seed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", 0, fmt.Errorf("read body: %w", err)
	}

	resHex := hex.EncodeToString(body)
	respPb, err := tt_protobuf.MakeSeedResponse(resHex)
	if err != nil {
		return "", 0, fmt.Errorf("decode response: %w", err)
	}
	if respPb.SeedDecrypt == "" {
		return "", 0, errors.New("empty seed_decrypt payload")
	}

	decryptedHex, err := endecode.MssdkDecrypt(respPb.SeedDecrypt, false, false)
	if err != nil {
		return "", 0, fmt.Errorf("mssdk decrypt: %w", err)
	}

	decryptedSeed, err := tt_protobuf.MakeSeedDecrypt(decryptedHex)
	if err != nil {
		return "", 0, fmt.Errorf("parse seed decrypt: %w", err)
	}

	seedValue := decryptedSeed.Seed
	var algorithm string
	if decryptedSeed.ExtraInfo != nil {
		algorithm = decryptedSeed.ExtraInfo.Algorithm
	}
	if seedValue == "" || algorithm == "" {
		return "", 0, errors.New("invalid seed response")
	}

	algBytes := []byte(algorithm)
	algInt, err := strconv.ParseInt(hex.EncodeToString(algBytes), 16, 64)
	if err != nil {
		return "", 0, fmt.Errorf("parse algorithm: %w", err)
	}
	return seedValue, int(algInt / 2), nil
}

func generateUUID() string {
	b := make([]byte, 16)
	rand.Read(b)
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%x%x%x%x%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}
