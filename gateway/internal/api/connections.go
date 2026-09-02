package api

import (
	"encoding/json"
	"errors"
	"net/http"

	"nxiiot-gateway/internal/connection"
)

// connectionDTO is the wire representation of a Connection — the physical
// link (protocol, interface/ip+port, baud/parity/stop-bits, timeout,
// retry) one or more devices share. See internal/connection's package doc.
type connectionDTO struct {
	ID        int64  `json:"id"`
	Name      string `json:"name"`
	Protocol  string `json:"protocol"`
	Interface string `json:"interface,omitempty"`
	IPAddress string `json:"ip_address,omitempty"`
	Port      int    `json:"port,omitempty"`
	BaudRate  int    `json:"baud_rate,omitempty"`
	DataBits  int    `json:"data_bits,omitempty"`
	Parity    string `json:"parity,omitempty"`
	StopBits  int    `json:"stop_bits,omitempty"`
	TimeoutMs         int  `json:"timeout_ms"`
	Retry             int  `json:"retry"`
	Enabled           bool `json:"enabled"`
	NextDeviceDelayMs int  `json:"next_device_delay_ms"`
}

func toConnectionDTO(c connection.Connection) connectionDTO {
	return connectionDTO{
		ID:                c.ID,
		Name:              c.Name,
		Protocol:          string(c.Protocol),
		Interface:         c.Interface,
		IPAddress:         c.IPAddress,
		Port:              c.Port,
		BaudRate:          c.BaudRate,
		DataBits:          c.DataBits,
		Parity:            c.Parity,
		StopBits:          c.StopBits,
		TimeoutMs:         c.TimeoutMs,
		Retry:             c.Retry,
		Enabled:           c.Enabled,
		NextDeviceDelayMs: c.NextDeviceDelayMs,
	}
}

func (dto connectionDTO) toConnection() connection.Connection {
	return connection.Connection{
		ID:                dto.ID,
		Name:              dto.Name,
		Protocol:          connection.Protocol(dto.Protocol),
		Interface:         dto.Interface,
		IPAddress:         dto.IPAddress,
		Port:              dto.Port,
		BaudRate:          dto.BaudRate,
		DataBits:          dto.DataBits,
		Parity:            dto.Parity,
		StopBits:          dto.StopBits,
		TimeoutMs:         dto.TimeoutMs,
		Retry:             dto.Retry,
		Enabled:           dto.Enabled,
		NextDeviceDelayMs: dto.NextDeviceDelayMs,
	}
}

func (s *Server) listConnections(w http.ResponseWriter, r *http.Request) {
	conns, err := s.connRepo.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	out := make([]connectionDTO, len(conns))
	for i, c := range conns {
		out[i] = toConnectionDTO(c)
	}
	writeJSON(w, http.StatusOK, out)
}

func (s *Server) createConnection(w http.ResponseWriter, r *http.Request) {
	var dto connectionDTO
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	c := dto.toConnection()
	if err := c.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	id, err := s.connRepo.Create(r.Context(), c)
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}
	c.ID = id

	s.reload(r)
	writeJSON(w, http.StatusCreated, toConnectionDTO(c))
}

func (s *Server) updateConnection(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	var dto connectionDTO
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	c := dto.toConnection()
	c.ID = id
	if err := c.Validate(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := s.connRepo.Update(r.Context(), id, c); err != nil {
		writeConnectionRepoError(w, err)
		return
	}

	s.reload(r)
	writeJSON(w, http.StatusOK, toConnectionDTO(c))
}

func (s *Server) deleteConnection(w http.ResponseWriter, r *http.Request) {
	id, err := parseID(r, "id")
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	if err := s.connRepo.Delete(r.Context(), id); err != nil {
		writeConnectionRepoError(w, err)
		return
	}

	s.reload(r)
	w.WriteHeader(http.StatusNoContent)
}

func writeConnectionRepoError(w http.ResponseWriter, err error) {
	if errors.Is(err, connection.ErrNotFound) {
		writeError(w, http.StatusNotFound, err.Error())
		return
	}
	if errors.Is(err, connection.ErrInUse) {
		writeError(w, http.StatusConflict, err.Error())
		return
	}
	writeError(w, http.StatusInternalServerError, err.Error())
}
