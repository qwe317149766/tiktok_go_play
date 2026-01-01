package main

import (
	"encoding/json"
	"strings"
	"time"
)

func GetDeviceMinAgeHours() int {
	// stats 项目：按你的要求【不需要】按时间过滤设备（无论 file/db）。
	// 为避免“设备少但 cookies 多”的不一致，这里统一关闭过滤。
	return 0
}

func extractJSONStringFieldFast(jsonStr string, field string) (string, bool) {
	// 轻量级提取：只支持 string 字段，例如 "create_time":"2025-12-31 12:00:00"
	// 找不到/格式不符合则返回 false，由上层回退到 json.Unmarshal
	if strings.TrimSpace(jsonStr) == "" || strings.TrimSpace(field) == "" {
		return "", false
	}
	key := `"` + field + `"`
	i := strings.Index(jsonStr, key)
	if i < 0 {
		return "", false
	}
	j := strings.Index(jsonStr[i+len(key):], ":")
	if j < 0 {
		return "", false
	}
	k := i + len(key) + j + 1
	for k < len(jsonStr) && (jsonStr[k] == ' ' || jsonStr[k] == '\t' || jsonStr[k] == '\r' || jsonStr[k] == '\n') {
		k++
	}
	if k >= len(jsonStr) || jsonStr[k] != '"' {
		return "", false
	}
	k++
	start := k
	escaped := false
	for k < len(jsonStr) {
		c := jsonStr[k]
		if escaped {
			escaped = false
			k++
			continue
		}
		if c == '\\' {
			escaped = true
			k++
			continue
		}
		if c == '"' {
			return jsonStr[start:k], true
		}
		k++
	}
	return "", false
}

func devicePassMinAge(deviceJSON string, minAgeHours int) bool {
	if minAgeHours <= 0 {
		return true
	}
	deviceJSON = strings.TrimSpace(deviceJSON)
	if deviceJSON == "" {
		return false
	}
	ctStr, ok := extractJSONStringFieldFast(deviceJSON, "create_time")
	if !ok {
		var m map[string]any
		if err := json.Unmarshal([]byte(deviceJSON), &m); err != nil {
			return false
		}
		ctRaw, ok2 := m["create_time"]
		if !ok2 {
			return false
		}
		ctStr, ok2 = ctRaw.(string)
		if !ok2 {
			return false
		}
		ctStr = strings.TrimSpace(ctStr)
	}
	ctStr = strings.TrimSpace(ctStr)
	if ctStr == "" {
		return false
	}
	t, err := time.ParseInLocation("2006-01-02 15:04:05", ctStr, time.Local)
	if err != nil {
		return false
	}
	threshold := time.Now().Add(-time.Duration(minAgeHours) * time.Hour)
	return t.Before(threshold) || t.Equal(threshold)
}


