package config

import (
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

// ForwarderConfig controls Store & Forward (§9). ServerURL targets the
// dev/test HTTP adapter (cmd/server-sim) — Phase 5 adds a real MQTT
// adapter (see MQTTConfig) as an alternative transport behind the same
// forwarder.Adapter interface.
type ForwarderConfig struct {
	ServerURL      string `yaml:"server_url"`
	BatchSize      int    `yaml:"batch_size"`
	PollIntervalMs int    `yaml:"poll_interval_ms"`
	SendTimeoutMs  int    `yaml:"send_timeout_ms"`
}

type LogConfig struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
}

type MQTTConfig struct {
	BrokerURL string `yaml:"broker_url"`
	ClientID  string `yaml:"client_id"`
	QoS       int    `yaml:"qos"`
}

type TimeConfig struct {
	NTPServer string `yaml:"ntp_server"`
	Timezone  string `yaml:"timezone"`
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

	return &cfg, nil
}
