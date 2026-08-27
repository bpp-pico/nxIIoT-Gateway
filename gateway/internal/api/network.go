package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"nxiiot-gateway/internal/netconfig"
)

// networkConfirmWindow is how long an applied static IP has to be
// confirmed before it's automatically reverted (netconfig.Service) —
// long enough for an operator to notice the UI came back at the new
// address and click Confirm, short enough that a lockout self-heals
// without anyone needing physical/console access.
const networkConfirmWindow = 45 * time.Second

// networkStatusDTO covers the host network card of the Config page.
// Supported is false on any host without nmcli — this dev environment's
// Windows host and Docker containers included (see internal/netconfig's
// package doc) — in which case every other field is zero-valued and the
// UI should show "not available on this host" rather than a form.
type networkStatusDTO struct {
	Supported bool     `json:"supported"`
	Pending   bool     `json:"pending_confirmation,omitempty"`
	Interface string   `json:"interface,omitempty"`
	Method    string   `json:"method,omitempty"`
	Address   string   `json:"address,omitempty"`
	Prefix    int      `json:"prefix,omitempty"`
	Gateway   string   `json:"gateway,omitempty"`
	DNS       []string `json:"dns,omitempty"`
}

func (s *Server) getNetworkStatus(w http.ResponseWriter, r *http.Request) {
	if s.netSvc == nil {
		writeJSON(w, http.StatusOK, networkStatusDTO{Supported: false})
		return
	}

	info, err := s.netSvc.Current()
	if err != nil {
		if errors.Is(err, netconfig.ErrUnsupported) {
			writeJSON(w, http.StatusOK, networkStatusDTO{Supported: false})
			return
		}
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, networkStatusDTO{
		Supported: true,
		Pending:   s.netSvc.Pending(),
		Interface: info.Interface,
		Method:    info.Method,
		Address:   info.Address,
		Prefix:    info.Prefix,
		Gateway:   info.Gateway,
		DNS:       info.DNS,
	})
}

type applyNetworkRequest struct {
	Interface string   `json:"interface"`
	Address   string   `json:"address"`
	Prefix    int      `json:"prefix"`
	Gateway   string   `json:"gateway"`
	DNS       []string `json:"dns,omitempty"`
}

// applyNetwork implements the risky half of "ตั้งค่า IP ของ gateway":
// applies a new static IP to the host's network interface immediately,
// then relies on netconfig.Service's revert timer to undo it
// automatically if confirmNetwork isn't called within
// networkConfirmWindow — see netconfig.Service's doc comment for why.
func (s *Server) applyNetwork(w http.ResponseWriter, r *http.Request) {
	if s.netSvc == nil {
		writeError(w, http.StatusServiceUnavailable, netconfig.ErrUnsupported.Error())
		return
	}

	var req applyNetworkRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	err := s.netSvc.Apply(netconfig.StaticConfig{
		Interface: req.Interface,
		Address:   req.Address,
		Prefix:    req.Prefix,
		Gateway:   req.Gateway,
		DNS:       req.DNS,
	}, networkConfirmWindow)
	if err != nil {
		if errors.Is(err, netconfig.ErrUnsupported) {
			writeError(w, http.StatusServiceUnavailable, err.Error())
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"applied":                true,
		"confirm_within_seconds": int(networkConfirmWindow.Seconds()),
	})
}

type applyNetworkDHCPRequest struct {
	Interface string `json:"interface"`
}

// applyNetworkDHCP is the DHCP counterpart of applyNetwork — switches
// the interface to automatic addressing immediately, guarded by the
// same confirm-or-revert timer (netconfig.Service.ApplyDHCP), since
// losing a fixed address a device depends on is the same class of
// lockout risk as a bad static apply.
func (s *Server) applyNetworkDHCP(w http.ResponseWriter, r *http.Request) {
	if s.netSvc == nil {
		writeError(w, http.StatusServiceUnavailable, netconfig.ErrUnsupported.Error())
		return
	}

	var req applyNetworkDHCPRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	if err := s.netSvc.ApplyDHCP(req.Interface, networkConfirmWindow); err != nil {
		if errors.Is(err, netconfig.ErrUnsupported) {
			writeError(w, http.StatusServiceUnavailable, err.Error())
			return
		}
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"applied":                true,
		"confirm_within_seconds": int(networkConfirmWindow.Seconds()),
	})
}

func (s *Server) confirmNetwork(w http.ResponseWriter, r *http.Request) {
	if s.netSvc == nil {
		writeError(w, http.StatusServiceUnavailable, netconfig.ErrUnsupported.Error())
		return
	}
	if err := s.netSvc.Confirm(); err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	writeJSON(w, http.StatusOK, map[string]bool{"confirmed": true})
}
