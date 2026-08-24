package api

import (
	"net/http"
	"time"

	"nxiiot-gateway/internal/queue"
	"nxiiot-gateway/internal/storage"
)

// storeForwardStatusDTO matches §16's Store & Forward panel fields.
type storeForwardStatusDTO struct {
	PendingRecords     int64      `json:"pending_records"`
	SendingRecords     int64      `json:"sending_records"`
	OldestPending      *time.Time `json:"oldest_pending,omitempty"`
	NewestPending      *time.Time `json:"newest_pending,omitempty"`
	RetryCount         int64      `json:"retry_count"`
	StorageUsedPercent *float64   `json:"storage_used_percent,omitempty"`
	StorageLevel       string     `json:"storage_level,omitempty"`
	ServerConnected    bool       `json:"server_connected"`
	ServerLastError    string     `json:"server_last_error,omitempty"`
	ServerLastSentAt   *time.Time `json:"server_last_sent_at,omitempty"`
}

func (s *Server) getStoreForwardStatus(w http.ResponseWriter, r *http.Request) {
	stats, err := s.queueRepo.Stats(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	dto := storeForwardStatusDTO{
		PendingRecords: stats.PendingCount,
		SendingRecords: stats.SendingCount,
		OldestPending:  stats.OldestPending,
		NewestPending:  stats.NewestPending,
		RetryCount:     stats.TotalRetries,
	}

	if pct, err := storage.DiskUsagePercent(s.cfg.Database.Path); err == nil {
		dto.StorageUsedPercent = &pct
		dto.StorageLevel = string(queue.ClassifyStorageLevel(pct, s.cfg.Queue.StorageFullPercent))
	} else {
		s.log.Warn("failed to read disk usage for store-forward status", "error", err)
	}

	if s.forwarder != nil {
		st := s.forwarder.Status()
		dto.ServerConnected = st.Connected
		dto.ServerLastError = st.LastError
		dto.ServerLastSentAt = st.LastSuccessAt
	}

	writeJSON(w, http.StatusOK, dto)
}

// getStoreForwardStatistics is currently the same view as status; kept as
// a separate endpoint per §21's API list since a dashboard vs. a
// diagnostics page may reasonably want different shapes later (e.g.
// historical throughput), which status alone doesn't cover.
func (s *Server) getStoreForwardStatistics(w http.ResponseWriter, r *http.Request) {
	s.getStoreForwardStatus(w, r)
}
