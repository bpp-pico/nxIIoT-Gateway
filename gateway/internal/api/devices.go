package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"nxiiot-gateway/internal/device"
)

// deviceDTO is the wire representation of a Device, per §5 Device Model.
type deviceDTO struct {
	ID                int64  `json:"id"`
	Name              string `json:"name"`
	Protocol          string `json:"protocol"`
	Interface         string `json:"interface,omitempty"`
	BaudRate          int    `json:"baud_rate,omitempty"`
	DataBits          int    `json:"data_bits,omitempty"`
	Parity            string `json:"parity,omitempty"`
	StopBits          int    `json:"stop_bits,omitempty"`
	IPAddress         string `json:"ip_address,omitempty"`
	Port              int    `json:"port,omitempty"`
	SlaveID           int    `json:"slave_id"`
	PollingIntervalMs int    `json:"polling_interval_ms"`
	TimeoutMs         int    `json:"timeout_ms"`
	Retry             int    `json:"retry"`
	Enabled           bool   `json:"enabled"`

	Status   string `json:"status,omitempty"`
	LastSeen string `json:"last_seen,omitempty"`
}

func (s *Server) toDeviceDTO(d device.Device) deviceDTO {
	dto := deviceDTO{
		ID:                d.ID,
		Name:              d.Name,
		Protocol:          string(d.Protocol),
		Interface:         d.Interface,
		BaudRate:          d.BaudRate,
		DataBits:          d.DataBits,
		Parity:            d.Parity,
		StopBits:          d.StopBits,
		IPAddress:         d.IPAddress,
		Port:              d.Port,
		SlaveID:           d.SlaveID,
		PollingIntervalMs: d.PollingIntervalMs,
		TimeoutMs:         d.TimeoutMs,
		Retry:             d.Retry,
		Enabled:           d.Enabled,
	}
	if info, ok := s.status.Get(d.ID); ok {
		dto.Status = info.Quality
		dto.LastSeen = info.LastSeen.Format("2006-01-02T15:04:05.000Z")
	}
	return dto
}

func (dto deviceDTO) toDevice() device.Device {
	return device.Device{
		ID:                dto.ID,
		Name:              dto.Name,
		Protocol:          device.Protocol(dto.Protocol),
		Interface:         dto.Interface,
		BaudRate:          dto.BaudRate,
		DataBits:          dto.DataBits,
		Parity:            dto.Parity,
		StopBits:          dto.StopBits,
		IPAddress:         dto.IPAddress,
		Port:              dto.Port,
		SlaveID:           dto.SlaveID,
		PollingIntervalMs: dto.PollingIntervalMs,
		TimeoutMs:         dto.TimeoutMs,
		Retry:             dto.Retry,
		Enabled:           dto.Enabled,
	}
}

func (s *Server) listDevices(w http.ResponseWriter, r *http.Request) {
	devices, err := s.deviceRepo.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	out := make([]deviceDTO, len(devices))
	for i, d := range devices {
		out[i] = s.toDeviceDTO(d)
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) createDevice(w http.ResponseWriter, r *http.Request) {
	var dto deviceDTO
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	d := dto.toDevice()
	if err := d.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	id, err := s.deviceRepo.Create(r.Context(), d)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	d.ID = id

	s.reload(r)
	writeJSON(w, http.StatusCreated, s.toDeviceDTO(d))
}

func (s *Server) updateDevice(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	var dto deviceDTO
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	d := dto.toDevice()
	d.ID = id
	if err := d.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := s.deviceRepo.Update(r.Context(), id, d); err != nil {
		writeDeviceRepoError(w, err)
		return
	}

	s.reload(r)
	writeJSON(w, http.StatusOK, s.toDeviceDTO(d))
}

func (s *Server) deleteDevice(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := s.deviceRepo.Delete(r.Context(), id); err != nil {
		writeDeviceRepoError(w, err)
		return
	}

	s.reload(r)
	w.WriteHeader(http.StatusNoContent)
}

func parseID(r *http.Request, param string) (int64, error) {
	return strconv.ParseInt(chi.URLParam(r, param), 10, 64)
}

func writeDeviceRepoError(w http.ResponseWriter, err error) {
	if errors.Is(err, device.ErrNotFound) {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	writeError(w, http.StatusInternalServerError, err.Error())
}
