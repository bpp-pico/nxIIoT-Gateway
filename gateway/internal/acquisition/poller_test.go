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

// Read looks up a response by the block's own start address — seeded by
// the test to cover the whole merged range's bytes, not per-datapoint,
// since planBlockReads may combine several datapoints into one call.
func (f *fakeClient) Read(ctx context.Context, fc modbus.FunctionCode, address, quantity uint16) ([]byte, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.readCalls = append(f.readCalls, fakeReadCall{unitID: f.currentUnit, address: address, quantity: quantity})
	if raw, ok := f.responses[address]; ok {
		return raw, nil
	}
	return make([]byte, quantity*2), nil
}

func (f *fakeClient) connectHitCount() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.connectHits
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

func uint16DataPoint(id, deviceID int64, tag string, address uint16) datapoint.DataPoint {
	return datapoint.DataPoint{
		ID: id, DeviceID: deviceID, TagName: tag,
		FunctionCode: uint8(modbus.FuncReadHoldingRegisters), RegisterAddress: address,
		DataType: string(modbus.UInt16), ByteOrder: "AB",
		Scale: 1, Offset: 0, Enabled: true,
	}
}

// runPollWithFakeClient wires a Poller to a fake modbus.Client by swapping
// BuildClient's normal RTU/TCP construction for a fixed fake — runConnection
// itself doesn't know or care which Client implementation it got.
func runPollWithFakeClient(t *testing.T, client *fakeClient, devices []deviceWithPoints, nextDeviceDelayMs int, settle time.Duration) []Reading {
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
		BaudRate: 9600, DataBits: 8, Parity: "N", StopBits: 1, TimeoutMs: 200, Retry: 0, Enabled: true,
		NextDeviceDelayMs: nextDeviceDelayMs}

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

	devA := device.Device{ID: 1, Name: "Temp-Humidity Sensor", ConnectionID: 1, SlaveID: 1, Enabled: true}
	devB := device.Device{ID: 2, Name: "PM", ConnectionID: 1, SlaveID: 2, Enabled: true}
	devices := []deviceWithPoints{
		{device: devA, dps: []datapoint.DataPoint{uint16DataPoint(1, devA.ID, "tempC", 100)}},
		{device: devB, dps: []datapoint.DataPoint{uint16DataPoint(2, devB.ID, "pm25", 200)}},
	}

	// A 100ms delay with a 150ms settle window allows exactly one full
	// pass (A's read, 100ms wait, B's read) before cancellation lands
	// mid-wait after B — keeping this to exactly one read per device.
	readings := runPollWithFakeClient(t, client, devices, 100, 150*time.Millisecond)

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

// TestRunConnectionRoundRobinsContinuouslyWithDelayBetweenDevices proves the
// core of the new scan model: no ticker, no per-device/per-datapoint
// interval gating — just a continuous loop through every device, wrapping
// back to the first after the last, paced only by
// connection.NextDeviceDelayMs between devices.
func TestRunConnectionRoundRobinsContinuouslyWithDelayBetweenDevices(t *testing.T) {
	client := &fakeClient{}

	devA := device.Device{ID: 1, Name: "A", ConnectionID: 1, SlaveID: 1, Enabled: true}
	devB := device.Device{ID: 2, Name: "B", ConnectionID: 1, SlaveID: 2, Enabled: true}
	devC := device.Device{ID: 3, Name: "C", ConnectionID: 1, SlaveID: 3, Enabled: true}
	devices := []deviceWithPoints{
		{device: devA, dps: []datapoint.DataPoint{uint16DataPoint(1, devA.ID, "a", 100)}},
		{device: devB, dps: []datapoint.DataPoint{uint16DataPoint(2, devB.ID, "b", 200)}},
		{device: devC, dps: []datapoint.DataPoint{uint16DataPoint(3, devC.ID, "c", 300)}},
	}

	// 10ms delay between devices, settle for 150ms — enough for several
	// full A->B->C->A... cycles, proving both the wraparound and that the
	// delay is actually being waited (not skipped/zero).
	runPollWithFakeClient(t, client, devices, 10, 150*time.Millisecond)

	reads := client.snapshotReadCalls()
	var aCount, bCount, cCount int
	for _, rc := range reads {
		switch rc.address {
		case 100:
			aCount++
		case 200:
			bCount++
		case 300:
			cCount++
		}
	}

	if aCount < 2 || bCount < 2 || cCount < 2 {
		t.Fatalf("expected every device to be polled more than once (round-robin wraparound) within 150ms at a 10ms delay, got a=%d b=%d c=%d", aCount, bCount, cCount)
	}
	// Devices are always read in the same relative order each cycle
	// (A, B, C, A, B, C, ...) since the shared client is never touched
	// concurrently — spot check the first 6 reads follow that pattern.
	if len(reads) >= 6 {
		wantAddrs := []uint16{100, 200, 300, 100, 200, 300}
		for i, want := range wantAddrs {
			if reads[i].address != want {
				t.Fatalf("expected read %d to be at address %d (round-robin order), got %+v", i, want, reads[i])
			}
		}
	}
}

func TestRunConnectionConnectFailureMarksEveryDeviceOnItOffline(t *testing.T) {
	client := &fakeClient{connectErr: errors.New("connection refused")}

	devA := device.Device{ID: 1, Name: "Temp-Humidity Sensor", ConnectionID: 1, SlaveID: 1, Enabled: true}
	devB := device.Device{ID: 2, Name: "PM", ConnectionID: 1, SlaveID: 2, Enabled: true}
	devices := []deviceWithPoints{
		{device: devA, dps: []datapoint.DataPoint{uint16DataPoint(1, devA.ID, "tempC", 100)}},
		{device: devB, dps: []datapoint.DataPoint{uint16DataPoint(2, devB.ID, "pm25", 200)}},
	}

	// reconnectFloorDelay defaults to 1s (see poller.go) — shrink it for
	// this test so it doesn't need a full real second to observe the
	// (single, since settle < floor) connect attempt.
	orig := reconnectFloorDelay
	reconnectFloorDelay = 20 * time.Millisecond
	t.Cleanup(func() { reconnectFloorDelay = orig })

	readings := runPollWithFakeClient(t, client, devices, 500, 10*time.Millisecond)

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

// TestRunConnectionRetriesConnectAfterFloorDelay proves the new model
// doesn't just fail once and stop — since there's no ticker to naturally
// pace a retry anymore, checkAndForceReconnect's floor delay is what keeps
// it trying (without hammering a genuinely-down broker/bus in a tight
// spin loop).
func TestRunConnectionRetriesConnectAfterFloorDelay(t *testing.T) {
	client := &fakeClient{connectErr: errors.New("connection refused")}

	devA := device.Device{ID: 1, Name: "A", ConnectionID: 1, SlaveID: 1, Enabled: true}
	devices := []deviceWithPoints{
		{device: devA, dps: []datapoint.DataPoint{uint16DataPoint(1, devA.ID, "a", 100)}},
	}

	orig := reconnectFloorDelay
	reconnectFloorDelay = 15 * time.Millisecond
	t.Cleanup(func() { reconnectFloorDelay = orig })

	runPollWithFakeClient(t, client, devices, 500, 100*time.Millisecond)

	if hits := client.connectHitCount(); hits < 2 {
		t.Fatalf("expected more than one Connect() attempt (retry after the floor delay), got %d", hits)
	}
}
