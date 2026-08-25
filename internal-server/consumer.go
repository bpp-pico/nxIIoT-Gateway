package main

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"strings"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
)

// wireEntry/wireBatch/wireAck mirror the payloads MQTTAdapter publishes
// (gateway/internal/forwarder/wire.go, mqttadapter.go) — this is the
// production counterpart to cmd/server-sim's dev-only in-memory version.
type wireEntry struct {
	GatewayID      string   `json:"gateway_id"`
	SequenceID     int64    `json:"sequence_id"`
	DeviceID       int64    `json:"device_id"`
	DatapointID    int64    `json:"datapoint_id"`
	Value          *float64 `json:"value"`
	Quality        string   `json:"quality"`
	EventTimestamp string   `json:"event_timestamp"`
	Priority       string   `json:"priority"`
}

type wireBatch struct {
	BatchID string      `json:"batch_id"`
	Entries []wireEntry `json:"entries"`
}

type wireAck struct {
	BatchID string `json:"batch_id"`
	Error   string `json:"error,omitempty"`
}

type consumer struct {
	client mqtt.Client
	cfg    config
	store  *store
	log    *slog.Logger

	mu        sync.Mutex
	connected bool
}

func newConsumer(cfg config, st *store, log *slog.Logger) *consumer {
	c := &consumer{cfg: cfg, store: st, log: log}

	opts := mqtt.NewClientOptions().
		AddBroker(cfg.MQTTBrokerURL).
		SetClientID(cfg.MQTTClientID).
		SetAutoReconnect(true).
		SetConnectRetry(true).
		SetConnectRetryInterval(5 * time.Second).
		SetKeepAlive(30 * time.Second).
		SetOnConnectHandler(c.onConnect).
		SetConnectionLostHandler(c.onConnectionLost)

	if cfg.MQTTUsername != "" {
		opts.SetUsername(cfg.MQTTUsername)
		opts.SetPassword(cfg.MQTTPassword)
	}

	c.client = mqtt.NewClient(opts)
	return c
}

func (c *consumer) Connect(ctx context.Context) error {
	token := c.client.Connect()
	if !token.WaitTimeout(15 * time.Second) {
		return errors.New("mqtt connect: timed out")
	}
	return token.Error()
}

func (c *consumer) Disconnect() { c.client.Disconnect(250) }

func (c *consumer) IsConnected() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.connected
}

// onConnect (re-)subscribes to the data topic filter every time paho
// (re)connects — including after a dropped connection — and, unlike the
// gateway-side onConnect this exists to compensate for, retries the
// subscribe itself rather than only logging a failure: a subscribe that
// loses the race with a fresh disconnect is retried up to 3 times with a
// short backoff before giving up and waiting for the next reconnect.
func (c *consumer) onConnect(cl mqtt.Client) {
	c.mu.Lock()
	c.connected = true
	c.mu.Unlock()
	c.log.Info("mqtt connected", "broker", c.cfg.MQTTBrokerURL, "topic_filter", c.cfg.MQTTDataTopicFilter)

	go c.subscribeWithRetry(cl)
}

func (c *consumer) subscribeWithRetry(cl mqtt.Client) {
	for attempt := 1; attempt <= 3; attempt++ {
		token := cl.Subscribe(c.cfg.MQTTDataTopicFilter, 1, c.handleMessage)
		token.Wait()
		if token.Error() == nil {
			return
		}
		c.log.Error("mqtt subscribe failed, retrying", "attempt", attempt, "error", token.Error())
		time.Sleep(time.Duration(attempt) * time.Second)
	}
	c.log.Error("mqtt subscribe gave up after 3 attempts; waiting for next reconnect")
}

func (c *consumer) onConnectionLost(_ mqtt.Client, err error) {
	c.mu.Lock()
	c.connected = false
	c.mu.Unlock()
	c.log.Warn("mqtt connection lost, will auto-reconnect", "error", err)
}

func (c *consumer) handleMessage(cl mqtt.Client, msg mqtt.Message) {
	var batch wireBatch
	if err := json.Unmarshal(msg.Payload(), &batch); err != nil {
		c.log.Warn("invalid batch payload", "topic", msg.Topic(), "error", err)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	accepted, duplicates, err := c.store.ingest(ctx, batch.Entries)
	if err != nil {
		// No ack published: the gateway's own retry (Rule 7) will resend,
		// and gateway_id+sequence_id idempotency makes that safe.
		c.log.Error("ingest failed, no ack sent", "batch_id", batch.BatchID, "error", err)
		return
	}

	c.log.Info("ingested batch", "topic", msg.Topic(), "batch_id", batch.BatchID,
		"received", len(batch.Entries), "accepted", accepted, "duplicates", duplicates)

	ackTopic := strings.TrimSuffix(msg.Topic(), "/data") + "/ack"
	payload, _ := json.Marshal(wireAck{BatchID: batch.BatchID})
	cl.Publish(ackTopic, 1, false, payload)
}
