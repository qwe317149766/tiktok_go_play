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

func tryReplaceCookieAt(idx int, oldID string) {
	if !shouldLoadCookiesFromRedis() {
		return
	}
	if oldID == "" || oldID == "default" {
		return
	}

	// 构造 exclude：当前池里的 cookie + 已 ban 的 cookie
	exclude := map[string]bool{}
	cookiePoolMu.RLock()
	for _, r := range globalCookiePool {
		if r.ID != "" {
			exclude[r.ID] = true
		}
	}
	cookiePoolMu.RUnlock()
	bannedCookieMu.RLock()
	for id := range bannedCookieIDs {
		exclude[id] = true
	}
	bannedCookieMu.RUnlock()

	newRec, err := pickOneStartupCookieFromRedis(exclude)
	if err != nil || newRec.ID == "" {
		return
	}

	cookiePoolMu.Lock()
	// 二次校验 idx 仍然有效
	if idx >= 0 && idx < len(globalCookiePool) && globalCookiePool[idx].ID == oldID {
		globalCookiePool[idx] = newRec
		banCookie(oldID)
		atomic.AddInt64(&cookieReplacedTotal, 1)
	}
	cookiePoolMu.Unlock()
}

// getCookiesForTask 按 taskID 轮询选择 cookie（返回 cookieID + cookies）。
// Redis 模式下：当 cookie 连续失败达到阈值，会自动替换为新 cookie。
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
		tryReplaceCookieAt(idx, rec.ID)
		cookiePoolMu.RLock()
		if idx >= 0 && idx < len(globalCookiePool) {
			rec = globalCookiePool[idx]
		}
		cookiePoolMu.RUnlock()
	}

	// 统计 cookies 使用次数（用于后台导入的 evict：use_count 最大淘汰）
	_ = incrStartupCookieUseInRedis(rec.ID, 1)
	return rec.ID, rec.Cookies
}


