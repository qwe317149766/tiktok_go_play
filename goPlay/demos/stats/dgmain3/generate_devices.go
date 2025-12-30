package main

import (
	"encoding/json"
	"fmt"
	"math/rand"
	"os"
	"time"
)

// DeviceData 设备信息结构（用于生成设备文件）
type DeviceData struct {
	CreateTime                string `json:"create_time"`
	DeviceID                  string `json:"device_id"`
	InstallID                 string `json:"install_id"`
	UA                        string `json:"ua"`
	WebUA                     string `json:"web_ua"`
	Resolution                string `json:"resolution"`
	DPI                       int    `json:"dpi"`
	DeviceType                string `json:"device_type"`
	DeviceBrand               string `json:"device_brand"`
	DeviceManufacturer        string `json:"device_manufacturer"`
	OSAPI                     int    `json:"os_api"`
	OSVersion                 string `json:"os_version"`
	ResolutionV2              string `json:"resolution_v2"`
	ROM                       string `json:"rom"`
	ROMVersion                string `json:"rom_version"`
	ClientUDID                string `json:"clientudid"`
	GoogleAID                 string `json:"google_aid"`
	ReleaseBuild              string `json:"release_build"`
	DisplayDensityV2          string `json:"display_density_v2"`
	RAMSize                   string `json:"ram_size"`
	DarkModeSettingValue      int    `json:"dark_mode_setting_value"`
	IsFoldable                int    `json:"is_foldable"`
	ScreenHeightDP            int    `json:"screen_height_dp"`
	ScreenWidthDP             int    `json:"screen_width_dp"`
	ApkLastUpdateTime         int64  `json:"apk_last_update_time"`
	ApkFirstInstallTime       int64  `json:"apk_first_install_time"`
	FilterWarn                int    `json:"filter_warn"`
	PriorityRegion            string `json:"priority_region"`
	UserPeriod                int    `json:"user_period"`
	IsKidsMode                int    `json:"is_kids_mode"`
	UserMode                  int    `json:"user_mode"`
	CDID                      string `json:"cdid"`
	OpenUDID                  string `json:"openudid"`
	VersionName               string `json:"version_name"`
	UpdateVersionCode         string `json:"update_version_code"`
	VersionCode               string `json:"version_code"`
	SDKVersionCode            int    `json:"sdk_version_code"`
	SDKTargetVersion          int    `json:"sdk_target_version"`
	SDKVersion                string `json:"sdk_version"`
	TTOkQuicVersion           string `json:"_tt_ok_quic_version"`
	MSSDKVersionStr           string `json:"mssdk_version_str"`
	GorgonSDKVersion          string `json:"gorgon_sdk_version"`
	MSSDKVersion              int    `json:"mssdk_version"`
	DeviceGuardData0          string `json:"device_guard_data0"`
	TTTicketGuardPublicKey    string `json:"tt_ticket_guard_public_key"`
	PrivKey                   string `json:"priv_key"`
}

// 设备型号列表
var deviceTypes = []string{
	"Pixel 6", "Pixel 7", "Pixel 8", "Pixel 6 Pro", "Pixel 7 Pro", "Pixel 8 Pro",
	"Samsung Galaxy S21", "Samsung Galaxy S22", "Samsung Galaxy S23",
	"OnePlus 9", "OnePlus 10", "OnePlus 11",
	"Xiaomi Mi 11", "Xiaomi Mi 12", "Xiaomi Mi 13",
}

var deviceBrands = []string{
	"google", "samsung", "oneplus", "xiaomi", "huawei", "oppo", "vivo",
}

var deviceManufacturers = []string{
	"Google", "Samsung", "OnePlus", "Xiaomi", "Huawei", "OPPO", "Vivo",
}

var resolutions = []string{
	"2209x1080", "2400x1080", "2340x1080", "2376x1080", "2400x1080",
}

var resolutionsV2 = []string{
	"2400x1080", "2340x1080", "2376x1080", "2400x1080", "2209x1080",
}

var dpiValues = []int{420, 440, 480, 560}

var ramSizes = []string{"6GB", "8GB", "12GB"}

var displayDensities = []string{"xxhdpi", "xxxhdpi"}

// generateRandomNumber 生成指定位数的随机数字字符串
func generateRandomNumber(length int) string {
	result := make([]byte, length)
	for i := range result {
		result[i] = byte('0' + rand.Intn(10))
	}
	return string(result)
}

// generateDeviceID 生成device_id（以75开头，19位）
func generateDeviceID() string {
	return "75" + generateRandomNumber(17) // 75 + 17位随机数 = 19位
}

// generateInstallID 生成install_id（19位）
func generateInstallID() string {
	return generateRandomNumber(19)
}

// generateDeviceUUID 生成UUID格式字符串（用于设备生成）
func generateDeviceUUID() string {
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		rand.Uint32(),
		rand.Uint32()&0xffff,
		rand.Uint32()&0xffff,
		rand.Uint32()&0xffff,
		rand.Uint64()&0xffffffffffff)
}

// generateOpenUDID 生成OpenUDID（16位hex）
func generateOpenUDID() string {
	return fmt.Sprintf("%016x", rand.Uint64())
}

// generateCDID 生成CDID（UUID格式）
func generateCDID() string {
	return generateDeviceUUID()
}

// generateDevice 生成单个设备信息
func generateDevice() DeviceData {
	now := time.Now()
	deviceType := deviceTypes[rand.Intn(len(deviceTypes))]
	brand := deviceBrands[rand.Intn(len(deviceBrands))]
	manufacturer := deviceManufacturers[rand.Intn(len(deviceManufacturers))]
	resolution := resolutions[rand.Intn(len(resolutions))]
	resolutionV2 := resolutionsV2[rand.Intn(len(resolutionsV2))]
	dpi := dpiValues[rand.Intn(len(dpiValues))]
	ramSize := ramSizes[rand.Intn(len(ramSizes))]
	displayDensity := displayDensities[rand.Intn(len(displayDensities))]
	
	deviceID := generateDeviceID()
	installID := generateInstallID()
	
	// 生成时间戳（最近30天内）
	daysAgo := rand.Intn(30)
	installTime := now.AddDate(0, 0, -daysAgo).Unix() * 1000
	updateTime := installTime + int64(rand.Intn(86400000)) // 安装后0-1天内更新
	
	// 生成UA
	ua := fmt.Sprintf("com.zhiliaoapp.musically/2024204030 (Linux; U; Android 15; en_US; %s; Build/BP1A.250505.005; Cronet/TTNetVersion:efce646d 2025-10-16 QuicVersion:c785494a 2025-09-30)", deviceType)
	webUA := fmt.Sprintf("Dalvik\\/2.1.0 (Linux; U; Android 15; %s Build\\/BP1A.250505.005)", deviceType)
	
	// 生成ROM版本号
	rom := fmt.Sprintf("%d", 13000000+rand.Intn(1000000))
	
	// 生成device_guard_data0（简化版本，实际应该更复杂）
	deviceGuardData := fmt.Sprintf(`{"device_token":"1|{\"aid\":1233,\"av\":\"42.4.3\",\"did\":\"%s\",\"iid\":\"%s\",\"fit\":\"%d\",\"s\":1,\"idc\":\"useast8\",\"ts\":\"%d\"}","dtoken_sign":"ts.1.MEUCIQCrOUCbHYSH1T9in7GRjy2WGZUEiRY4/U8yx/iSwPV2uAIgJU6loacL5PxoTK82niJo+6e+89oD3fCO777+QFTjicE="}`,
		deviceID, installID, installTime/1000, now.Unix())
	
	// 生成私钥（64位hex）
	privKey := fmt.Sprintf("%064x%032x", rand.Uint64(), rand.Uint32())
	
	return DeviceData{
		CreateTime:            now.Format("2006-01-02 15:04:05"),
		DeviceID:              deviceID,
		InstallID:             installID,
		UA:                    ua,
		WebUA:                 webUA,
		Resolution:            resolution,
		DPI:                   dpi,
		DeviceType:            deviceType,
		DeviceBrand:           brand,
		DeviceManufacturer:    manufacturer,
		OSAPI:                 35,
		OSVersion:             "15",
		ResolutionV2:          resolutionV2,
		ROM:                   rom,
		ROMVersion:            "BP1A.250505.005",
		ClientUDID:            generateDeviceUUID(),
		GoogleAID:             generateDeviceUUID(),
		ReleaseBuild:          "4ca920e_20250626",
		DisplayDensityV2:      displayDensity,
		RAMSize:               ramSize,
		DarkModeSettingValue:  rand.Intn(2),
		IsFoldable:            0,
		ScreenHeightDP:        800 + rand.Intn(200),
		ScreenWidthDP:         360 + rand.Intn(100),
		ApkLastUpdateTime:     updateTime,
		ApkFirstInstallTime:   installTime,
		FilterWarn:            0,
		PriorityRegion:        "US",
		UserPeriod:            0,
		IsKidsMode:            0,
		UserMode:              1,
		CDID:                  generateCDID(),
		OpenUDID:              generateOpenUDID(),
		VersionName:           "42.4.3",
		UpdateVersionCode:     "2024204030",
		VersionCode:           "420403",
		SDKVersionCode:        2051090,
		SDKTargetVersion:      30,
		SDKVersion:            "2.5.10",
		TTOkQuicVersion:       "Cronet/TTNetVersion:efce646d 2025-10-16 QuicVersion:c785494a 2025-09-30",
		MSSDKVersionStr:       "v05.02.02-ov-android",
		GorgonSDKVersion:      "0000000020020205",
		MSSDKVersion:          84017696,
		DeviceGuardData0:      deviceGuardData,
		TTTicketGuardPublicKey: "BN7u7VUOP55h2Wz+o+j2Mt28VWDtk/THuigzCgniBSvOFdRvEWojy9hu+NAFMrm6m/QjFDBZnBgUGXx6UPzKF10=",
		PrivKey:               privKey,
	}
}

// GenerateDevicesFile 生成设备文件
func GenerateDevicesFile(filename string, count int) error {
	// 初始化随机种子
	rand.Seed(time.Now().UnixNano())
	
	file, err := os.Create(filename)
	if err != nil {
		return fmt.Errorf("创建文件失败: %w", err)
	}
	defer file.Close()
	
	// 生成设备并写入文件
	for i := 0; i < count; i++ {
		device := generateDevice()
		jsonData, err := json.Marshal(device)
		if err != nil {
			return fmt.Errorf("序列化设备失败: %w", err)
		}
		
		_, err = file.WriteString(string(jsonData) + "\n")
		if err != nil {
			return fmt.Errorf("写入文件失败: %w", err)
		}
		
		if (i+1)%100 == 0 {
			fmt.Printf("已生成 %d/%d 个设备\n", i+1, count)
		}
	}
	
	fmt.Printf("成功生成 %d 个设备到 %s\n", count, filename)
	return nil
}

// 如果直接运行此文件，生成设备
func init() {
	// 这个函数会在包初始化时执行，但不会自动生成
	// 需要通过调用 GenerateDevicesFile 来生成
}

