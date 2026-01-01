package main

import (
	"context"
	"encoding/json"
	"fmt"
	"hash/crc32"
	"strconv"
	"strings"
)

// --- shared ---

func stableShard(key string, shards int) int {
	if shards <= 1 {
		return 0
	}
	key = strings.TrimSpace(key)
	if key == "" {
		return 0
	}
	return int(crc32.ChecksumIEEE([]byte(key)) % uint32(shards))
}

func dbDevicePoolShards() int {
	n := getenvInt("DB_DEVICE_POOL_SHARDS", 1)
	if n <= 0 {
		return 1
	}
	return n
}

func dbCookiePoolShards() int {
	n := getenvInt("DB_COOKIE_POOL_SHARDS", 1)
	if n <= 0 {
		return 1
	}
	return n
}

func dbMaxDevices() int64 {
	return int64(getenvInt("DB_MAX_DEVICES", 0))
}

func dbMaxCookies() int64 {
	return int64(getenvInt("DB_MAX_COOKIES", 0))
}

func dbDeviceTable() string {
	t := strings.TrimSpace(getenv("DB_DEVICE_POOL_TABLE", "device_pool_devices"))
	if t == "" {
		return "device_pool_devices"
	}
	return t
}

func dbCookieTable() string {
	t := strings.TrimSpace(getenv("DB_COOKIE_POOL_TABLE", "startup_cookie_accounts"))
	if t == "" {
		return "startup_cookie_accounts"
	}
	return t
}

// --- devices import ---

type DeviceImportMode string

const (
	DeviceImportOverwrite DeviceImportMode = "overwrite"
	DeviceImportEvict     DeviceImportMode = "evict"
)

type PoolStat struct {
	Idx   int   `json:"idx"`
	Count int64 `json:"count"`
}

type DeviceImportResult struct {
	Mode         DeviceImportMode `json:"mode"`
	Table        string           `json:"table"`
	Shards       int              `json:"shards"`
	MaxDevices   int64            `json:"max_devices"`
	InputCount   int              `json:"input_count"`
	AddedCount   int              `json:"added_count"`
	InvalidCount int              `json:"invalid_count"`
	PerShard     []PoolStat       `json:"per_shard"`
	Message      string           `json:"message"`
}

func normalizeJSONLine(line string) (string, map[string]any, error) {
	line = strings.TrimSpace(line)
	if line == "" {
		return "", nil, fmt.Errorf("empty line")
	}
	var m map[string]any
	if err := json.Unmarshal([]byte(line), &m); err != nil {
		return "", nil, err
	}
	b, _ := json.Marshal(m)
	return string(b), m, nil
}

func extractDeviceID(m map[string]any, idField string) (string, bool) {
	if m == nil {
		return "", false
	}
	if v, ok := m[idField]; ok {
		switch t := v.(type) {
		case string:
			s := strings.TrimSpace(t)
			if s != "" {
				return s, true
			}
		case float64:
			return fmt.Sprintf("%.0f", t), true
		default:
			s := strings.TrimSpace(fmt.Sprintf("%v", t))
			if s != "" {
				return s, true
			}
		}
	}
	if v, ok := m["device_id"]; ok {
		switch t := v.(type) {
		case string:
			s := strings.TrimSpace(t)
			if s != "" {
				return s, true
			}
		case float64:
			return fmt.Sprintf("%.0f", t), true
		}
	}
	return "", false
}

func (s *Server) importDevicesToDBSharded(ctx context.Context, mode DeviceImportMode, lines []string) (*DeviceImportResult, error) {
	if s == nil || s.repo == nil || s.repo.db == nil {
		return nil, fmt.Errorf("db not initialized")
	}
	tbl := dbDeviceTable()
	shards := dbDevicePoolShards()
	max := dbMaxDevices()
	if max < 0 {
		max = 0
	}

	if mode == DeviceImportOverwrite {
		if _, err := s.repo.db.ExecContext(ctx, fmt.Sprintf("DELETE FROM `%s`", tbl)); err != nil {
			return nil, err
		}
	}

	idField := strings.TrimSpace(getenv("DEVICE_ID_FIELD", "device_id"))
	if idField == "" {
		idField = "device_id"
	}

	var added, invalid int
	for _, line := range lines {
		raw, m, err := normalizeJSONLine(line)
		if err != nil {
			invalid++
			continue
		}
		did, ok := extractDeviceID(m, idField)
		if !ok || did == "" {
			invalid++
			continue
		}
		sh := stableShard(did, shards)
		_, err = s.repo.db.ExecContext(ctx,
			fmt.Sprintf(`INSERT INTO %s (shard_id, device_id, device_json) VALUES (?,?,?)
ON DUPLICATE KEY UPDATE shard_id=VALUES(shard_id), device_json=VALUES(device_json)`, "`"+tbl+"`"),
			sh, did, raw)
		if err != nil {
			return nil, err
		}
		added++
	}

	// evict 维持上限：按 use_count 最大淘汰
	if mode == DeviceImportEvict && max > 0 {
		for sh := 0; sh < shards; sh++ {
			var cnt int64
			if err := s.repo.db.QueryRowContext(ctx, fmt.Sprintf("SELECT COUNT(*) FROM `%s` WHERE shard_id=?", tbl), sh).Scan(&cnt); err != nil {
				return nil, err
			}
			if cnt > max {
				need := cnt - max
				_, _ = s.repo.db.ExecContext(ctx, fmt.Sprintf("DELETE FROM `%s` WHERE shard_id=? ORDER BY use_count DESC LIMIT %d", tbl, need), sh)
			}
		}
	}

	// stats
	per := make([]PoolStat, 0, shards)
	for sh := 0; sh < shards; sh++ {
		var cnt int64
		if err := s.repo.db.QueryRowContext(ctx, fmt.Sprintf("SELECT COUNT(*) FROM `%s` WHERE shard_id=?", tbl), sh).Scan(&cnt); err != nil {
			return nil, err
		}
		per = append(per, PoolStat{Idx: sh, Count: cnt})
	}

	return &DeviceImportResult{
		Mode:         mode,
		Table:        tbl,
		Shards:       shards,
		MaxDevices:   max,
		InputCount:   len(lines),
		AddedCount:   added,
		InvalidCount: invalid,
		PerShard:     per,
		Message:      "ok",
	}, nil
}

// --- cookies/accounts import ---

type CookieImportMode string

const (
	CookieImportAppend    CookieImportMode = "append"
	CookieImportOverwrite CookieImportMode = "overwrite"
	CookieImportEvict     CookieImportMode = "evict"
)

type CookieImportResult struct {
	Mode         CookieImportMode `json:"mode"`
	Table        string           `json:"table"`
	Shards       int              `json:"shards"`
	MaxCookies   int64            `json:"max_cookies"`
	InputCount   int              `json:"input_count"`
	AddedCount   int              `json:"added_count"`
	InvalidCount int              `json:"invalid_count"`
	PerShard     []PoolStat       `json:"per_shard"`
	Message      string           `json:"message"`
}

func parseCookieLine(line string) (map[string]string, bool) {
	line = strings.TrimSpace(line)
	if line == "" {
		return nil, false
	}
	if strings.HasPrefix(line, "{") {
		var m map[string]string
		if err := json.Unmarshal([]byte(line), &m); err == nil && len(m) > 0 {
			return m, true
		}
		// 可能是账号 JSON：{"cookies":{...}}
		var anym map[string]any
		if err := json.Unmarshal([]byte(line), &anym); err == nil {
			if raw, ok := anym["cookies"]; ok {
				b, _ := json.Marshal(raw)
				var mm map[string]string
				if err := json.Unmarshal(b, &mm); err == nil && len(mm) > 0 {
					return mm, true
				}
			}
		}
		return nil, false
	}
	out := map[string]string{}
	parts := strings.Split(line, ";")
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
		v := strings.TrimSpace(kv[1])
		if k == "" {
			continue
		}
		out[k] = v
	}
	if len(out) == 0 {
		return nil, false
	}
	return out, true
}

func cookieIDFromMap(cookies map[string]string) string {
	if v := strings.TrimSpace(cookies["sessionid"]); v != "" {
		return v
	}
	if v := strings.TrimSpace(cookies["sid_tt"]); v != "" {
		return v
	}
	b, _ := json.Marshal(cookies)
	return strconv.FormatUint(uint64(crc32.ChecksumIEEE(b)), 10)
}

func (s *Server) importCookiesToDBSharded(ctx context.Context, mode CookieImportMode, lines []string) (*CookieImportResult, error) {
	if s == nil || s.repo == nil || s.repo.db == nil {
		return nil, fmt.Errorf("db not initialized")
	}
	tbl := dbCookieTable()
	shards := dbCookiePoolShards()
	max := dbMaxCookies()
	if max < 0 {
		max = 0
	}

	if mode == CookieImportOverwrite {
		if _, err := s.repo.db.ExecContext(ctx, fmt.Sprintf("DELETE FROM `%s`", tbl)); err != nil {
			return nil, err
		}
	}

	var added, invalid int
	for _, line := range lines {
		ck, ok := parseCookieLine(line)
		if !ok || len(ck) == 0 {
			invalid++
			continue
		}
		deviceKey := cookieIDFromMap(ck)
		sh := stableShard(deviceKey, shards)
		acc := map[string]any{
			"device_id": deviceKey,
			"cookies":   ck,
		}
		raw, _ := json.Marshal(acc)
		_, err := s.repo.db.ExecContext(ctx,
			fmt.Sprintf(`INSERT INTO %s (shard_id, device_key, account_json) VALUES (?,?,?)
ON DUPLICATE KEY UPDATE shard_id=VALUES(shard_id), account_json=VALUES(account_json)`, "`"+tbl+"`"),
			sh, deviceKey, string(raw))
		if err != nil {
			return nil, err
		}
		added++
	}

	if mode == CookieImportEvict && max > 0 {
		for sh := 0; sh < shards; sh++ {
			var cnt int64
			if err := s.repo.db.QueryRowContext(ctx, fmt.Sprintf("SELECT COUNT(*) FROM `%s` WHERE shard_id=?", tbl), sh).Scan(&cnt); err != nil {
				return nil, err
			}
			if cnt > max {
				need := cnt - max
				_, _ = s.repo.db.ExecContext(ctx, fmt.Sprintf("DELETE FROM `%s` WHERE shard_id=? ORDER BY use_count DESC LIMIT %d", tbl, need), sh)
			}
		}
	}

	per := make([]PoolStat, 0, shards)
	for sh := 0; sh < shards; sh++ {
		var cnt int64
		if err := s.repo.db.QueryRowContext(ctx, fmt.Sprintf("SELECT COUNT(*) FROM `%s` WHERE shard_id=?", tbl), sh).Scan(&cnt); err != nil {
			return nil, err
		}
		per = append(per, PoolStat{Idx: sh, Count: cnt})
	}

	return &CookieImportResult{
		Mode:         mode,
		Table:        tbl,
		Shards:       shards,
		MaxCookies:   max,
		InputCount:   len(lines),
		AddedCount:   added,
		InvalidCount: invalid,
		PerShard:     per,
		Message:      "ok",
	}, nil
}

func (s *Server) clearCookieAccounts(ctx context.Context) error {
	tbl := dbCookieTable()
	_, err := s.repo.db.ExecContext(ctx, fmt.Sprintf("DELETE FROM `%s`", tbl))
	return err
}

func (s *Server) getPoolsStatsDB(ctx context.Context) (any, error) {
	devTbl := dbDeviceTable()
	ckTbl := dbCookieTable()
	devSh := dbDevicePoolShards()
	ckSh := dbCookiePoolShards()

	devPer := make([]PoolStat, 0, devSh)
	var devTotal int64
	for i := 0; i < devSh; i++ {
		var cnt int64
		if err := s.repo.db.QueryRowContext(ctx, fmt.Sprintf("SELECT COUNT(*) FROM `%s` WHERE shard_id=?", devTbl), i).Scan(&cnt); err != nil {
			return nil, err
		}
		devTotal += cnt
		devPer = append(devPer, PoolStat{Idx: i, Count: cnt})
	}

	ckPer := make([]PoolStat, 0, ckSh)
	var ckTotal int64
	for i := 0; i < ckSh; i++ {
		var cnt int64
		if err := s.repo.db.QueryRowContext(ctx, fmt.Sprintf("SELECT COUNT(*) FROM `%s` WHERE shard_id=?", ckTbl), i).Scan(&cnt); err != nil {
			return nil, err
		}
		ckTotal += cnt
		ckPer = append(ckPer, PoolStat{Idx: i, Count: cnt})
	}

	return map[string]any{
		"device_table": devTbl,
		"cookie_table": ckTbl,
		"devices": map[string]any{
			"count":     devTotal,
			"shards":    devSh,
			"per_shard": devPer,
		},
		"cookies": map[string]any{
			"count":     ckTotal,
			"shards":    ckSh,
			"per_shard": ckPer,
		},
	}, nil
}


