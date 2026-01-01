package main

import (
	"context"
	"encoding/json"
	"io"
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

func readFormTextOrFile(r *http.Request, textField string, fileField string, maxBytes int64) (string, error) {
	// 优先 file upload
	if fileField != "" {
		f, _, err := r.FormFile(fileField)
		if err == nil && f != nil {
			defer f.Close()
			limited := io.LimitReader(f, maxBytes)
			b, err := io.ReadAll(limited)
			if err != nil {
				return "", err
			}
			return string(b), nil
		}
	}
	// fallback textarea
	return strings.TrimSpace(r.FormValue(textField)), nil
}

func (s *Server) handleAdminImportDevices(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseMultipartForm(32 << 20); err != nil && err != http.ErrNotMultipart {
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

	raw, err := readFormTextOrFile(r, "devices", "devices_file", 32<<20)
	if err != nil {
		writeJSONAny(w, http.StatusBadRequest, map[string]string{"error": "read devices failed: " + err.Error()})
		return
	}
	if raw == "" {
		writeJSONAny(w, http.StatusBadRequest, map[string]string{"error": "missing devices"})
		return
	}

	lines := splitLines(raw)

	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	res, err := s.importDevicesToDBSharded(ctx, mode, lines)
	if err != nil {
		writeJSONAny(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSONAny(w, http.StatusOK, res)
}

// ---- cookies/accounts -> mysql (startup_cookie_accounts) ----

func (s *Server) handleAdminImportCookies(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseMultipartForm(32 << 20); err != nil && err != http.ErrNotMultipart {
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
	raw, err := readFormTextOrFile(r, "cookies", "cookies_file", 32<<20)
	if err != nil {
		writeJSONAny(w, http.StatusBadRequest, map[string]string{"error": "read cookies failed: " + err.Error()})
		return
	}
	if raw == "" {
		writeJSONAny(w, http.StatusBadRequest, map[string]string{"error": "missing cookies"})
		return
	}
	lines := splitLines(raw)
	ctx, cancel := context.WithTimeout(r.Context(), 30*time.Second)
	defer cancel()
	res, err := s.importCookiesToDBSharded(ctx, mode, lines)
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
	if err := s.clearCookieAccounts(ctx); err != nil {
		writeHTML(w, http.StatusInternalServerError, "db error: "+err.Error())
		return
	}
	writeHTML(w, http.StatusOK, "ok")
}

func (s *Server) handleAdminCookiesStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
	defer cancel()
	stats, err := s.getPoolsStatsDB(ctx)
	if err != nil {
		writeJSONAny(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	// 兼容：只返回 cookies 维度
	m := stats.(map[string]any)
	writeJSONAny(w, http.StatusOK, m["cookies"])
}

func (s *Server) handleAdminPoolsStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 8*time.Second)
	defer cancel()
	stats, err := s.getPoolsStatsDB(ctx)
	if err != nil {
		writeJSONAny(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
		return
	}
	writeJSONAny(w, http.StatusOK, stats)
}


