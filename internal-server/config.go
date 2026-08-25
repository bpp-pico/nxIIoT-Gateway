package main

import "os"

type config struct {
	MQTTBrokerURL       string
	MQTTClientID        string
	MQTTDataTopicFilter string
	MQTTUsername        string
	MQTTPassword        string
	DatabaseURL         string
	HTTPAddr            string
}

func loadConfig() config {
	return config{
		MQTTBrokerURL:       getenv("MQTT_BROKER_URL", "tcp://localhost:1883"),
		MQTTClientID:        getenv("MQTT_CLIENT_ID", "internal-server"),
		MQTTDataTopicFilter: getenv("MQTT_DATA_TOPIC_FILTER", "gateway/+/data"),
		MQTTUsername:        os.Getenv("MQTT_USERNAME"),
		MQTTPassword:        os.Getenv("MQTT_PASSWORD"),
		DatabaseURL:         getenv("DATABASE_URL", "postgres://internal_server:internal_server@localhost:5432/internal_server?sslmode=disable"),
		HTTPAddr:            getenv("HTTP_ADDR", ":9100"),
	}
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
