package main

import (
	"encoding/json"
	"log"
	"os"
	"path/filepath"
	"sync"
)

var (
	startupAccountsOnce sync.Once
	startupAccountsMu   sync.Mutex
	startupAccountsPath string
)

func startupAccountsJSONLPath() string {
	startupAccountsOnce.Do(func() {
		// 固定写入 Log（可通过 DGEMAIL_RESULTS_DIR 覆盖目录）
		outDir := resolveResultsDir()
		if err := os.MkdirAll(outDir, 0755); err != nil {
			log.Printf("创建 Log dir 失败: %v", err)
			// 回退到当前目录，至少让文件能写出来
			outDir = "."
		}

		nameTpl := getEnvStr("DGEMAIL_STARTUP_ACCOUNTS_JSONL_NAME", "startup_accounts_%s_part%s.jsonl")
		startupAccountsPath = filepath.Join(outDir, sprintf2(nameTpl, resultsWorkerID(), resultsPartID()))
	})
	return startupAccountsPath
}

func sprintf2(tpl, a, b string) string {
	// 避免额外引入 fmt：这里模板只支持两个 %s
	// 例：startup_accounts_%s_part%s.jsonl
	out := tpl
	out = replaceFirst(out, "%s", a)
	out = replaceFirst(out, "%s", b)
	return out
}

func replaceFirst(s, old, new string) string {
	i := indexOf(s, old)
	if i < 0 {
		return s
	}
	return s[:i] + new + s[i+len(old):]
}

func indexOf(s, sub string) int {
	// strings.Index 的轻量替代，避免再引入 strings（该文件仅做路径拼接）
	// O(n*m) 足够
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

// appendStartupAccountJSONLFixed 将“与 Redis 账号池一致的 account JSON”实时追加写入 Log JSONL
func appendStartupAccountJSONLFixed(acc map[string]interface{}) {
	if acc == nil {
		return
	}
	if !getEnvBool("DGEMAIL_SAVE_STARTUP_ACCOUNTS_JSONL", true) {
		return
	}

	b, err := json.Marshal(acc)
	if err != nil {
		return
	}

	// 多 goroutine 并发写同一个文件：用 mutex 串行化，避免行互相穿插
	startupAccountsMu.Lock()
	defer startupAccountsMu.Unlock()

	p := startupAccountsJSONLPath()
	f, err := os.OpenFile(p, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
	if err != nil {
		log.Printf("写入 startup accounts jsonl 失败: %v", err)
		return
	}
	defer f.Close()

	_, _ = f.Write(append(b, '\n'))
}


