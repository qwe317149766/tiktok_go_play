package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// VideoInfoResponse represents the relevant fields from the TikTok API response
type VideoInfoResponse struct {
	// For detail API
	ItemInfo struct {
		ItemStruct struct {
			Stats struct {
				PlayCount    int64 `json:"playCount"`
				DiggCount    int64 `json:"diggCount"`
				CommentCount int64 `json:"commentCount"`
				ShareCount   int64 `json:"shareCount"`
			} `json:"stats"`
			Video struct {
				ID string `json:"id"`
			} `json:"video"`
		} `json:"itemStruct"`
	} `json:"itemInfo"`

	// For feed API
	ItemList []struct {
		Stats struct {
			PlayCount    int64 `json:"playCount"`
			DiggCount    int64 `json:"diggCount"`
			CommentCount int64 `json:"commentCount"`
			ShareCount   int64 `json:"shareCount"`
		} `json:"stats"`
		Video struct {
			ID string `json:"id"`
		} `json:"video"`
	} `json:"itemList"`

	StatusCode int    `json:"statusCode"`
	StatusMsg  string `json:"status_msg"`
}

// GetVideoInfo fetches video information from TikTok API using the video ID
func GetVideoInfo(videoID string) (*VideoInfoResponse, error) {
	// 基础 URL
	baseURL := "https://www.tiktok.com/api/item/detail/"

	// 使用原始查询字符串以避免编码差异（WebIdLastTime 和 msToken 等参数可能需要根据实际情况动态更新，这里暂时沿用能通的硬编码值作为基础）
	// 注意：itemId 需要替换为传入的 videoID
	// 为了简化，我们使用字符串替换，但保留其他鉴权参数
	rawQueryTemplate := "WebIdLastTime=1767083930&aid=1988&app_language=en-GB&app_name=tiktok_web&browser_language=en-IE&browser_name=Mozilla&browser_online=true&browser_platform=Win32&browser_version=5.0%20%28MeeGo%3B%20NokiaN9%29%20AppleWebKit%2F534.13%20%28KHTML%2C%20like%20Gecko%29%20NokiaBrowser%2F8.5.0%20Mobile%20Safari%2F534.13&channel=tiktok_web&cookie_enabled=true&data_collection_enabled=true&device_id=7589567659773068817&device_platform=web_pc&focus_state=false&from_page=video&history_len=11&is_fullscreen=false&is_page_visible=true&itemId=__VIDEO_ID__&os=unknown&priority_region=JP&referer=&region=JP&screen_height=854&screen_width=480&tz_name=Asia%2FShanghai&user_is_login=true&verifyFp=verify_mjvc6b0t_DveU8fMQ_UJGE_4bUO_Bkx4_lwVLknixAOZy&webcast_language=en-GB&msToken=w7IMDLNpgcLgWnLzHJg6UDyjSM6JlxNXTgPqVfgA82Zjs4lRWiYwCrQBtPA3I6nNNlV_87dNvgHJKbJwnEYM4y8cA72N-Qq2uNbni3pNCWvrHyrIKNdPwgn3fUQnUYZnVzkJWCLqXiBiOj_6oj3XavNqAq7r"

	rawQuery := strings.Replace(rawQueryTemplate, "__VIDEO_ID__", videoID, 1)

	// User-Agent（用于生成加密字段，必须与 Header 一致）
	userAgent := "Mozilla/5.0 (MeeGo; NokiaN9) AppleWebKit/534.13 (KHTML, like Gecko) NokiaBrowser/8.5.0 Mobile Safari/534.13"

	// 请求体（GET 请求为空）
	postData := ""

	// 获取时间戳
	timestamp := uint32(time.Now().Unix())
	timestampMs := time.Now().UnixMilli()

	// 使用 xbogus.go 生成 X-Bogus
	xBogus := Encrypt(rawQuery, postData, userAgent, timestamp)

	// 使用 xgnarly.go 生成 X-Gnarly
	xGnarly, err := EncryptXgnarly(rawQuery, postData, userAgent, 0, "5.1.1", timestampMs)
	if err != nil {
		return nil, fmt.Errorf("generate X-Gnarly failed: %w", err)
	}

	// 构建完整的 URL
	fullURL := baseURL + "?" + rawQuery + "&X-Bogus=" + xBogus + "&X-Gnarly=" + url.QueryEscape(xGnarly)

	req, err := http.NewRequest("GET", fullURL, nil)
	if err != nil {
		return nil, err
	}

	// ===== headers (Strictly match the successful request) =====
	req.Header.Set("accept", "*/*")
	req.Header.Set("accept-language", "en-IE,en;q=0.9,zh-CN;q=0.8,zh;q=0.7")
	req.Header.Set("cache-control", "no-cache")
	req.Header.Set("pragma", "no-cache")
	// req.Header.Set("priority", "u=1, i") // Optional based on previous tests
	// req.Header.Set("sec-fetch-dest", "empty")
	// req.Header.Set("sec-fetch-mode", "cors")
	// req.Header.Set("sec-fetch-site", "same-origin")

	// Referer could theoretically change per video, but static one often works.
	// Ideally construct: https://www.tiktok.com/@<author>/video/<videoID>
	// But we don't know author yet. TikTok API is often lenient with generic Referer.
	req.Header.Set("referer", "https://www.tiktok.com/")
	req.Header.Set("referrer-policy", "strict-origin-when-cross-origin")
	req.Header.Set("user-agent", userAgent)

	// Cookie is critical. Using the hardcoded one that works.
	req.Header.Set("cookie", "delay_guest_mode_vid=5; passport_csrf_token=0e13af79fdc46a732d68cc0fb2265204; passport_csrf_token_default=0e13af79fdc46a732d68cc0fb2265204; cookie-consent={%22optional%22:true%2C%22ga%22:true%2C%22af%22:true%2C%22fbp%22:true%2C%22lip%22:true%2C%22bing%22:true%2C%22ttads%22:true%2C%22reddit%22:true%2C%22hubspot%22:true%2C%22version%22:%22v10%22}; living_user_id=316558428368; g_state={\"i_l\":0,\"i_ll\":1766902363752}; passport_auth_status=d61c0ac16a28e25b4dc4718040b7b4c7%2C; passport_auth_status_ss=d61c0ac16a28e25b4dc4718040b7b4c7%2C; d_ticket=f13113dc3f0065dff71bfa8568d1c1f1ada04; multi_sids=7231173793783251974%3Af8056c41f41b025bd976ecc04e3f747d; cmpl_token=AgQYAPOu_hfkTtK0ZKTnT6JdP_MxUwJZlH-P2WCi_HI; uid_tt=45f624f6a28c71e02c90c6430f8a4fa6c694e4561ec1ed177fa2cded88eed080; uid_tt_ss=45f624f6a28c71e02c90c6430f8a4fa6c694e4561ec1ed177fa2cded88eed080; sid_tt=f8056c41f41b025bd976ecc04e3f747d; sessionid=f8056c41f41b025bd976ecc04e3f747d; sessionid_ss=f8056c41f41b025bd976ecc04e3f747d; store-idc=alisg; store-country-code=kr; store-country-code-src=uid; tt-target-idc=alisg; tt-target-idc-sign=i6golz6OigQvNBmNbvMpB5AzWExVZ0GTRVOHkg_AgRZuHRe5TTPmdYysyCIxfCJIoBGA7onCq4aD_934SaAq5-v5wrCYVGuuEh-hZTBLq9-XyWRxuzU2xo4eAZgBGK8oVFcOggYQ4x8nv-teAIS44j-b9mrLISqsy3n3kxMW_8gL1Fo2CO-ImG_reIEWvmNRkOxVjrYZ4sV6CIaWjIHOKBC5mCnq1dLMPAgS8RiMXMzrJ-wihu0GAaxL9nXEefDHP0-gCOehQxz3eg4mvedw8jHC2b_mEC22T4WrIGI1N1XlHXu_Kuqj_rqlIij6TmvwXUeEGnaq6jJhOWEDZB9RC-PGs45v4uDZ9rGKoLw5P4ODe_tc-jA9Bzz87GoIBJAsdsOlhPbv1OGDkFBTBjdTY9CiOUR9uCUzXEixbnfRGBeduuT2GxjpBuU9oD7FR6dHZrAZrTr9yXE_gVtKDTQYCisJhrR6yhL0gBBrId4W8SSj-Yr_gDEKsMFO2M8cehDs; last_login_method=email; sid_guard=f8056c41f41b025bd976ecc04e3f747d%7C1767083943%7C15551974%7CSun%2C+28-Jun-2026+08%3A38%3A37+GMT; tt_session_tlb_tag=sttt%7C5%7C-AVsQfQbAlvZduzATj90ff________-d3J1Omnvi2FQ_P3odr4yAGf87XJn_F9DAHMOR4NMjPRI%3D; sid_ucp_v1=1.0.1-KDNjNDYwYTdkNGU3YTkzODEwNzFlMTkyNGYzZWY2MTc4NzgxMDZhMGEKGgiGiMK8jMiSrWQQp5_OygYYsws4AUDqB0gEEAMaA3NnMSIgZjgwNTZjNDFmNDFiMDI1YmQ5NzZlY2MwNGUzZjc0N2QyTgogS16Zt6pGh4Kzp-_XbnE7BQJwCuZYASvfonjw2SsAckwSIJvPGc8fM8YfUP1JAEujFoE4lZzIKqG6FK6-IRd4pUKXGAMiBnRpa3Rvaw; ssid_ucp_v1=1.0.1-KDNjNDYwYTdkNGU3YTkzODEwNzFlMTkyNGYzZWY2MTc4NzgxMDZhMGEKGgiGiMK8jMiSrWQQp5_OygYYsws4AUDqB0gEEAMaA3NnMSIgZjgwNTZjNDFmNDFiMDI1YmQ5NzZlY2MwNGUzZjc0N2QyTgogS16Zt6pGh4Kzp-_XbnE7BQJwCuZYASvfonjw2SsAckwSIJvPGc8fM8YfUP1JAEujFoE4lZzIKqG6FK6-IRd4pUKXGAMiBnRpa3Rvaw; tt_chain_token=q3iNBhdAS17cCjv7GZi/cQ==; tt_csrf_token=KQM7VFXM-K8E0X6SSy93tgPmwhGCVRha_uv8; s_v_web_id=verify_mjvc6b0t_DveU8fMQ_UJGE_4bUO_Bkx4_lwVLknixAOZy; perf_feed_cache={%22expireTimestamp%22:1767438000000%2C%22itemIds%22:[%227587068201716944142%22%2C%227586491085908741431%22]}; odin_tt=7a03503ab188d38982e49f4aef5730bb2683a8d323dac1e5e060c3907c72f3a802ac92880bd6609e909a7cdf2db1c9c33e71479f8563c5e92aef39f8395fcb3b914bd2fdfad246e33bbfef2c80f5a0e3; store-country-sign=MEIEDOHFH4qMRRBtbfjgmwQgH3NLB4-CIZyhK-Xmedjdqsn6yY4G1Vh9fseOw4ayMCMEEAUKg9h_K5ZpDV_frBJ4edw; passport_fe_beating_status=false; msToken=w7IMDLNpgcLgWnLzHJg6UDyjSM6JlxNXTgPqVfgA82Zjs4lRWiYwCrQBtPA3I6nNNlV_87dNvgHJKbJwnEYM4y8cA72N-Qq2uNbni3pNCWvrHyrIKNdPwgn3fUQnUYZnVzkJWCLqXiBiOj_6oj3XavNqAq7r; msToken=4Qd8_z-5UQeSe2TddBMBZH8MYxZ9feZ0XdJ7XCkMMy0FvmLADIOwCyvlCuPONMXo-AI_JzyMJ5uS95aC6SOP0yGnScQ7DBKtCAQrz8Ro_ZfLgCQ6nhh7QKDGnhv0piTs-PDc2E1QPxFHfeYYCVXQ9F-OJEyZ; tiktok_webapp_theme_source=light; tiktok_webapp_theme=light; ttwid=1%7CMNnXQ2A4Eb3NcIcfr6Cv2JgAH-khvVLcw0pdrLffgoY%7C1767275004%7C8483d36e676a1cb76303f039406fe44f117743bc82662cd2e8269d83eb74a8b9")

	client := &http.Client{
		Timeout: 30 * time.Second,
	}

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	// fmt.Printf("DEBUG BODY: %s\n", string(body))

	var data VideoInfoResponse
	if err := json.Unmarshal(body, &data); err != nil {
		// Log the body for debugging
		fmt.Printf("GetVideoInfo raw body: %s\n", string(body))
		return nil, fmt.Errorf("json unmarshal failed: %w", err)
	}

	return &data, nil
}

// GetStartCount is a utility wrapper for simple play count retrieval
// GetStartCount is a utility wrapper for simple play count retrieval
func GetStartCount(videoID string) (int64, error) {
	info, err := GetVideoInfo(videoID)
	if err != nil {
		return 0, fmt.Errorf("api error: %w", err)
	}

	// Check ItemInfo (Detail API)
	// We check if we got a valid response structure.
	// If stats are present, we return them.
	// Note: We don't strictly check for ID match here to be lenient with API variations,
	// but effectively we are checking if playCount is populated.
	// Actually, 0 play count is valid for new videos.
	// But usually a valid response will have some structure.

	if info.ItemInfo.ItemStruct.Video.ID != "" {
		return info.ItemInfo.ItemStruct.Stats.PlayCount, nil
	}

	// Check ItemList (Feed API)
	if len(info.ItemList) > 0 {
		return info.ItemList[0].Stats.PlayCount, nil
	}

	return 0, fmt.Errorf("video info not found in response")
}
