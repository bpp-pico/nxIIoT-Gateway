package api

import (
	"net/http"
	"strconv"

	"nxiiot-gateway/internal/logger"
)

// getLogs implements §16 "Logs" from the in-memory ring buffer
// (internal/logger.RingBuffer) — there is no log file, this is exactly
// what's been logged to stdout since the process started, capped by the
// buffer's fixed capacity. ?limit= caps how many of the most recent
// entries are returned (default 200).
func (s *Server) getLogs(w http.ResponseWriter, r *http.Request) {
	if s.logBuf == nil {
		writeJSON(w, http.StatusOK, []logger.Entry{})
		return
	}

	entries := s.logBuf.Entries()

	limit := 200
	if v := r.URL.Query().Get("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			limit = n
		}
	}
	if len(entries) > limit {
		entries = entries[len(entries)-limit:]
	}

	writeJSON(w, http.StatusOK, entries)
}
