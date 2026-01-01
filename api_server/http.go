package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"log"
	"net/http"
	"strconv"
	"strings"
)

type Server struct {
	cfg  Config
	repo *Repo
	cache *APIKeyCache
}

func NewServer(cfg Config, repo *Repo, cache *APIKeyCache) *Server {
	return &Server{cfg: cfg, repo: repo, cache: cache}
}

func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/api", s.handleAPI)
	s.routesAdmin(mux)
	return mux
}

func (s *Server) handleAPI(w http.ResponseWriter, r *http.Request) {
	// API.md: form-data / x-www-form-urlencoded 都行，这里统一 ParseForm
	if err := r.ParseForm(); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid form"})
		return
	}

	key := r.FormValue("key")
	action := r.FormValue("action")

	if strings.TrimSpace(key) == "" {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "missing key"})
		return
	}
	// 全 DB：进程内 TTL cache 校验：优先读 cache，miss 再回源 DB 并回填 cache
	apiKeyRow, err := s.validateAPIKey(r.Context(), key)
	if err != nil {
		if err == sql.ErrNoRows {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid key"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "auth error"})
		return
	}
	if apiKeyRow == nil || !apiKeyRow.IsActive {
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "key disabled"})
		return
	}

	switch strings.ToLower(action) {
	case "add":
		s.handleAdd(w, r, key)
	case "status":
		s.handleStatus(w, r, key)
	default:
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid action"})
	}
}

func (s *Server) handleAdd(w http.ResponseWriter, r *http.Request, apiKey string) {
	link := r.FormValue("link")
	qtyStr := r.FormValue("quantity")

	qty, err := strconv.ParseInt(strings.TrimSpace(qtyStr), 10, 64)
	if err != nil || qty <= 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid quantity"})
		return
	}

	awemeID, ok := parseAwemeID(link)
	if !ok {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid link"})
		return
	}

	startCount := fetchStartCount(awemeID, link)

	ctx, cancel := withTimeout(r.Context())
	defer cancel()
	orderID, err := s.repo.CreateOrderAndConsumeCredit(ctx, apiKey, awemeID, link, qty, startCount)
	if err != nil {
		msg := err.Error()
		if strings.Contains(msg, "insufficient credit") {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "insufficient credit"})
			return
		}
		if strings.Contains(msg, "disabled") {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "key disabled"})
			return
		}
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "db error"})
		return
	}
	// 额度扣减成功后：刷新 cache（避免缓存变脏）
	_ = s.refreshAPIKeyCache(r.Context(), apiKey)

	// API.md: {"order":"12421"}
	writeJSON(w, http.StatusOK, map[string]string{"order": orderID})
}

func (s *Server) validateAPIKey(parent context.Context, key string) (*APIKeyRow, error) {
	ctx, cancel := withTimeout(parent)
	defer cancel()

	// 1) cache hit
	if s.cache != nil {
		if row, ok, err := s.cache.Get(ctx, key); err == nil && ok {
			return row, nil
		}
	}

	// 2) DB fallback
	row, err := s.repo.GetAPIKey(ctx, key)
	if err != nil {
		return nil, err
	}

	// 3) 回填 cache
	if s.cache != nil {
		_ = s.cache.Set(ctx, row)
	}
	return row, nil
}

func (s *Server) refreshAPIKeyCache(parent context.Context, key string) error {
	if s.cache == nil {
		return nil
	}
	ctx, cancel := withTimeout(parent)
	defer cancel()
	row, err := s.repo.GetAPIKey(ctx, key)
	if err != nil {
		return err
	}
	return s.cache.Set(ctx, row)
}

func (s *Server) handleStatus(w http.ResponseWriter, r *http.Request, apiKey string) {
	// 单订单：order=xxx
	if o := strings.TrimSpace(r.FormValue("order")); o != "" {
		if len(o) < 8 {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid order"})
			return
		}
		ctx, cancel := withTimeout(r.Context())
		defer cancel()
		order, err := s.repo.GetOrderForAPIKeyByOrderID(ctx, apiKey, o)
		if err != nil {
			if err == sql.ErrNoRows {
				writeJSON(w, http.StatusNotFound, map[string]string{"error": "order not found"})
				return
			}
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "db error"})
			return
		}
		writeJSON(w, http.StatusOK, statusResp(order))
		return
	}

	// 批量：orders=1,2,3
	orders := strings.TrimSpace(r.FormValue("orders"))
	if orders == "" {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "missing order/orders"})
		return
	}

	parts := strings.Split(orders, ",")
	oids := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p == "" {
			continue
		}
		if len(p) < 8 {
			continue
		}
		oids = append(oids, p)
	}
	if len(oids) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid orders"})
		return
	}

	ctx, cancel := withTimeout(r.Context())
	defer cancel()
	m, err := s.repo.GetOrdersForAPIKeyByOrderIDs(ctx, apiKey, oids)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "db error"})
		return
	}

	out := make(map[string]any, len(oids))
	for _, oid := range oids {
		if o, ok := m[oid]; ok {
			out[oid] = statusResp(o)
		} else {
			out[oid] = map[string]string{"error": "order not found"}
		}
	}
	writeJSON(w, http.StatusOK, out)
}

func statusResp(o *Order) map[string]string {
	// API.md 要求字符串字段
	return map[string]string{
		"charge":      "0.00000",
		"start_count": strconv.FormatInt(o.StartCount, 10),
		"status":      o.Status,
		"remains":     strconv.FormatInt(o.Remains(), 10),
	}
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		log.Printf("write json error: %v", err)
	}
}


