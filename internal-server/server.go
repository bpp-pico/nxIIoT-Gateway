package main

import (
	"context"
	"embed"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"time"
)

//go:embed static/dashboard.html
var dashboardHTML embed.FS

func newHTTPServer(addr string, st *store, c *consumer, log *slog.Logger, onlineThreshold time.Duration) *http.Server {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 3*time.Second)
		defer cancel()
		writeJSON(w, http.StatusOK, map[string]any{
			"status":         "ok",
			"database_ok":    st.Ping(ctx) == nil,
			"mqtt_connected": c.IsConnected(),
		})
	})

	mux.HandleFunc("GET /readings", func(w http.ResponseWriter, r *http.Request) {
		gatewayID := r.URL.Query().Get("gateway_id")
		deviceID := parseOptionalInt64(r.URL.Query().Get("device_id"))
		datapointID := parseOptionalInt64(r.URL.Query().Get("datapoint_id"))
		limitParam := r.URL.Query().Get("limit")
		limit := 100
		if limitParam != "" {
			if n, err := strconv.Atoi(limitParam); err == nil && n > 0 && n <= 5000 {
				limit = n
			}
		}
		var since *time.Time
		if v := r.URL.Query().Get("since"); v != "" {
			if t, err := time.Parse(time.RFC3339, v); err == nil {
				since = &t
				if limitParam == "" {
					limit = 5000 // an unbounded time window with no explicit limit still needs a cap
				}
			}
		}
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		rows, err := st.recent(ctx, gatewayID, deviceID, datapointID, since, limit)
		if err != nil {
			log.Error("query readings failed", "error", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, rows)
	})

	mux.HandleFunc("GET /latest", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		rows, err := st.latest(ctx)
		if err != nil {
			log.Error("query latest failed", "error", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, rows)
	})

	mux.HandleFunc("GET /stats", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 5*time.Second)
		defer cancel()
		rows, err := st.stats(ctx, onlineThreshold)
		if err != nil {
			log.Error("query stats failed", "error", err)
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		writeJSON(w, http.StatusOK, rows)
	})

	mux.HandleFunc("GET /dashboard", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		f, err := dashboardHTML.ReadFile("static/dashboard.html")
		if err != nil {
			http.Error(w, "dashboard unavailable", http.StatusInternalServerError)
			return
		}
		_, _ = w.Write(f)
	})

	mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/dashboard", http.StatusFound)
	})

	return &http.Server{Addr: addr, Handler: mux}
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func parseOptionalInt64(s string) *int64 {
	if s == "" {
		return nil
	}
	n, err := strconv.ParseInt(s, 10, 64)
	if err != nil {
		return nil
	}
	return &n
}
