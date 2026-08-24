// server-sim is a minimal fake "Internal Server" used only for local
// development and testing of the Store & Forward engine (internal/forwarder).
// It is not part of the gateway binary and is never deployed to production.
//
// It demonstrates the server-side half of Rule 7/10 (at-least-once
// delivery + idempotent processing): it deduplicates on gateway_id +
// sequence_id, so a batch retried after a lost ACK does not double-count.
//
// It speaks both transports behind forwarder.Adapter (§15): HTTP (/ingest,
// for HTTPAdapter) and, when -mqtt-broker is set, MQTT (for MQTTAdapter) —
// subscribing to the data topic and publishing an application-level ack
// back per batch, exactly what a real Internal Server's MQTT consumer
// would do. Both share the same in-memory dedup store.
package main

import (
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"os"
	"strings"
	"sync"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

type entry struct {
	GatewayID      string   `json:"gateway_id"`
	SequenceID     int64    `json:"sequence_id"`
	DeviceID       int64    `json:"device_id"`
	DatapointID    int64    `json:"datapoint_id"`
	Value          *float64 `json:"value"`
	Quality        string   `json:"quality"`
	EventTimestamp string   `json:"event_timestamp"`
	Priority       string   `json:"priority"`
}

type key struct {
	gatewayID  string
	sequenceID int64
}

type store struct {
	mu       sync.Mutex
	seen     map[key]entry
	received []entry
}

func newStore() *store {
	return &store{seen: make(map[key]entry)}
}

func (s *store) ingest(batch []entry) (accepted, duplicates int) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, e := range batch {
		k := key{e.GatewayID, e.SequenceID}
		if _, dup := s.seen[k]; dup {
			duplicates++
			continue
		}
		s.seen[k] = e
		s.received = append(s.received, e)
		accepted++
	}
	return accepted, duplicates
}

func (s *store) all() []entry {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]entry, len(s.received))
	copy(out, s.received)
	return out
}

func main() {
	addr := flag.String("addr", ":9000", "listen address")
	mqttBroker := flag.String("mqtt-broker", "", "MQTT broker URL (e.g. tcp://mosquitto:1883); empty disables the MQTT consumer")
	mqttDataTopic := flag.String("mqtt-data-topic", "gateway/+/data", "MQTT topic filter to subscribe to for incoming batches")
	flag.Parse()

	if v := os.Getenv("MQTT_BROKER_URL"); v != "" {
		*mqttBroker = v
	}

	st := newStore()

	if *mqttBroker != "" {
		startMQTTConsumer(*mqttBroker, *mqttDataTopic, st)
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/ingest", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var batch []entry
		if err := json.NewDecoder(r.Body).Decode(&batch); err != nil {
			http.Error(w, "invalid JSON: "+err.Error(), http.StatusBadRequest)
			return
		}

		accepted, duplicates := st.ingest(batch)
		log.Printf("ingest: received=%d accepted=%d duplicates=%d", len(batch), accepted, duplicates)

		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]int{"accepted": accepted, "duplicates": duplicates})
	})
	mux.HandleFunc("/received", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(st.all())
	})

	log.Printf("server-sim listening on %s", *addr)
	if err := http.ListenAndServe(*addr, mux); err != nil {
		log.Fatal(err)
	}
}

// mqttBatchIn mirrors forwarder.mqttBatch — the payload MQTTAdapter
// publishes to the data topic.
type mqttBatchIn struct {
	BatchID string  `json:"batch_id"`
	Entries []entry `json:"entries"`
}

// mqttAckOut mirrors the ack MQTTAdapter waits for on the ack topic.
type mqttAckOut struct {
	BatchID string `json:"batch_id"`
	Error   string `json:"error,omitempty"`
}

// startMQTTConsumer subscribes to dataTopicFilter (e.g. "gateway/+/data")
// and, for every batch received, dedupes it into st and publishes an
// application-level ack (§15) back to the sender's ack topic — derived by
// swapping the topic's "/data" suffix for "/ack", matching the default
// topic scheme MQTTAdapter derives in config.Load.
func startMQTTConsumer(broker, dataTopicFilter string, st *store) {
	opts := mqtt.NewClientOptions().
		AddBroker(broker).
		SetClientID("server-sim").
		SetAutoReconnect(true).
		SetConnectRetry(true)

	opts.SetOnConnectHandler(func(c mqtt.Client) {
		log.Printf("mqtt: connected to %s, subscribing to %s", broker, dataTopicFilter)
		token := c.Subscribe(dataTopicFilter, 1, func(client mqtt.Client, msg mqtt.Message) {
			var batch mqttBatchIn
			if err := json.Unmarshal(msg.Payload(), &batch); err != nil {
				log.Printf("mqtt: invalid batch payload on %s: %v", msg.Topic(), err)
				return
			}

			accepted, duplicates := st.ingest(batch.Entries)
			log.Printf("mqtt ingest: topic=%s batch_id=%s received=%d accepted=%d duplicates=%d",
				msg.Topic(), batch.BatchID, len(batch.Entries), accepted, duplicates)

			ackTopic := strings.TrimSuffix(msg.Topic(), "/data") + "/ack"
			ack, _ := json.Marshal(mqttAckOut{BatchID: batch.BatchID})
			client.Publish(ackTopic, 1, false, ack)
		})
		token.Wait()
		if token.Error() != nil {
			log.Printf("mqtt: subscribe failed: %v", token.Error())
		}
	})
	opts.SetConnectionLostHandler(func(_ mqtt.Client, err error) {
		log.Printf("mqtt: connection lost, will auto-reconnect: %v", err)
	})

	client := mqtt.NewClient(opts)
	token := client.Connect()
	token.Wait()
	if token.Error() != nil {
		log.Fatalf("mqtt: initial connect to %s failed: %v", broker, token.Error())
	}
}
