package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"nxiiot-gateway/internal/datapoint"
)

// dataPointDTO is the wire representation of a Data Point, per §6 Data Point Model.
type dataPointDTO struct {
	ID                int64   `json:"id"`
	DeviceID          int64   `json:"device_id"`
	TagName           string  `json:"tag_name"`
	FunctionCode      uint8   `json:"function_code"`
	RegisterAddress   uint16  `json:"register_address"`
	DataType          string  `json:"data_type"`
	ByteOrder         string  `json:"byte_order,omitempty"`
	WordOrder         string  `json:"word_order,omitempty"`
	Scale             float64 `json:"scale"`
	Offset            float64 `json:"offset"`
	Unit              string  `json:"unit,omitempty"`
	PollingIntervalMs int     `json:"polling_interval_ms"`
	Priority          string  `json:"priority,omitempty"`
	Enabled           bool    `json:"enabled"`
}

func toDataPointDTO(dp datapoint.DataPoint) dataPointDTO {
	return dataPointDTO{
		ID:                dp.ID,
		DeviceID:          dp.DeviceID,
		TagName:           dp.TagName,
		FunctionCode:      dp.FunctionCode,
		RegisterAddress:   dp.RegisterAddress,
		DataType:          dp.DataType,
		ByteOrder:         dp.ByteOrder,
		WordOrder:         dp.WordOrder,
		Scale:             dp.Scale,
		Offset:            dp.Offset,
		Unit:              dp.Unit,
		PollingIntervalMs: dp.PollingIntervalMs,
		Priority:          string(dp.Priority),
		Enabled:           dp.Enabled,
	}
}

func (dto dataPointDTO) toDataPoint() datapoint.DataPoint {
	return datapoint.DataPoint{
		ID:                dto.ID,
		DeviceID:          dto.DeviceID,
		TagName:           dto.TagName,
		FunctionCode:      dto.FunctionCode,
		RegisterAddress:   dto.RegisterAddress,
		DataType:          dto.DataType,
		ByteOrder:         dto.ByteOrder,
		WordOrder:         dto.WordOrder,
		Scale:             dto.Scale,
		Offset:            dto.Offset,
		Unit:              dto.Unit,
		PollingIntervalMs: dto.PollingIntervalMs,
		Priority:          datapoint.Priority(dto.Priority),
		Enabled:           dto.Enabled,
	}
}

func (s *Server) listDataPoints(w http.ResponseWriter, r *http.Request) {
	deviceID, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	points, err := s.datapointRepo.ListByDevice(r.Context(), deviceID)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	out := make([]dataPointDTO, len(points))
	for i, dp := range points {
		out[i] = toDataPointDTO(dp)
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) createDataPoint(w http.ResponseWriter, r *http.Request) {
	deviceID, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	if _, err := s.deviceRepo.Get(r.Context(), deviceID); err != nil {
		writeDeviceRepoError(w, err)
		return
	}

	var dto dataPointDTO
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	dp := dto.toDataPoint()
	dp.DeviceID = deviceID
	if err := dp.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	id, err := s.datapointRepo.Create(r.Context(), dp)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	dp.ID = id

	s.reload(r)
	writeJSON(w, http.StatusCreated, toDataPointDTO(dp))
}

func (s *Server) updateDataPoint(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	existing, err := s.datapointRepo.Get(r.Context(), id)
	if err != nil {
		writeDataPointRepoError(w, err)
		return
	}

	var dto dataPointDTO
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	dp := dto.toDataPoint()
	dp.ID = id
	dp.DeviceID = existing.DeviceID // device_id is immutable via this endpoint
	if err := dp.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := s.datapointRepo.Update(r.Context(), id, dp); err != nil {
		writeDataPointRepoError(w, err)
		return
	}

	s.reload(r)
	writeJSON(w, http.StatusOK, toDataPointDTO(dp))
}

func (s *Server) deleteDataPoint(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := s.datapointRepo.Delete(r.Context(), id); err != nil {
		writeDataPointRepoError(w, err)
		return
	}

	s.reload(r)
	w.WriteHeader(http.StatusNoContent)
}

func writeDataPointRepoError(w http.ResponseWriter, err error) {
	if errors.Is(err, datapoint.ErrNotFound) {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeError(w, http.StatusInternalServerError, err.Error())
}
