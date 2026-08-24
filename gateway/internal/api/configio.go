package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"
)

// forwarderExportDTO/mqttExportDTO are read-only backup views of the
// process's static config.yaml settings — see importConfig for why only
// Devices/DataPoints actually get restored.
type forwarderExportDTO struct {
	Transport      string `json:"transport"`
	ServerURL      string `json:"server_url,omitempty"`
	BatchSize      int    `json:"batch_size"`
	PollIntervalMs int    `json:"poll_interval_ms"`
	SendTimeoutMs  int    `json:"send_timeout_ms"`
}

// mqttExportDTO deliberately omits Password (§18: "Do not include
// plaintext passwords in exported configuration unless explicitly
// encrypted") — everything else here is non-secret.
type mqttExportDTO struct {
	BrokerURL    string `json:"broker_url"`
	ClientID     string `json:"client_id"`
	Username     string `json:"username,omitempty"`
	QoS          int    `json:"qos"`
	DataTopic    string `json:"data_topic"`
	AckTopic     string `json:"ack_topic"`
	KeepAliveSec int    `json:"keepalive_seconds"`
	TLSEnabled   bool   `json:"tls_enabled"`
}

type deviceExportDTO struct {
	deviceDTO
	DataPoints []dataPointDTO `json:"data_points"`
}

type configExportDTO struct {
	ExportedAt time.Time          `json:"exported_at"`
	Gateway    gatewayExportDTO   `json:"gateway"`
	Forwarder  forwarderExportDTO `json:"forwarder"`
	MQTT       mqttExportDTO      `json:"mqtt"`
	Time       timeExportDTO      `json:"time"`
	Devices    []deviceExportDTO  `json:"devices"`
}

type gatewayExportDTO struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type timeExportDTO struct {
	NTPServer string `json:"ntp_server,omitempty"`
	Timezone  string `json:"timezone"`
}

// exportConfig implements §18: export the gateway's configuration as
// downloadable JSON. Device/Data Point configuration round-trips exactly
// via importConfig; Gateway/Forwarder/MQTT/Time settings are included for
// backup/documentation but are static config.yaml values at runtime — see
// importConfig.
func (s *Server) exportConfig(w http.ResponseWriter, r *http.Request) {
	devices, err := s.deviceRepo.List(r.Context())
	if err != nil {
		writeError(w, http.StatusInternalServerError, err.Error())
		return
	}

	out := configExportDTO{
		ExportedAt: time.Now().UTC(),
		Gateway:    gatewayExportDTO{ID: s.cfg.Gateway.ID, Name: s.cfg.Gateway.Name},
		Forwarder: forwarderExportDTO{
			Transport:      s.cfg.Forwarder.Transport,
			ServerURL:      s.cfg.Forwarder.ServerURL,
			BatchSize:      s.cfg.Forwarder.BatchSize,
			PollIntervalMs: s.cfg.Forwarder.PollIntervalMs,
			SendTimeoutMs:  s.cfg.Forwarder.SendTimeoutMs,
		},
		MQTT: mqttExportDTO{
			BrokerURL:    s.cfg.MQTT.BrokerURL,
			ClientID:     s.cfg.MQTT.ClientID,
			Username:     s.cfg.MQTT.Username,
			QoS:          s.cfg.MQTT.QoS,
			DataTopic:    s.cfg.MQTT.DataTopic,
			AckTopic:     s.cfg.MQTT.AckTopic,
			KeepAliveSec: s.cfg.MQTT.KeepAliveSec,
			TLSEnabled:   s.cfg.MQTT.TLS.Enabled,
		},
		Time: timeExportDTO{NTPServer: s.cfg.Time.NTPServer, Timezone: s.cfg.Time.Timezone},
	}

	out.Devices = make([]deviceExportDTO, len(devices))
	for i, d := range devices {
		dps, err := s.datapointRepo.ListByDevice(r.Context(), d.ID)
		if err != nil {
			writeError(w, http.StatusInternalServerError, err.Error())
			return
		}
		dpDTOs := make([]dataPointDTO, len(dps))
		for j, dp := range dps {
			dpDTOs[j] = toDataPointDTO(dp)
		}
		out.Devices[i] = deviceExportDTO{deviceDTO: s.toDeviceDTO(d), DataPoints: dpDTOs}
	}

	w.Header().Set("Content-Disposition", `attachment; filename="gateway-config.json"`)
	writeJSON(w, http.StatusOK, out)
}

type configImportResult struct {
	DevicesCreated    int      `json:"devices_created"`
	DevicesUpdated    int      `json:"devices_updated"`
	DataPointsCreated int      `json:"data_points_created"`
	DataPointsUpdated int      `json:"data_points_updated"`
	Errors            []string `json:"errors,omitempty"`
}

// importConfig applies a config export's Devices/DataPoints (§18). Only
// DB-backed state — devices and data points — is actually restored:
// Gateway ID, Forwarder, MQTT, and Time settings live in config.yaml, not
// the database, so there is nothing at runtime for those fields to
// persist into; restoring them means editing the config file and
// restarting the gateway. A device/data point is matched by id: an
// existing id is updated, a missing or zero id is created — restoring
// onto a different (empty) gateway therefore creates everything fresh
// with new ids, which is the common "brand new hardware" restore case.
func (s *Server) importConfig(w http.ResponseWriter, r *http.Request) {
	var payload configExportDTO
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		writeError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return
	}

	var errs []string
	for _, dexp := range payload.Devices {
		if err := dexp.toDevice().Validate(); err != nil {
			errs = append(errs, fmt.Sprintf("device %q: %v", dexp.Name, err))
			continue
		}
		for _, dpDTO := range dexp.DataPoints {
			if err := dpDTO.toDataPoint().Validate(); err != nil {
				errs = append(errs, fmt.Sprintf("device %q, data point %q: %v", dexp.Name, dpDTO.TagName, err))
			}
		}
	}
	if len(errs) > 0 {
		writeJSON(w, http.StatusBadRequest, configImportResult{Errors: errs})
		return
	}

	result := configImportResult{}
	ctx := r.Context()
	for _, dexp := range payload.Devices {
		d := dexp.toDevice()

		var deviceID int64
		if dexp.ID != 0 {
			if _, err := s.deviceRepo.Get(ctx, dexp.ID); err == nil {
				if err := s.deviceRepo.Update(ctx, dexp.ID, d); err != nil {
					result.Errors = append(result.Errors, fmt.Sprintf("update device %q: %v", dexp.Name, err))
					continue
				}
				deviceID = dexp.ID
				result.DevicesUpdated++
			}
		}
		if deviceID == 0 {
			id, err := s.deviceRepo.Create(ctx, d)
			if err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("create device %q: %v", dexp.Name, err))
				continue
			}
			deviceID = id
			result.DevicesCreated++
		}

		for _, dpDTO := range dexp.DataPoints {
			dp := dpDTO.toDataPoint()
			dp.DeviceID = deviceID

			if dpDTO.ID != 0 {
				if _, err := s.datapointRepo.Get(ctx, dpDTO.ID); err == nil {
					if err := s.datapointRepo.Update(ctx, dpDTO.ID, dp); err != nil {
						result.Errors = append(result.Errors, fmt.Sprintf("update data point %q: %v", dpDTO.TagName, err))
						continue
					}
					result.DataPointsUpdated++
					continue
				}
			}
			if _, err := s.datapointRepo.Create(ctx, dp); err != nil {
				result.Errors = append(result.Errors, fmt.Sprintf("create data point %q: %v", dpDTO.TagName, err))
				continue
			}
			result.DataPointsCreated++
		}
	}

	s.reload(r)
	writeJSON(w, http.StatusOK, result)
}
