package api

import (
	"context"
	"net/http"
	"time"

	"nxiiot-gateway/internal/acquisition"
	"nxiiot-gateway/internal/modbus"
)

type testConnectionResult struct {
	Quality string `json:"quality"`
	Error   string `json:"error,omitempty"`
}

// testDeviceConnection implements the §16 "Test Connection" action: open a
// short-lived Modbus connection to the device and report whether it
// succeeded. It does not touch the Manager's long-lived polling connection.
func (s *Server) testDeviceConnection(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	d, err := s.deviceRepo.Get(r.Context(), id)
	if err != nil {
		writeDeviceRepoError(w, err)
		return
	}

	client, err := acquisition.BuildClient(d)
	if err != nil {
		writeJSON(w, http.StatusOK, testConnectionResult{Quality: string(modbus.Invalid), Error: err.Error()})
		return
	}

	connErr := client.Connect()
	_ = client.Close()

	result := testConnectionResult{Quality: string(modbus.QualityFromError(connErr))}
	if connErr != nil {
		result.Error = connErr.Error()
	}
	writeJSON(w, http.StatusOK, result)
}

type testReadResult struct {
	Tag     string   `json:"tag"`
	Value   *float64 `json:"value"`
	Unit    string   `json:"unit,omitempty"`
	Quality string   `json:"quality"`
	Error   string   `json:"error,omitempty"`
}

// testDataPointRead implements the §16 "Test Read" action: perform one
// on-demand read of a single Data Point using the same decode/scale/quality
// logic as the polling engine.
func (s *Server) testDataPointRead(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	dp, err := s.datapointRepo.Get(r.Context(), id)
	if err != nil {
		writeDataPointRepoError(w, err)
		return
	}

	d, err := s.deviceRepo.Get(r.Context(), dp.DeviceID)
	if err != nil {
		writeDeviceRepoError(w, err)
		return
	}

	result := testReadResult{Tag: dp.TagName, Unit: dp.Unit}

	client, err := acquisition.BuildClient(d)
	if err != nil {
		result.Quality = string(modbus.Invalid)
		result.Error = err.Error()
		writeJSON(w, http.StatusOK, result)
		return
	}

	if err := client.Connect(); err != nil {
		result.Quality = string(modbus.QualityFromError(err))
		result.Error = err.Error()
		writeJSON(w, http.StatusOK, result)
		return
	}
	defer client.Close()

	dt := modbus.DataType(dp.DataType)
	qty, err := dt.RegisterCount()
	if err != nil {
		result.Quality = string(modbus.Invalid)
		result.Error = err.Error()
		writeJSON(w, http.StatusOK, result)
		return
	}

	ctx, cancel := context.WithTimeout(r.Context(), time.Duration(d.TimeoutMs)*time.Millisecond)
	defer cancel()

	raw, _, err := modbus.ReadWithRetry(ctx, client, modbus.FunctionCode(dp.FunctionCode), dp.RegisterAddress, qty, d.Retry)
	if err != nil {
		result.Quality = string(modbus.QualityFromError(err))
		result.Error = err.Error()
		writeJSON(w, http.StatusOK, result)
		return
	}

	decoded, err := modbus.Decode(raw, dt, dp.ByteOrder)
	if err != nil {
		result.Quality = string(modbus.Invalid)
		result.Error = err.Error()
		writeJSON(w, http.StatusOK, result)
		return
	}

	value := modbus.ApplyScale(decoded, dp.Scale, dp.Offset)
	result.Value = &value
	result.Quality = string(modbus.Good)
	writeJSON(w, http.StatusOK, result)
}
