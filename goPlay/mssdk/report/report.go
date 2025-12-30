package report

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

const (
	reportZlibHexLength = 9872
	defaultProxyAddr    = "http://127.0.0.1:7777"
)

// SendReport 复刻Python版本的send_report逻辑
func SendReport(
	device map[string]string,
	token string,
	seed string,
	seedType int,
	proxy string,
	p237 string,
	p1410 string,
	reportType int,
) ([]byte, error) {
	deviceID := strings.TrimSpace(device["device_id"])
	installID := strings.TrimSpace(device["install_id"])
	if deviceID == "" || installID == "" {
		return nil, errors.New("device_id and install_id are required")
	}

	// TODO: integrate p237/p1410/reportType once protobuf fields are ready.
	_ = p237
	_ = p1410
	_ = reportType

	ua := device["ua"]
	if ua == "" {
		ua = device["User-Agent"]
	}
	if ua == "" {
		ua = "com.zhiliaoapp.musically/2024204030 (Linux; U; Android 15; en_US; Pixel 6; Build/BP1A.250505.005; Cronet/TTNetVersion:efce646d 2025-10-16 QuicVersion:c785494a 2025-09-30)"
	}

	queryString := fmt.Sprintf(
		"lc_id=2142840551&platform=android&device_platform=android&sdk_ver=v05.02.02-alpha.12-ov-android&sdk_ver_code=84017696&app_ver=42.4.3&version_code=2024204030&aid=1233&sdkid&subaid&iid=%s&did=%s&bd_did&client_type=inhouse&region_type=ov&mode=2&msgtk=1&msrep=1",
		installID, deviceID,
	)
	requestURL := fmt.Sprintf("https://aggr16-normal.tiktokv.us/ri/report?%s", queryString)

	now := time.Now()
	stime := now.Unix()
	utime := now.UnixMilli()

	reportEncryptHex, err := buildReportEncryptHex(device, token, stime, p237, p1410)
	if err != nil {
		return nil, fmt.Errorf("build report encrypt payload: %w", err)
	}

	encryptedPayload, err := endecode.MssdkEncrypt(reportEncryptHex, true, reportZlibHexLength)
	if err != nil {
		return nil, fmt.Errorf("mssdk encrypt: %w", err)
	}

	postData, err := tt_protobuf.MakeReportRequest(encryptedPayload, utime)
	if err != nil {
		return nil, fmt.Errorf("make report request: %w", err)
	}

	rand.Seed(time.Now().UnixNano())
	signCount := rand.Intn(21) + 20
	reportCount := rand.Intn(401) + 100
	settingCount := rand.Intn(401) + 100
	appLaunchTime := stime - int64(rand.Intn(51)+50)

	headersResult := headers.MakeHeaders(
		deviceID,
		stime,
		signCount,
		reportCount,
		settingCount,
		appLaunchTime,
		token,
		getOrDefault(device, "device_type", "Pixel 6"),
		seed,
		seedType,
		"",
		"",
		"",
		queryString,
		postData,
		getOrDefault(device, "version_name", "42.4.3"),
		getOrDefault(device, "sdk_version_str", "v05.02.02-alpha.12-ov-android"),
		0x05020220,
		738,
		0,
	)

	postBytes, err := hex.DecodeString(postData)
	if err != nil {
		return nil, fmt.Errorf("decode post data: %w", err)
	}

	req, err := http.NewRequest("POST", requestURL, bytes.NewReader(postBytes))
	if err != nil {
		return nil, fmt.Errorf("new request: %w", err)
	}

	req.Header.Set("content-type", "application/octet-stream")
	req.Header.Set("x-vc-bdturing-sdk-version", "2.3.17.i18n")
	req.Header.Set("sdk-version", "2")
	req.Header.Set("x-tt-dm-status", "login=0;ct=1;rt=8")
	req.Header.Set("x-ss-req-ticket", fmt.Sprintf("%d", utime))
	req.Header.Set("passport-sdk-version", "-1")
	req.Header.Set("x-ss-stub", headersResult.XSSStub)
	req.Header.Set("rpc-persist-pyxis-policy-state-law-is-ca", "1")
	req.Header.Set("rpc-persist-pyxis-policy-v-tnc", "1")
	req.Header.Set("x-tt-ttnet-origin-host", "mssdk16-normal-useast8.tiktokv.us")
	req.Header.Set("x-ss-dp", "1233")
	req.Header.Set("user-agent", ua)
	req.Header.Set("accept-encoding", "gzip, deflate")
	req.Header.Set("x-argus", headersResult.XArgus)
	req.Header.Set("x-gorgon", headersResult.XGorgon)
	req.Header.Set("x-khronos", headersResult.XKhronos)
	req.Header.Set("x-ladon", headersResult.XLadon)
	req.Header.Set("x-tt-request-tag", "t=0;n=0")
	req.Header.Set("Host", "aggr16-normal.tiktokv.us")
	req.Header.Set("Connection", "Keep-Alive")

	req.AddCookie(&http.Cookie{Name: "store-idc", Value: "useast5"})
	req.AddCookie(&http.Cookie{Name: "store-country-code", Value: "us"})
	req.AddCookie(&http.Cookie{Name: "store-country-code-src", Value: "did"})
	req.AddCookie(&http.Cookie{Name: "install_id", Value: installID})
	req.AddCookie(&http.Cookie{Name: "tt-target-idc", Value: "useast5"})
	req.AddCookie(&http.Cookie{Name: "reg-store-region", Value: "US"})

	client := &http.Client{Timeout: 30 * time.Second}
	proxyAddr := proxy
	if proxyAddr == "" {
		proxyAddr = defaultProxyAddr
	}
	if proxyAddr != "" {
		if proxyURL, err := url.Parse(proxyAddr); err == nil {
			client.Transport = &http.Transport{Proxy: http.ProxyURL(proxyURL)}
		}
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request report: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	return body, nil
}

func buildReportEncryptHex(device map[string]string, token string, stime int64, p237, p1410 string) (string, error) {
	deviceID := strings.TrimSpace(device["device_id"])
	installID := strings.TrimSpace(device["install_id"])
	if deviceID == "" || installID == "" {
		return "", errors.New("device_id and install_id are required")
	}
	openUDID := getOrDefault(device, "openudid", installID)

	sdkVersion := parseInt(device["sdk_version"], 5)
	if sdkVersion <= 0 {
		sdkVersion = 5
	}
	osVersion := getOrDefault(device, "os_version", "15")
	deviceModel := getOrDefault(device, "device_type", "Pixel 6")
	deviceBrand := getOrDefault(device, "device_brand", "google")
	cpuABI := getOrDefault(device, "cpu_abi", "arm64")
	resolution := getOrDefault(device, "resolution", "2209x1080")
	dpi := parseInt(device["dpi"], 420)
	if dpi <= 0 {
		dpi = 420
	}
	aid := parseInt(device["aid"], 1233)
	if aid <= 0 {
		aid = 1233
	}
	channel := getOrDefault(device, "channel", "samsung_store")
	packageName := getOrDefault(device, "package_name", "com.zhiliaoapp.musically")
	secSDKVersion := getOrDefault(device, "update_version_code", "2024204030")

	reportEncrypt := tt_protobuf.MakeReportEncrypt(
		deviceID,
		installID,
		stime,
		sdkVersion,
		token,
		osVersion,
		deviceModel,
		deviceBrand,
		cpuABI,
		resolution,
		dpi,
		aid,
		channel,
		packageName,
		secSDKVersion,
		openUDID,
		p237,
		p1410,
	)

	reportEncryptBytes := tt_protobuf.EncodeReportEncrypt(reportEncrypt)
	return hex.EncodeToString(reportEncryptBytes), nil
}

func getOrDefault(data map[string]string, key, def string) string {
	if v := strings.TrimSpace(data[key]); v != "" {
		return v
	}
	return def
}

func parseInt(val string, def int) int {
	if val == "" {
		return def
	}
	if i, err := strconv.Atoi(val); err == nil {
		return i
	}
	return def
}
