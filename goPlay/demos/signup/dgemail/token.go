package main

import (
	"bytes"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	"tt_code/headers"
	"tt_code/mssdk/endecode"
	"tt_code/tt_protobuf"
)

// GetToken 获取token - 使用代理管理器来复用HTTP客户端，避免连接被强制关闭
func GetToken(cookieData map[string]string, proxy string) string {
	// 使用代理管理器获取HTTP客户端
	var httpClient *http.Client
	if proxyManager := GetProxyManager(); proxyManager != nil {
		httpClient = proxyManager.GetClient(proxy)
	} else {
		// 如果代理管理器不可用，创建新的客户端（禁用连接复用以避免问题）
		httpClient = createTokenClientWithNoKeepAlive(proxy)
	}

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
	// 改为 Close 而不是 Keep-Alive，避免连接复用导致的问题
	req.Header.Set("Connection", "close")

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

// createTokenClientWithNoKeepAlive 创建HTTP客户端（禁用连接复用，避免连接被强制关闭）
func createTokenClientWithNoKeepAlive(proxy string) *http.Client {
	var transport *http.Transport
	if proxy != "" {
		if proxyURL, err := url.Parse(proxy); err == nil {
			transport = &http.Transport{
				Proxy:                 http.ProxyURL(proxyURL),
				MaxIdleConns:          0,  // 禁用连接池
				MaxIdleConnsPerHost:   0, // 禁用每个主机的连接池
				MaxConnsPerHost:       0, // 不限制连接数
				IdleConnTimeout:       0, // 立即关闭空闲连接
				TLSHandshakeTimeout:   15 * time.Second,   // 增加TLS握手超时
				ResponseHeaderTimeout: 30 * time.Second,   // 增加响应头超时
				DisableKeepAlives:     true, // 禁用连接复用，每次请求都新建连接
				ForceAttemptHTTP2:     false,
			}
		}
	}
	if transport == nil {
		if proxyURL, err := url.Parse("http://127.0.0.1:7777"); err == nil {
			transport = &http.Transport{
				Proxy:                 http.ProxyURL(proxyURL),
				MaxIdleConns:          0,
				MaxIdleConnsPerHost:   0,
				MaxConnsPerHost:       0,
				IdleConnTimeout:       0,
				TLSHandshakeTimeout:   15 * time.Second,   // 增加TLS握手超时
				ResponseHeaderTimeout: 30 * time.Second,   // 增加响应头超时
				DisableKeepAlives:     true,
				ForceAttemptHTTP2:     false,
			}
		}
	}
	return &http.Client{
		Timeout:   45 * time.Second, // 增加超时时间以提高成功率
		Transport: transport,
	}
}

