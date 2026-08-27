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

	// WatchdogInterval and ReconnectStuckAfter tune RunReconnectWatchdog
	// (see its doc comment) — zero on either defaults to the production
	// values (15s / 45s) in NewMQTTAdapter. Exposed so tests can shrink
	// them to make the stuck-reconnect path observable in well under a
	// second instead of the real 45s.
	WatchdogInterval    time.Duration
	ReconnectStuckAfter time.Duration
}

const (
	defaultWatchdogInterval    = 15 * time.Second
	defaultReconnectStuckAfter = 45 * time.Second
)

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

	mu             sync.Mutex
	pending        map[string]chan mqttAck
	disconnectedAt time.Time // zero value means "currently connected"
}

func NewMQTTAdapter(cfg MQTTAdapterConfig, log *slog.Logger) *MQTTAdapter {
	if cfg.WatchdogInterval <= 0 {
		cfg.WatchdogInterval = defaultWatchdogInterval
	}
	if cfg.ReconnectStuckAfter <= 0 {
		cfg.ReconnectStuckAfter = defaultReconnectStuckAfter
	}
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
	a.mu.Lock()
	a.disconnectedAt = time.Time{}
	a.mu.Unlock()
	go a.subscribeWithRetry(c)
}

// subscribeWithRetry mirrors internal-server/consumer.go's subscribeWithRetry
// — a subscribe that loses the race with a fresh disconnect is retried up to
// 3 times with a short backoff before giving up and waiting for the next
// reconnect, rather than only logging a single failure (the gap this closes:
// a real log had exactly one subscribe failure with no follow-up attempt,
// leaving the ack topic unsubscribed until the next full reconnect).
func (a *MQTTAdapter) subscribeWithRetry(c mqtt.Client) {
	for attempt := 1; attempt <= 3; attempt++ {
		token := c.Subscribe(a.cfg.AckTopic, a.cfg.QoS, a.handleAck)
		token.Wait()
		if token.Error() == nil {
			return
		}
		a.log.Error("mqtt: failed to subscribe to ack topic, retrying", "topic", a.cfg.AckTopic, "attempt", attempt, "error", token.Error())
		time.Sleep(time.Duration(attempt) * time.Second)
	}
	a.log.Error("mqtt: failed to subscribe to ack topic after 3 attempts; waiting for next reconnect", "topic", a.cfg.AckTopic)
}

func (a *MQTTAdapter) onConnectionLost(_ mqtt.Client, err error) {
	a.log.Warn("mqtt connection lost, will auto-reconnect", "error", err)
	a.mu.Lock()
	a.disconnectedAt = time.Now()
	a.mu.Unlock()
}

// RunReconnectWatchdog is the automated version of the fix for a real
// 2026-08-27 incident: paho's own AutoReconnect (SetAutoReconnect(true) +
// SetConnectRetry(true), 5s retry interval — see NewMQTTAdapter) is
// supposed to recover the connection on its own after onConnectionLost,
// but was observed to silently stall after a "pingresp not received"
// disconnect — the broker stayed reachable throughout (confirmed
// independently with a direct TCP check), yet the client sat disconnected
// for 7+ minutes with zero further log activity until the gateway process
// was restarted by hand. That restart is the only thing that actually
// unstuck it, which is exactly what this watchdog now does automatically,
// scoped to just the MQTT client rather than the whole process.
//
// It intentionally waits ReconnectStuckAfter (default 45s — several
// multiples of paho's own 5s retry interval) before concluding the
// built-in retry has stalled rather than merely being slow, so a broker
// that's genuinely still unreachable isn't hammered by two overlapping
// reconnect loops. Runs until ctx is cancelled (the gateway's top-level
// shutdown context — see cmd/gateway/adapter.go).
func (a *MQTTAdapter) RunReconnectWatchdog(ctx context.Context) {
	ticker := time.NewTicker(a.cfg.WatchdogInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			a.checkAndForceReconnect()
		}
	}
}

func (a *MQTTAdapter) checkAndForceReconnect() {
	if a.client.IsConnectionOpen() {
		return
	}

	a.mu.Lock()
	stuckSince := a.disconnectedAt
	a.mu.Unlock()
	if stuckSince.IsZero() || time.Since(stuckSince) < a.cfg.ReconnectStuckAfter {
		return
	}

	a.log.Warn("mqtt client disconnected too long, forcing reconnect", "disconnected_for", time.Since(stuckSince))
	a.client.Disconnect(250)
	token := a.client.Connect()
	token.WaitTimeout(a.cfg.ConnectTimeout)
	if err := token.Error(); err != nil {
		a.log.Error("mqtt: forced reconnect attempt failed, will retry at next watchdog tick", "error", err)
	}

	if !a.client.IsConnectionOpen() {
		// Still down — push the deadline out so the next forced attempt
		// waits a full ReconnectStuckAfter rather than retrying every
		// tick against a broker that's genuinely still unreachable. If it
		// DID come back up, onConnect already zeroed disconnectedAt and
		// this is skipped, so a fast recovery isn't mislabeled as stuck.
		a.mu.Lock()
		a.disconnectedAt = time.Now()
		a.mu.Unlock()
	}
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
