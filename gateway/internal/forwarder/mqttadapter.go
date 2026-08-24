package forwarder

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/google/uuid"

	"nxiiot-gateway/internal/queue"
)

// MQTTAdapterConfig configures MQTTAdapter. Durations and a resolved
// *tls.Config are used here (rather than config.MQTTConfig's raw
// yaml-facing ints/paths) the same way forwarder.Config translates
// config.ForwarderConfig — the translation happens once, in main.go.
type MQTTAdapterConfig struct {
	BrokerURL      string
	ClientID       string
	Username       string
	Password       string
	QoS            byte
	DataTopic      string
	AckTopic       string
	KeepAlive      time.Duration
	ConnectTimeout time.Duration
	PublishTimeout time.Duration
	// AckTimeout bounds how long Send waits for the application-level ack
	// (§15) after the broker has accepted the publish. A batch that times
	// out here is reported as a failed Send and retried like any other
	// failure (Rule 7) — the server may still have processed it, but
	// gateway_id+sequence_id idempotency (Rule 6) makes that safe.
	AckTimeout time.Duration
	TLS        *tls.Config // nil disables TLS
}

// mqttBatch is the payload published to DataTopic. BatchID correlates the
// publish with the application-level ack published back to AckTopic — QoS
// 1's PUBACK only confirms the broker received it, not that the Internal
// Server processed it, so §15's "ACK/application-level acknowledgement" is
// a separate round trip on top of QoS 1.
type mqttBatch struct {
	BatchID string      `json:"batch_id"`
	Entries []WireEntry `json:"entries"`
}

// mqttAck is the payload the Internal Server publishes to AckTopic once it
// has processed (or rejected) a batch.
type mqttAck struct {
	BatchID string `json:"batch_id"`
	Error   string `json:"error,omitempty"`
}

// MQTTAdapter is the production Adapter (§15): publishes batches to
// DataTopic at QoS 1 and waits for an application-level ack on AckTopic.
// Connect/reconnect is delegated to paho's built-in auto-reconnect.
type MQTTAdapter struct {
	client mqtt.Client
	cfg    MQTTAdapterConfig
	log    *slog.Logger

	mu      sync.Mutex
	pending map[string]chan mqttAck
}

func NewMQTTAdapter(cfg MQTTAdapterConfig, log *slog.Logger) *MQTTAdapter {
	a := &MQTTAdapter{cfg: cfg, log: log, pending: make(map[string]chan mqttAck)}

	opts := mqtt.NewClientOptions().
		AddBroker(cfg.BrokerURL).
		SetClientID(cfg.ClientID).
		SetKeepAlive(cfg.KeepAlive).
		SetConnectTimeout(cfg.ConnectTimeout).
		SetAutoReconnect(true).
		SetConnectRetry(true).
		SetConnectRetryInterval(5 * time.Second).
		SetOrderMatters(false).
		SetOnConnectHandler(a.onConnect).
		SetConnectionLostHandler(a.onConnectionLost)

	if cfg.Username != "" {
		opts.SetUsername(cfg.Username)
		opts.SetPassword(cfg.Password)
	}
	if cfg.TLS != nil {
		opts.SetTLSConfig(cfg.TLS)
	}

	a.client = mqtt.NewClient(opts)
	return a
}

// Connect blocks until the initial connection succeeds, ctx is done, or
// ConnectTimeout elapses. After this call returns successfully, paho's
// AutoReconnect (§15 "Reconnect") keeps the session alive on its own —
// Send simply fails while disconnected, and the Forwarder retries.
func (a *MQTTAdapter) Connect(ctx context.Context) error {
	token := a.client.Connect()
	if err := waitToken(ctx, token, a.cfg.ConnectTimeout); err != nil {
		return fmt.Errorf("mqtt connect: %w", err)
	}
	return nil
}

func (a *MQTTAdapter) Disconnect(quiesceMs uint) {
	a.client.Disconnect(quiesceMs)
}

func (a *MQTTAdapter) IsConnected() bool {
	return a.client.IsConnectionOpen()
}

func (a *MQTTAdapter) onConnect(c mqtt.Client) {
	a.log.Info("mqtt connected", "broker", a.cfg.BrokerURL, "client_id", a.cfg.ClientID)
	token := c.Subscribe(a.cfg.AckTopic, a.cfg.QoS, a.handleAck)
	go func() {
		if token.Wait() && token.Error() != nil {
			a.log.Error("mqtt: failed to subscribe to ack topic", "topic", a.cfg.AckTopic, "error", token.Error())
		}
	}()
}

func (a *MQTTAdapter) onConnectionLost(_ mqtt.Client, err error) {
	a.log.Warn("mqtt connection lost, will auto-reconnect", "error", err)
}

func (a *MQTTAdapter) handleAck(_ mqtt.Client, msg mqtt.Message) {
	var ack mqttAck
	if err := json.Unmarshal(msg.Payload(), &ack); err != nil {
		a.log.Warn("mqtt: invalid ack payload", "error", err)
		return
	}

	a.mu.Lock()
	ch, ok := a.pending[ack.BatchID]
	a.mu.Unlock()
	if !ok {
		// Late ack for a batch Send() already gave up waiting on, or a
		// duplicate ack — either way there is nothing left to signal.
		return
	}
	ch <- ack
}

// Send publishes batch to DataTopic at QoS 1 and waits for the
// application-level ack on AckTopic (§15). Duplicate handling is the
// server's responsibility via gateway_id+sequence_id (Rule 6/7) — Send
// does not attempt client-side deduplication, matching HTTPAdapter.
func (a *MQTTAdapter) Send(ctx context.Context, batch []queue.DispatchEntry) error {
	if !a.IsConnected() {
		return fmt.Errorf("mqtt: not connected to %s", a.cfg.BrokerURL)
	}

	payload := mqttBatch{BatchID: uuid.NewString(), Entries: toWireEntries(batch)}
	body, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal batch: %w", err)
	}

	ackCh := make(chan mqttAck, 1)
	a.mu.Lock()
	a.pending[payload.BatchID] = ackCh
	a.mu.Unlock()
	defer func() {
		a.mu.Lock()
		delete(a.pending, payload.BatchID)
		a.mu.Unlock()
	}()

	token := a.client.Publish(a.cfg.DataTopic, a.cfg.QoS, false, body)
	if err := waitToken(ctx, token, a.cfg.PublishTimeout); err != nil {
		return fmt.Errorf("mqtt publish: %w", err)
	}

	select {
	case ack := <-ackCh:
		if ack.Error != "" {
			return fmt.Errorf("server rejected batch %s: %s", payload.BatchID, ack.Error)
		}
		return nil
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(a.cfg.AckTimeout):
		return fmt.Errorf("mqtt: timed out waiting for ack of batch %s", payload.BatchID)
	}
}

// waitToken blocks until token completes, ctx is done, or timeout elapses.
func waitToken(ctx context.Context, token mqtt.Token, timeout time.Duration) error {
	done := make(chan struct{})
	go func() {
		token.Wait()
		close(done)
	}()

	select {
	case <-done:
		return token.Error()
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(timeout):
		return fmt.Errorf("timed out after %s", timeout)
	}
}
