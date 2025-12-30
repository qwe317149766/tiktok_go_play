package get_token

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

// GetGetToken 获取token - 完全匹配Python的get_get_token函数签名
// cookieData: 包含install_id, device_id, ua/User-Agent等字段的map
// proxy: 代理地址，默认为空字符串
// client: 可选的HTTP客户端，如果提供则使用，否则创建新的
// 返回: token字符串，如果失败返回空字符串
func GetGetToken(cookieData map[string]string, proxy string, client ...*http.Client) string {
	var httpClient *http.Client
	if len(client) > 0 && client[0] != nil {
		httpClient = client[0]
	} else {
		// 使用原有的逻辑创建客户端
		httpClient = createClient(proxy)
	}
	// 从cookieData中提取参数 - 完全匹配Python逻辑
	ua := cookieData["ua"]
	if ua == "" {
		ua = cookieData["User-Agent"]
	}
	iid := cookieData["install_id"]
	deviceID := cookieData["device_id"]

	// 构建query_string - 完全匹配Python
	queryString := fmt.Sprintf("lc_id=2142840551&platform=android&device_platform=android&sdk_ver=v05.02.02-alpha.12-ov-android&sdk_ver_code=84017696&app_ver=42.4.3&version_code=2024204030&aid=1233&sdkid&subaid&iid=%s&did=%s&bd_did&client_type=inhouse&region_type=ov&mode=2", iid, deviceID)
	urlStr := fmt.Sprintf("https://mssdk16-normal-useast5.tiktokv.us/sdi/get_token?%s", queryString)

	// 生成时间戳
	timee := time.Now().Unix()
	utime := timee * 1000
	stime := timee

	// 创建protobuf - 使用make_token_encrypt(stime, device_id)
	tem, err := tt_protobuf.MakeTokenEncryptHex(stime, deviceID)
	if err != nil {
		return ""
	}
	// fmt.Println("tem===>", tem)
	// 加密 - 完全匹配Python: mssdk_encrypt(tem, False)
	// token的zlib压缩后需要填充到1274字符（hex字符串长度）- 匹配Python的token_test.py
	tokenEncrypt, err := endecode.MssdkEncrypt(tem, false, 1274)
	if err != nil {
		return ""
	}
	// fmt.Println("tokenEncrypt===>", tokenEncrypt)
	// 创建post_data - 使用make_token_request
	postData, err := tt_protobuf.MakeTokenRequest(tokenEncrypt, utime)
	if err != nil {
		return ""
	}

	// 生成headers - 完全匹配Python的make_headers调用
	// make_headers(device_id,stime, 53,2,4,stime-6, "","Pixel 6", "","","","","", query_string, post_data)
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

	// 设置headers - 完全匹配Python的headers列表
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

	// 解析响应 - 完全匹配Python逻辑
	resHex := hex.EncodeToString(body)
	resPb, err := tt_protobuf.MakeTokenResponse(resHex)
	if err != nil {
		return ""
	}

	tokenDecrypt := resPb.TokenDecrypt
	if tokenDecrypt == "" {
		return ""
	}

	// 解密 - 完全匹配Python: mssdk_decrypt(tokenDecrypt, False, False)
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
	// fmt.Printf("token is ===> %s\n", token)

	return token
}

// createClient 创建HTTP客户端（原有逻辑）
func createClient(proxy string) *http.Client {
	var transport *http.Transport
	if proxy != "" {
		if proxyURL, err := url.Parse(proxy); err == nil {
			transport = &http.Transport{
				Proxy:                 http.ProxyURL(proxyURL),
				MaxIdleConns:          100,
				MaxIdleConnsPerHost:   20,
				MaxConnsPerHost:       30,
				IdleConnTimeout:       60 * time.Second,
				TLSHandshakeTimeout:   8 * time.Second,
				ResponseHeaderTimeout: 15 * time.Second,
				DisableKeepAlives:     false,
				ForceAttemptHTTP2:     false,
			}
		}
	}
	if transport == nil {
		if proxyURL, err := url.Parse("http://127.0.0.1:7777"); err == nil {
			transport = &http.Transport{
				Proxy:                 http.ProxyURL(proxyURL),
				MaxIdleConns:          100,
				MaxIdleConnsPerHost:   20,
				MaxConnsPerHost:       30,
				IdleConnTimeout:       60 * time.Second,
				TLSHandshakeTimeout:   8 * time.Second,
				ResponseHeaderTimeout: 15 * time.Second,
				DisableKeepAlives:     false,
				ForceAttemptHTTP2:     false,
			}
		}
	}
	return &http.Client{
		Timeout:   25 * time.Second,
		Transport: transport,
	}
}

// func main() {
// 	var cookie_data = map[string]string{"install_id": "7560648754732271373", "ttreq": "1$084e38a4169433286a4d5b3624876b9a081fdb26", "passport_csrf_token": "b1271b9e8b3200cc84af7068542ed385", "passport_csrf_token_default": "b1271b9e8b3200cc84af7068542ed385", "cmpl_token": "AgQQAPNSF-RPsLjSPIU4_p0O8o1I8YdL_4_ZYNw2xQ", "d_ticket": "fd4617f2988fd35a13be0470ee38138f75ff0", "multi_sids": "7560648920885118007%3Aa1787f5786d0d68bba95c02478ad08a7", "sessionid": "a1787f5786d0d68bba95c02478ad08a7", "sessionid_ss": "a1787f5786d0d68bba95c02478ad08a7", "sid_guard": "a1787f5786d0d68bba95c02478ad08a7%7C1760351110%7C15552000%7CSat%2C+11-Apr-2026+10%3A25%3A10+GMT", "sid_tt": "a1787f5786d0d68bba95c02478ad08a7", "uid_tt": "868053b1a8bbc871eababd16bdac646cab192f7db7ef051d32b98af91ec80312", "uid_tt_ss": "868053b1a8bbc871eababd16bdac646cab192f7db7ef051d32b98af91ec80312", "msToken": "3_GH6YKViUaCI47ca0jK3JRcVHkxXiPfLElY8SBRz00SsdYcxmJMBmtAZtprvXwz0eQXgx6C5VdheQe3iuLNTt2mdkirkaMVpEB0dhRaa0Ua0_MMqwry0r2sVbGY", "odin_tt": "bd4e6506b3c04fbeaacbaa258910260595b22fb8279109200481a0a6bf5d7967c75094734ea42322d45a13a2c1accde39a13fb9c487d6a4cd9ddb5e7fc68d024f2a1c438997c8b62aa91af62094cbaf6", "store-country-code": "us", "store-country-code-src": "uid", "store-country-sign": "MEIEDHcKMd5AY9nmmyG-AgQgu4rvy4TjCuUrFIJp9gRls14g-YBiHF33hFihFd61Ys0EEHQAqhMHWqkeGVjMJbWEVtc", "store-idc": "useast5", "tt-target-idc": "useast8", "s_v_web_id": "", "username": "user3457778153976", "password": "BDHlA2598@", "X-Tt-Token": "04a1787f5786d0d68bba95c02478ad08a702bbe3f8d53c9b17000d05c94157772ebd1a7dd970394451dae4765b1b94548ccf0c0f36affb66d05de2df00bd336047d3115e4e79c653799ec66ae57516f601d4af84b3368445dc2093ae078d7b35e8551--0a4e0a200facb2b79eb92e14c95430de6a7c536d5d4f700a8f87fe6e7af40a85115d3cd81220dc99b4e6dc81c575c88ff18e6d6913cf36ceda5d7e229f576cd4efb115f903861801220674696b746f6b-3.0.1", "phone": "9432409396", "url": "https://a.62-us.com/api/get_sms?key=62sms_b965cb70222fc5ca31134d4a5c270936", "ts_sign_ree": "ts.1.35c626f3611a9d7634bc8eb72936bd2d7cb83e463de0ce265a6c3427727d61937a50e8a417df069df9a555bd16c66ef8b3639a56b642d7d8f9c881f42b9329ec", "User-Agent": "Mozilla/5.0 (Linux; Android 12; SM-S9010 Build/UP1A.231005.007; wv) AppleWebKit/537.36 (KHTML, like Gecko) Version/4.0 Chrome/91.0.4472.114 Mobile Safari/537.36", "uid": "7560648920885118007", "device_id": "7560648506287113741", "twofa": "CRKCXBPBSPCICREBFUZ22J5VVMM5FLGY", "ua": "1231", "device_type": "Pixel"}
// 	proxy := "socks5h://ax6h11466-region-US-sid-Kkpbf2De-t-5:mtpb2pdw@us.novproxy.io:1000"
// 	for i := 0; i < 100; i++ {
// 		token := GetGetToken(cookie_data, proxy)
// 		fmt.Println("token is ===>", token)

// 	}
// }
