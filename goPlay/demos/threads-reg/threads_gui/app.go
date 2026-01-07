package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"threads_gui/auth"
	"threads_gui/config"
	"threads_gui/registration"
	"time"

	wailsRuntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

// App struct
type App struct {
	ctx        context.Context
	config     *config.AppConfig
	cancelFunc context.CancelFunc
	mu         sync.Mutex
	proxyMgr   *registration.ProxyManager
	smsMgr     *registration.SMSManager
	logChan    chan map[string]interface{}
	statsChan  chan map[string]interface{}
}

// NewApp creates a new App struct
func NewApp() *App {
	return &App{
		config:    config.LoadConfig(),
		logChan:   make(chan map[string]interface{}, 5000),
		statsChan: make(chan map[string]interface{}, 100),
	}
}

// startup is called when the app starts. The context is saved
// so we can call the runtime methods
func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.startLogFlusher()
	a.startStatsFlusher()

	// Check card code automatically on startup
	if a.config.CardCode != "" {
		go a.CheckLogin(a.config.CardCode)
	}

	// Init Proxy Manager
	// Note: It's important to pass ctx here
	pm, err := registration.NewProxyManager(ctx, "proxies.txt")
	if err != nil {
		a.Log("[System] Warning: Could not load proxies.txt: " + err.Error())
	}
	a.proxyMgr = pm
}

// CheckLogin verifies the card code
func (a *App) CheckLogin(code string) map[string]interface{} {
	_, expiry, err := auth.CheckCardCode(code)
	if err != nil {
		return map[string]interface{}{"success": false, "error": err.Error()}
	}

	a.config.CardCode = code
	a.config.Save()

	return map[string]interface{}{
		"success": true,
		"expiry":  expiry.Format("2006-01-02 15:04:05"),
		"mid":     auth.GetMachineID(),
	}
}

// SelectFile opens a file selection dialog
func (a *App) SelectFile(title string) string {
	selection, err := wailsRuntime.OpenFileDialog(a.ctx, wailsRuntime.OpenDialogOptions{
		Title: title,
		Filters: []wailsRuntime.FileFilter{
			{DisplayName: "Text Files (*.txt)", Pattern: "*.txt"},
		},
	})
	if err != nil {
		return ""
	}
	return selection
}

// SelectDirectory opens a directory selection dialog
func (a *App) SelectDirectory(title string) string {
	selection, err := wailsRuntime.OpenDirectoryDialog(a.ctx, wailsRuntime.OpenDialogOptions{
		Title: title,
	})
	if err != nil {
		return ""
	}
	return selection
}

// Log message to frontend (buffered)
func (a *App) Log(msg string) {
	select {
	case a.logChan <- map[string]interface{}{
		"time": time.Now().Format("15:04:05"),
		"msg":  msg,
	}:
	default:
		// Drop log if buffer full to prevent blocking
	}
}

// startLogFlusher starts a goroutine to batch log updates
func (a *App) startLogFlusher() {
	buffer := make([]map[string]interface{}, 0, 100)
	ticker := time.NewTicker(200 * time.Millisecond) // Update UI 5 times per sec max

	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-a.ctx.Done():
				return
			case entry := <-a.logChan:
				buffer = append(buffer, entry)
				if len(buffer) >= 200 {
					logsToFlush := make([]map[string]interface{}, len(buffer))
					copy(logsToFlush, buffer)
					a.flushLogs(logsToFlush)
					buffer = buffer[:0]
				}
			case <-ticker.C:
				if len(buffer) > 0 {
					logsToFlush := make([]map[string]interface{}, len(buffer))
					copy(logsToFlush, buffer)
					a.flushLogs(logsToFlush)
					buffer = buffer[:0]
				}
			}
		}
	}()
}

func (a *App) startStatsFlusher() {
	ticker := time.NewTicker(300 * time.Millisecond) // Update stats max 3 times per sec
	var lastStats map[string]interface{}

	go func() {
		defer ticker.Stop()
		for {
			select {
			case <-a.ctx.Done():
				return
			case s := <-a.statsChan:
				lastStats = s
			case <-ticker.C:
				if lastStats != nil {
					wailsRuntime.EventsEmit(a.ctx, "stats", lastStats)
					lastStats = nil
				}
			}
		}
	}()
}

func (a *App) flushLogs(logs []map[string]interface{}) {
	wailsRuntime.EventsEmit(a.ctx, "log_batch", logs)
}

// RunRegistration is the entry point for the registration process
func (a *App) RunRegistration(params map[string]interface{}) {
	a.mu.Lock()
	if a.cancelFunc != nil {
		a.cancelFunc()
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.cancelFunc = cancel
	a.mu.Unlock()

	mode := params["mode"].(string)

	a.Log(fmt.Sprintf("[System] Registration engine started in %s mode", strings.ToUpper(mode)))

	// Reload Config
	a.config = config.LoadConfig()

	// Start everything in background to prevent UI hang
	go func() {
		defer cancel()

		// 1. Initialize SMS Manager (inside background)
		a.Log("[System] Initializing SMS manager...")
		smsMgr := registration.NewSMSManager()
		a.mu.Lock()
		a.smsMgr = smsMgr
		a.mu.Unlock()

		if a.config.SmsFile == "" {
			a.Log("[Error] No SMS file selected in settings.")
			wailsRuntime.EventsEmit(a.ctx, "engine_status", "stopped")
			return
		}

		a.Log("[System] Loading SMS file: " + a.config.SmsFile)
		err := smsMgr.Load(a.config.SmsFile, a.config.MaxPhoneUsage)
		if err != nil {
			a.Log(fmt.Sprintf("[Error] Failed to load SMS file: %v", err))
			wailsRuntime.EventsEmit(a.ctx, "engine_status", "stopped")
			return
		}
		a.Log("[System] SMS file loaded successfully.")

		// 2. Initialize Proxy Manager (inside background)
		proxyFile := a.config.ProxyFile
		if proxyFile == "" {
			proxyFile = "proxies.txt"
		}

		isURL := strings.HasPrefix(proxyFile, "http://") || strings.HasPrefix(proxyFile, "https://")
		absProxyFile := proxyFile
		if !isURL {
			absProxyFile, _ = filepath.Abs(proxyFile)
			if _, err := os.Stat(proxyFile); os.IsNotExist(err) {
				parentProxy := filepath.Join("..", proxyFile)
				if _, err := os.Stat(parentProxy); err == nil {
					proxyFile = parentProxy
					absProxyFile, _ = filepath.Abs(proxyFile)
				}
			}
		}

		a.Log("[System] Initializing proxy manager...")
		pm, err := registration.NewProxyManager(ctx, absProxyFile)
		if err != nil {
			a.Log("[Error] Failed to load proxy file. " + err.Error())
			wailsRuntime.EventsEmit(a.ctx, "engine_status", "stopped")
			return
		}
		a.Log(fmt.Sprintf("[System] Loaded %d proxies.", len(pm.Proxies)))

		a.mu.Lock()
		a.proxyMgr = pm
		a.mu.Unlock()

		// Get Phones list
		var phones []string
		for p := range smsMgr.Configs {
			phones = append(phones, p)
		}
		a.Log(fmt.Sprintf("[System] Loaded %d phones.", len(phones)))

		if len(phones) == 0 {
			a.Log("[Error] No phones found.")
			wailsRuntime.EventsEmit(a.ctx, "engine_status", "stopped")
			return
		}

		engineLogger := func(workerID int, phone, step, msg string) {
			a.Log(fmt.Sprintf("[w%d][%s] %s", workerID, phone, msg))
		}

		regEngine := registration.NewRegistrationEngine(a.config.Concurrency, engineLogger)

		// Worker Pool
		phoneChan := make(chan string, len(phones))
		for _, p := range phones {
			phoneChan <- p
		}
		close(phoneChan)

		var wg sync.WaitGroup
		sem := make(chan struct{}, a.config.Concurrency)

		var successCnt int64
		var failCnt int64

		regConf := registration.RegConfig{
			PollTimeoutSec:        a.config.PollTimeoutSec,
			SMSWaitTimeoutSec:     a.config.SMSWaitTimeoutSec,
			EnableAuto2FA:         a.config.Auto2FA,
			FinalizeRetries:       a.config.FinalizeRetries,
			EnableHeaderRotation:  a.config.EnableHeaderRotation,
			EnableAnomalousUA:     a.config.EnableAnomalousUA,
			EnableIOS:             a.config.EnableIOS,
			HTTPRequestTimeoutSec: a.config.HttpRequestTimeoutSec,
		}

		wailsRuntime.EventsEmit(a.ctx, "stats", map[string]interface{}{
			"success":       0,
			"failed":        0,
			"total_success": smsMgr.GetTotalSuccessCount(),
		})

		if a.config.MaxRegCount > 0 && smsMgr.GetTotalSuccessCount() >= a.config.MaxRegCount {
			a.Log(fmt.Sprintf("[System] Max registration count (%d) already reached. Task skipped.", a.config.MaxRegCount))
			wailsRuntime.EventsEmit(a.ctx, "engine_status", "stopped")
			return
		}

		a.Log(fmt.Sprintf("[System] Spawning workers (Concurrency: %d)...", a.config.Concurrency))
		workerID := 0

	SpawnLoop:
		for phone := range phoneChan {
			select {
			case <-ctx.Done():
				break SpawnLoop
			default:
			}

			if a.config.MaxRegCount > 0 && smsMgr.GetTotalSuccessCount() >= a.config.MaxRegCount {
				a.Log("[System] Max registration count reached. Stopping...")
				langCode := a.config.Language
				if langCode == "" {
					langCode = "en-US"
				}
				tmpl := config.Languages[langCode].AlertTaskCompleted
				if tmpl == "" {
					tmpl = config.Languages["en-US"].AlertTaskCompleted
				}
				wailsRuntime.EventsEmit(a.ctx, "show_alert", fmt.Sprintf(tmpl, a.config.MaxRegCount))
				wailsRuntime.EventsEmit(a.ctx, "engine_status", "stopped")
				cancel() // Stop all running workers immediately
				break SpawnLoop
			}

			sem <- struct{}{}
			wg.Add(1)
			workerID++

			go func(p string, wid int) {
				defer wg.Done()
				defer func() { <-sem }()

				select {
				case <-ctx.Done():
					return
				default:
				}

				currProxy := pm.GetProxyWithConn(wid)
				transport := &http.Transport{
					MaxIdleConns:        100,
					MaxIdleConnsPerHost: 10,
					IdleConnTimeout:     90 * time.Second,
					DisableKeepAlives:   false, // Explicitly ensure Keep-Alive is ON
				}
				if proxyURL, err := url.Parse(currProxy); err == nil {
					transport.Proxy = http.ProxyURL(proxyURL)
				}
				client := &http.Client{
					Timeout:   time.Duration(a.config.HttpRequestTimeoutSec) * time.Second,
					Transport: transport,
				}

				success, resMsg := regEngine.ProcessRegistration(ctx, client, p, wid, currProxy, smsMgr, pm, regConf)

				if success {
					content := strings.TrimPrefix(resMsg, "SUCCESS:")
					content = strings.TrimPrefix(content, "SUCCESS_2FA:")
					has2FA := strings.Contains(resMsg, "2FA")
					totalSuccess := smsMgr.GetTotalSuccessCount()

					a.saveResult(a.config.CookiePath, p, content, has2FA, int64(totalSuccess))

					targetPath := a.config.SuccessPath
					if !has2FA {
						targetPath = a.config.FailurePath
					}
					a.saveResult(targetPath, p, content, has2FA, int64(totalSuccess))

					newCount := smsMgr.IncrementSuccess(p)
					a.appendBackup(p, newCount)
					newSuccess := atomic.AddInt64(&successCnt, 1)

					select {
					case a.statsChan <- map[string]interface{}{
						"success":       newSuccess,
						"failed":        atomic.LoadInt64(&failCnt),
						"total_success": int64(totalSuccess) + 1,
					}:
					default:
					}

					username := "unknown"
					if parts := strings.Split(content, ":"); len(parts) > 0 {
						username = parts[0]
					}
					a.Log(fmt.Sprintf("[Success] Account %s registered (2FA: %v)", username, has2FA))

					if a.config.PushURL != "" {
						go a.pushToAPI(map[string]interface{}{
							"account": p,
							"status":  "success",
							"data":    content,
							"2fa":     has2FA,
						})
					}
				} else {
					if resMsg == "STOPPED" {
						return
					}
					a.saveFailure(p, resMsg, wid)
					newFail := atomic.AddInt64(&failCnt, 1)

					select {
					case a.statsChan <- map[string]interface{}{
						"success":       atomic.LoadInt64(&successCnt),
						"failed":        newFail,
						"total_success": smsMgr.GetTotalSuccessCount(),
					}:
					default:
					}
					a.Log(fmt.Sprintf("[Failed] Worker %d finished FAILED: %s", wid, resMsg))
				}
			}(phone, workerID)
		}

		wg.Wait()
		a.Log(fmt.Sprintf("[System] All workers finished. Success: %d, Failed: %d", successCnt, failCnt))
		wailsRuntime.EventsEmit(a.ctx, "engine_status", "stopped")
	}()
}

func (a *App) saveResult(dir, phone, content string, is2FA bool, currentTotal int64) {
	if dir == "" || dir == "." {
		if is2FA {
			dir = "./2fa-sccess"
		} else {
			dir = "./2fa-fail"
		}
	}
	os.MkdirAll(dir, 0755)

	maxPerFile := int64(a.config.MaxSuccessPerFile)
	var filename string

	if maxPerFile <= 0 {
		// Unlimited: Single file per day
		filename = fmt.Sprintf("%s-注册成功.txt", time.Now().Format("2006-01-02"))
	} else {
		fileIdx := (currentTotal / maxPerFile) + 1
		filename = fmt.Sprintf("%s-注册成功-%d.txt", time.Now().Format("2006-01-02"), fileIdx)
	}

	fullPath := filepath.Join(dir, filename)

	f, err := os.OpenFile(fullPath, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err == nil {
		defer f.Close()
		content = strings.ReplaceAll(content, "Barcelona", "Instagram")
		f.WriteString(content + "\n")
	}
}

func (a *App) appendBackup(phone string, count int) {
	f, err := os.OpenFile("reg_backup.txt", os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err == nil {
		defer f.Close()
		f.WriteString(fmt.Sprintf("%s----%d\n", phone, count))
	}
}

func (a *App) ResetTotalStats() {
	a.mu.Lock()
	defer a.mu.Unlock()

	// 1. Reset Memory Stats
	if a.smsMgr != nil {
		a.smsMgr.ResetStats()
	}

	// 2. Clear Backup File
	os.WriteFile("reg_backup.txt", []byte(""), 0644)

	// 3. Emit update
	a.statsChan <- map[string]interface{}{
		"success":       0,
		"failed":        0,
		"total_success": 0,
	}
	a.Log("[System] Total statistics and backup cleared.")
}

func (a *App) saveFailure(phone, reason string, workerID int) {
	dt := time.Now().Format("2006-01-02 15:04:05")
	content := fmt.Sprintf("[%s] [w%d] %s: %s", dt, workerID, phone, reason)
	dir := a.config.FailurePath
	if dir == "" || dir == "." {
		dir = "failure"
	}
	os.MkdirAll(dir, 0755)

	filename := fmt.Sprintf("failed_%s.txt", time.Now().Format("2006-01-02"))
	f, err := os.OpenFile(filepath.Join(dir, filename), os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err == nil {
		defer f.Close()
		f.WriteString(content + "\n")
	}
}

func (a *App) GetTotalStats() int {
	smsMgr := registration.NewSMSManager()
	smsMgr.LoadBackup("reg_backup.txt")
	return smsMgr.GetTotalSuccessCount()
}

func (a *App) UpdateConfig(newConf *config.AppConfig) bool {
	a.config = newConf
	return a.config.Save() == nil
}

func (a *App) GetConfig() *config.AppConfig {
	return a.config
}

func (a *App) SetLanguage(lang string) {
	a.config.Language = lang
	a.config.Save()
}

func (a *App) GetLanguagePack(lang string) config.LanguagePack {
	if pack, ok := config.Languages[lang]; ok {
		return pack
	}
	return config.Languages["en-US"]
}

type ArchiveItem struct {
	Name string `json:"name"`
	Path string `json:"path"`
	Size int64  `json:"size"`
	Time string `json:"time"`
}

func (a *App) GetArchives() []ArchiveItem {
	dir := a.config.CookiePath
	if dir == "" {
		dir = "./success_cookies"
	}
	files, err := os.ReadDir(dir)
	if err != nil {
		return []ArchiveItem{}
	}
	var items []ArchiveItem
	for _, f := range files {
		if !f.IsDir() && strings.HasSuffix(f.Name(), ".txt") {
			info, _ := f.Info()
			items = append(items, ArchiveItem{
				Name: f.Name(),
				Path: filepath.Join(dir, f.Name()),
				Size: info.Size(),
				Time: info.ModTime().Format("2006-01-02 15:04"),
			})
		}
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Time > items[j].Time })
	return items
}

func (a *App) OpenFolder(path string) {
	if path == "" {
		return
	}
	abs, _ := filepath.Abs(path)
	if runtime.GOOS == "windows" {
		exec.Command("explorer", abs).Run()
	} else {
		wailsRuntime.BrowserOpenURL(a.ctx, abs)
	}
}

func (a *App) DeleteArchive(path string) bool {
	return os.Remove(path) == nil
}

func (a *App) OpenFile(path string) {
	if path == "" {
		return
	}
	abs, _ := filepath.Abs(path)
	if runtime.GOOS == "windows" {
		exec.Command("explorer", abs).Run()
	} else {
		wailsRuntime.BrowserOpenURL(a.ctx, abs)
	}
}

func (a *App) pushToAPI(data interface{}) {
	if a.config.PushURL == "" {
		return
	}
	jsonData, _ := json.Marshal(data)
	resp, err := http.Post(a.config.PushURL, "application/json", bytes.NewBuffer(jsonData))
	if err == nil {
		resp.Body.Close()
	}
}

func (a *App) TestAPIPushURL(url string) bool {
	if url == "" {
		return false
	}
	testData := map[string]interface{}{"type": "test"}
	jd, _ := json.Marshal(testData)
	c := &http.Client{Timeout: 5 * time.Second}
	resp, err := c.Post(url, "application/json", bytes.NewBuffer(jd))
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode >= 200 && resp.StatusCode < 300
}

func (a *App) StopRegistration() {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.cancelFunc != nil {
		a.Log("[System] Stopping registration engine (User Request)...")
		a.cancelFunc()
		a.cancelFunc = nil
	} else {
		// If already stopped (or nil), ensure frontend knows it's stopped
		a.Log("[System] Engine is not running (Force Stop).")
	}

	// Always emit stopped status to unblock frontend UI
	wailsRuntime.EventsEmit(a.ctx, "engine_status", "stopped")
}
