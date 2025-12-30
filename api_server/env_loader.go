package main

import (
	"log"
	"os"
	"path/filepath"
	"runtime"

	"github.com/joho/godotenv"
)

// 尝试加载仓库根目录 env.windows/env.linux（复用现有配置习惯）
func loadEnv() {
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

	// 回退到仓库根目录（api_server -> root）
	for _, p := range candidates {
		rootPath := filepath.Join("..", p)
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


