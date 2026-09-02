package main

import (
	"os"
	"strconv"
)

type config struct {
	MQTTBrokerURL       string
	MQTTClientID        string
	MQTTDataTopicFilter string
	MQTTUsername        string
	MQTTPassword        string
	DatabaseURL         string
	HTTPAddr            string
	// GatewayOnlineThresholdSeconds: a gateway is considered "online" if its
	// most recent ingested reading is newer than this many seconds ago —
	// last-seen-data is a proxy for "MQTT link is up," not a direct
	// connection check (see spec.md's design note on this).
	GatewayOnlineThresholdSeconds int
}

func loadConfig() config {
	return config{
		MQTTBrokerURL:                 getenv("MQTT_BROKER_URL", "tcp://localhost:1883"),
		MQTTClientID:                  getenv("MQTT_CLIENT_ID", "internal-server"),
		MQTTDataTopicFilter:           getenv("MQTT_DATA_TOPIC_FILTER", "gateway/+/data"),
		MQTTUsername:                  os.Getenv("MQTT_USERNAME"),
		MQTTPassword:                  os.Getenv("MQTT_PASSWORD"),
		DatabaseURL:                   getenv("DATABASE_URL", "postgres://internal_server:internal_server@localhost:5432/internal_server?sslmode=disable"),
		HTTPAddr:                      getenv("HTTP_ADDR", ":9100"),
		GatewayOnlineThresholdSeconds: getenvInt("GATEWAY_ONLINE_THRESHOLD_SECONDS", 60),
	}
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func getenvInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}
