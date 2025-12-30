package main

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"time"
)

func (s *Server) adminAuth(r *http.Request) bool {
	if s.cfg.AdminPasswordMD5 == "" {
		return false
	}
	pass := r.FormValue("password")
	return md5HexLower(pass) == s.cfg.AdminPasswordMD5
}

func writeJSONAny(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	_ = enc.Encode(v)
}

func splitLines(text string) []string {
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")
	parts := strings.Split(text, "\n")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		out = append(out, p)
	}
	return out
}

func (s *Server) handleAdminImportDevices(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		writeJSONAny(w, http.StatusBadRequest, map[string]string{"error": "invalid form"})
		return
	}
	if !s.adminAuth(r) {
		writeJSONAny(w, http.StatusUnauthorized, map[string]string{"error": "invalid password"})
		return
	}

	mode := DeviceImportMode(strings.ToLower(strings.TrimSpace(r.FormValue("mode"))))
	if mode != DeviceImportOverwrite && mode != DeviceImportEvict {
		mode = DeviceImportEvict
	}

	raw := strings.TrimSpace(r.FormValue("devices"))
	if raw == "" {
		writeJSONAny(w, http.StatusBadRequest, map[string]string{"error": "missing devices"})
		return
	}

	lines := splitLines(raw)

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	res, err := s.importDevicesToRedis(ctx, mode, lines)
	if err != nil {
		writeJSONAny(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSONAny(w, http.StatusOK, res)
}

// ---- cookies -> redis (startUp cookie pool) ----

func parseCookieLine(line string) (map[string]string, bool) {
	line = strings.TrimSpace(line)
	if line == "" {
		return nil, false
	}
	if strings.HasPrefix(line, "{") {
		var m map[string]string
		if err := json.Unmarshal([]byte(line), &m); err != nil || len(m) == 0 {
			return nil, false
		}
		return m, true
	}
	// "k=v; k2=v2"
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

func (s *Server) handleAdminImportCookies(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		writeJSONAny(w, http.StatusBadRequest, map[string]string{"error": "invalid form"})
		return
	}
	if !s.adminAuth(r) {
		writeJSONAny(w, http.StatusUnauthorized, map[string]string{"error": "invalid password"})
		return
	}

	mode := CookieImportMode(strings.ToLower(strings.TrimSpace(r.FormValue("mode"))))
	if mode != CookieImportAppend && mode != CookieImportOverwrite && mode != CookieImportEvict {
		mode = CookieImportAppend
	}
	raw := strings.TrimSpace(r.FormValue("cookies"))
	if raw == "" {
		writeJSONAny(w, http.StatusBadRequest, map[string]string{"error": "missing cookies"})
		return
	}
	lines := splitLines(raw)
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	res, err := s.importCookiesToRedis(ctx, mode, lines)
	if err != nil {
		writeJSONAny(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSONAny(w, http.StatusOK, res)
}

func (s *Server) handleAdminClearCookies(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		writeHTML(w, http.StatusBadRequest, "invalid form")
		return
	}
	if !s.adminAuth(r) {
		writeHTML(w, http.StatusUnauthorized, "invalid password")
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
	defer cancel()
	if err := s.clearStartupCookiePool(ctx, getStartupCookiePoolPrefix()); err != nil {
		writeHTML(w, http.StatusInternalServerError, "redis error: "+err.Error())
		return
	}
	writeHTML(w, http.StatusOK, "ok")
}

func (s *Server) handleAdminCookiesStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	cache, err := s.redisClient()
	if err != nil {
		writeJSONAny(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	prefix := getStartupCookiePoolPrefix()
	idsKey := prefix + ":ids"
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	n, err := cache.rdb.SCard(ctx, idsKey).Result()
	if err != nil {
		writeJSONAny(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSONAny(w, http.StatusOK, map[string]any{"count": n})
}


