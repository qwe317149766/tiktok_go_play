package registration

import (
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"time"
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

// ProxyManager handles a pool of proxies
type ProxyManager struct {
	Proxies  []string
	filePath string
	mu       sync.Mutex
}

func NewProxyManager(ctx context.Context, filePath string) (*ProxyManager, error) {
	pm := &ProxyManager{filePath: filePath}
	if err := pm.Reload(ctx); err != nil {
		return nil, err
	}
	return pm, nil
}

func (pm *ProxyManager) Reload(ctx context.Context) error {
	pm.mu.Lock()
	defer pm.mu.Unlock()

	var data []byte
	var err error

	if strings.HasPrefix(pm.filePath, "http://") || strings.HasPrefix(pm.filePath, "https://") {
		client := &http.Client{Timeout: 30 * time.Second}
		req, err := http.NewRequestWithContext(ctx, "GET", pm.filePath, nil)
		if err != nil {
			return err
		}
		resp, err := client.Do(req)
		if err != nil {
			return fmt.Errorf("failed to fetch proxy from URL: %v", err)
		}
		defer resp.Body.Close()
		data, err = io.ReadAll(resp.Body)
		if err != nil {
			return err
		}
	} else {
		data, err = os.ReadFile(pm.filePath)
		if err != nil {
			return fmt.Errorf("failed to read proxy file: %v", err)
		}
	}

	lines := strings.Split(string(data), "\n")
	var proxies []string
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" {
			// Basic validation/normalization
			if !strings.Contains(line, "://") {
				line = "http://" + line
			}
			proxies = append(proxies, line)
		}
	}

	if len(proxies) == 0 {
		return fmt.Errorf("no proxies found in %s", pm.filePath)
	}

	pm.Proxies = proxies
	return nil
}

func (pm *ProxyManager) GetProxyWithConn(workerID int) string {
	pm.mu.Lock()
	defer pm.mu.Unlock()
	if len(pm.Proxies) == 0 {
		return ""
	}
	return pm.Proxies[workerID%len(pm.Proxies)]
}

func (pm *ProxyManager) RotateProxy(workerID int) string {
	// Simple rotation: get the next one in list.
	// Since GetProxyWithConn uses modulo workerID, it always gives same proxy to same worker.
	// In a real scenario, you might want a global counter or per-worker counter.
	// For now, we just return the same one or implement a shift if needed.
	return pm.GetProxyWithConn(workerID)
}

// RegistrationEngine handles the overall registration workflow
type RegistrationEngine struct {
	Concurrency int
	Logger      func(workerID int, phone, step, msg string)
	ApiSem      chan struct{}
}

func NewRegistrationEngine(concurrency int, logger func(int, string, string, string)) *RegistrationEngine {
	return &RegistrationEngine{
		Concurrency: concurrency,
		Logger:      logger,
		ApiSem:      make(chan struct{}, 100), // Protect against too many concurrent API hits
	}
}

// ExecuteThreadsAPI executes a generic Bloks API request with full flexibility
func (e *RegistrationEngine) ExecuteThreadsAPI(ctx context.Context, cfg ThreadsRequestConfig) (string, http.Header, error) {
	apiUrl := "https://i.instagram.com/graphql_www"

	// 1. Header Management
	hm := cfg.HeaderManager
	if hm == nil {
		hm = NewThreadHeaderManager()
		hm.SetIDs(cfg.DeviceID, cfg.AndroidID, cfg.FamilyDeviceID, cfg.WaterfallID, "") // sessionID is handled later if needed
		hm.RandomizeUserAgent()
	}

	hm.FriendlyName = cfg.FriendlyName
	hm.BloksAppID = cfg.BloksAppID

	// 2. Body Construction
	var variables string
	if cfg.VariablesJSON != "" {
		variables = cfg.VariablesJSON
	} else {
		variables = fmt.Sprintf(`{"params":{"params":%q,"bloks_versioning_id":"%s","infra_params":{"device_id":"%s"},"app_id":"%s"},"bk_context":{"is_flipper_enabled":false,"theme_params":[],"debug_tooling_metadata_token":null}}`,
			cfg.InnerParams, hm.BloksVersionID, hm.DeviceID, cfg.BloksAppID)
	}

	// Manual construction to maintain order
	rawPayload := fmt.Sprintf("method=post&pretty=false&format=json&server_timestamps=true&locale=user&purpose=fetch&fb_api_req_friendly_name=%s&client_doc_id=%s&enable_canonical_naming=true&enable_canonical_variable_overrides=true&enable_canonical_naming_ambiguous_type_prefixing=true",
		url.QueryEscape(cfg.FriendlyName),
		url.QueryEscape(hm.ClientDocID))

	for k, v := range cfg.FormOverrides {
		if k != "variables" {
			rawPayload += fmt.Sprintf("&%s=%s", url.QueryEscape(k), url.QueryEscape(v))
		}
	}
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

	req, err := http.NewRequestWithContext(ctx, "POST", apiUrl, payload)
	if err != nil {
		return "", nil, err
	}

	hm.Apply(req)

	e.ApiSem <- struct{}{}
	defer func() { <-e.ApiSem }()

	client := cfg.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
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

	if res.StatusCode != 200 {
		return string(body), res.Header, fmt.Errorf("bad status code: %d", res.StatusCode)
	}

	return string(body), res.Header, nil
}

func (e *RegistrationEngine) RotateClientProxy(client *http.Client, pm *ProxyManager, workerID int) string {
	newProxy := pm.RotateProxy(workerID)
	if client != nil {
		if u, err := url.Parse(newProxy); err == nil {
			if transport, ok := client.Transport.(*http.Transport); ok {
				transport.Proxy = http.ProxyURL(u)
			}
		}
	}
	return newProxy
}

func (e *RegistrationEngine) PollUntilParamSuccess(ctx context.Context, targetApi string, cfg ThreadsRequestConfig, minSleepSec, maxSleepSec int, maxRetries int, pm *ProxyManager, pollTimeoutSec int) (map[string]string, error) {
	round := 1
	consecutiveErrors := 0

	startTime := time.Now()
	timeout := time.Duration(pollTimeoutSec) * time.Second

	for {
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		default:
		}

		if time.Since(startTime) > timeout {
			return nil, fmt.Errorf("polling timeout reached (%v) for %s", timeout, targetApi)
		}

		if maxRetries > 0 && round > maxRetries {
			return nil, fmt.Errorf("reached maximum retries (%d) for %s", maxRetries, targetApi)
		}

		if e.Logger != nil {
			e.Logger(cfg.WorkerID, cfg.PhoneNumber, cfg.BloksAppID, fmt.Sprintf("Poll %d", round))
		}

		body, _, err := e.ExecuteThreadsAPI(ctx, cfg)
		if err != nil {
			if e.Logger != nil {
				e.Logger(cfg.WorkerID, cfg.PhoneNumber, cfg.BloksAppID, fmt.Sprintf("Err: %v (Rotating...)", err))
			}
			e.RotateClientProxy(cfg.HTTPClient, pm, cfg.WorkerID)
			consecutiveErrors++
			if maxRetries > 0 && consecutiveErrors >= 5 {
				return nil, fmt.Errorf("consecutive errors reached limit (5): %v", err)
			}
		} else {
			consecutiveErrors = 0

			if strings.Contains(body, "Please wait a few minutes before you try again") || strings.Contains(body, "rate_limit_error") {
				if e.Logger != nil {
					e.Logger(cfg.WorkerID, cfg.PhoneNumber, cfg.BloksAppID, "Rate limited! Rotating proxy...")
				}
				e.RotateClientProxy(cfg.HTTPClient, pm, cfg.WorkerID)
			}

			if targetApi == "" || targetApi == "null" {
				return map[string]string{"full_response": body}, nil
			}

			params := GetParamsByApiName(targetApi, body)
			if len(params) > 0 {
				return params, nil
			}
			if e.Logger != nil {
				e.Logger(cfg.WorkerID, cfg.PhoneNumber, cfg.BloksAppID, "Wait params...")
			}
		}

		// Use a fixed or random sleep
		var sleepTime int
		if maxSleepSec > minSleepSec {
			// simplistic random
			sleepTime = minSleepSec + (time.Now().Nanosecond() % (maxSleepSec - minSleepSec + 1))
		} else {
			sleepTime = minSleepSec
		}

		if err := Sleep(ctx, time.Duration(sleepTime)*time.Second); err != nil {
			return nil, err
		}
		round++
	}
}
