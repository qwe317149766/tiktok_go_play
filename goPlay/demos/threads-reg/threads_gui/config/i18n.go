package config

type LanguagePack struct {
	LoginTitle       string `json:"login_title"`
	LoginDesc        string `json:"login_desc"`
	CardCode         string `json:"card_code"`
	LoginBtn         string `json:"login_btn"`
	HardwareID       string `json:"hardware_id"`
	ExpiryDate       string `json:"expiry_date"`
	Logout           string `json:"logout"`
	Settings         string `json:"settings"`
	MenuExecution    string `json:"menu_execution"`
	MenuParams       string `json:"menu_params"`
	MenuArchives     string `json:"menu_archives"`
	MenuAppSettings  string `json:"menu_app_settings"`
	LangSelect       string `json:"lang_select"`
	StatsSuccess     string `json:"stats_success"`
	StatsFailed      string `json:"stats_failed"`
	StatsTotal       string `json:"stats_total"`
	SmsMode          string `json:"sms_mode"`
	EmailMode        string `json:"email_mode"`
	Run              string `json:"run"`
	SourceData       string `json:"source_data"`
	OutputConfig     string `json:"output_config"`
	SelectFile       string `json:"select_file"`
	SavePath         string `json:"save_path"`
	Logs             string `json:"logs"`
	LogsClear        string `json:"logs_clear"`
	LogsEmpty        string `json:"logs_empty"`
	SuccessFolder    string `json:"success_folder"`
	FailureFolder    string `json:"failure_folder"`
	CookieFolder     string `json:"cookie_folder"`
	TwoFactorFolder  string `json:"two_factor_folder"`
	APIPushURL       string `json:"api_push_url"`
	MaxRegLimit      string `json:"max_reg_limit"`
	MaxPhoneUsage    string `json:"max_phone_usage"`
	SaveConfig       string `json:"save_config"`
	ParamDesc        string `json:"param_desc"`
	EnginePerf       string `json:"engine_perf"`
	EnginePerfDesc   string `json:"engine_perf_desc"`
	ParallelWorkers  string `json:"parallel_workers"`
	Auto2FASec       string `json:"auto_2fa_sec"`
	Auto2FADesc      string `json:"auto_2fa_desc"`
	TaskSafety       string `json:"task_safety"`
	TaskSafetyDesc   string `json:"task_safety_desc"`
	ExternalInt      string `json:"external_int"`
	ExternalIntDesc  string `json:"external_int_desc"`
	StoragePaths     string `json:"storage_paths"`
	StoragePathsDesc string `json:"storage_paths_desc"`
	FontSize         string `json:"font_size"`
	ThemeColor       string `json:"theme_color"`
	SuccessPathTitle string `json:"success_path_title"`
	FailurePathTitle string `json:"failure_path_title"`
	CookiePathTitle  string `json:"cookie_path_title"`
	UnderDev         string `json:"under_dev"`
	ProxySource      string `json:"proxy_source"`

	// Alerts
	AlertTaskCompleted      string `json:"alert_task_completed"`
	AlertLoginFailed        string `json:"alert_login_failed"`
	AlertSelectFile         string `json:"alert_select_file"`
	AlertSettingsSaved      string `json:"alert_settings_saved"`
	AlertSettingsSaveFailed string `json:"alert_settings_save_failed"`
	AlertAPISuccess         string `json:"alert_api_success"`
	AlertAPIFailed          string `json:"alert_api_failed"`
	ConfirmClearStats       string `json:"confirm_clear_stats"`
	ConfirmDeleteFile       string `json:"confirm_delete_file"`
}

var Languages = map[string]LanguagePack{
	"zh-CN": {
		LoginTitle:              "IG Registration Assistant",
		LoginDesc:               "请激活您的订阅以继续使用",
		CardCode:                "请输入卡密",
		LoginBtn:                "激活并登录",
		HardwareID:              "硬件 ID:",
		ExpiryDate:              "有效期至",
		Logout:                  "退出登录",
		Settings:                "配置面板",
		MenuExecution:           "运行任务",
		MenuParams:              "参数设置",
		MenuArchives:            "历史记录",
		MenuAppSettings:         "软件设置",
		LangSelect:              "语言选择",
		StatsSuccess:            "成功数",
		StatsFailed:             "失败数",
		StatsTotal:              "累计注册",
		SmsMode:                 "短信注册",
		EmailMode:               "邮箱注册",
		Run:                     "启动引擎",
		SourceData:              "源数据文件",
		OutputConfig:            "输出配置",
		SelectFile:              "选择文件",
		SavePath:                "保存目录",
		Logs:                    "引擎实时日志",
		LogsClear:               "清空",
		LogsEmpty:               "控制台就绪，等待监控信号...",
		SuccessFolder:           "2FA 成功目录",
		FailureFolder:           "2FA 失败目录",
		CookieFolder:            "Cookies 成功目录",
		TwoFactorFolder:         "2FA 备份目录",
		APIPushURL:              "API 推送地址",
		MaxRegLimit:             "最大注册数量",
		MaxPhoneUsage:           "手机号最大使用次数",
		SaveConfig:              "保存配置",
		ParamDesc:               "配置全局引擎行为、安全限制和数据端点。",
		EnginePerf:              "引擎性能",
		EnginePerfDesc:          "调整注册引擎处理并发任务和安全层的方式。",
		ParallelWorkers:         "并行工作线程",
		Auto2FASec:              "自动 2FA 安全",
		Auto2FADesc:             "注册后立即开启",
		TaskSafety:              "操作安全",
		TaskSafetyDesc:          "通过设置严格的运行边界，防止账号被封禁并控制成本。",
		ExternalInt:             "外部集成",
		ExternalIntDesc:         "通过 Webhooks 实时将成功事件推送到您的 CRM 或后端系统。",
		StoragePaths:            "存储与路径",
		StoragePathsDesc:        "定义机器存储生成的帐户、会话和备份机密的位置。",
		FontSize:                "全局字体大小",
		ThemeColor:              "皮肤颜色",
		SuccessPathTitle:        "2FA 成功路径",
		FailurePathTitle:        "2FA 失败路径",
		CookiePathTitle:         "Cookies 成功路径",
		UnderDev:                "该功能正在开发中...",
		ProxySource:             "代理来源 (文件/URL)",
		AlertTaskCompleted:      "任务已完成：达到最大限制 %d",
		AlertLoginFailed:        "登录失败: ",
		AlertSelectFile:         "请选择输入文件",
		AlertSettingsSaved:      "设置保存成功",
		AlertSettingsSaveFailed: "保存设置失败",
		AlertAPISuccess:         "API 连接成功！",
		AlertAPIFailed:          "API 连接失败，请检查日志。",
		ConfirmClearStats:       "确定要清空所有统计数据吗？",
		ConfirmDeleteFile:       "确定要删除这个文件吗？",
	},
	"en-US": {
		LoginTitle:              "IG Registration Assistant",
		LoginDesc:               "Activate your subscription to continue",
		CardCode:                "Card Key",
		LoginBtn:                "Activate & Login",
		HardwareID:              "HARDWARE ID:",
		ExpiryDate:              "Expires At",
		Logout:                  "Logout",
		Settings:                "Settings",
		MenuExecution:           "Execution",
		MenuParams:              "Parameters",
		MenuArchives:            "Archives",
		MenuAppSettings:         "App Settings",
		LangSelect:              "Language",
		StatsSuccess:            "SUCCESS",
		StatsFailed:             "FAILED",
		StatsTotal:              "TOTAL REG",
		SmsMode:                 "SMS Mode",
		EmailMode:               "Email Mode",
		Run:                     "RUN ENGINE",
		SourceData:              "Source Data",
		OutputConfig:            "Output Config",
		SelectFile:              "Select",
		SavePath:                "Save Path",
		Logs:                    "Live Engine Logs",
		LogsClear:               "Clear",
		LogsEmpty:               "Console ready. Monitoring signals...",
		SuccessFolder:           "2FA Success Path",
		FailureFolder:           "2FA Failure Path",
		CookieFolder:            "Cookies Success Path",
		TwoFactorFolder:         "2FA Backup Path",
		APIPushURL:              "API Push URL",
		MaxRegLimit:             "Max Registration Limit",
		MaxPhoneUsage:           "Max Phone Usage",
		SaveConfig:              "Save Configuration",
		ParamDesc:               "Configure global engine behavior, safety limits, and data endpoints.",
		EnginePerf:              "Engine Performance",
		EnginePerfDesc:          "Adjust how the registration engine handles concurrent tasks and security layers.",
		ParallelWorkers:         "Parallel Workers",
		Auto2FASec:              "Auto 2FA Security",
		Auto2FADesc:             "Enable after registration",
		TaskSafety:              "Task Safety",
		TaskSafetyDesc:          "Prevent account flags and excessive costs by setting strict operational boundaries.",
		ExternalInt:             "External Integration",
		ExternalIntDesc:         "Push success events to your CRM or backend system in real-time via Webhooks.",
		StoragePaths:            "Storage & Paths",
		StoragePathsDesc:        "Define where the machine stores generated accounts, sessions, and backup secrets.",
		FontSize:                "Global Font Size",
		ThemeColor:              "Theme Color",
		SuccessPathTitle:        "2FA SUCCESS PATH",
		FailurePathTitle:        "2FA FAILURE PATH",
		CookiePathTitle:         "COOKIES SUCCESS PATH",
		UnderDev:                "This feature is under development...",
		ProxySource:             "Proxy Source (File/URL)",
		AlertTaskCompleted:      "Task Completed: Max limit of %d reached.",
		AlertLoginFailed:        "Login Failed: ",
		AlertSelectFile:         "Please select input file",
		AlertSettingsSaved:      "Settings saved successfully",
		AlertSettingsSaveFailed: "Failed to save settings",
		AlertAPISuccess:         "API Connection Successful!",
		AlertAPIFailed:          "API Connection Failed. Please check the logs.",
		ConfirmClearStats:       "Are you sure you want to clear all statistics?",
		ConfirmDeleteFile:       "Are you sure you want to delete this file?",
	},
	"ru-RU": {
		LoginTitle:              "IG Registration Assistant",
		LoginDesc:               "Активируйте подписку, чтобы продолжить",
		CardCode:                "Ключ карты",
		LoginBtn:                "Активировать",
		HardwareID:              "ID ОБОРУДОВАНИЯ:",
		ExpiryDate:              "Истекает в",
		Logout:                  "Выйти",
		Settings:                "Настройки",
		MenuExecution:           "Выпуск",
		MenuParams:              "Параметры",
		MenuArchives:            "Архивы",
		MenuAppSettings:         "Настройки ПО",
		LangSelect:              "Язык",
		StatsSuccess:            "УСПЕХ",
		StatsFailed:             "ОШИБКА",
		StatsTotal:              "ВСЕГО РЕГ",
		SmsMode:                 "SMS Режим",
		EmailMode:               "Email Режим",
		Run:                     "ЗАПУСК",
		SourceData:              "Данные",
		OutputConfig:            "Конфигурация",
		SelectFile:              "Выбрать",
		SavePath:                "Путь",
		Logs:                    "Логи двигателя",
		LogsClear:               "Очистить",
		LogsEmpty:               "Консоль готова. Ожидание сигналов...",
		SuccessFolder:           "2FA Путь Успеха",
		FailureFolder:           "2FA Путь Неудачи",
		CookieFolder:            "Путь Cookies",
		TwoFactorFolder:         "2FA Резерв ПУТЬ",
		APIPushURL:              "API Push URL",
		MaxRegLimit:             "Макс. Лимит",
		MaxPhoneUsage:           "Макс. Использование",
		SaveConfig:              "Сохранить",
		ParamDesc:               "Конфигурация поведения движка и лимитов.",
		EnginePerf:              "Производительность",
		EnginePerfDesc:          "Настройка параллельных задач.",
		ParallelWorkers:         "Рабочие",
		Auto2FASec:              "Авто 2FA",
		Auto2FADesc:             "Включить после регистрации",
		TaskSafety:              "Безопасность",
		TaskSafetyDesc:          "Предотвращение флагов аккаунтов.",
		ExternalInt:             "Интеграция",
		ExternalIntDesc:         "Push-уведомления через Webhooks.",
		StoragePaths:            "Хранилище",
		StoragePathsDesc:        "Места хранения данных.",
		FontSize:                "Размер шрифта",
		ThemeColor:              "Цвет Темы",
		SuccessPathTitle:        "2FA ПУТЬ УСПЕХА",
		FailurePathTitle:        "2FA ПУТЬ НЕУДАЧИ",
		CookiePathTitle:         "ПУТЬ COOKIES",
		UnderDev:                "В разработке...",
		ProxySource:             "Источник прокси (Файл/URL)",
		AlertTaskCompleted:      "Задание выполнено: достигнут лимит %d",
		AlertLoginFailed:        "Ошибка входа: ",
		AlertSelectFile:         "Пожалуйста, выберите входной файл",
		AlertSettingsSaved:      "Настройки успешно сохранены",
		AlertSettingsSaveFailed: "Не удалось сохранить настройки",
		AlertAPISuccess:         "Соединение с API успешно!",
		AlertAPIFailed:          "Ошибка соединения с API. Проверьте логи.",
		ConfirmClearStats:       "Вы уверены, что хотите очистить всю статистику?",
		ConfirmDeleteFile:       "Вы уверены, что хотите удалить этот файл?",
	},
}
