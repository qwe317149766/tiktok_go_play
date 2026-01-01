package main

import (
	"sync"
	"sync/atomic"
)

var (
	globalCookiePool []CookieRecord
	cookieOnce       sync.Once
	cookiePoolMu     sync.RWMutex

	// 连续失败触发替换后，避免继续选中同一个 cookie
	bannedCookieMu  sync.RWMutex
	bannedCookieIDs = map[string]bool{}

	cookieReplacedTotal int64
)

func banCookie(id string) {
	if id == "" || id == "default" {
		return
	}
	bannedCookieMu.Lock()
	bannedCookieIDs[id] = true
	bannedCookieMu.Unlock()
}

func getBannedCookieCount() int {
	bannedCookieMu.RLock()
	n := len(bannedCookieIDs)
	bannedCookieMu.RUnlock()
	return n
}

func getCookieReplacedTotal() int64 {
	return atomic.LoadInt64(&cookieReplacedTotal)
}

// getCookiesForTask 按 taskID 轮询选择 cookie（返回 cookieID + cookies）。
// 全 DB 模式：cookies 只能来自 signup 写入的账号 JSON（globalCookiePool 由账号 JSON 抽取得到）。
// 如果没有 cookie，则返回 ("", nil)（stats.go 会发无 cookie 请求，通常会失败）。
func getCookiesForTask(taskID int) (string, map[string]string) {
	cookiePoolMu.RLock()
	n := len(globalCookiePool)
	if n == 0 {
		cookiePoolMu.RUnlock()
		return "", nil
	}
	idx := taskID % n
	rec := globalCookiePool[idx]
	cookiePoolMu.RUnlock()

	// 连续失败阈值触发替换
	if cm := GetCookieManager(); cm != nil && !cm.IsHealthy(rec.ID) {
		// 不再从外部 cookies 池拉取；改为在当前池内轮询找下一个“健康且未 ban”的 cookie。
		banCookie(rec.ID)
		cookiePoolMu.RLock()
		for i := 1; i <= n; i++ {
			j := (idx + i) % n
			cand := globalCookiePool[j]
			bannedCookieMu.RLock()
			banned := bannedCookieIDs[cand.ID]
			bannedCookieMu.RUnlock()
			if banned {
				continue
			}
			if cand.ID == "" {
				continue
			}
			if cm2 := GetCookieManager(); cm2 != nil && !cm2.IsHealthy(cand.ID) {
				continue
			}
			rec = cand
			break
		}
		cookiePoolMu.RUnlock()
	}

	return rec.ID, rec.Cookies
}


