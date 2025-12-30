package main

import (
	"sync"
)

var (
	globalCookiePool []CookieRecord
	cookieOnce       sync.Once
)

// getCookiesForTask 按 taskID 轮询选择 cookie。
// 如果没有 cookie，则返回 nil（stats.go 会发无 cookie 请求，通常会失败）。
func getCookiesForTask(taskID int) map[string]string {
	if len(globalCookiePool) == 0 {
		return nil
	}
	idx := taskID % len(globalCookiePool)
	return globalCookiePool[idx].Cookies
}


