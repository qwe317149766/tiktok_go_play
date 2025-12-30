package main

import (
	"bytes"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"strconv"
	"strings"
	"time"

	"tt_code/headers"
	"tt_code/mssdk/endecode"
	"tt_code/tt_protobuf"
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

// GetSeedAsync 异步获取seed，返回channel（带panic恢复）
func GetSeedAsync(cookieData map[string]string, client *http.Client) <-chan SeedResult {
	resultChan := make(chan SeedResult, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				// 捕获panic，转换为error
				err := fmt.Errorf("panic in GetSeed: %v", r)
				resultChan <- SeedResult{
					Seed:     "",
					SeedType: 0,
					Err:      err,
				}
			}
		}()
		seed, seedType, err := GetSeed(cookieData, client)
		resultChan <- SeedResult{
			Seed:     seed,
			SeedType: seedType,
			Err:      err,
		}
	}()
	return resultChan
}

// SeedResult seed获取结果
type SeedResult struct {
	Seed     string
	SeedType int
	Err      error
}

// GetSeed 调用 /ms/get_seed，返回 seed 及 seed_type（使用共享的HTTP客户端）
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
		ua = "Mozilla/5.0 (Linux; Android 12; Pixel 6 Build/TQ1A.230205.002; wv) AppleWebKit/537.36 (KHTML, like Gecko) Version/4.0 Chrome/91.0.4472.114 Mobile Safari/537.36"
	}

	queryString := fmt.Sprintf("lc_id=2142840551&platform=android&device_platform=android&sdk_ver=v05.02.02-alpha.12-ov-android&sdk_ver_code=84017696&app_ver=42.4.3&version_code=2024204030&aid=1233&sdkid&subaid&iid=%s&did=%s&bd_did&client_type=inhouse&region_type=ov&mode=2", iid, deviceID)
	requestURL := fmt.Sprintf("https://mssdk16-normal-useast5.tiktokv.us/ms/get_seed?%s", queryString)

	sessionID := generateUUID()

	seedEncryptHex, err := tt_protobuf.MakeSeedEncrypt(sessionID, deviceID, "android", "v05.02.02")
	if err != nil {
		return "", 0, fmt.Errorf("make seed encrypt: %w", err)
	}

	encryptedPayload, err := endecode.MssdkEncrypt(seedEncryptHex, false, 170)
	// fmt.Println("encryptPayload===>", encryptedPayload)
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

	// 使用传入的共享HTTP客户端
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

// func main() {
// 	var cookie_data = map[string]string{"install_id": "7560648754732271373", "ttreq": "1$084e38a4169433286a4d5b3624876b9a081fdb26", "passport_csrf_token": "b1271b9e8b3200cc84af7068542ed385", "passport_csrf_token_default": "b1271b9e8b3200cc84af7068542ed385", "cmpl_token": "AgQQAPNSF-RPsLjSPIU4_p0O8o1I8YdL_4_ZYNw2xQ", "d_ticket": "fd4617f2988fd35a13be0470ee38138f75ff0", "multi_sids": "7560648920885118007%3Aa1787f5786d0d68bba95c02478ad08a7", "sessionid": "a1787f5786d0d68bba95c02478ad08a7", "sessionid_ss": "a1787f5786d0d68bba95c02478ad08a7", "sid_guard": "a1787f5786d0d68bba95c02478ad08a7%7C1760351110%7C15552000%7CSat%2C+11-Apr-2026+10%3A25%3A10+GMT", "sid_tt": "a1787f5786d0d68bba95c02478ad08a7", "uid_tt": "868053b1a8bbc871eababd16bdac646cab192f7db7ef051d32b98af91ec80312", "uid_tt_ss": "868053b1a8bbc871eababd16bdac646cab192f7db7ef051d32b98af91ec80312", "msToken": "3_GH6YKViUaCI47ca0jK3JRcVHkxXiPfLElY8SBRz00SsdYcxmJMBmtAZtprvXwz0eQXgx6C5VdheQe3iuLNTt2mdkirkaMVpEB0dhRaa0Ua0_MMqwry0r2sVbGY", "odin_tt": "bd4e6506b3c04fbeaacbaa258910260595b22fb8279109200481a0a6bf5d7967c75094734ea42322d45a13a2c1accde39a13fb9c487d6a4cd9ddb5e7fc68d024f2a1c438997c8b62aa91af62094cbaf6", "store-country-code": "us", "store-country-code-src": "uid", "store-country-sign": "MEIEDHcKMd5AY9nmmyG-AgQgu4rvy4TjCuUrFIJp9gRls14g-YBiHF33hFihFd61Ys0EEHQAqhMHWqkeGVjMJbWEVtc", "store-idc": "useast5", "tt-target-idc": "useast8", "s_v_web_id": "", "username": "user3457778153976", "password": "BDHlA2598@", "X-Tt-Token": "04a1787f5786d0d68bba95c02478ad08a702bbe3f8d53c9b17000d05c94157772ebd1a7dd970394451dae4765b1b94548ccf0c0f36affb66d05de2df00bd336047d3115e4e79c653799ec66ae57516f601d4af84b3368445dc2093ae078d7b35e8551--0a4e0a200facb2b79eb92e14c95430de6a7c536d5d4f700a8f87fe6e7af40a85115d3cd81220dc99b4e6dc81c575c88ff18e6d6913cf36ceda5d7e229f576cd4efb115f903861801220674696b746f6b-3.0.1", "phone": "9432409396", "url": "https://a.62-us.com/api/get_sms?key=62sms_b965cb70222fc5ca31134d4a5c270936", "ts_sign_ree": "ts.1.35c626f3611a9d7634bc8eb72936bd2d7cb83e463de0ce265a6c3427727d61937a50e8a417df069df9a555bd16c66ef8b3639a56b642d7d8f9c881f42b9329ec", "User-Agent": "Mozilla/5.0 (Linux; Android 12; SM-S9010 Build/UP1A.231005.007; wv) AppleWebKit/537.36 (KHTML, like Gecko) Version/4.0 Chrome/91.0.4472.114 Mobile Safari/537.36", "uid": "7560648920885118007", "device_id": "7560648506287113741", "twofa": "CRKCXBPBSPCICREBFUZ22J5VVMM5FLGY", "ua": "1231", "device_type": "Pixel"}
// 	proxy := "socks5h://ax6h11466-region-US-sid-Kkpbf2De-t-5:mtpb2pdw@us.novproxy.io:1000"
// 	for i := 0; i < 100; i++ {
// 		seed, seed_type, err := GetSeed(cookie_data, proxy)
// 		fmt.Println("seed===>", seed)
// 		fmt.Println("seed_type===>", seed_type)
// 		fmt.Println("err===>", err)
// 	}
// }
