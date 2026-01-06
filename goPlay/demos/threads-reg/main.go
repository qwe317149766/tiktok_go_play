package main

import (
	"bytes"
	"compress/gzip"
	crand "crypto/rand"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"
	"unsafe"

	"github.com/google/uuid"
)

// ThreadsRequestConfig holds all configuration for a Threads API request
type ThreadsRequestConfig struct {
	DeviceID        string
	AndroidID       string
	FamilyDeviceID  string
	WaterfallID     string
	InnerParams     string               // The inner "params" string inside variables
	BloksAppID      string               // The app_id in variables
	FriendlyName    string               // Friendly name for headers and body
	HeaderOverrides map[string]string    // Custom headers to add or override
	FormOverrides   map[string]string    // Custom form fields to add or override
	VariablesJSON   string               // If set, replaces the default variables construction
	ProxyURL        string               // Proxy URL to use for the request (optional if client is provided)
	HeaderManager   *ThreadHeaderManager // Reuse an existing manager (singleton in loop)
	HTTPClient      *http.Client         // Reuse a client for keep-alive
	PhoneNumber     string               // For logging
	WorkerID        int                  // For logging
}

// ExecuteThreadsAPI executes a generic Bloks API request with full flexibility
func ExecuteThreadsAPI(cfg ThreadsRequestConfig) (string, http.Header, error) {
	apiUrl := "https://i.instagram.com/graphql_www"

	// 1. Header Management
	hm := cfg.HeaderManager
	if hm == nil {
		hm = NewThreadHeaderManager()
		hm.SetIDs(cfg.DeviceID, cfg.AndroidID, cfg.FamilyDeviceID, cfg.WaterfallID)
		hm.RandomizeUserAgent()
	}

	hm.FriendlyName = cfg.FriendlyName
	hm.BloksAppID = cfg.BloksAppID
	// hm.RandomizeUserAgent() // Don't randomize on every call anymore

	// 2. Body Construction
	var variables string
	if cfg.VariablesJSON != "" {
		variables = cfg.VariablesJSON
	} else {
		variables = fmt.Sprintf(`{"params":{"params":%q,"bloks_versioning_id":"%s","infra_params":{"device_id":"%s"},"app_id":"%s"},"bk_context":{"is_flipper_enabled":false,"theme_params":[],"debug_tooling_metadata_token":null}}`,
			cfg.InnerParams, hm.BloksVersionID, hm.DeviceID, cfg.BloksAppID)
	}

	// Manual construction to maintain order as requested:
	// method=post&pretty=false&format=json&server_timestamps=true&locale=user&purpose=fetch&fb_api_req_friendly_name=...&client_doc_id=...&enable_canonical_naming=true&enable_canonical_variable_overrides=true&enable_canonical_naming_ambiguous_type_prefixing=true&variables=...

	// Base parameters in order
	rawPayload := fmt.Sprintf("method=post&pretty=false&format=json&server_timestamps=true&locale=user&purpose=fetch&fb_api_req_friendly_name=%s&client_doc_id=%s&enable_canonical_naming=true&enable_canonical_variable_overrides=true&enable_canonical_naming_ambiguous_type_prefixing=true",
		url.QueryEscape(cfg.FriendlyName),
		url.QueryEscape(hm.ClientDocID))

	// Add Form Overrides if any (except variables which is handled separately)
	for k, v := range cfg.FormOverrides {
		if k != "variables" {
			rawPayload += fmt.Sprintf("&%s=%s", url.QueryEscape(k), url.QueryEscape(v))
		}
	}

	// Always put variables at the end
	rawPayload += fmt.Sprintf("&variables=%s", url.QueryEscape(variables))

	// 3. Request Preparation
	var payload io.Reader
	// Apply Header Overrides to Manager before Apply or compression check
	for k, v := range cfg.HeaderOverrides {
		hm.SetHeader(k, v)
	}

	// Check if request should be compressed
	if hm.Headers["content-encoding"] == "gzip" {
		var buf bytes.Buffer
		gz := gzip.NewWriter(&buf)
		gz.Write([]byte(rawPayload))
		gz.Close()
		payload = bytes.NewReader(buf.Bytes())
	} else {
		payload = strings.NewReader(rawPayload)
	}

	req, err := http.NewRequest("POST", apiUrl, payload)
	if err != nil {
		return "", nil, err
	}

	hm.Apply(req)

	// 4. Send Request
	apiSem <- struct{}{}
	defer func() { <-apiSem }()

	client := cfg.HTTPClient
	if client == nil {
		transport := &http.Transport{}
		if cfg.ProxyURL != "" {
			u, err := url.Parse(cfg.ProxyURL)
			if err == nil {
				transport.Proxy = http.ProxyURL(u)
			}
		}
		client = &http.Client{
			Timeout:   30 * time.Second,
			Transport: transport,
		}
	}
	res, err := client.Do(req)
	if err != nil {
		return "", nil, err
	}
	defer res.Body.Close()

	var reader io.ReadCloser
	var errBody error
	switch res.Header.Get("Content-Encoding") {
	case "gzip":
		reader, errBody = gzip.NewReader(res.Body)
		if errBody != nil {
			return "", nil, errBody
		}
		defer reader.Close()
	default:
		reader = res.Body
	}

	body, err := io.ReadAll(reader)
	if err != nil {
		return "", nil, err
	}

	// Log request/response asynchronously
	AsyncLog(fmt.Sprintf("[w%d] [%s] %s\nURL: %s\nPAYLOAD: %s\nRESPONSE: %s",
		cfg.WorkerID, cfg.PhoneNumber, cfg.BloksAppID, apiUrl, rawPayload, string(body)))

	if res.StatusCode != 200 {
		return string(body), res.Header, fmt.Errorf("bad status code: %d", res.StatusCode)
	}

	return string(body), res.Header, nil
}

// ExecuteMobileConfig executes the sessionless mobileconfig request
func ExecuteMobileConfig(client *http.Client, hm *ThreadHeaderManager, deviceID, sessionID string, workerID int) (string, http.Header, error) {
	apiUrl := "https://b.i.instagram.com/api/v1/launcher/mobileconfig/"

	hm.Headers["host"] = "b.i.instagram.com"
	hm.FriendlyName = "IgApi: launcher/mobileconfig/sessionless"
	// User-Agent is already randomized if hm is reused

	// Data for signed_body
	data := map[string]any{
		"bool_opt_policy":         "0",
		"mobileconfigsessionless": "",
		"api_version":             "10",
		"unit_type":               "1",
		"use_case":                "STANDARD",
		"query_hash":              "00206ccbc326cc23a5645dd4e586f1eaa7be8b123fe5508ae6f384caac8406dd",
		"tier":                    "-1",
		"device_id":               deviceID,
		"fetch_mode":              "CONFIG_SYNC_ONLY",
		"fetch_type":              "SYNC_FULL",
		"family_device_id":        "EMPTY_FAMILY_DEVICE_ID",
		"unauthenticated_id":      deviceID,
		"capabilities":            "3brTv10=",
	}

	jsonData, _ := json.Marshal(data)
	payloadStr := "signed_body=SIGNATURE." + string(jsonData)

	var payload io.Reader
	if hm.Headers["content-encoding"] == "gzip" {
		var buf bytes.Buffer
		gz := gzip.NewWriter(&buf)
		gz.Write([]byte(payloadStr))
		gz.Close()
		payload = bytes.NewReader(buf.Bytes())
	} else {
		payload = strings.NewReader(payloadStr)
	}

	req, err := http.NewRequest("POST", apiUrl, payload)
	if err != nil {
		return "", nil, err
	}

	hm.Apply(req)
	// Specific headers for mobileconfig
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=UTF-8")
	req.Header.Set("x-ig-app-id", "3419628305025917")

	// 4. Send Request
	apiSem <- struct{}{}
	defer func() { <-apiSem }()

	if client == nil {
		return "", nil, fmt.Errorf("http client is nil")
	}
	res, err := client.Do(req)
	if err != nil {
		return "", nil, err
	}
	defer res.Body.Close()

	var reader io.ReadCloser
	var errBody error
	switch res.Header.Get("Content-Encoding") {
	case "gzip":
		reader, errBody = gzip.NewReader(res.Body)
		if errBody != nil {
			return "", nil, errBody
		}
		defer reader.Close()
	default:
		reader = res.Body
	}

	body, err := io.ReadAll(reader)
	if err != nil {
		return "", nil, err
	}

	// Log request/response asynchronously
	AsyncLog(fmt.Sprintf("[w%d] [%s] mobileconfig\nURL: %s\nPAYLOAD: %s\nRESPONSE: %s",
		workerID, deviceID, apiUrl, payloadStr, string(body)))

	return string(body), res.Header, nil
}

// ExecuteCreateAndroidKeystore executes the create_android_keystore request
func ExecuteCreateAndroidKeystore(client *http.Client, hm *ThreadHeaderManager, deviceID string, workerID int) (string, http.Header, error) {
	apiUrl := "https://i.instagram.com/api/v1/attestation/create_android_keystore/"

	hm.Headers["host"] = "i.instagram.com"
	hm.FriendlyName = "IgApi: attestation/create_android_keystore/"

	formData := url.Values{}
	formData.Set("app_scoped_device_id", deviceID)
	formData.Set("key_hash", "")
	payloadStr := formData.Encode()

	var payload io.Reader
	if hm.Headers["content-encoding"] == "gzip" {
		var buf bytes.Buffer
		gz := gzip.NewWriter(&buf)
		gz.Write([]byte(payloadStr))
		gz.Close()
		payload = bytes.NewReader(buf.Bytes())
	} else {
		payload = strings.NewReader(payloadStr)
	}

	req, err := http.NewRequest("POST", apiUrl, payload)
	if err != nil {
		return "", nil, err
	}

	hm.Apply(req)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded; charset=UTF-8")
	req.Header.Set("x-ig-app-id", "3419628305025917")

	apiSem <- struct{}{}
	defer func() { <-apiSem }()

	// 4. Send Request
	if client == nil {
		return "", nil, fmt.Errorf("http client is nil")
	}
	res, err := client.Do(req)
	if err != nil {
		return "", nil, err
	}
	defer res.Body.Close()

	var reader io.ReadCloser
	var errBody error
	switch res.Header.Get("Content-Encoding") {
	case "gzip":
		reader, errBody = gzip.NewReader(res.Body)
		if errBody != nil {
			return "", nil, errBody
		}
		defer reader.Close()
	default:
		reader = res.Body
	}

	body, err := io.ReadAll(reader)
	if err != nil {
		return "", nil, err
	}

	// Log request/response asynchronously
	AsyncLog(fmt.Sprintf("[w%d] [%s] create_android_keystore\nURL: %s\nPAYLOAD: %s\nRESPONSE: %s",
		workerID, deviceID, apiUrl, payloadStr, string(body)))

	return string(body), res.Header, nil
}

var logChan = make(chan string, 1000)

func init() {
	go startFileLogger()
	loadEnvConfig()
	loadGlobalConfig()
	loadBackup()
	// Enable ANSI colors on Windows
	if runtime.GOOS == "windows" {
		kernel32 := syscall.NewLazyDLL("kernel32.dll")
		setConsoleMode := kernel32.NewProc("SetConsoleMode")
		getConsoleMode := kernel32.NewProc("GetConsoleMode")
		getStdHandle := kernel32.NewProc("GetStdHandle")

		const STD_OUTPUT_HANDLE = uint32(0xfffffff5) // -11
		const ENABLE_VIRTUAL_TERMINAL_PROCESSING = 0x0004

		handle, _, _ := getStdHandle.Call(uintptr(STD_OUTPUT_HANDLE))
		var mode uint32
		getConsoleMode.Call(handle, uintptr(unsafe.Pointer(&mode)))
		setConsoleMode.Call(handle, uintptr(mode|ENABLE_VIRTUAL_TERMINAL_PROCESSING))
	}

	go func() {
		os.MkdirAll("log", 0755)
		var currentFile *os.File
		var currentDay string
		const maxLogSize = 50 * 1024 * 1024 // 50MB

		for msg := range logChan {
			now := time.Now()
			day := now.Format("2006-01-02")
			needRotate := false

			if day != currentDay {
				needRotate = true
				// Trigger cleanup of previous days' logs
				if currentDay != "" {
					files, _ := os.ReadDir("log")
					for _, f := range files {
						if !f.IsDir() && strings.HasPrefix(f.Name(), "threads_reg_") {
							// If file date is not today, remove it
							if !strings.Contains(f.Name(), day) {
								os.Remove("log/" + f.Name())
							}
						}
					}
				}
			} else if currentFile != nil {
				if stat, err := currentFile.Stat(); err == nil && stat.Size() >= maxLogSize {
					needRotate = true
				}
			}

			if needRotate || currentFile == nil {
				if currentFile != nil {
					currentFile.Close()
				}
				currentDay = day
				idx := 0
				for {
					path := fmt.Sprintf("log/threads_reg_%s_%d.log", day, idx)
					if stat, err := os.Stat(path); err == nil && stat.Size() >= maxLogSize {
						idx++
						continue
					}
					f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
					if err == nil {
						currentFile = f
						break
					}
					time.Sleep(100 * time.Millisecond)
				}
			}

			if currentFile != nil {
				timestamp := now.Format("2006-01-02 15:04:05.000")
				currentFile.WriteString(fmt.Sprintf("================================================================================\n"))
				currentFile.WriteString(fmt.Sprintf("[%s] %s\n\n", timestamp, msg))
			}
		}
	}()
}

func AsyncLog(msg string) {
	select {
	case logChan <- msg:
	default:
		// Drop log if channel is full to prevent blocking
	}
}

func JSONStringify(v any) string {
	b, _ := json.Marshal(v)
	return string(b)
}

// GenerateRandomPassword generates a random password with specified length
// containing uppercase, lowercase letters and !@~ symbols
func GenerateRandomPassword(length int) string {
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ!@~"
	var seededRand *rand.Rand = rand.New(rand.NewSource(time.Now().UnixNano()))
	b := make([]byte, length)
	for i := range b {
		b[i] = charset[seededRand.Intn(len(charset))]
	}
	return string(b)
}

// GenerateRandomAndroidID generates a random 16-character hex string
func GenerateRandomAndroidID() string {
	const charset = "0123456789abcdef"
	b := make([]byte, 16)
	for i := range b {
		b[i] = charset[rand.Intn(len(charset))]
	}
	return fmt.Sprintf("%s-%s", "android", string(b))
}

// ExtractTokenAndUsername attempts to pull username, registration token, pkid, and sessionid from a potentially complex JSON response
func ExtractTokenAndUsername(data string) (string, string, string, string, string) {
	// 1. Username pattern
	userRe := regexp.MustCompile(`(?i)username[\\"]+:[\\"\s]*[\\"]+([^"\\]+)`)
	userMatch := userRe.FindStringSubmatch(data)
	username := ""
	if len(userMatch) > 1 {
		username = userMatch[1]
	}

	// 2. Authorization Token pattern
	authRe := regexp.MustCompile(`(?i)IG-Set-Authorization[\\"]+:[\\"\s]+Bearer\s+IGT:2:([^"\\]+)`)
	authMatch := authRe.FindStringSubmatch(data)
	token := ""
	fullAuth := ""
	pkid := ""
	sessionid := ""

	if len(authMatch) > 1 {
		token = authMatch[1]
		fullAuth = "Bearer IGT:2:" + token

		// Decode the base64 token to get sessionid and ds_user_id
		decoded, err := base64.StdEncoding.DecodeString(token)
		if err == nil {
			var authMap map[string]any
			if err := json.Unmarshal(decoded, &authMap); err == nil {
				if sid, ok := authMap["sessionid"].(string); ok {
					sessionid = sid
				}
				if uid, ok := authMap["ds_user_id"].(string); ok {
					pkid = uid
				}
			}
		}
	}

	// 3. Fallback PKID pattern if decoding failed or didn't provide it
	if pkid == "" {
		pkRe := regexp.MustCompile(`(?i)pk[\\"]+:[\\"\s]*[\\"]*(\d+)`)
		pkMatch := pkRe.FindStringSubmatch(data)
		if len(pkMatch) > 1 {
			pkid = pkMatch[1]
		}
	}

	// 4. Nonce pattern (if needed elsewhere)
	nonceRe := regexp.MustCompile(`(?i)(?:session_flush_nonce|partially_created_account_nonce)[\\"]+:[\\"\s]*[\\"]+([^"\\]+)`)
	nonceMatch := nonceRe.FindStringSubmatch(data)
	nonce := ""
	if len(nonceMatch) > 1 {
		nonce = nonceMatch[1]
	}

	return username, fullAuth, pkid, sessionid, nonce
}

// PollUntilParamSuccess repeatedly calls the API until GetParamsByApiName succeeds or maxRetries is reached
func PollUntilParamSuccess(targetApi string, cfg ThreadsRequestConfig, minSleepSec, maxSleepSec int, maxRetries int, workerID int, pm *ProxyManager) (map[string]string, error) {
	round := 1
	consecutiveErrors := 0
	rand.Seed(time.Now().UnixNano())

	startTime := time.Now()
	timeout := time.Duration(globalConfig.PollTimeoutSec) * time.Second

	for {
		if time.Since(startTime) > timeout {
			return nil, fmt.Errorf("polling timeout reached (%v) for %s", timeout, targetApi)
		}

		if maxRetries > 0 && round > maxRetries {
			return nil, fmt.Errorf("reached maximum retries (%d) for %s", maxRetries, targetApi)
		}

		updateDisplay(workerID, cfg.PhoneNumber, cfg.BloksAppID, fmt.Sprintf("Poll %d", round))
		body, _, err := ExecuteThreadsAPI(cfg)
		if err != nil {
			updateDisplay(workerID, cfg.PhoneNumber, cfg.BloksAppID, fmt.Sprintf("Err: %v (Rotating...)", err))
			RotateClientProxy(cfg.HTTPClient, pm, workerID)
			consecutiveErrors++
			if maxRetries > 0 && consecutiveErrors >= 3 {
				return nil, fmt.Errorf("consecutive errors reached limit (3): %v", err)
			}
		} else {
			consecutiveErrors = 0

			if strings.Contains(body, "Please wait a few minutes before you try again") || strings.Contains(body, "rate_limit_error") {
				updateDisplay(workerID, cfg.PhoneNumber, cfg.BloksAppID, "Rate limited! Rotating proxy...")
				RotateClientProxy(cfg.HTTPClient, pm, workerID)
			}

			if targetApi == "" || targetApi == "null" {
				return map[string]string{"full_response": body}, nil
			}

			// Try to extract
			params := GetParamsByApiName(targetApi, body)
			if len(params) > 0 {
				return params, nil
			}
			updateDisplay(workerID, cfg.PhoneNumber, cfg.BloksAppID, "Wait params...")
		}

		// Random sleep
		sleepTime := rand.Intn(maxSleepSec-minSleepSec+1) + minSleepSec
		time.Sleep(time.Duration(sleepTime) * time.Second)
		round++
	}
}

func RotateClientProxy(client *http.Client, pm *ProxyManager, workerID int) string {
	if pm == nil || client == nil {
		return ""
	}
	newProxy := pm.GetProxyWithConn(workerID)
	if t, ok := client.Transport.(*http.Transport); ok {
		if u, err := url.Parse(newProxy); err == nil {
			t.Proxy = http.ProxyURL(u)
			// Clear connection pool to ensure the new proxy is used immediately
			t.CloseIdleConnections()
		}
	}
	return newProxy
}

// ProxyManager handles proxy rotation and health checks
type ProxyManager struct {
	Proxies  []string
	Index    int
	filePath string
	mu       sync.Mutex
}

func NewProxyManager(filePath string) (*ProxyManager, error) {
	pm := &ProxyManager{filePath: filePath}
	if err := pm.Reload(); err != nil {
		return nil, err
	}
	return pm, nil
}

func (pm *ProxyManager) Reload() error {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	content, err := os.ReadFile(pm.filePath)
	if err != nil {
		return err
	}
	lines := strings.Split(string(content), "\n")
	var proxies []string
	for _, line := range lines {
		p := strings.TrimSpace(line)
		if p != "" {
			proxies = append(proxies, p)
		}
	}
	if len(proxies) == 0 {
		return fmt.Errorf("proxy file is empty")
	}
	pm.Proxies = proxies
	pm.Index = 0
	return nil
}

func (pm *ProxyManager) GetNext() string {
	pm.mu.Lock()
	if len(pm.Proxies) == 0 {
		pm.mu.Unlock()
		pm.Reload()
		pm.mu.Lock()
	}
	defer pm.mu.Unlock()

	if len(pm.Proxies) == 0 {
		return ""
	}
	p := pm.Proxies[pm.Index%len(pm.Proxies)]
	pm.Index++
	return p
}

func (pm *ProxyManager) GetProxyWithConn(connID int) string {
	base := pm.GetNext()
	u, err := url.Parse(base)
	if err != nil {
		return base
	}
	if u.User != nil {
		username := u.User.Username()
		password, hasPassword := u.User.Password()
		newUsername := fmt.Sprintf("%s-conn-%d", username, connID)
		if hasPassword {
			u.User = url.UserPassword(newUsername, password)
		} else {
			u.User = url.User(newUsername)
		}
	}
	return u.String()
}

type AppConfig struct {
	EnableHeaderRotation  bool `json:"enable_header_rotation"`
	MaxFinalizeRetries    int  `json:"max_finalize_retries"`
	PollTimeoutSec        int  `json:"poll_timeout_sec"`
	DisableUI             bool `json:"disable_ui"`
	UIRefreshIntervalMs   int  `json:"ui_refresh_interval_ms"`
	SMSWaitTimeoutSec     int  `json:"sms_wait_timeout_sec"`
	SMSPollTimeoutSec     int  `json:"sms_poll_timeout_sec"`
	SMSPollIntervalMs     int  `json:"sms_poll_interval_ms"`
	SMSMaxParallel        int  `json:"sms_max_parallel"`
	MaxWorkers            int  `json:"max_workers"`
	MaxSuccessPerFile     int  `json:"max_success_per_file"`
	TotalSuccessLimit     int  `json:"total_success_limit"`
	HttpRequestTimeoutSec int  `json:"http_request_timeout_sec"`
}

var (
	successCount         int64
	failCount            int64
	sessionSuccessCount  int64
	sessionFailCount     int64
	displayMu            sync.Mutex
	successMutex         sync.Mutex
	logMutex             sync.Mutex
	successHeaderMu      sync.Mutex
	successHeaderPrinted bool
	workerIPs            = make(map[int]string)
	wg                   sync.WaitGroup

	globalConfig = AppConfig{
		EnableHeaderRotation:  true,
		MaxFinalizeRetries:    20,
		PollTimeoutSec:        300,   // 5 minutes default
		DisableUI:             false, // Show UI by default
		UIRefreshIntervalMs:   2000,  // 2 seconds default to reduce IO overhead
		SMSWaitTimeoutSec:     120,   // 2 minutes default
		SMSPollTimeoutSec:     120,   // Default 2 minutes
		SMSPollIntervalMs:     5000,  // 5 seconds default
		SMSMaxParallel:        50,    // Limit concurrent SMS fetches
		MaxWorkers:            500,   // Default max registrations in flight
		MaxSuccessPerFile:     10,    // Default 10 successes per file
		TotalSuccessLimit:     1000,  // Default total success limit
		HttpRequestTimeoutSec: 30,    // Default 30 seconds
	}

	apiSem chan struct{} // Throttler for Instagram API
	smsSem chan struct{} // Throttler for SMS API

	phoneRegCounts        = make(map[string]int)  // Current successful reg count per phone (from backup)
	phoneMaxCounts        = make(map[string]int)  // Max count allowed per phone (from reg.txt)
	phoneProcessing       = make(map[string]bool) // Gate to prevent concurrent reg for same phone
	globalMaxSuccessCount = 5                     // SUCCESS_COUNT
	concurrencyLimit      = 100                   // CONCURRENCY
	finalizeRetries       = 20                    // FINALIZE_RETRIES
	enableIOSRotation     = true                  // ENABLE_IOS_UA
	dataMu                sync.Mutex
	workerStatuses        = make(map[int]workerState)
	totalPhones           int

	// High-performance async file logging
	fileLogChan = make(chan fileLogTask, 10000)

	geoSem = make(chan struct{}, 50) // Independent semaphore for GeoIP to prevent bottlenecks
)

type fileLogTask struct {
	fileType string // "success", "fail", "backup"
	content  string
}

func startFileLogger() {
	go func() {
		// Prepare rotating success file based on date and per-file limit
		dateStr := time.Now().Format("2006-01-02")
		fileIdx := 1
		var successF *os.File
		var successCountInFile int
		openSuccessFile := func() {
			if successF != nil {
				successF.Close()
			}
			filename := fmt.Sprintf("%s-注册成功-%d.txt", dateStr, fileIdx)
			f, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
			if err != nil {
				// Fallback to a generic name if creation fails
				f, _ = os.OpenFile("success_accounts.txt", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
			}
			successF = f
			successCountInFile = 0
		}

		openSuccessFile()

		failF, _ := os.OpenFile("failed_reg.txt", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
		backupF, _ := os.OpenFile("reg_backup.txt", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)

		defer func() {
			if successF != nil {
				successF.Close()
			}
			if failF != nil {
				failF.Close()
			}
			if backupF != nil {
				backupF.Close()
			}
		}()

		for task := range fileLogChan {
			switch task.fileType {
			case "success":
				if successF != nil {
					if successCountInFile >= globalConfig.MaxSuccessPerFile {
						fileIdx++
						openSuccessFile()
					}
					successF.WriteString(task.content + "\n")
					successCountInFile++
					if atomic.LoadInt64(&successCount) >= int64(globalConfig.TotalSuccessLimit) {
						fmt.Printf("\n[SYSTEM] Total success limit (%d) reached. Stopping...\n", globalConfig.TotalSuccessLimit)
						return
					}
				}
			case "fail":
				if failF != nil {
					failF.WriteString(task.content + "\n")
				}
			case "backup":
				if backupF != nil {
					backupF.WriteString(task.content + "\n")
				}
			}
		}
	}()
}

type workerState struct {
	phone      string
	api        string
	status     string
	lastUpdate time.Time
}

func startDisplayRefresher() {
	go func() {
		for {
			interval := globalConfig.UIRefreshIntervalMs
			if interval < 500 {
				interval = 500
			}
			time.Sleep(time.Duration(interval) * time.Millisecond)

			if globalConfig.DisableUI {
				continue
			}

			sc := atomic.LoadInt64(&successCount)
			fc := atomic.LoadInt64(&failCount)
			ssc := atomic.LoadInt64(&sessionSuccessCount)
			sfc := atomic.LoadInt64(&sessionFailCount)
			total := sc + fc

			displayMu.Lock()
			activeCount := 0
			type workerEntry struct {
				id int
				s  workerState
			}
			var entries []workerEntry
			for id := 1; id <= globalConfig.MaxWorkers; id++ {
				s := workerStatuses[id]
				if s.phone != "N/A" && s.phone != "" {
					activeCount++
					entries = append(entries, workerEntry{id, s})
				}
			}
			displayMu.Unlock()

			// 2. Render Header (Pinned to Top)
			// Added Session stats here
			fmt.Printf("\x1b[1;1H\x1b[K\x1b[32mSuccess: %d  \x1b[31mFail: %d  \x1b[36mTotal: %d  \x1b[33mActive: %d/%d  \x1b[35mPhones: %d  \x1b[32mSessionSuccess: %d  \x1b[31mSessionFail: %d\x1b[0m",
				sc, fc, total, activeCount, globalConfig.MaxWorkers, totalPhones, ssc, sfc)

			// 3. Sort by ID
			sort.Slice(entries, func(i, j int) bool {
				return entries[i].id < entries[j].id
			})

			// 4. Render Active Worker Rows
			currentRow := 2
			maxRows := 30 // Show top 30 active workers to prevent scrolling
			for _, entry := range entries {
				if currentRow > maxRows {
					break
				}
				id := entry.id
				state := entry.s

				cleanStatus := strings.ReplaceAll(state.status, "\n", " ")
				if len(cleanStatus) > 60 {
					cleanStatus = cleanStatus[:60] + "..."
				}

				color := "\x1b[36m"
				if strings.Contains(cleanStatus, "SUCCESS") || strings.Contains(cleanStatus, "DONE") {
					color = "\x1b[32m"
				} else if strings.Contains(cleanStatus, "Error") || strings.Contains(cleanStatus, "FAILED") {
					color = "\x1b[31m"
				}

				// Move to specific row and clear line before printing
				fmt.Printf("\x1b[%d;1H\x1b[K[\x1b[33mw%d\x1b[0m] \x1b[35m%-12s\x1b[0m %-8s %s%s\x1b[0m",
					currentRow, id, state.phone, state.api, color, cleanStatus)
				currentRow++
			}

			// 5. Clear remaining lines below the list
			for r := currentRow; r <= maxRows+1; r++ {
				fmt.Printf("\x1b[%d;1H\x1b[K", r)
			}
		}
	}()
}

const (
	colorReset  = "\x1b[0m"
	colorRed    = "\x1b[31m"
	colorGreen  = "\x1b[32m"
	colorYellow = "\x1b[33m"
	colorCyan   = "\x1b[36m"
)

func loadEnvConfig() {
	content, err := os.ReadFile("reg.env")
	if err != nil {
		// Create default if not exists
		defaultEnv := "SUCCESS_COUNT=5\nENABLE_IOS_UA=true\nCONCURRENCY=100\nFINALIZE_RETRIES=20"
		os.WriteFile("reg.env", []byte(defaultEnv), 0644)
		globalMaxSuccessCount = 5
		enableIOSRotation = true
		concurrencyLimit = 100
		finalizeRetries = 20
		return
	}
	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		// Strip comments
		if idx := strings.Index(line, "#"); idx != -1 {
			line = strings.TrimSpace(line[:idx])
		}
		if line == "" {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.TrimSpace(parts[0])
		valStr := strings.TrimSpace(parts[1])

		switch key {
		case "SUCCESS_COUNT":
			if val, err := strconv.Atoi(valStr); err == nil {
				globalMaxSuccessCount = val
				// Also sync to TotalSuccessLimit if not explicitly set later
				if globalConfig.TotalSuccessLimit == 1000 {
					globalConfig.TotalSuccessLimit = val * 5000 // reasonable upper bound
				}
			}
		case "ENABLE_IOS_UA":
			enableIOSRotation = (strings.ToLower(valStr) == "true")
		case "CONCURRENCY":
			if val, err := strconv.Atoi(valStr); err == nil {
				concurrencyLimit = val
			}
		case "MAX_WORKERS":
			if val, err := strconv.Atoi(valStr); err == nil {
				globalConfig.MaxWorkers = val
			}
		case "FINALIZE_RETRIES":
			if val, err := strconv.Atoi(valStr); err == nil {
				finalizeRetries = val
				globalConfig.MaxFinalizeRetries = val
			}
		case "MAX_SUCCESS_PER_FILE":
			if val, err := strconv.Atoi(valStr); err == nil {
				globalConfig.MaxSuccessPerFile = val
			}
		case "TOTAL_SUCCESS_LIMIT":
			if val, err := strconv.Atoi(valStr); err == nil {
				globalConfig.TotalSuccessLimit = val
			}
		case "SMS_POLL_TIMEOUT_SEC":
			if val, err := strconv.Atoi(valStr); err == nil {
				globalConfig.SMSPollTimeoutSec = val
				globalConfig.SMSWaitTimeoutSec = val
			}
		case "HTTP_REQUEST_TIMEOUT_SEC":
			if val, err := strconv.Atoi(valStr); err == nil {
				globalConfig.HttpRequestTimeoutSec = val
			}
		case "PollTimeoutSec":
			if val, err := strconv.Atoi(valStr); err == nil {
				globalConfig.PollTimeoutSec = val
			}
		case "SMSWaitTimeoutSec":
			if val, err := strconv.Atoi(valStr); err == nil {
				globalConfig.SMSWaitTimeoutSec = val
			}
		}
	}
}

func loadGlobalConfig() {
	data, err := os.ReadFile("config.json")
	if err == nil {
		json.Unmarshal(data, &globalConfig)
	} else {
		// Create default if not exists
		data, _ = json.MarshalIndent(globalConfig, "", "  ")
		os.WriteFile("config.json", data, 0644)
	}
}

func loadBackup() {
	dataMu.Lock()
	defer dataMu.Unlock()
	data, err := os.ReadFile("reg_backup.txt")
	if err != nil {
		return
	}
	lines := strings.Split(string(data), "\n")
	var totalSuccess int64
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Split(line, "----")
		if len(parts) >= 2 {
			count, _ := strconv.Atoi(parts[1])
			phoneRegCounts[parts[0]] = count
			totalSuccess += int64(count)
		}
	}
	atomic.StoreInt64(&successCount, totalSuccess)
}

func saveBackup(phone string, count int) {
	dataMu.Lock()
	phoneRegCounts[phone] = count
	dataMu.Unlock()

	// Async append to backup - loadBackup naturally takes the last entry for each phone
	if atomic.LoadInt64(&successCount) >= int64(globalConfig.TotalSuccessLimit) {
		return
	}
	select {
	case fileLogChan <- fileLogTask{fileType: "backup", content: fmt.Sprintf("%s----%d", phone, count)}:
	default:
	}
}

func clearScreen() {
	fmt.Print("\033[H\033[2J")
}

func updateDisplay(workerID int, phone string, apiName string, status string) {
	displayMu.Lock()
	defer displayMu.Unlock()
	workerStatuses[workerID] = workerState{
		phone:      phone,
		api:        apiName,
		status:     status,
		lastUpdate: time.Now(),
	}
}

func logSuccessAtomically(line string) {
	if atomic.LoadInt64(&successCount) >= int64(globalConfig.TotalSuccessLimit) {
		return
	}
	atomic.AddInt64(&successCount, 1)
	atomic.AddInt64(&sessionSuccessCount, 1)
	select {
	case fileLogChan <- fileLogTask{fileType: "success", content: line}:
	default:
	}
}

func logFailureAtomically(workerID int, phone, apiName, reason string) {
	atomic.AddInt64(&failCount, 1)
	atomic.AddInt64(&sessionFailCount, 1)
	logSilentFailure(phone, reason)
	updateDisplay(workerID, phone, apiName, "FAILED: "+reason)
}

func logSilentFailure(phone, reason string) {
	if atomic.LoadInt64(&successCount) >= int64(globalConfig.TotalSuccessLimit) {
		return
	}
	dt := time.Now().Format("2006-01-02 15:04:05")
	content := fmt.Sprintf("[%s] %s: %s", dt, phone, reason)
	select {
	case fileLogChan <- fileLogTask{fileType: "fail", content: content}:
	default:
	}
}

func main() {
	startFileLogger()
	loadEnvConfig()
	loadBackup()
	loadGlobalConfig()

	smsMgr := NewSMSManager()
	if err := smsMgr.Load("reg.txt"); err != nil {
		fmt.Printf("[Error] Failed to load reg.txt: %v\n", err)
		return
	}
	totalPhones = len(smsMgr.Configs)

	pm, err := NewProxyManager("proxy.txt")
	if err != nil {
		fmt.Printf("[Warning] Failed to load proxy.txt: %v. Running without proxies.\n", err)
	}

	var phoneList []string
	for p := range smsMgr.Configs {
		phoneList = append(phoneList, p)
	}

	// Initialize semaphores for decoupling
	apiSem = make(chan struct{}, concurrencyLimit)
	smsSem = make(chan struct{}, globalConfig.SMSMaxParallel)

	// If MaxWorkers not set or too small, default to a larger pool to allow SMS waiting without blocking
	if globalConfig.MaxWorkers < concurrencyLimit {
		globalConfig.MaxWorkers = concurrencyLimit * 5
	}

	chanSize := globalConfig.MaxWorkers * 2
	if chanSize < 200 {
		chanSize = 200
	}
	phoneChan := make(chan string, chanSize)

	clearScreen()
	startDisplayRefresher()
	AsyncLog(fmt.Sprintf("Starting System: ActiveAPI=%d, MaxTotalWorkers=%d, SMSMaxParallel=%d",
		concurrencyLimit, globalConfig.MaxWorkers, globalConfig.SMSMaxParallel))

	for i := 1; i <= globalConfig.MaxWorkers; i++ {
		// Initial display placeholder
		updateDisplay(i, "N/A", "IDLE", "WAITING")

		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			transport := &http.Transport{}
			proxyURL := ""
			if pm != nil {
				proxyURL = pm.GetProxyWithConn(id)
				if u, err := url.Parse(proxyURL); err == nil {
					transport.Proxy = http.ProxyURL(u)
				}
			}
			client := &http.Client{
				Timeout:   60 * time.Second,
				Transport: transport,
			}

			for phoneNum := range phoneChan {
				func() {
					// Mark as processing
					dataMu.Lock()
					phoneProcessing[phoneNum] = true
					dataMu.Unlock()

					// Ensure release on finish
					defer func() {
						dataMu.Lock()
						phoneProcessing[phoneNum] = false
						dataMu.Unlock()
						// Clear display status
						updateDisplay(id, "N/A", "IDLE", "WAITING")
					}()

					ProcessRegistration(client, phoneNum, id, proxyURL, smsMgr, pm)
				}()

				// Rotate proxy after each registration attempt (success or failure)
				proxyURL = RotateClientProxy(client, pm, id)
			}
		}(i)
	}

	// Continuous Dispatcher: Monitor reg.txt for new tasks
	go func() {
		for {
			// Live reload config from reg.env
			loadEnvConfig()

			if err := smsMgr.Load("reg.txt"); err == nil {
				dataMu.Lock()
				var fresh []string
				var recyclable []string
				skippedByLimit := 0

				eligibleCount := 0
				for phone := range smsMgr.Configs {
					count := phoneRegCounts[phone]
					max := phoneMaxCounts[phone]
					isProcessing := phoneProcessing[phone]

					// STRICT ELIGIBILITY: Under individual max AND under global override max
					if count < max && count < globalMaxSuccessCount {
						eligibleCount++

						// Skip if currently processing for the dispatch lists
						if isProcessing {
							continue
						}

						if atomic.LoadInt64(&successCount) >= int64(globalConfig.TotalSuccessLimit) {
							continue // Still count as eligible for the total but don't dispatch
						}
						if count == 0 {
							fresh = append(fresh, phone)
						} else {
							recyclable = append(recyclable, phone)
						}
					} else {
						skippedByLimit++
					}
				}
				totalPhones = eligibleCount
				dataMu.Unlock()

				if atomic.LoadInt64(&successCount) >= int64(globalConfig.TotalSuccessLimit) {
					fmt.Print("\n[Dispatcher] Total success limit reached. Waiting for workers to finish...\n")
					time.Sleep(10 * time.Second)
					continue
				}

				// Improved dispatcher logging when no phones are eligible
				if len(fresh) == 0 && len(recyclable) == 0 && skippedByLimit > 0 {
					// Silent wait if all phones reached limit
					// AsyncLog(fmt.Sprintf("[Dispatcher] All %d eligible phones have reached their limits. Waiting for new tasks or config changes.", skippedByLimit))
				} else if len(fresh) > 0 || len(recyclable) > 0 {
					// Shuffle both groups independently
					if len(fresh) > 0 {
						rand.Shuffle(len(fresh), func(i, j int) { fresh[i], fresh[j] = fresh[j], fresh[i] })
					}
					if len(recyclable) > 0 {
						rand.Shuffle(len(recyclable), func(i, j int) { recyclable[i], recyclable[j] = recyclable[j], recyclable[i] })
					}

					// Combine: Fresh accounts come first
					eligible := append(fresh, recyclable...)

					for _, phone := range eligible {
						dataMu.Lock()
						// Fix redundancy in phoneProcessing lock during dispatch
						phoneProcessing[phone] = true // Mark as processing before unlocking for select
						dataMu.Unlock()
						// Use non-blocking send or small delay to avoid hanging dispatcher if channel is full
						select {
						case phoneChan <- phone:
						default:
							dataMu.Lock()
							phoneProcessing[phone] = false
							dataMu.Unlock()
						}
					}
				}
			}
			// Slower poll for large files to avoid CPU spike
			time.Sleep(5 * time.Second)
		}
	}()

	wg.Wait()
	AsyncLog("All registrations complete (Unexpected exit).")
}

func ProcessRegistration(client *http.Client, phoneNum string, workerID int, proxyURL string, smsMgr *SMSManager, pm *ProxyManager) bool {
	updateDisplay(workerID, phoneNum, "GEO", "Fetching GeoIP...")
	// 获取 IP 和 时区 (使用当前代理)
	var geoInfo *GeoInfo
	deadline := time.Now().Add(time.Duration(globalConfig.PollTimeoutSec) * time.Second)
	for {
		if time.Now().After(deadline) {
			logFailureAtomically(workerID, phoneNum, "GEO", "GEO_TIMEOUT")
			return false
		}
		info, err := GetIPAndTimezone(proxyURL)
		if err == nil {
			geoInfo = info
			displayMu.Lock()
			workerIPs[workerID] = info.IP
			displayMu.Unlock()
			break
		}
		updateDisplay(workerID, phoneNum, "GEO", "Geo failed, rotating...")
		proxyURL = RotateClientProxy(client, pm, workerID)
		time.Sleep(2 * time.Second)
	}

	// Create Header Manager Singleton for this registration loop
	hm := NewThreadHeaderManager()
	hm.RandomizeUserAgent()

	// Generate core device IDs for the entire session
	DeviceID := uuid.New().String()
	AndroidID := GenerateRandomAndroidID()
	FamilyDeviceID := uuid.New().String()
	WaterfallID := uuid.New().String()

	// --- STEP 0: Mobile Config ---
	updateDisplay(workerID, phoneNum, "MobileConfig", "Init...")
	var mid string
	var pubKey string
	var keyIdInt int
	for {
		if time.Now().After(deadline) {
			logFailureAtomically(workerID, phoneNum, "MobileConfig", "MC_TIMEOUT")
			return false
		}
		sessionID_mc := "UFS-" + uuid.New().String() + "-0"

		hm.SetIDs(DeviceID, AndroidID, FamilyDeviceID, WaterfallID, sessionID_mc)
		_, mcHeaders, err := ExecuteMobileConfig(client, hm, DeviceID, sessionID_mc, workerID)

		if err != nil {
			updateDisplay(workerID, phoneNum, "MobileConfig", "Failed, rotating...")
			proxyURL = RotateClientProxy(client, pm, workerID)
			time.Sleep(5 * time.Second)
			continue
		}

		// 提取 Header 参数
		mid = mcHeaders.Get("Ig-Set-X-Mid")
		pubKey = mcHeaders.Get("ig-set-password-encryption-pub-key")
		keyIdStr := mcHeaders.Get("ig-set-password-encryption-key-id")

		if pubKey == "" {
			pubKey = mcHeaders.Get("Ig-Set-X-Pub-Key")
			keyIdStr = mcHeaders.Get("Ig-Set-X-Key-Id")
		}

		keyIdInt, _ = strconv.Atoi(keyIdStr)

		if mid != "" {
			break
		}
		RotateClientProxy(client, pm, workerID)
		time.Sleep(1 * time.Second)
	}

	// --- STEP 0.5: Create Android Keystore ---
	updateDisplay(workerID, phoneNum, "Keystore", "Preparing...")
	var challengeNonce string
	for {
		if time.Now().After(deadline) {
			logFailureAtomically(workerID, phoneNum, "Keystore", "KS_TIMEOUT")
			return false
		}
		ksResp, _, err := ExecuteCreateAndroidKeystore(client, hm, AndroidID, workerID)
		if err != nil {
			updateDisplay(workerID, phoneNum, "Keystore", "Failed, rotating...")
			proxyURL = RotateClientProxy(client, pm, workerID)
			time.Sleep(1 * time.Second)
			continue
		}

		var ksMap map[string]any
		if err := json.Unmarshal([]byte(ksResp), &ksMap); err == nil {
			if nonce, ok := ksMap["challenge_nonce"].(string); ok {
				challengeNonce = nonce
			}
		}
		break
	}

	// --- STEP 1: Process start.async ---
	step1Config := ThreadsRequestConfig{
		DeviceID:       DeviceID,
		AndroidID:      AndroidID,
		FamilyDeviceID: FamilyDeviceID,
		WaterfallID:    WaterfallID,
		BloksAppID:     "com.bloks.www.bloks.caa.reg.start.async",
		FriendlyName:   "IGBloksAppRootQuery-com.bloks.www.bloks.caa.reg.start.async",
		ProxyURL:       proxyURL,
		HeaderManager:  hm,
		HTTPClient:     client,
		PhoneNumber:    phoneNum,
		WorkerID:       workerID,
	}

	jsonParams := map[string]any{
		"family_device_id": step1Config.FamilyDeviceID,
		"qe_device_id":     step1Config.DeviceID,
		"device_id":        step1Config.AndroidID,
		"waterfall_id":     step1Config.WaterfallID,
		"reg_flow_source":  "threads_ig_account_creation",
		"skip_welcome":     false,
		"is_from_spc":      false,
	}

	InnerParamsString, _ := json.Marshal(jsonParams)
	step1Config.InnerParams = string(InnerParamsString)
	targetApi := "com.bloks.www.bloks.caa.reg.async.contactpoint_phone.async"
	paramsStep1, err := PollUntilParamSuccess(targetApi, step1Config, 1, 2, 0, workerID, pm)
	if err != nil {
		AsyncLog(fmt.Sprintf("[w%d] [%s] Step 1 failed: %v", workerID, phoneNum, err))
		logFailureAtomically(workerID, phoneNum, "START", "STEP1_FAILED")
		return false
	}

	jsonParamsStep1, _ := json.Marshal(paramsStep1)

	// --- STEP 2: Process contactpoint_phone.async ---
	AsyncLog(fmt.Sprintf("[w%d] [%s] Starting Step 2...", workerID, phoneNum))

	jsonParamsStep2 := map[string]any{
		"params": JSONStringify(map[string]any{
			"client_input_params": JSONStringify(map[string]any{
				"aac":                           "",
				"device_id":                     step1Config.AndroidID,
				"was_headers_prefill_available": 0,
				"login_upsell_phone_list":       []any{},
				"whatsapp_installed_on_client":  0,
				"zero_balance_state":            "",
				"network_bssid":                 nil,
				"msg_previous_cp":               "",
				"switch_cp_first_time_loading":  1,
				"accounts_list":                 []any{},
				"confirmed_cp_and_code":         map[string]any{},
				"country_code":                  "",
				"family_device_id":              step1Config.FamilyDeviceID,
				"block_store_machine_id":        "",
				"fb_ig_device_id":               []any{},
				"phone":                         phoneNum,
				"lois_settings": map[string]any{
					"lois_token": "",
				},
				"cloud_trust_token":        nil,
				"was_headers_prefill_used": 0,
				"headers_infra_flow_id":    "",
				"build_type":               "release",
				"encrypted_msisdn":         "",
				"switch_cp_have_seen_suma": 0,
			}),
			"server_params": string(jsonParamsStep1),
		}),
	}

	step1Config.BloksAppID = "com.bloks.www.bloks.caa.reg.async.contactpoint_phone.async"
	step1Config.FriendlyName = "IGBloksAppRootQuery-com.bloks.www.bloks.caa.reg.async.contactpoint_phone.async"
	step1Config.InnerParams = JSONStringify(jsonParamsStep2)

	targetApi = "com.bloks.www.bloks.caa.reg.confirmation.async"
	paramsStep2, err := PollUntilParamSuccess(targetApi, step1Config, 1, 2, 0, workerID, pm)
	if err != nil {
		logFailureAtomically(workerID, phoneNum, targetApi, "STEP2_FAILED")
		return false
	}

	// --- STEP 3: Fetch SMS Code and Verify ---
	smsTimeout := time.Duration(globalConfig.SMSWaitTimeoutSec) * time.Second
	code, rawMsg, err := smsMgr.PollCode(phoneNum, smsTimeout, workerID)
	if err != nil {
		updateDisplay(workerID, phoneNum, "SMS", "Timeout")
		// Log as silent failure (don't increment failCount) to allow dispatcher to retry fairly
		logSilentFailure(phoneNum, "SMS_TIMEOUT")
		return false
	}
	AsyncLog(fmt.Sprintf("[w%d] [%s] SMS Received: %s", workerID, phoneNum, rawMsg))

	jsonParamsStep3 := map[string]any{
		"params": JSONStringify(map[string]any{
			"client_input_params": JSONStringify(map[string]any{
				"confirmed_cp_and_code":  map[string]any{},
				"aac":                    "",
				"block_store_machine_id": "",
				"code":                   code,
				"fb_ig_device_id":        []any{},
				"device_id":              step1Config.AndroidID,
				"lois_settings": map[string]any{
					"lois_token": "",
				},
				"cloud_trust_token": nil,
				"network_bssid":     nil,
			}),
			"server_params": JSONStringify(paramsStep2),
		}),
	}

	step1Config.BloksAppID = "com.bloks.www.bloks.caa.reg.confirmation.async"
	step1Config.FriendlyName = "IGBloksAppRootQuery-com.bloks.www.bloks.caa.reg.confirmation.async"
	step1Config.InnerParams = JSONStringify(jsonParamsStep3)

	targetApi = "com.bloks.www.bloks.caa.reg.password.async"
	paramsStep3, err := PollUntilParamSuccess(targetApi, step1Config, 1, 2, 0, workerID, pm)
	if err != nil {
		logFailureAtomically(workerID, phoneNum, targetApi, "STEP3_FAILED")
		return false
	}

	passwordLen := rand.Intn(5) + 8
	password := GenerateRandomPassword(passwordLen)
	updateDisplay(workerID, phoneNum, "Password", "Encrypting...")
	timestamp := time.Now().Unix()
	safetynet_response := "API_ERROR:+class+com.google.android.gms.common.api.ApiException:7:+"

	tokenBytes := make([]byte, 24)
	crand.Read(tokenBytes)
	tokenData := fmt.Sprintf("%s|%d|", phoneNum, timestamp)
	safetynet_token := base64.StdEncoding.EncodeToString(append([]byte(tokenData), tokenBytes...))

	passwordEnd, _, err := EncryptPassword4NodeCompatible(pubKey, keyIdInt, password)
	// if err != nil {
	// AsyncLog(fmt.Sprintf("[%s] RSA encryption failed: %v. Using fallback.", phoneNum, err))
	passwordEnd = fmt.Sprintf("#PWD_INSTAGRAM:0:%d:%s", timestamp, password)
	// }
	jsonParamsStepPassword := map[string]any{
		"params": JSONStringify(map[string]any{
			"client_input_params": JSONStringify(map[string]any{
				"safetynet_response":                    safetynet_response,
				"caa_play_integrity_attestation_result": "",
				"aac":                                   "",
				"safetynet_token":                       safetynet_token,
				"whatsapp_installed_on_client":          0,
				"zero_balance_state":                    "",
				"network_bssid":                         nil,
				"machine_id":                            mid,
				"headers_last_infra_flow_id_safetynet":  "",
				"email_oauth_token_map":                 map[string]any{},
				"block_store_machine_id":                "",
				"fb_ig_device_id":                       []any{},
				"encrypted_msisdn_for_safetynet":        "",
				"lois_settings": map[string]any{
					"lois_token": "",
				},
				"cloud_trust_token":     nil,
				"client_known_key_hash": "",
				"encrypted_password":    passwordEnd,
			}),
			"server_params": JSONStringify(paramsStep3),
		}),
	}

	step1Config.BloksAppID = "com.bloks.www.bloks.caa.reg.password.async"
	step1Config.FriendlyName = "IGBloksAppRootQuery-com.bloks.www.bloks.caa.reg.password.async"
	step1Config.InnerParams = JSONStringify(jsonParamsStepPassword)

	targetApi = "com.bloks.www.bloks.caa.reg.birthday.async"
	paramsStep4, err := PollUntilParamSuccess(targetApi, step1Config, 1, 2, 0, workerID, pm)
	if err != nil {
		logFailureAtomically(workerID, phoneNum, targetApi, "STEP4_FAILED")
		return false
	}

	day, month, year := GenerateRandomBirthday(18, 40)
	birthdayStr := fmt.Sprintf("%02d-%02d-%d", day, month, year)
	birthDayTimestamp, err := GetTimestampByBirthdayAndTZ(birthdayStr, geoInfo.Timezone)
	if err != nil {
		birthDayTimestamp = time.Date(year, time.Month(month), day, 0, 0, 0, 0, time.Local).Unix()
	}

	AsyncLog(fmt.Sprintf("[w%d] [%s] Starting Step 5...", workerID, phoneNum))
	jsonParamsStep5 := map[string]any{
		"params": JSONStringify(map[string]any{
			"client_input_params": JSONStringify(map[string]any{
				"client_timezone":                 geoInfo.Timezone,
				"aac":                             "",
				"birthday_or_current_date_string": birthdayStr,
				"os_age_range":                    "",
				"birthday_timestamp":              birthDayTimestamp,
				"lois_settings": map[string]any{
					"lois_token": "",
				},
				"zero_balance_state":                "",
				"network_bssid":                     nil,
				"should_skip_youth_tos":             0,
				"is_youth_regulation_flow_complete": 0,
			}),
			"server_params": JSONStringify(paramsStep4),
		}),
	}

	step1Config.BloksAppID = "com.bloks.www.bloks.caa.reg.birthday.async"
	step1Config.FriendlyName = "IGBloksAppRootQuery-com.bloks.www.bloks.caa.reg.birthday.async"
	step1Config.InnerParams = JSONStringify(jsonParamsStep5)

	targetApi = "com.bloks.www.bloks.caa.reg.name_ig_and_soap.async"
	paramsStep5, err := PollUntilParamSuccess(targetApi, step1Config, 1, 2, 0, workerID, pm)
	if err != nil {
		logFailureAtomically(workerID, phoneNum, targetApi, "STEP5_FAILED")
		return false
	}

	AsyncLog(fmt.Sprintf("[w%d] [%s] Starting Step 6...", workerID, phoneNum))
	jsonParamsStep6 := map[string]any{
		"params": JSONStringify(map[string]any{
			"client_input_params": JSONStringify(map[string]any{
				"accounts_list": []any{},
				"aac":           "",
				"lois_settings": map[string]any{
					"lois_token": "",
				},
				"zero_balance_state": "",
				"network_bssid":      nil,
				"name":               "",
			}),
			"server_params": JSONStringify(paramsStep5),
		}),
	}

	step1Config.BloksAppID = "com.bloks.www.bloks.caa.reg.name_ig_and_soap.async"
	step1Config.FriendlyName = "IGBloksAppRootQuery-com.bloks.www.bloks.caa.reg.name_ig_and_soap.async"
	step1Config.InnerParams = JSONStringify(jsonParamsStep6)

	targetApi = "com.bloks.www.bloks.caa.reg.username.async"
	paramsStep6, err := PollUntilParamSuccess(targetApi, step1Config, 1, 2, 0, workerID, pm)
	if err != nil {
		logFailureAtomically(workerID, phoneNum, targetApi, "STEP6_FAILED")
		return false
	}

	regInfo := paramsStep6["reg_info"]
	regInfoMap := make(map[string]any)
	if regInfo != "" {
		_ = json.Unmarshal([]byte(regInfo), &regInfoMap)
	}
	usernameArg := ""
	if u, ok := regInfoMap["username_prefill"].(string); ok {
		usernameArg = u
	}

	AsyncLog(fmt.Sprintf("[w%d] [%s] Starting Step 7...", workerID, phoneNum))
	jsonParamsStep7 := map[string]any{
		"params": JSONStringify(map[string]any{
			"client_input_params": JSONStringify(map[string]any{
				"validation_text":  usernameArg,
				"aac":              "",
				"family_device_id": step1Config.FamilyDeviceID,
				"device_id":        step1Config.AndroidID,
				"lois_settings": map[string]any{
					"lois_token": "",
				},
				"zero_balance_state": "",
				"network_bssid":      nil,
				"qe_device_id":       step1Config.DeviceID,
			}),
			"server_params": JSONStringify(paramsStep6),
		}),
	}

	step1Config.BloksAppID = "com.bloks.www.bloks.caa.reg.username.async"
	step1Config.FriendlyName = "IGBloksAppRootQuery-com.bloks.www.bloks.caa.reg.username.async"
	step1Config.InnerParams = JSONStringify(jsonParamsStep7)

	targetApi = "com.bloks.www.bloks.caa.reg.create.account.async"
	paramsStep7, err := PollUntilParamSuccess(targetApi, step1Config, 1, 2, 0, workerID, pm)
	if err != nil {
		logFailureAtomically(workerID, phoneNum, targetApi, "STEP7_FAILED")
		return false
	}

	// Step 8: com.bloks.www.bloks.caa.reg.create.account.async
	updateDisplay(workerID, phoneNum, "CreateAccount", "Finalizing...")
	jsonParamsStep8 := map[string]any{
		"params": JSONStringify(map[string]any{
			"client_input_params": JSONStringify(map[string]any{
				"ck_error":                   "",
				"aac":                        "",
				"device_id":                  step1Config.AndroidID,
				"waterfall_id":               step1Config.WaterfallID,
				"zero_balance_state":         "",
				"network_bssid":              nil,
				"failed_birthday_year_count": "",
				"headers_last_infra_flow_id": "",
				"ig_partially_created_account_nonce_expiry": 0,
				"machine_id":                         mid,
				"should_ignore_existing_login":       0,
				"reached_from_tos_screen":            1,
				"ig_partially_created_account_nonce": "",
				"ck_nonce":                           "",
				"force_sessionless_nux_experience":   0,
				"lois_settings": map[string]any{
					"lois_token": "",
				},
				"ig_partially_created_account_user_id": 0,
				"ck_id":                                "",
				"no_contact_perm_email_oauth_token":    "",
				"encrypted_msisdn":                     "",
			}),
			"server_params": JSONStringify(paramsStep7),
		}),
	}

	step1Config.BloksAppID = "com.bloks.www.bloks.caa.reg.create.account.async"
	step1Config.FriendlyName = "IGBloksAppRootQuery-com.bloks.www.bloks.caa.reg.create.account.async"
	step1Config.InnerParams = JSONStringify(jsonParamsStep8)

	if challengeNonce != "" {
		step1Config.HeaderOverrides = map[string]string{
			"x-ig-attest-params": fmt.Sprintf(`{"attestation":[{"version":2,"type":"keystore","errors":[-1014],"challenge_nonce":"%s","signed_nonce":"","key_hash":""}]}`, challengeNonce),
		}
	}

	success := false
	for i := 1; i <= finalizeRetries; i++ {
		if time.Now().After(deadline) {
			return false
		}
		updateDisplay(workerID, phoneNum, "Finalize", fmt.Sprintf("Finalizing (Attempt %d)...", i))
		paramsStep8, err := PollUntilParamSuccess("", step1Config, 1, 2, 1, workerID, pm)
		if err != nil {
			// PollUntilParamSuccess internally rotates IP on error
			if globalConfig.EnableHeaderRotation {
				if enableIOSRotation {
					hm.RandomizeIOSUserAgent()
				} else {
					hm.RandomizeUserAgent()
				}
			}
			continue
		}

		if paramsStep8 != nil {
			body := paramsStep8["full_response"]
			finalUsername, finalToken, finalPKID, finalSessionID, _ := ExtractTokenAndUsername(body)

			// Reject usernames containing "Instagram User"
			if strings.Contains(finalUsername, "Instagram User") {
				logFailureAtomically(workerID, phoneNum, "Finalize", "Instagram User detected")
				continue
			}

			// If response tells us to try again or we failed to get registration tokens
			if strings.Contains(body, "Please try again") || finalUsername == "" || finalToken == "" {
				uaType := "UA"
				if enableIOSRotation {
					uaType = "iOS UA"
				}
				updateDisplay(workerID, phoneNum, "Finalize", fmt.Sprintf("Failed, rotating IP & %s...", uaType))
				RotateClientProxy(client, pm, workerID)

				if globalConfig.EnableHeaderRotation {
					if enableIOSRotation {
						hm.RandomizeIOSUserAgent()
					} else {
						hm.RandomizeUserAgent()
					}
				}
				// Refresh IP info for display and logging
				if info, err := GetIPAndTimezone(pm.GetProxyWithConn(workerID)); err == nil {
					geoInfo = info
					displayMu.Lock()
					workerIPs[workerID] = info.IP
					displayMu.Unlock()
				}
				time.Sleep(time.Second * 3)
				continue
			}

			// Success!
			// Format: username:password|User-Agent|android_id;device_id;family_device_id;waterfall_id|X-MID=mid;sessionid=sid;IG-U-DS-USER-ID=pkid;Authorization=token;|||
			ua := hm.Headers["user-agent"]
			ids := fmt.Sprintf("%s;%s;%s;%s", AndroidID, DeviceID, FamilyDeviceID, WaterfallID)
			authString := fmt.Sprintf("X-MID=%s;sessionid=%s;IG-U-DS-USER-ID=%s;Authorization=%s;", mid, finalSessionID, finalPKID, finalToken)

			resultLine := fmt.Sprintf("%s:%s|%s|%s|%s;done:%d|||", finalUsername, password, ua, ids, authString, i)
			logSuccessAtomically(resultLine)

			// Increment and save success count for this phone
			dataMu.Lock()
			phoneRegCounts[phoneNum]++
			newCount := phoneRegCounts[phoneNum]
			dataMu.Unlock()
			saveBackup(phoneNum, newCount)

			// Force a display refresh to show the incremented Success: total and phone [1/1]
			updateDisplay(workerID, phoneNum, "DONE", fmt.Sprintf("SUCCESS (done:%d)", i))

			success = true
			break
		}
		time.Sleep(time.Second * 2)
	}

	if !success {
		logFailureAtomically(workerID, phoneNum, "Finalize", "STEP8_FAILED")
	}

	return success
}
