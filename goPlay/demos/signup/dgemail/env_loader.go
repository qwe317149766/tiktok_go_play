package main

import (
	"log"
	"os"
	"path/filepath"
	"runtime"

	"github.com/joho/godotenv"
)

// loadEnvForDemo 尝试加载 env 文件，使 Go demo 复用 Python 同一套 REDIS_* / MAX_GENERATE 等配置。
//
// 加载优先级：
// 1) ENV_FILE 显式指定
// 2) 当前目录：.env.windows / env.windows 或 .env.linux / env.linux
// 3) 仓库根目录（从 dgemail 回退 4 层）：../../../../.env.* 或 ../../../../env.*
//
// 注意：只要找到文件就加载；未找到则跳过（依赖系统环境变量）。
func loadEnvForDemo() {
	if p := os.Getenv("ENV_FILE"); p != "" {
		_ = godotenv.Overload(p)
		log.Printf("[env] loaded: %s", p)
		return
	}

	var candidates []string
	if runtime.GOOS == "windows" {
		candidates = []string{".env.windows", "env.windows"}
	} else {
		candidates = []string{".env.linux", "env.linux"}
	}

	// 当前目录
	for _, p := range candidates {
		if fileExists(p) {
			_ = godotenv.Overload(p)
			log.Printf("[env] loaded: %s", p)
			return
		}
	}

	// 回退到仓库根目录（dgemail -> signup -> demos -> goPlay -> root）
	for _, p := range candidates {
		rootPath := filepath.Join("..", "..", "..", "..", p)
		if fileExists(rootPath) {
			_ = godotenv.Overload(rootPath)
			log.Printf("[env] loaded: %s", rootPath)
			return
		}
	}
}

func fileExists(path string) bool {
	st, err := os.Stat(path)
	return err == nil && !st.IsDir()
}



