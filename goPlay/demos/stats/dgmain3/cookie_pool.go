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
		// 兜底：Redis 没有可替换 cookies 时，用 DEFAULT_COOKIES_JSON 的默认 cookies 替换
		if defRec, ok, _ := defaultCookieFromEnv(); ok {
			newRec = defRec
		} else {
			return
		}
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
		if shouldLoadCookiesFromRedis() {
			// 旧逻辑：从 Redis cookie 池替换
			tryReplaceCookieAt(idx, rec.ID)
			cookiePoolMu.RLock()
			if idx >= 0 && idx < len(globalCookiePool) {
				rec = globalCookiePool[idx]
			}
			cookiePoolMu.RUnlock()
		} else {
			// ✅ 新要求：cookies 只能来自 signup 产出的账号 JSON（本地池/账号池解析结果）
			// 不再从 Redis 单独拉 cookies；改为在当前池内轮询找下一个“健康且未 ban”的 cookie。
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
	}

	// 统计 cookies 使用次数（用于后台导入的 evict：use_count 最大淘汰）
	if shouldLoadCookiesFromRedis() {
		_ = incrStartupCookieUseInRedis(rec.ID, 1)
	}
	return rec.ID, rec.Cookies
}


