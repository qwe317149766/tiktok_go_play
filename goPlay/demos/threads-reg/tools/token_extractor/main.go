package main

import (
	"fmt"
	"io/ioutil"
	"regexp"
)

func main() {
	// 1. 读取 data.txt
	filePath := "data.txt"
	dataBytes, err := ioutil.ReadFile(filePath)
	if err != nil {
		fmt.Printf("无法读取文件 %s: %v\n", filePath, err)
		return
	}
	data := string(dataBytes)

	// 2. 正则表达式定义 (匹配多层转义的键值对)
	// Go 的正则不支持 JS 的 i 修饰符在编译外设置，使用 (?i)
	// 我们需要匹配: 键名 + [\" \\]* + : + [\" \\]* + 值

	patterns := map[string]string{
		"用户名":           `username[\\"]+:[\\"\s]*[\\"]+([^"\\]+)`,
		"用户 ID":         `pk[\\"]+:[\\"\s]*[\\"]*(\d+)`,
		"Session Nonce": `(?:session_flush_nonce|partially_created_account_nonce)[\\"]+:[\\"\s]*[\\"]+([^"\\]+)`,
	}

	fmt.Println("=== Go 提取工具测试 (Regex 模式) ===")

	for label, pattern := range patterns {
		re := regexp.MustCompile("(?i)" + pattern)
		match := re.FindStringSubmatch(data)
		if len(match) > 1 {
			fmt.Printf("[+] %s: %s\n", label, match[1])
		} else {
			fmt.Printf("[-] %s: 未找到\n", label)
		}
	}

	// 3. 特殊处理 Authorization Token (包含 Bearer IGT:2: 前缀)
	authRe := regexp.MustCompile(`(?i)IG-Set-Authorization[\\"]+:[\\"\s]+Bearer\s+IGT:2:([^"\\]+)`)
	authMatch := authRe.FindStringSubmatch(data)
	if len(authMatch) > 1 {
		token := authMatch[1]
		fmt.Printf("[+] 认证 Token (Part): %s...\n", token[:30])
		fmt.Println("\n[完整 Authorization Header]")
		fmt.Printf("Bearer IGT:2:%s\n", token)
	} else {
		fmt.Println("[-] 认证 Token: 未找到")
	}
}
