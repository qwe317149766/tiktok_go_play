package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

type ResultJSONL struct {
	Time       string            `json:"time"`
	Email      string            `json:"email"`
	Success    bool              `json:"success"`
	DeviceID   string            `json:"device_id,omitempty"`
	Proxy      string            `json:"proxy,omitempty"`
	Username   string            `json:"username,omitempty"`
	Cookies    map[string]string `json:"cookies,omitempty"`
	UserSession map[string]string `json:"user_session,omitempty"`
	Error      string            `json:"error,omitempty"`
}

func repoRootDir() string {
	// 尽量定位仓库根目录：向上找 go.mod
	if p := findTopmostFileUpwards("go.mod", 12); strings.TrimSpace(p) != "" {
		return filepath.Dir(p)
	}
	wd, _ := os.Getwd()
	if wd == "" {
		return "."
	}
	return wd
}

func resolveResultsDir() string {
	// 固定目录（相对路径默认相对于仓库根目录）
	// 你要求：默认写到 Log 目录下
	dir := strings.TrimSpace(getEnvStr("DGEMAIL_RESULTS_DIR", "Log"))
	if dir == "" {
		dir = "Log"
	}
	if filepath.IsAbs(dir) {
		return dir
	}
	return filepath.Join(repoRootDir(), dir)
}

func resultsWorkerID() string {
	// 例：w01
	w := strings.TrimSpace(getEnvStr("DGEMAIL_WORKER", getEnvStr("WORKER_ID", "w01")))
	if w == "" {
		return "w01"
	}
	return w
}

func resultsPartID() string {
	// 例：0002
	if s := strings.TrimSpace(getEnvStr("DGEMAIL_PART", "")); s != "" {
		if n, err := strconv.Atoi(s); err == nil && n >= 0 {
			return fmt.Sprintf("%04d", n)
		}
		// 允许直接传 "0002"
		return s
	}
	return "0001"
}

func resultsJSONLPath() string {
	name := fmt.Sprintf("results_%s_part%s.jsonl", resultsWorkerID(), resultsPartID())
	return filepath.Join(resolveResultsDir(), name)
}

func saveResultsJSONLFixed() {
	// 如果不需要 JSONL 结果日志，可以关闭
	if !getEnvBool("DGEMAIL_SAVE_RESULTS_JSONL", true) {
		return
	}
	// 快照结果，避免持锁写文件
	resultMutex.Lock()
	snap := make([]RegisterResult, len(results))
	copy(snap, results)
	resultMutex.Unlock()

	if len(snap) == 0 {
		return
	}

	outDir := resolveResultsDir()
	if err := os.MkdirAll(outDir, 0755); err != nil {
		log.Printf("创建 results dir 失败: %v", err)
		return
	}

	outPath := resultsJSONLPath()
	appendMode := getEnvBool("DGEMAIL_RESULTS_APPEND", true)
	flag := os.O_CREATE | os.O_WRONLY
	if appendMode {
		flag |= os.O_APPEND
	} else {
		flag |= os.O_TRUNC
	}
	f, err := os.OpenFile(outPath, flag, 0644)
	if err != nil {
		log.Printf("创建 results jsonl 失败: %v", err)
		return
	}
	defer f.Close()

	w := bufio.NewWriter(f)
	defer w.Flush()

	now := time.Now().Format("2006-01-02 15:04:05")
	for _, r := range snap {
		line := ResultJSONL{
			Time:       now,
			Email:      r.Email,
			Success:    r.Success,
			DeviceID:   r.DeviceID,
			Proxy:      r.Proxy,
			Username:   r.UserSession["username"],
			Cookies:    r.Cookies,
			UserSession: r.UserSession,
		}
		if r.Error != nil {
			line.Error = r.Error.Error()
		}
		b, err := json.Marshal(line)
		if err != nil {
			continue
		}
		_, _ = w.WriteString(string(b) + "\n")
	}
	fmt.Printf("JSONL 结果日志已写入: %s\n", outPath)
}


