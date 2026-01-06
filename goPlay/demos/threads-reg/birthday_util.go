package main

import (
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"time"
)

// GeoInfo represents the response from the IP geolocation API
type GeoInfo struct {
	IP       string `json:"ip"`
	Timezone string `json:"timezone"`
}

// BirthdayUtils provides utility functions for birthday-related calculations
type BirthdayUtils struct{}

// GetTikTokBirthdayTimestamp calculates the TikTok-style timestamp based on birthday and timezone
// birthdayStr: "DD-MM-YYYY"
// timezoneStr: e.g., "Asia/Yerevan"
func (u BirthdayUtils) GetTikTokBirthdayTimestamp(birthdayStr string, timezoneStr string) (int64, error) {
	// 1. 解析生日字符串 DD-MM-YYYY
	layout := "02-01-2006"
	birthday, err := time.Parse(layout, birthdayStr)
	if err != nil {
		return 0, fmt.Errorf("解析生日失败: %v", err)
	}

	// 2. 加载时区
	loc, err := time.LoadLocation(timezoneStr)
	if err != nil {
		// 如果时区加载失败，回退到 UTC 或报警
		return 0, fmt.Errorf("加载时区失败: %v", err)
	}

	// 3. 构造本地时间 00:00:00
	birthdayLocal := time.Date(
		birthday.Year(),
		birthday.Month(),
		birthday.Day(),
		0, 0, 0, 0,
		loc,
	)

	// 4. TikTok 固定秒偏移 (根据 1.go 中的逻辑)
	// 这个值 355 可能是硬编码的特定偏移量
	tiktokOffset := int64(355)

	// 5. 返回最终 timestamp
	return birthdayLocal.Unix() + tiktokOffset, nil
}

// GenerateRandomBirthday generates a random birthday
// for ages between minAge and maxAge (inclusive).
// Returns day, month, year separately.
func (u BirthdayUtils) GenerateRandomBirthday(minAge, maxAge int) (int, int, int) {
	now := time.Now()

	startYear := now.Year() - maxAge
	endYear := now.Year() - minAge

	r := rand.New(rand.NewSource(time.Now().UnixNano()))

	year := r.Intn(endYear-startYear+1) + startYear
	month := r.Intn(12) + 1
	day := r.Intn(28) + 1

	return day, month, year
}

// GetTimestampByBirthdayAndTZ is a static-like helper if a struct instance isn't desired
func GetTimestampByBirthdayAndTZ(dateStr string, tz string) (int64, error) {
	// 生日格式：DD-MM-YYYY
	layout := "02-01-2006"

	// 加载时区
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return 0, err
	}

	// 在指定时区解析生日（默认 00:00:00）
	t, err := time.ParseInLocation(layout, dateStr, loc)
	if err != nil {
		return 0, err
	}

	// 返回 Unix 秒数（UTC）
	return t.Unix(), nil
}

// GenerateRandomBirthday is a static-like helper
func GenerateRandomBirthday(minAge, maxAge int) (int, int, int) {
	return BirthdayUtils{}.GenerateRandomBirthday(minAge, maxAge)
}

// GetIPAndTimezone fetches the current public IP and timezone from multiple APIs with polling until success
func GetIPAndTimezone(proxyURL string) (*GeoInfo, error) {
	// Added more reliable and diverse endpoints
	endpoints := []string{
		"http://ip-api.com/json/",
		"https://ipapi.co/json/",
		"https://ipinfo.io/json",
		"https://freeipapi.com/api/json",
	}

	transport := &http.Transport{
		DisableKeepAlives: true, // Prevent socket leakage during polling
	}
	if proxyURL != "" {
		u, err := url.Parse(proxyURL)
		if err == nil {
			transport.Proxy = http.ProxyURL(u)
		}
	}

	// Increased timeout to 30s for better proxy compatibility
	client := &http.Client{
		Timeout:   30 * time.Second,
		Transport: transport,
	}

	for {
		for _, targetUrl := range endpoints {
			// req construction...
			req, err := http.NewRequest("GET", targetUrl, nil)
			if err != nil {
				AsyncLog(fmt.Sprintf("[GEO][Warning] Failed to create request for %s: %v", targetUrl, err))
				continue
			}

			// Some APIs require a User-Agent or return 403/timeout
			req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")

			geoSem <- struct{}{}
			resp, err := client.Do(req)
			<-geoSem
			if err != nil {
				AsyncLog(fmt.Sprintf("[GEO][Warning] Failed to fetch from %s: %v", targetUrl, err))
				continue
			}

			body, err := io.ReadAll(resp.Body)
			resp.Body.Close()
			if err != nil {
				AsyncLog(fmt.Sprintf("[GEO][Warning] Failed to read response from %s: %v", targetUrl, err))
				continue
			}

			var rawMap map[string]interface{}
			if err := json.Unmarshal(body, &rawMap); err != nil {
				AsyncLog(fmt.Sprintf("[GEO][Warning] Failed to parse JSON from %s: %v", targetUrl, err))
				continue
			}

			var info GeoInfo
			// Flexible mapping for different API responses
			// IP mapping
			if ip, ok := rawMap["query"].(string); ok { // ip-api.com
				info.IP = ip
			} else if ip, ok := rawMap["ip"].(string); ok { // ipapi.co, ipinfo.io
				info.IP = ip
			} else if ip, ok := rawMap["ipAddress"].(string); ok { // freeipapi.com
				info.IP = ip
			}

			// Timezone mapping
			if tz, ok := rawMap["timezone"].(string); ok { // ip-api.com, ipapi.co, ipinfo.io
				info.Timezone = tz
			} else if tz, ok := rawMap["timeZone"].(string); ok { // freeipapi.com
				info.Timezone = tz
			}

			if info.IP != "" && info.Timezone != "" {
				AsyncLog(fmt.Sprintf("[GEO][Success] GeoInfo acquired: IP=%s, TZ=%s", info.IP, info.Timezone))
				return &info, nil
			}
			AsyncLog(fmt.Sprintf("[GEO][Warning] Incomplete GeoInfo from %s. Response: %s", targetUrl, string(body)))
		}

		AsyncLog("[GEO][Wait] All GeoInfo endpoints failed. Retrying in 5 seconds...")
		time.Sleep(5 * time.Second)
	}
}
