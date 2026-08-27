package acquisition

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"

	"nxiiot-gateway/internal/connection"
	"nxiiot-gateway/internal/datapoint"
	"nxiiot-gateway/internal/device"
	"nxiiot-gateway/internal/modbus"
)

// fakeClient is an in-memory modbus.Client used to verify that runConnection
// serializes access to one shared client across several devices on the same
// connection — the actual fix for the real shared-RTU-port TIMEOUT incident
// (see MEMORY.md and gateway/migrations/0004_connection_split.sql): before
// the connection/device split, two devices sharing a port each opened their
// own client and corrupted each other's traffic on the wire.
type fakeClient struct {
	mu sync.Mutex

	connectErr  error
	connectHits int
	currentUnit byte
	unitIDCalls []byte
	readCalls   []fakeReadCall
	responses   map[uint16][]byte
	closed      bool
}

type fakeReadCall struct {
	unitID   byte
	address  uint16
	quantity uint16
}

func (f *fakeClient) Connect() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.connectHits++
	return f.connectErr
}

func (f *fakeClient) Close() error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.closed = true
	return nil
}

func (f *fakeClient) SetUnitID(id byte) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.currentUnit = id
	f.unitIDCalls = append(f.unitIDCalls, id)
}

func (f *fakeClient) Read(ctx context.Context, fc modbus.FunctionCode, address, quantity uint16) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.readCalls = append(f.readCalls, fakeReadCall{unitID: f.currentUnit, address: address, quantity: quantity})
	if raw, ok := f.responses[address]; ok {
		return raw, nil
	}
	return make([]byte, quantity*2), nil
}

func (f *fakeClient) snapshotUnitIDCalls() []byte {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]byte, len(f.unitIDCalls))
	copy(out, f.unitIDCalls)
	return out
}

func (f *fakeClient) snapshotReadCalls() []fakeReadCall {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]fakeReadCall, len(f.readCalls))
	copy(out, f.readCalls)
	return out
}

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(io.Discard, nil))
}

func uint16DataPoint(id, deviceID int64, tag string, address uint16, intervalMs int) datapoint.DataPoint {
	return datapoint.DataPoint{
		ID: id, DeviceID: deviceID, TagName: tag,
		FunctionCode: uint8(modbus.FuncReadHoldingRegisters), RegisterAddress: address,
		DataType: string(modbus.UInt16), ByteOrder: "AB",
		Scale: 1, Offset: 0, PollingIntervalMs: intervalMs, Enabled: true,
	}
}

// runConnectionWithFake wires a Poller to a fake modbus.Client by swapping
// BuildClient's normal RTU/TCP construction for a fixed fake — runConnection
// itself doesn't know or care which Client implementation it got.
func runPollWithFakeClient(t *testing.T, client *fakeClient, devices []deviceWithPoints, settle time.Duration) []Reading {
	t.Helper()

	var mu sync.Mutex
	var readings []Reading
	p := &Poller{
		log: testLogger(),
		onReading: func(r Reading) {
			mu.Lock()
			defer mu.Unlock()
			readings = append(readings, r)
		},
	}

	conn := connection.Connection{ID: 1, Name: "test-conn", Protocol: connection.RTU, Interface: "/dev/ttyUSB0",
		BaudRate: 9600, DataBits: 8, Parity: "N", StopBits: 1, TimeoutMs: 200, Retry: 0, Enabled: true}

	// runConnection calls the package-level BuildClient, which this test
	// can't intercept without a real serial device — so it drives the
	// same polling logic directly against the fake via a private helper
	// that mirrors runConnection's body but takes a pre-built client.
	ctx, cancel := context.WithCancel(context.Background())
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		p.runConnectionWithClient(ctx, conn, devices, client)
	}()

	time.Sleep(settle)
	cancel()
	wg.Wait()

	mu.Lock()
	defer mu.Unlock()
	out := make([]Reading, len(readings))
	copy(out, readings)
	return out
}

func TestRunConnectionSharesOneClientAcrossDevicesInDeviceOrder(t *testing.T) {
	client := &fakeClient{}

	devA := device.Device{ID: 1, Name: "Temp-Humidity Sensor", ConnectionID: 1, SlaveID: 1, PollingIntervalMs: 5000, Enabled: true}
	devB := device.Device{ID: 2, Name: "PM", ConnectionID: 1, SlaveID: 2, PollingIntervalMs: 5000, Enabled: true}
	devices := []deviceWithPoints{
		{device: devA, dps: []datapoint.DataPoint{uint16DataPoint(1, devA.ID, "tempC", 100, 5000)}},
		{device: devB, dps: []datapoint.DataPoint{uint16DataPoint(2, devB.ID, "pm25", 200, 5000)}},
	}

	readings := runPollWithFakeClient(t, client, devices, 50*time.Millisecond)

	unitCalls := client.snapshotUnitIDCalls()
	if len(unitCalls) != 2 || unitCalls[0] != 1 || unitCalls[1] != 2 {
		t.Fatalf("expected SetUnitID(1) then SetUnitID(2) exactly once each, got %v", unitCalls)
	}

	reads := client.snapshotReadCalls()
	if len(reads) != 2 {
		t.Fatalf("expected exactly 2 reads (one per device), got %+v", reads)
	}
	if reads[0].unitID != 1 || reads[0].address != 100 {
		t.Fatalf("expected device 1's read to use unit id 1 at address 100, got %+v", reads[0])
	}
	if reads[1].unitID != 2 || reads[1].address != 200 {
		t.Fatalf("expected device 2's read to use unit id 2 at address 200, got %+v", reads[1])
	}

	if len(readings) != 2 {
		t.Fatalf("expected 2 readings emitted, got %d: %+v", len(readings), readings)
	}
	for _, r := range readings {
		if r.Quality != modbus.Good || r.Value == nil {
			t.Fatalf("expected a GOOD reading with a value, got %+v", r)
		}
	}
}

func TestRunConnectionGatesEachDeviceByItsOwnPollingInterval(t *testing.T) {
	client := &fakeClient{}

	fast := device.Device{ID: 1, Name: "fast", ConnectionID: 1, SlaveID: 1, PollingIntervalMs: 20, Enabled: true}
	slow := device.Device{ID: 2, Name: "slow", ConnectionID: 1, SlaveID: 2, PollingIntervalMs: 2000, Enabled: true}
	devices := []deviceWithPoints{
		{device: fast, dps: []datapoint.DataPoint{uint16DataPoint(1, fast.ID, "fastTag", 100, 20)}},
		{device: slow, dps: []datapoint.DataPoint{uint16DataPoint(2, slow.ID, "slowTag", 200, 2000)}},
	}

	runPollWithFakeClient(t, client, devices, 120*time.Millisecond)

	reads := client.snapshotReadCalls()
	var fastReads, slowReads int
	for _, rc := range reads {
		switch rc.address {
		case 100:
			fastReads++
		case 200:
			slowReads++
		}
	}

	if slowReads != 1 {
		t.Fatalf("expected the slow (2000ms) device to be read exactly once (the initial poll), got %d", slowReads)
	}
	if fastReads < 2 {
		t.Fatalf("expected the fast (20ms) device to be read more than once within 120ms, got %d", fastReads)
	}
}

func TestRunConnectionConnectFailureMarksEveryDeviceOnItOffline(t *testing.T) {
	client := &fakeClient{connectErr: errors.New("connection refused")}

	devA := device.Device{ID: 1, Name: "Temp-Humidity Sensor", ConnectionID: 1, SlaveID: 1, PollingIntervalMs: 5000, Enabled: true}
	devB := device.Device{ID: 2, Name: "PM", ConnectionID: 1, SlaveID: 2, PollingIntervalMs: 5000, Enabled: true}
	devices := []deviceWithPoints{
		{device: devA, dps: []datapoint.DataPoint{uint16DataPoint(1, devA.ID, "tempC", 100, 5000)}},
		{device: devB, dps: []datapoint.DataPoint{uint16DataPoint(2, devB.ID, "pm25", 200, 5000)}},
	}

	readings := runPollWithFakeClient(t, client, devices, 50*time.Millisecond)

	if len(readings) != 2 {
		t.Fatalf("expected one bad reading per device on connect failure, got %d: %+v", len(readings), readings)
	}
	for _, r := range readings {
		if r.Quality != modbus.DeviceOffline || r.Value != nil {
			t.Fatalf("expected a DEVICE_OFFLINE reading with no value, got %+v", r)
		}
	}
	if len(client.snapshotReadCalls()) != 0 {
		t.Fatalf("expected no reads to be attempted after a connect failure")
	}
}
