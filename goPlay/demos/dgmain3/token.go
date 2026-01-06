package main

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"time"

	"tt_code/headers"
	"tt_code/mssdk/endecode"
	"tt_code/tt_protobuf"
)

// TokenResult 异步token结果
type TokenResult struct {
	Token string
	Err   error
}

// GetTokenAsync 异步获取token
func GetTokenAsync(cookieData map[string]string, client *http.Client) chan TokenResult {
	resultChan := make(chan TokenResult, 1)
	go func() {
		token := GetToken(cookieData, client)
		resultChan <- TokenResult{Token: token}
	}()
	return resultChan
}

// GetToken 获取token
func GetToken(cookieData map[string]string, httpClient *http.Client) string {
	// 从cookieData中提取参数
	ua := cookieData["ua"]
	if ua == "" {
		ua = cookieData["User-Agent"]
	}
	iid := cookieData["install_id"]
	deviceID := cookieData["device_id"]

	// 构建query_string
	queryString := fmt.Sprintf("lc_id=2142840551&platform=android&device_platform=android&sdk_ver=v05.02.02-alpha.12-ov-android&sdk_ver_code=84017696&app_ver=42.4.3&version_code=2024204030&aid=1233&sdkid&subaid&iid=%s&did=%s&bd_did&client_type=inhouse&region_type=ov&mode=2", iid, deviceID)
	urlStr := fmt.Sprintf("https://mssdk16-normal-useast5.tiktokv.us/sdi/get_token?%s", queryString)

	// 生成时间戳
	timee := time.Now().Unix()
	utime := timee * 1000
	stime := timee

	// 创建protobuf
	tem, err := tt_protobuf.MakeTokenEncryptHex(stime, deviceID)
	if err != nil {
		return ""
	}

	// 加密
	tokenEncrypt, err := endecode.MssdkEncrypt(tem, false, 1274)
	if err != nil {
		return ""
	}

	// 创建post_data
	postData, err := tt_protobuf.MakeTokenRequest(tokenEncrypt, utime)
	if err != nil {
		return ""
	}

	// 生成headers
	_ = headers.MakeHeaders(
		deviceID, stime, 53, 2, 4, stime-6,
		"", "Pixel 6", "", 0, "", "", "",
		queryString, postData,
		"42.4.3", "v05.02.02-alpha.12-ov-android", 0x05020220, 738, 0,
	)

	// 创建请求
	postDataBytes, _ := hex.DecodeString(postData)
	req, err := http.NewRequest("POST", urlStr, bytes.NewReader(postDataBytes))
	if err != nil {
		return ""
	}

	// 设置headers
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

	// 发送请求
	resp, err := httpClient.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()

	// 读取响应
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return ""
	}

	// 解析响应
	resHex := hex.EncodeToString(body)
	resPb, err := tt_protobuf.MakeTokenResponse(resHex)
	if err != nil {
		return ""
	}

	tokenDecrypt := resPb.TokenDecrypt
	if tokenDecrypt == "" {
		return ""
	}

	// 解密
	tokenDecryptRes, err := endecode.MssdkDecrypt(tokenDecrypt, false, false)
	if err != nil {
		return ""
	}

	// 解析解密后的数据
	afterDecryptToken, err := tt_protobuf.MakeTokenDecrypt(tokenDecryptRes)
	if err != nil {
		return ""
	}

	token := afterDecryptToken.Token
	return token
}

