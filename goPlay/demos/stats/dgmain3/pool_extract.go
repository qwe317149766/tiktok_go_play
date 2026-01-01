package main

import (
	"crypto/sha1"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strings"
)

func devicePoolIDFromDevice(device map[string]interface{}) string {
	// stats 内部用于“健康统计/排除”的主键：默认优先 device_id
	idField := strings.TrimSpace(envStr("DEVICE_POOL_ID_FIELD", envStr("DEVICE_ID_FIELD", "device_id")))
	if idField != "" {
		if v, ok := device[idField]; ok {
			switch t := v.(type) {
			case string:
				if strings.TrimSpace(t) != "" {
					return strings.TrimSpace(t)
				}
			case float64:
				return fmt.Sprintf("%.0f", t)
			default:
				s := strings.TrimSpace(fmt.Sprintf("%v", t))
				if s != "" {
					return s
				}
			}
		}
	}
	// fallback：常见字段
	if v, ok := device["device_id"].(string); ok && strings.TrimSpace(v) != "" {
		return strings.TrimSpace(v)
	}
	if v, ok := device["cdid"].(string); ok && strings.TrimSpace(v) != "" {
		return strings.TrimSpace(v)
	}
	return ""
}

type CookieRecord struct {
	ID      string            `json:"id"`
	Cookies map[string]string `json:"cookies"`
}

func shouldLoadCookiesFromDevicesFile() bool {
	// 从“设备列表/账号 JSON 每行的 cookies 字段”构建 cookie 池
	return strings.EqualFold(envStr("COOKIES_SOURCE", ""), "devices_file") ||
		strings.EqualFold(envStr("COOKIES_SOURCE", ""), "startup_devices_file")
}

func shouldLoadCookiesFromDefault() bool {
	// 使用默认 cookies（从 DEFAULT_COOKIES_JSON 环境变量读取）
	// 支持 COOKIES_SOURCE=default 或 COOKIES_SINK=default
	source := strings.ToLower(strings.TrimSpace(envStr("COOKIES_SOURCE", "")))
	sink := strings.ToLower(strings.TrimSpace(envStr("COOKIES_SINK", "")))
	return source == "default" || sink == "default"
}

func shouldLoadCookiesFromDB() bool {
	// 从数据库加载 cookies（从 startup_cookie_accounts 表读取）
	source := strings.ToLower(strings.TrimSpace(envStr("COOKIES_SOURCE", "")))
	return source == "db" || source == "mysql"
}

func defaultCookieFromEnv() (CookieRecord, bool, error) {
	// DEFAULT_COOKIES_JSON 格式：{"sessionid":"...","sid_tt":"...","uid_tt":"...", ...}
	raw := strings.TrimSpace(os.Getenv("DEFAULT_COOKIES_JSON"))
	if raw == "" {
		return CookieRecord{}, false, nil
	}
	var ck map[string]string
	if err := json.Unmarshal([]byte(raw), &ck); err != nil || len(ck) == 0 {
		return CookieRecord{}, false, fmt.Errorf("DEFAULT_COOKIES_JSON 解析失败或为空")
	}
	return CookieRecord{ID: "default", Cookies: ck}, true, nil
}

func cookieIDFromMap(cookies map[string]string) string {
	if v := strings.TrimSpace(cookies["sessionid"]); v != "" {
		return v
	}
	if v := strings.TrimSpace(cookies["sid_tt"]); v != "" {
		return v
	}
	b, _ := json.Marshal(cookies)
	h := sha1.Sum(b)
	return hex.EncodeToString(h[:])
}

var rePyCookiePair = regexp.MustCompile(`'([^']+)'\s*:\s*'([^']*)'`)

func parseCookiesAny(v any) map[string]string {
	// 支持：
	// 1) JSON object: {"k":"v"}
	// 2) Python dict string: "{'k':'v', 'k2':'v2'}"
	// 3) Cookie header: "k=v; k2=v2"
	switch t := v.(type) {
	case map[string]string:
		if len(t) == 0 {
			return nil
		}
		out := make(map[string]string, len(t))
		for k, v := range t {
			k = strings.TrimSpace(k)
			if k == "" {
				continue
			}
			out[k] = strings.TrimSpace(v)
		}
		if len(out) == 0 {
			return nil
		}
		return out
	case map[string]any:
		if len(t) == 0 {
			return nil
		}
		out := make(map[string]string, len(t))
		for k, vv := range t {
			k = strings.TrimSpace(k)
			if k == "" {
				continue
			}
			if s, ok := vv.(string); ok {
				out[k] = strings.TrimSpace(s)
			}
		}
		if len(out) == 0 {
			return nil
		}
		return out
	case string:
		s := strings.TrimSpace(t)
		if s == "" {
			return nil
		}
		// JSON object
		if strings.HasPrefix(s, "{") && strings.HasSuffix(s, "}") && strings.Contains(s, "\":") {
			var m map[string]string
			if err := json.Unmarshal([]byte(s), &m); err == nil && len(m) > 0 {
				return m
			}
			var m2 map[string]any
			if err := json.Unmarshal([]byte(s), &m2); err == nil && len(m2) > 0 {
				return parseCookiesAny(m2)
			}
		}
		// Python dict string
		if strings.HasPrefix(s, "{") && strings.HasSuffix(s, "}") && strings.Contains(s, "':") {
			matches := rePyCookiePair.FindAllStringSubmatch(s, -1)
			if len(matches) == 0 {
				return nil
			}
			out := make(map[string]string, len(matches))
			for _, mm := range matches {
				if len(mm) != 3 {
					continue
				}
				k := strings.TrimSpace(mm[1])
				if k == "" {
					continue
				}
				out[k] = strings.TrimSpace(mm[2])
			}
			if len(out) == 0 {
				return nil
			}
			return out
		}
		// Cookie header
		if strings.Contains(s, "=") {
			out := map[string]string{}
			parts := strings.Split(s, ";")
			for _, p := range parts {
				p = strings.TrimSpace(p)
				if p == "" {
					continue
				}
				kv := strings.SplitN(p, "=", 2)
				if len(kv) != 2 {
					continue
				}
				k := strings.TrimSpace(kv[0])
				if k == "" {
					continue
				}
				out[k] = strings.TrimSpace(kv[1])
			}
			if len(out) == 0 {
				return nil
			}
			return out
		}
		return nil
	default:
		return nil
	}
}

func cookiesFromStartupDeviceJSONLine(line string) (CookieRecord, bool) {
	line = strings.TrimSpace(line)
	if line == "" {
		return CookieRecord{}, false
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(line), &m); err != nil {
		return CookieRecord{}, false
	}
	raw, ok := m["cookies"]
	if !ok {
		return CookieRecord{}, false
	}
	ck := parseCookiesAny(raw)
	if len(ck) == 0 {
		return CookieRecord{}, false
	}
	id := ""
	if did, ok := m["device_id"]; ok {
		switch t := did.(type) {
		case string:
			id = strings.TrimSpace(t)
		case float64:
			id = fmt.Sprintf("%.0f", t)
		default:
			id = strings.TrimSpace(fmt.Sprintf("%v", t))
		}
	}
	if id == "" {
		id = cookieIDFromMap(ck)
	}
	return CookieRecord{ID: id, Cookies: ck}, true
}

func loadCookiesFromStartupDevices(lines []string, limit int) []CookieRecord {
	out := make([]CookieRecord, 0, len(lines))
	seen := map[string]bool{}
	for _, line := range lines {
		rec, ok := cookiesFromStartupDeviceJSONLine(line)
		if !ok || rec.ID == "" {
			continue
		}
		if seen[rec.ID] {
			continue
		}
		seen[rec.ID] = true
		out = append(out, rec)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out
}


