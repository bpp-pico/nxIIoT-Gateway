package config

import (
	"os"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Gateway  GatewayConfig  `yaml:"gateway"`
	API      APIConfig      `yaml:"api"`
	Database DatabaseConfig `yaml:"database"`
	Queue    QueueConfig    `yaml:"queue"`
	Log      LogConfig      `yaml:"log"`
	MQTT     MQTTConfig     `yaml:"mqtt"`
	Time     TimeConfig     `yaml:"time"`
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

// QueueConfig controls retention of the persistent data_queue (§9, §17).
type QueueConfig struct {
	RetentionDays int `yaml:"retention_days"`
	SweepInterval int `yaml:"sweep_interval_minutes"`
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

	return &cfg, nil
}
