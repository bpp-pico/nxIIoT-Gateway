package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"nxiiot-gateway/internal/config"
)

// settingsDTO covers the config.yaml fields the user asked to edit from
// the Web UI (Gateway identity, MQTT broker, Time/NTP, queue retention) —
// everything else in config.yaml (database path, other queue thresholds,
// forwarder batch tuning, log level) stays file-only, no UI surface for it.
type settingsDTO struct {
	Gateway struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	} `json:"gateway"`
	MQTT struct {
		BrokerURL string `json:"broker_url"`
		ClientID  string `json:"client_id"`
		Username  string `json:"username,omitempty"`
		// Password is never sent to the browser (§18) and left unchanged
		// unless a new value is submitted here.
		Password     string `json:"password,omitempty"`
		QoS          int    `json:"qos"`
		DataTopic    string `json:"data_topic,omitempty"`
		AckTopic     string `json:"ack_topic,omitempty"`
		KeepAliveSec int    `json:"keepalive_seconds"`
	} `json:"mqtt"`
	Time struct {
		NTPServer       string `json:"ntp_server"`
		Timezone        string `json:"timezone"`
		SyncIntervalSec int    `json:"sync_interval_seconds"`
	} `json:"time"`
	Queue struct {
		RetentionDays int `json:"retention_days"`
	} `json:"queue"`
}

// normalizeNTPServer strips a leading /etc/ntp.conf-style "server "/"pool "
// token (a common paste mistake — the Web UI field wants a bare
// hostname/IP, not a config-file line) and rejects anything that still
// contains whitespace afterward, since that can never resolve as a
// hostname and would otherwise fail silently at DNS-lookup time with no
// error surfaced anywhere in the UI (see MEMORY.md's 2026-08-27 entry).
// An empty string is valid and disables NTP sync.
func normalizeNTPServer(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", nil
	}
	for _, prefix := range []string{"server ", "pool "} {
		if strings.HasPrefix(strings.ToLower(s), prefix) {
			s = strings.TrimSpace(s[len(prefix):])
			break
		}
	}
	if strings.ContainsAny(s, " \t\r\n") {
		return "", fmt.Errorf("ntp_server must be a bare hostname or IP address, not an ntp.conf-style line (got %q)", raw)
	}
	return s, nil
}

func (s *Server) getSettings(w http.ResponseWriter, r *http.Request) {
	var dto settingsDTO
	dto.Gateway.ID = s.cfg.Gateway.ID
	dto.Gateway.Name = s.cfg.Gateway.Name
	dto.MQTT.BrokerURL = s.cfg.MQTT.BrokerURL
	dto.MQTT.ClientID = s.cfg.MQTT.ClientID
	dto.MQTT.Username = s.cfg.MQTT.Username
	dto.MQTT.QoS = s.cfg.MQTT.QoS
	dto.MQTT.DataTopic = s.cfg.MQTT.DataTopic
	dto.MQTT.AckTopic = s.cfg.MQTT.AckTopic
	dto.MQTT.KeepAliveSec = s.cfg.MQTT.KeepAliveSec
	dto.Time.NTPServer = s.cfg.Time.NTPServer
	dto.Time.Timezone = s.cfg.Time.Timezone
	dto.Time.SyncIntervalSec = s.cfg.Time.SyncIntervalSec
	dto.Queue.RetentionDays = s.cfg.Queue.RetentionDays
	writeJSON(w, http.StatusOK, dto)
}

// saveSettings writes the submitted Gateway/MQTT/Time fields into
// config.yaml and restarts the process so they take effect — the "apply
// behavior" the user chose over live-reloading the MQTT client and Time
// Service in place. In this dev docker-compose stack, air's file watcher
// (which already watches configs/) restarts the rebuilt binary on its
// own; os.Exit here additionally makes this work under any supervisor
// that restarts on exit (systemd Restart=always, Docker's
// restart:unless-stopped), which is what a non-dev deployment actually
// runs under.
func (s *Server) saveSettings(w http.ResponseWriter, r *http.Request) {
	var dto settingsDTO
	if err := json.NewDecoder(r.Body).Decode(&dto); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	if dto.Gateway.ID == "" {
		writeError(w, http.StatusBadRequest, "gateway.id is required")
		return
	}
	if dto.MQTT.BrokerURL == "" {
		writeError(w, http.StatusBadRequest, "mqtt.broker_url is required")
		return
	}

	s.cfg.Gateway.ID = dto.Gateway.ID
	s.cfg.Gateway.Name = dto.Gateway.Name
	s.cfg.MQTT.BrokerURL = dto.MQTT.BrokerURL
	s.cfg.MQTT.ClientID = dto.MQTT.ClientID
	s.cfg.MQTT.Username = dto.MQTT.Username
	if dto.MQTT.Password != "" {
		s.cfg.MQTT.Password = dto.MQTT.Password
	}
	if dto.MQTT.QoS > 0 {
		s.cfg.MQTT.QoS = dto.MQTT.QoS
	}
	if dto.MQTT.DataTopic != "" {
		s.cfg.MQTT.DataTopic = dto.MQTT.DataTopic
	}
	if dto.MQTT.AckTopic != "" {
		s.cfg.MQTT.AckTopic = dto.MQTT.AckTopic
	}
	if dto.MQTT.KeepAliveSec > 0 {
		s.cfg.MQTT.KeepAliveSec = dto.MQTT.KeepAliveSec
	}
	ntpServer, err := normalizeNTPServer(dto.Time.NTPServer)
	if err != nil {
		writeError(w, http.StatusBadRequest, err.Error())
		return
	}
	s.cfg.Time.NTPServer = ntpServer
	s.cfg.Time.Timezone = dto.Time.Timezone
	if dto.Time.SyncIntervalSec > 0 {
		s.cfg.Time.SyncIntervalSec = dto.Time.SyncIntervalSec
	}
	if dto.Queue.RetentionDays > 0 {
		s.cfg.Queue.RetentionDays = dto.Queue.RetentionDays
	}

	if s.configPath == "" {
		writeError(w, http.StatusInternalServerError, "config file path is unknown; cannot save")
		return
	}
	if err := config.Save(s.configPath, s.cfg); err != nil {
		writeError(w, http.StatusInternalServerError, "failed to save config: "+err.Error())
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{"saved": true, "restarting": true})

	s.log.Warn("settings saved via Web UI, restarting gateway to apply")
	go func() {
		time.Sleep(300 * time.Millisecond) // let the HTTP response flush before the process exits
		os.Exit(0)
	}()
}
