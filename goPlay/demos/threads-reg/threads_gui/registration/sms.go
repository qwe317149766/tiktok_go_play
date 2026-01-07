package registration

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
)

type SMSManager struct {
	Configs        map[string]string
	PhoneMaxCounts map[string]int
	PhoneRegCounts map[string]int
	mu             sync.Mutex
	smsSem         chan struct{}
}

type SMSResponse struct {
	Code int    `json:"code"`
	Msg  string `json:"msg"`
	Data struct {
		Code        string `json:"code"`
		CodeTime    string `json:"code_time"`
		ExpiredDate string `json:"expired_date"`
	} `json:"data"`
}

func NewSMSManager() *SMSManager {
	return &SMSManager{
		Configs:        make(map[string]string),
		PhoneMaxCounts: make(map[string]int),
		PhoneRegCounts: make(map[string]int),
		smsSem:         make(chan struct{}, 50), // Default limit
	}
}

func (s *SMSManager) Load(filePath string, defaultMax int) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}

	lines := strings.Split(string(data), "\n")
	seen := make(map[string]bool)
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Split(line, "----")
		if len(parts) >= 2 {
			phone := strings.TrimSpace(parts[0])
			if seen[phone] {
				continue
			}
			seen[phone] = true
			s.Configs[phone] = strings.TrimSpace(parts[1])

			max := defaultMax
			if len(parts) >= 3 {
				if m, err := strconv.Atoi(parts[2]); err == nil {
					max = m
				}
			}
			s.PhoneMaxCounts[phone] = max
		}
	}

	// Always try to load backup to remember previous counts
	// Note: We use an internal version to avoid deadlock
	s.loadBackupInternal("reg_backup.txt")

	return nil
}

func (s *SMSManager) LoadBackup(filePath string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.loadBackupInternal(filePath)
}

func (s *SMSManager) loadBackupInternal(filePath string) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}

	lines := strings.Split(string(data), "\n")
	// No lock here, assumes caller holds it
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.Split(line, "----")
		if len(parts) >= 2 {
			phone := strings.TrimSpace(parts[0])
			count, _ := strconv.Atoi(parts[1])
			// Only update if current count is higher (handling multiple entries)
			if count > s.PhoneRegCounts[phone] {
				s.PhoneRegCounts[phone] = count
			}
		}
	}
	return nil
}

func (s *SMSManager) PollCode(ctx context.Context, phone string, timeout time.Duration, workerID int, logger func(string)) (string, string, error) {
	s.mu.Lock()
	cur := s.PhoneRegCounts[phone]
	max := s.PhoneMaxCounts[phone]
	// If max is 0, use default or assume loaded correctly. If not in map, maybe unlimited or error?
	// Ideally Load sets max.
	if max == 0 {
		max = 5
	} // Fallback
	s.mu.Unlock()

	if cur >= max {
		return "", "", fmt.Errorf("phone %s reached max success limit (%d/%d)", phone, cur, max)
	}

	apiUrl, ok := s.Configs[phone]
	if !ok {
		return "", "", fmt.Errorf("no API URL found for phone: %s", phone)
	}

	deadline := time.Now().Add(timeout)
	if logger != nil {
		logger(fmt.Sprintf("SMS Polling started (Timeout: %v)", timeout))
	}

	pollInterval := 5 * time.Second

	for time.Now().Before(deadline) {
		select {
		case <-ctx.Done():
			return "", "", ctx.Err()
		default:
		}

		code, raw, err := s.fetchCode(apiUrl)

		if err == nil && code != "" {
			return code, raw, nil
		}
		if logger != nil {
			if raw != "" {
				logger(fmt.Sprintf("SMS Poll: %s", raw))
			} else {
				logger("SMS Poll: Waiting...")
			}
		}

		select {
		case <-ctx.Done():
			return "", "", ctx.Err()
		case <-time.After(pollInterval):
		}
	}

	return "", "", fmt.Errorf("timeout waiting for SMS code on %s", phone)
}

func (s *SMSManager) fetchCode(apiUrl string) (string, string, error) {
	s.smsSem <- struct{}{}
	defer func() { <-s.smsSem }()

	req, err := http.NewRequest("GET", apiUrl, nil)
	if err != nil {
		return "", "", err
	}
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", err
	}

	rawBody := strings.TrimSpace(string(body))
	// Log for debug (can be removed)
	// fmt.Printf("SMS RAW: %s\n", rawBody)

	var res SMSResponse
	contentToParse := rawBody
	if err := json.Unmarshal(body, &res); err == nil && res.Data.Code != "" {
		contentToParse = res.Data.Code
	}

	// 1. Try 6 consecutive digits
	re6 := regexp.MustCompile(`\b(\d{6})\b`)
	if m := re6.FindStringSubmatch(contentToParse); len(m) > 1 {
		return m[1], contentToParse, nil
	}

	// 2. Try 3 digits + space + 3 digits
	re33 := regexp.MustCompile(`\b(\d{3})\s+(\d{3})\b`)
	if m := re33.FindStringSubmatch(contentToParse); len(m) > 2 {
		return m[1] + m[2], contentToParse, nil
	}

	// 3. Fallback: Search anywhere for 6 digits if word boundary check failed (e.g. <code>123456</code>)
	reRelaxed := regexp.MustCompile(`(\d{6})`)
	if m := reRelaxed.FindStringSubmatch(contentToParse); len(m) > 1 {
		return m[1], contentToParse, nil
	}

	return "", contentToParse, nil
}

func (s *SMSManager) IncrementSuccess(phone string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.PhoneRegCounts[phone]++
	return s.PhoneRegCounts[phone]
}

func (s *SMSManager) GetSuccessCount(phone string) int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.PhoneRegCounts[phone]
}

func (s *SMSManager) GetTotalSuccessCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	total := 0
	for _, count := range s.PhoneRegCounts {
		total += count
	}
	return total
}

func (s *SMSManager) ResetStats() {
	s.mu.Lock()
	defer s.mu.Unlock()
	for k := range s.PhoneRegCounts {
		s.PhoneRegCounts[k] = 0
	}
}
