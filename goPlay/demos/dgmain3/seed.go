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

// SeedResult 异步seed结果
type SeedResult struct {
	Seed     string
	SeedType int
	Err      error
}

// GetSeedAsync 异步获取seed
func GetSeedAsync(cookieData map[string]string, client *http.Client) chan SeedResult {
	resultChan := make(chan SeedResult, 1)
	go func() {
		seed, seedType, err := GetSeed(cookieData, client)
		resultChan <- SeedResult{Seed: seed, SeedType: seedType, Err: err}
	}()
	return resultChan
}

// GetSeed 调用 /ms/get_seed，返回 seed 及 seed_type
func GetSeed(cookieData map[string]string, httpClient *http.Client) (string, int, error) {
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

	resp, err := httpClient.Do(req)
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

