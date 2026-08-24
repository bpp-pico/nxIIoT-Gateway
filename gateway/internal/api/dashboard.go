package api

import "net/http"

// dashboardSummaryDTO covers the counts the Dashboard needs (§16) that
// aren't already served by an existing endpoint — the rest of the
// Dashboard panel is assembled by the frontend from GET /api/system,
// /api/store-forward/status, and /api/time.
type dashboardSummaryDTO struct {
	DeviceCount        int64 `json:"device_count"`
	EnabledDeviceCount int64 `json:"enabled_device_count"`
	DataPointCount     int64 `json:"data_point_count"`
}

func (s *Server) getDashboardSummary(w http.ResponseWriter, r *http.Request) {
	total, enabled, err := s.deviceRepo.Count(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	dpCount, err := s.datapointRepo.CountAll(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, dashboardSummaryDTO{
		DeviceCount:        total,
		EnabledDeviceCount: enabled,
		DataPointCount:     dpCount,
	})
}
