package forwarder_test

import (
	"context"
	"encoding/json"
	"net"
	"testing"
	"time"

	pahomqtt "github.com/eclipse/paho.mqtt.golang"
	mochi "github.com/mochi-mqtt/server/v2"
	"github.com/mochi-mqtt/server/v2/hooks/auth"
	"github.com/mochi-mqtt/server/v2/listeners"

	"nxiiot-gateway/internal/forwarder"
	"nxiiot-gateway/internal/queue"
)

// startTestBroker runs a real, embedded, pure-Go MQTT broker for the
// duration of the test — matching this package's existing preference
// (openTestRepo, forwarder_test.go) for exercising real infrastructure
// over hand-rolled fakes. It returns the "tcp://host:port" URL to connect
// to.
func startTestBroker(t *testing.T) string {
	t.Helper()

	// Reserve a free port, then hand it to mochi — mochi's own listener
	// binds it a moment later, which is an accepted small race for tests.
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("reserve free port: %v", err)
	}
	addr := l.Addr().String()
	l.Close()

	server := mochi.New(nil)
	if err := server.AddHook(new(auth.AllowHook), nil); err != nil {
		t.Fatalf("add allow-all hook: %v", err)
	}
	tcp := listeners.NewTCP(listeners.Config{ID: "test", Address: addr})
	if err := server.AddListener(tcp); err != nil {
		t.Fatalf("add tcp listener: %v", err)
	}

	go func() {
		_ = server.Serve()
	}()
	t.Cleanup(func() { _ = server.Close() })

	return "tcp://" + addr
}

func newTestMQTTAdapter(t *testing.T, brokerURL string, ackTimeout time.Duration) *forwarder.MQTTAdapter {
	t.Helper()
	adapter := forwarder.NewMQTTAdapter(forwarder.MQTTAdapterConfig{
		BrokerURL:      brokerURL,
		ClientID:       "GW001-adapter",
		QoS:            1,
		DataTopic:      "gateway/GW001/data",
		AckTopic:       "gateway/GW001/ack",
		KeepAlive:      10 * time.Second,
		ConnectTimeout: 2 * time.Second,
		PublishTimeout: 2 * time.Second,
		AckTimeout:     ackTimeout,
	}, newTestLogger(t))

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := adapter.Connect(ctx); err != nil {
		t.Fatalf("adapter.Connect: %v", err)
	}
	t.Cleanup(func() { adapter.Disconnect(100) })
	return adapter
}

// startFakeInternalServer stands in for the real Internal Server's MQTT
// consumer (cmd/server-sim's role, but over MQTT): it subscribes to
// dataTopic and, for every batch received, publishes an ack to ackTopic —
// exercising the adapter's application-level ack round trip for real,
// not by asserting on internal adapter state.
type receivedBatch struct {
	BatchID string
	Entries []forwarder.WireEntry
}

func startFakeInternalServer(t *testing.T, brokerURL, dataTopic, ackTopic string) <-chan receivedBatch {
	t.Helper()
	received := make(chan receivedBatch, 10)

	opts := pahomqtt.NewClientOptions().AddBroker(brokerURL).SetClientID("fake-internal-server")
	opts.SetDefaultPublishHandler(func(client pahomqtt.Client, msg pahomqtt.Message) {
		var batch struct {
			BatchID string                `json:"batch_id"`
			Entries []forwarder.WireEntry `json:"entries"`
		}
		if err := json.Unmarshal(msg.Payload(), &batch); err != nil {
			t.Errorf("fake server: invalid batch payload: %v", err)
			return
		}
		received <- receivedBatch{BatchID: batch.BatchID, Entries: batch.Entries}

		ack, _ := json.Marshal(map[string]string{"batch_id": batch.BatchID})
		client.Publish(ackTopic, 1, false, ack)
	})

	client := pahomqtt.NewClient(opts)
	if token := client.Connect(); token.Wait() && token.Error() != nil {
		t.Fatalf("fake server connect: %v", token.Error())
	}
	if token := client.Subscribe(dataTopic, 1, nil); token.Wait() && token.Error() != nil {
		t.Fatalf("fake server subscribe: %v", token.Error())
	}
	t.Cleanup(func() { client.Disconnect(100) })

	return received
}

func sampleBatch() []queue.DispatchEntry {
	v := 230.2
	return []queue.DispatchEntry{
		{Entry: queue.Entry{
			ID: 1, GatewayID: "GW001", SequenceID: 1, DeviceID: 1, DatapointID: 1,
			Value: &v, Quality: "GOOD", EventTimestamp: time.Now(), Priority: "NORMAL",
		}},
	}
}

func TestMQTTAdapterSendSucceedsWhenServerAcks(t *testing.T) {
	brokerURL := startTestBroker(t)
	adapter := newTestMQTTAdapter(t, brokerURL, 3*time.Second)
	received := startFakeInternalServer(t, brokerURL, "gateway/GW001/data", "gateway/GW001/ack")

	// Give the fake server's subscription a moment to register with the
	// broker before the adapter publishes.
	time.Sleep(200 * time.Millisecond)

	if !adapter.IsConnected() {
		t.Fatal("expected adapter to report connected after Connect")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := adapter.Send(ctx, sampleBatch()); err != nil {
		t.Fatalf("Send: %v", err)
	}

	select {
	case got := <-received:
		if len(got.Entries) != 1 || got.Entries[0].SequenceID != 1 || got.Entries[0].GatewayID != "GW001" {
			t.Errorf("fake server received unexpected batch: %+v", got.Entries)
		}
	case <-time.After(time.Second):
		t.Fatal("fake server never received the published batch")
	}
}

func TestMQTTAdapterSendFailsWhenAckNeverArrives(t *testing.T) {
	brokerURL := startTestBroker(t)
	// No fake server subscribed — nothing will ever ack.
	adapter := newTestMQTTAdapter(t, brokerURL, 300*time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err := adapter.Send(ctx, sampleBatch())
	if err == nil {
		t.Fatal("expected Send to fail when no application-level ack arrives")
	}
}

func TestMQTTAdapterSendFailsFastWhenNotConnected(t *testing.T) {
	brokerURL := startTestBroker(t)
	adapter := newTestMQTTAdapter(t, brokerURL, 5*time.Second)
	adapter.Disconnect(100)

	start := time.Now()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err := adapter.Send(ctx, sampleBatch())
	elapsed := time.Since(start)

	if err == nil {
		t.Fatal("expected Send to fail once disconnected")
	}
	if elapsed > time.Second {
		t.Errorf("expected Send to fail fast on disconnect, took %s", elapsed)
	}
}
