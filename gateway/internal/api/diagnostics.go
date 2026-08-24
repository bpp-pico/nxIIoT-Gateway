package api

import "net/http"

// diagnosticsDTO matches §16's Diagnostics panel fields.
type diagnosticsDTO struct {
	ModbusTX          int64   `json:"modbus_tx"`
	ModbusRX          int64   `json:"modbus_rx"`
	AvgResponseTimeMs float64 `json:"avg_response_time_ms"`
	TimeoutCount      int64   `json:"timeout_count"`
	CRCErrorCount     int64   `json:"crc_error_count"`
	RetryCount        int64   `json:"retry_count"`
}

func (s *Server) getDiagnostics(w http.ResponseWriter, r *http.Request) {
	if s.diag == nil {
		writeJSON(w, http.StatusOK, diagnosticsDTO{})
		return
	}
	snap := s.diag.Snapshot()
	writeJSON(w, http.StatusOK, diagnosticsDTO{
		ModbusTX:          snap.TXCount,
		ModbusRX:          snap.RXCount,
		AvgResponseTimeMs: snap.AvgResponseTimeMs,
		TimeoutCount:      snap.TimeoutCount,
		CRCErrorCount:     snap.CRCErrorCount,
		RetryCount:        snap.RetryCount,
	})
}
