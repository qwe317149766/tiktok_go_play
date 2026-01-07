package config

import (
	"encoding/json"
	"os"
)

type AppConfig struct {
	Language  string `json:"language"`
	CardCode  string `json:"card_code"`
	SmsFile   string `json:"sms_file"`
	EmailFile string `json:"email_file"`

	// Paths
	CookiePath   string `json:"cookie_path"`    // 账号+Cookie 主输出
	SuccessPath  string `json:"success_path"`   // 2FA 成功专用
	FailurePath  string `json:"failure_path"`   // 2FA 失败专用
	TwoFactorDir string `json:"two_factor_dir"` // 备份

	Concurrency       int    `json:"concurrency"`
	Auto2FA           bool   `json:"auto_2fa"`
	RegistrationMode  string `json:"reg_mode"`
	PushURL           string `json:"push_url"`
	MaxRegCount       int    `json:"max_reg_count"`
	MaxPhoneUsage     int    `json:"max_phone_usage"`
	FontSize          int    `json:"font_size"`
	ThemeColor        string `json:"theme_color"`
	ProxyFile         string `json:"proxy_file"`
	MaxSuccessPerFile int    `json:"max_success_per_file"`

	PollTimeoutSec        int  `json:"poll_timeout_sec"`
	SMSWaitTimeoutSec     int  `json:"sms_wait_timeout_sec"`
	FinalizeRetries       int  `json:"finalize_retries"`
	EnableHeaderRotation  bool `json:"enable_header_rotation"`
	EnableAnomalousUA     bool `json:"enable_anomalous_ua"`
	EnableIOS             bool `json:"enable_ios"`
	HttpRequestTimeoutSec int  `json:"http_request_timeout_sec"`
}

func GetDefaultConfig() *AppConfig {
	return &AppConfig{
		Language:              "zh-CN",
		CookiePath:            "./success_cookies",
		SuccessPath:           "./success_2fa",
		FailurePath:           "./failed_2fa",
		TwoFactorDir:          "./backup_2fa",
		Concurrency:           10,
		Auto2FA:               true,
		RegistrationMode:      "sms",
		PushURL:               "",
		MaxRegCount:           0,
		MaxPhoneUsage:         1,
		FontSize:              14,
		ThemeColor:            "indigo",
		ProxyFile:             "",
		PollTimeoutSec:        300,
		SMSWaitTimeoutSec:     120,
		FinalizeRetries:       20,
		EnableHeaderRotation:  true,
		EnableAnomalousUA:     false,
		EnableIOS:             false,
		HttpRequestTimeoutSec: 30,
	}
}

func (c *AppConfig) Save() error {
	data, _ := json.MarshalIndent(c, "", "  ")
	return os.WriteFile("gui_config.json", data, 0644)
}

func LoadConfig() *AppConfig {
	data, err := os.ReadFile("gui_config.json")
	if err != nil {
		return GetDefaultConfig()
	}
	conf := GetDefaultConfig()
	json.Unmarshal(data, conf)

	if conf.PollTimeoutSec <= 0 {
		conf.PollTimeoutSec = 300
	}
	if conf.SMSWaitTimeoutSec <= 0 {
		conf.SMSWaitTimeoutSec = 120
	}
	if conf.HttpRequestTimeoutSec <= 0 {
		conf.HttpRequestTimeoutSec = 30
	}
	if conf.FinalizeRetries <= 0 {
		conf.FinalizeRetries = 20
	}

	return conf
}
