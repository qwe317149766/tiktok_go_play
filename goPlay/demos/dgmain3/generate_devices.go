package main

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"time"
)

func init() {
	rand.Seed(time.Now().UnixNano())
}

// GenerateDevicesFile 生成设备文件
func GenerateDevicesFile(filename string, count int) error {
	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("创建设备文件失败: %w", err)
	}
	defer file.Close()

	for i := 0; i < count; i++ {
		device := generateRandomDevice()
		data, err := json.Marshal(device)
		if err != nil {
			continue
		}
		file.WriteString(string(data) + "\n")
	}

	return nil
}

// generateRandomDevice 生成随机设备
func generateRandomDevice() map[string]interface{} {
	deviceID := fmt.Sprintf("%d", rand.Int63n(9000000000000000000)+1000000000000000000)
	installID := fmt.Sprintf("%d", rand.Int63n(9000000000000000000)+1000000000000000000)

	now := time.Now().Unix() * 1000
	firstInstallTime := now - rand.Int63n(86400000*30) // 随机30天内
	lastUpdateTime := firstInstallTime + rand.Int63n(86400000*7) // 安装后7天内更新

	device := map[string]interface{}{
		"device_id":              deviceID,
		"install_id":             installID,
		"device_type":            "Pixel 6",
		"device_brand":           "google",
		"os_version":             "15",
		"os_api":                 35,
		"ua":                     "com.zhiliaoapp.musically/2024204030 (Linux; U; Android 15; en_US; Pixel 6; Build/AP3A.241005.015.A2; Cronet/TTNetVersion:1d04069f 2024-09-12 QuicVersion:721c671c 2024-09-30)",
		"apk_first_install_time": float64(firstInstallTime),
		"apk_last_update_time":   float64(lastUpdateTime),
		"priv_key":               generateRandomHex(64),
		"device_guard_data0":     "{}",
	}

	return device
}

// generateRandomHex 生成随机十六进制字符串
func generateRandomHex(length int) string {
	const hexChars = "0123456789abcdef"
	result := make([]byte, length)
	for i := range result {
		result[i] = hexChars[rand.Intn(len(hexChars))]
	}
	return string(result)
}

