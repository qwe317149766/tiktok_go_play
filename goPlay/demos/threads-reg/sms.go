package main

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"
)

type SMSConfig struct {
	Phone  string
	APIURL string
}

type SMSManager struct {
	Configs map[string]string
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
		Configs: make(map[string]string),
	}
}

func (s *SMSManager) Load(filePath string) error {
	data, err := os.ReadFile(filePath)
	if err != nil {
		return err
	}

	lines := strings.Split(string(data), "\n")
	seen := make(map[string]bool)
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

			// Use global max if not specified in reg.txt
			max := globalMaxSuccessCount
			if len(parts) >= 3 {
				if m, err := strconv.Atoi(parts[2]); err == nil {
					max = m
				}
			}
			// Update global max counts
			dataMu.Lock()
			phoneMaxCounts[phone] = max
			dataMu.Unlock()
		}
	}
	return nil
}

func (s *SMSManager) GetAPI(phone string) string {
	return s.Configs[phone]
}

func (s *SMSManager) PollCode(phone string, timeout time.Duration, workerID int) (string, string, error) {
	// Check if this phone has already reached its limit
	dataMu.Lock()
	cur := phoneRegCounts[phone]
	max := phoneMaxCounts[phone]
	dataMu.Unlock()

	if cur >= max {
		return "", "", fmt.Errorf("phone %s reached max success limit (%d/%d)", phone, cur, max)
	}

	apiUrl, ok := s.Configs[phone]
	if !ok {
		return "", "", fmt.Errorf("no API URL found for phone: %s", phone)
	}

	deadline := time.Now().Add(timeout)
	AsyncLog(fmt.Sprintf("[w%d] [%s] SMS Polling started (Timeout: %v)", workerID, phone, timeout))

	pollInterval := time.Duration(globalConfig.SMSPollIntervalMs) * time.Millisecond
	if pollInterval < 1*time.Second {
		pollInterval = 1 * time.Second
	}

	for time.Now().Before(deadline) {
		code, raw, err := s.fetchCode(apiUrl)

		// Update display... (keep existing logic)
		statusMsg := "Polling..."
		if code != "" {
			statusMsg = "Code: " + code + " (" + raw + ")"
		} else if raw != "" {
			statusMsg = raw
		} else if err != nil {
			statusMsg = fmt.Sprintf("NetErr: %v", err)
		}
		updateDisplay(workerID, phone, "SMS", statusMsg)

		if err == nil && code != "" {
			return code, raw, nil
		}

		time.Sleep(pollInterval)
	}

	return "", "", fmt.Errorf("timeout waiting for SMS code on %s", phone)
}

func (s *SMSManager) fetchCode(apiUrl string) (string, string, error) {
	smsSem <- struct{}{}
	defer func() { <-smsSem }()

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(apiUrl)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", "", err
	}

	rawBody := string(body)

	// Try JSON first
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

	// 2. Try 3 digits + space + 3 digits (Instagram style: "123 456")
	re33 := regexp.MustCompile(`\b(\d{3})\s+(\d{3})\b`)
	if m := re33.FindStringSubmatch(contentToParse); len(m) > 2 {
		return m[1] + m[2], contentToParse, nil
	}

	return "", contentToParse, nil
}
