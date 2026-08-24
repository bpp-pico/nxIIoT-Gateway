package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Gateway   GatewayConfig   `yaml:"gateway"`
	API       APIConfig       `yaml:"api"`
	Database  DatabaseConfig  `yaml:"database"`
	Queue     QueueConfig     `yaml:"queue"`
	Forwarder ForwarderConfig `yaml:"forwarder"`
	Log       LogConfig       `yaml:"log"`
	MQTT      MQTTConfig      `yaml:"mqtt"`
	Time      TimeConfig      `yaml:"time"`
}

type GatewayConfig struct {
	ID   string `yaml:"id"`
	Name string `yaml:"name"`
}

type APIConfig struct {
	ListenAddr string `yaml:"listen_addr"`
}

type DatabaseConfig struct {
	Path string `yaml:"path"`
}

// QueueConfig controls retention and storage-pressure handling of the
// persistent data_queue (§9, §17).
type QueueConfig struct {
	RetentionDays        int     `yaml:"retention_days"`
	SweepInterval        int     `yaml:"sweep_interval_minutes"`
	StorageFullPercent   float64 `yaml:"storage_full_percent"`
	StorageSweepInterval int     `yaml:"storage_sweep_interval_seconds"`
	EvictBatchSize       int     `yaml:"evict_batch_size"`
}

// ForwarderConfig controls Store & Forward (§9). Transport selects which
// Adapter main.go wires up: "http" targets the dev/test adapter
// (cmd/server-sim, ServerURL); "mqtt" targets MQTTAdapter (see MQTTConfig).
type ForwarderConfig struct {
	Transport      string `yaml:"transport"`
	ServerURL      string `yaml:"server_url"`
	BatchSize      int    `yaml:"batch_size"`
	PollIntervalMs int    `yaml:"poll_interval_ms"`
	SendTimeoutMs  int    `yaml:"send_timeout_ms"`
}

type LogConfig struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
}

// MQTTConfig configures MQTTAdapter (Phase 5, §15). DataTopic/AckTopic
// default to "gateway/<client_id>/data" and "/ack" when left blank — see
// Load. TLS is only applied when TLS.Enabled is true.
type MQTTConfig struct {
	BrokerURL        string        `yaml:"broker_url"`
	ClientID         string        `yaml:"client_id"`
	Username         string        `yaml:"username"`
	Password         string        `yaml:"password"`
	QoS              int           `yaml:"qos"`
	DataTopic        string        `yaml:"data_topic"`
	AckTopic         string        `yaml:"ack_topic"`
	KeepAliveSec     int           `yaml:"keepalive_seconds"`
	ConnectTimeoutMs int           `yaml:"connect_timeout_ms"`
	PublishTimeoutMs int           `yaml:"publish_timeout_ms"`
	AckTimeoutMs     int           `yaml:"ack_timeout_ms"`
	TLS              MQTTTLSConfig `yaml:"tls"`
}

// MQTTTLSConfig is deliberately file-path based rather than embedding raw
// PEM in YAML, so the exported config JSON (§18) never has to worry about
// leaking key material — only paths, which are meaningless off the host.
type MQTTTLSConfig struct {
	Enabled            bool   `yaml:"enabled"`
	CAFile             string `yaml:"ca_file"`
	CertFile           string `yaml:"cert_file"`
	KeyFile            string `yaml:"key_file"`
	InsecureSkipVerify bool   `yaml:"insecure_skip_verify"`
}

// TimeConfig configures the Time Service (Phase 6, §11-§14). NTPServer
// empty disables NTP sync (the service still reports RTC/UNSYNCED per
// §11's fallback priority). RTCDevice is only used on Linux — see
// internal/time/rtc_linux.go.
type TimeConfig struct {
	NTPServer       string `yaml:"ntp_server"`
	Timezone        string `yaml:"timezone"`
	SyncIntervalSec int    `yaml:"sync_interval_seconds"`
	QueryTimeoutMs  int    `yaml:"query_timeout_ms"`
	RTCDevice       string `yaml:"rtc_device"`
}

// Load reads configuration from path, then applies GATEWAY_CONFIG env var
// overrides are handled by the caller; environment variable expansion in the
// YAML file itself (e.g. ${VAR}) is supported via os.ExpandEnv.
func Load(path string) (*Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}

	expanded := os.ExpandEnv(string(raw))

	var cfg Config
	if err := yaml.Unmarshal([]byte(expanded), &cfg); err != nil {
		return nil, err
	}

	if cfg.Queue.RetentionDays <= 0 {
		cfg.Queue.RetentionDays = 30
	}
	if cfg.Queue.SweepInterval <= 0 {
		cfg.Queue.SweepInterval = 60
	}
	if cfg.Queue.StorageFullPercent <= 0 {
		cfg.Queue.StorageFullPercent = 95
	}
	if cfg.Queue.StorageSweepInterval <= 0 {
		cfg.Queue.StorageSweepInterval = 60
	}
	if cfg.Queue.EvictBatchSize <= 0 {
		cfg.Queue.EvictBatchSize = 100
	}
	if cfg.Forwarder.BatchSize <= 0 {
		cfg.Forwarder.BatchSize = 100
	}
	if cfg.Forwarder.PollIntervalMs <= 0 {
		cfg.Forwarder.PollIntervalMs = 1000
	}
	if cfg.Forwarder.SendTimeoutMs <= 0 {
		cfg.Forwarder.SendTimeoutMs = 5000
	}
	if cfg.Forwarder.Transport == "" {
		cfg.Forwarder.Transport = "http"
	}

	if cfg.MQTT.QoS <= 0 {
		cfg.MQTT.QoS = 1
	}
	if cfg.MQTT.ClientID == "" {
		cfg.MQTT.ClientID = cfg.Gateway.ID
	}
	if cfg.MQTT.DataTopic == "" {
		cfg.MQTT.DataTopic = "gateway/" + cfg.MQTT.ClientID + "/data"
	}
	if cfg.MQTT.AckTopic == "" {
		cfg.MQTT.AckTopic = "gateway/" + cfg.MQTT.ClientID + "/ack"
	}
	if cfg.MQTT.KeepAliveSec <= 0 {
		cfg.MQTT.KeepAliveSec = 30
	}
	if cfg.MQTT.ConnectTimeoutMs <= 0 {
		cfg.MQTT.ConnectTimeoutMs = 5000
	}
	if cfg.MQTT.PublishTimeoutMs <= 0 {
		cfg.MQTT.PublishTimeoutMs = 5000
	}
	if cfg.MQTT.AckTimeoutMs <= 0 {
		cfg.MQTT.AckTimeoutMs = 10000
	}

	if cfg.Time.SyncIntervalSec <= 0 {
		cfg.Time.SyncIntervalSec = 300
	}
	if cfg.Time.QueryTimeoutMs <= 0 {
		cfg.Time.QueryTimeoutMs = 3000
	}
	if cfg.Time.RTCDevice == "" {
		cfg.Time.RTCDevice = "/dev/rtc0"
	}
	if cfg.Time.Timezone == "" {
		cfg.Time.Timezone = "UTC"
	}

	return &cfg, nil
}

// Save writes cfg back to path as YAML, replacing its previous contents.
// Used by the Web UI's Settings save (§16/§18): the Gateway/MQTT/Time
// settings the doc describes as configured via config.yaml can now be
// edited from the browser instead of by hand.
//
// This is a full re-marshal of the struct, not a targeted patch, so any
// hand-written comments/formatting in the existing file are lost once a
// save happens through the API. That's a deliberate simplicity tradeoff
// (a comment-preserving YAML patcher is significantly more code) — the
// file remains fully valid, readable YAML, just without the original
// inline comments.
func Save(path string, cfg *Config) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	return nil
}
