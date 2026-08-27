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
// Connection-level fields (protocol, interface, baud rate, etc.) live on
// connectionDTO now — a device only names its connection_id, slave id,
// polling interval, and enabled flag.
type deviceDTO struct {
	ID                int64  `json:"id"`
	Name              string `json:"name"`
	ConnectionID      int64  `json:"connection_id"`
	SlaveID           int    `json:"slave_id"`
	PollingIntervalMs int    `json:"polling_interval_ms"`
	Enabled           bool   `json:"enabled"`

	Status   string `json:"status,omitempty"`
	LastSeen string `json:"last_seen,omitempty"`

	// LastPollDurationMs/DatapointsPolled reflect the most recent full poll
	// cycle for this device (see acquisition.OnPollCycle) — for tuning
	// polling_interval_ms against real hardware response times. Zero/absent
	// until the device has completed at least one poll cycle.
	LastPollDurationMs *int64 `json:"last_poll_duration_ms,omitempty"`
	DatapointsPolled   int    `json:"datapoints_polled,omitempty"`
}

func (s *Server) toDeviceDTO(d device.Device) deviceDTO {
	dto := deviceDTO{
		ID:                d.ID,
		Name:              d.Name,
		ConnectionID:      d.ConnectionID,
		SlaveID:           d.SlaveID,
		PollingIntervalMs: d.PollingIntervalMs,
		Enabled:           d.Enabled,
	}
	if info, ok := s.status.Get(d.ID); ok {
		dto.Status = info.Quality
		dto.LastSeen = info.LastSeen.Format("2006-01-02T15:04:05.000Z")
		if info.DatapointsPolled > 0 {
			dto.LastPollDurationMs = &info.LastPollDurationMs
			dto.DatapointsPolled = info.DatapointsPolled
		}
	}
	return dto
}

func (dto deviceDTO) toDevice() device.Device {
	return device.Device{
		ID:                dto.ID,
		Name:              dto.Name,
		ConnectionID:      dto.ConnectionID,
		SlaveID:           dto.SlaveID,
		PollingIntervalMs: dto.PollingIntervalMs,
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
