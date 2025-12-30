package main

import (
	"strings"
	"time"
)

func getSignupDeviceMinAgeHours() int {
	// 设备最小“年龄”（小时）：只使用 create_time 早于 now-Nh 的设备
	// 0=不筛选
	if v := getEnvInt("SIGNUP_DEVICE_MIN_AGE_HOURS", 0); v > 0 {
		return v
	}
	if v := getEnvInt("DEVICE_MIN_AGE_HOURS", 0); v > 0 {
		return v
	}
	return 0
}

func deviceCreateTimeOK(device map[string]interface{}, minAgeHours int) bool {
	if minAgeHours <= 0 {
		return true
	}
	if device == nil {
		return false
	}
	raw, ok := device["create_time"]
	if !ok {
		return false
	}
	s, ok := raw.(string)
	if !ok {
		return false
	}
	s = strings.TrimSpace(s)
	if s == "" {
		return false
	}
	t, err := time.ParseInLocation("2006-01-02 15:04:05", s, time.Local)
	if err != nil {
		return false
	}
	threshold := time.Now().Add(-time.Duration(minAgeHours) * time.Hour)
	return t.Before(threshold) || t.Equal(threshold)
}

func filterDevicesByMinAge(devices []map[string]interface{}, minAgeHours int) []map[string]interface{} {
	if minAgeHours <= 0 {
		return devices
	}
	out := make([]map[string]interface{}, 0, len(devices))
	for _, d := range devices {
		if deviceCreateTimeOK(d, minAgeHours) {
			out = append(out, d)
		}
	}
	return out
}


