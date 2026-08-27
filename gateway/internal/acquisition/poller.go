// Package acquisition schedules polling of configured Modbus devices and
// emits Readings. It must never depend on Server/queue availability (Rule 1)
// — it only produces Readings via a callback; what happens to them
// downstream (Phase 3/4) is not this package's concern.
package acquisition

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"nxiiot-gateway/internal/connection"
	"nxiiot-gateway/internal/datapoint"
	"nxiiot-gateway/internal/device"
	"nxiiot-gateway/internal/diagnostics"
	"nxiiot-gateway/internal/modbus"
)

// OnReading is called for every acquired (or failed) Data Point read. It
// must return quickly; slow consumers should buffer internally.
type OnReading func(Reading)

// OnPollCycle is called once per device after its data points have been
// polled for the current cycle (i.e. only on ticks where the device's own
// polling_interval_ms gate lets it through), reporting how long that took —
// for the Web UI's per-device polling-timing display, so intervals can be
// tuned against real hardware response times instead of guessed.
type OnPollCycle func(deviceID int64, durationMs int64, datapointsRead int, at time.Time)

// deviceWithPoints pairs a device with its enabled data points, for polling
// within one connection's shared goroutine.
type deviceWithPoints struct {
	device device.Device
	dps    []datapoint.DataPoint
}

type Poller struct {
	log         *slog.Logger
	onReading   OnReading
	onPollCycle OnPollCycle
	diag        *diagnostics.Store
}

// diag may be nil (e.g. in tests that don't care about diagnostics
// counters) — every diag.RecordResult call below is guarded accordingly.
// onPollCycle may also be nil (e.g. in tests) — every call below is guarded.
func NewPoller(log *slog.Logger, onReading OnReading, onPollCycle OnPollCycle, diag *diagnostics.Store) *Poller {
	return &Poller{log: log, onReading: onReading, onPollCycle: onPollCycle, diag: diag}
}

// runConnection owns exactly one Client for the lifetime of conn's polling
// — one goroutine per physical connection (not per device) is what makes
// sharing that Client across several devices at different slave IDs safe:
// nothing else ever touches it concurrently. This is the fix for a real
// live incident (see MEMORY.md, gateway/migrations/0004_connection_split.sql):
// before the connection/device split, two devices sharing one RTU interface
// each opened their own independent connection, and their Modbus frames
// corrupted each other on the wire.
func (p *Poller) runConnection(ctx context.Context, conn connection.Connection, devices []deviceWithPoints) {
	client, err := BuildClient(conn)
	if err != nil {
		p.log.Error("invalid connection configuration", "connection", conn.Name, "error", err)
		return
	}
	p.runConnectionWithClient(ctx, conn, devices, client)
}

// runConnectionWithClient is runConnection's body, taking an already-built
// Client so tests can substitute a fake instead of a real serial/TCP handle.
func (p *Poller) runConnectionWithClient(ctx context.Context, conn connection.Connection, devices []deviceWithPoints, client modbus.Client) {
	// Tick at least as often as the fastest device on this connection
	// needs — the per-device gate below still limits each device to its
	// own configured interval, so a device sharing a connection with a
	// faster sibling doesn't get polled any faster than it asked for, it
	// just gets *checked* more often (cheap; matches the pre-existing
	// per-datapoint gating pattern this generalizes, see MEMORY.md's note
	// on polling_interval_ms existing at two levels).
	interval := time.Duration(0)
	for _, dg := range devices {
		di := time.Duration(dg.device.PollingIntervalMs) * time.Millisecond
		if di <= 0 {
			di = time.Second
		}
		if interval == 0 || di < interval {
			interval = di
		}
	}
	if interval <= 0 {
		interval = time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	connected := false
	lastPolledDevice := make(map[int64]time.Time, len(devices))
	lastPolledPoint := make(map[int64]time.Time)

	poll := func() {
		now := time.Now().UTC()

		if !connected {
			if err := client.Connect(); err != nil {
				for _, dg := range devices {
					p.emitDeviceOffline(dg.device, dg.dps, now, err)
				}
				return
			}
			connected = true
		}

		for _, dg := range devices {
			di := time.Duration(dg.device.PollingIntervalMs) * time.Millisecond
			if di <= 0 {
				di = time.Second
			}
			if last, ok := lastPolledDevice[dg.device.ID]; ok && now.Sub(last) < di {
				continue
			}
			lastPolledDevice[dg.device.ID] = now
			client.SetUnitID(byte(dg.device.SlaveID))

			deviceStart := time.Now()
			readCount := 0
			for _, dp := range dg.dps {
				dpInterval := time.Duration(dp.PollingIntervalMs) * time.Millisecond
				if last, ok := lastPolledPoint[dp.ID]; ok && now.Sub(last) < dpInterval {
					continue
				}
				lastPolledPoint[dp.ID] = now
				p.readOne(ctx, client, dg.device, dp, now)
				readCount++
			}
			if readCount > 0 && p.onPollCycle != nil {
				p.onPollCycle(dg.device.ID, time.Since(deviceStart).Milliseconds(), readCount, now)
			}
		}
	}

	// Read immediately on startup instead of waiting a full interval.
	poll()

	for {
		select {
		case <-ctx.Done():
			_ = client.Close()
			return
		case <-ticker.C:
			poll()
		}
	}
}

func (p *Poller) readOne(ctx context.Context, client modbus.Client, d device.Device, dp datapoint.DataPoint, eventTime time.Time) {
	dt := modbus.DataType(dp.DataType)
	qty, err := dt.RegisterCount()
	if err != nil {
		p.log.Error("invalid data point configuration", "device", d.Name, "tag", dp.TagName, "error", err)
		p.onReading(p.badReading(d, dp, eventTime, modbus.Invalid))
		return
	}

	readCtx, cancel := context.WithTimeout(ctx, time.Duration(d.PollingIntervalMs)*time.Millisecond)
	start := time.Now()
	raw, attempts, err := modbus.ReadWithRetry(readCtx, client, modbus.FunctionCode(dp.FunctionCode), dp.RegisterAddress, qty, 0)
	elapsed := time.Since(start)
	cancel()

	if err != nil {
		q := modbus.QualityFromError(err)
		p.log.Warn("modbus read failed", "device", d.Name, "tag", dp.TagName, "quality", q, "error", err)
		if p.diag != nil {
			p.diag.RecordResult(q, elapsed, attempts)
		}
		p.onReading(p.badReading(d, dp, eventTime, q))
		return
	}
	if p.diag != nil {
		p.diag.RecordResult(modbus.Good, elapsed, attempts)
	}

	decoded, err := modbus.Decode(raw, dt, dp.ByteOrder)
	if err != nil {
		p.log.Error("decode failed", "device", d.Name, "tag", dp.TagName, "error", err)
		p.onReading(p.badReading(d, dp, eventTime, modbus.Invalid))
		return
	}

	value := modbus.ApplyScale(decoded, dp.Scale, dp.Offset)
	p.onReading(Reading{
		DeviceID:       d.ID,
		DeviceName:     d.Name,
		DatapointID:    dp.ID,
		Tag:            dp.TagName,
		Value:          &value,
		Quality:        modbus.Good,
		Unit:           dp.Unit,
		Priority:       string(dp.Priority),
		EventTimestamp: eventTime,
	})
}

func (p *Poller) emitDeviceOffline(d device.Device, dps []datapoint.DataPoint, now time.Time, err error) {
	q := modbus.QualityFromError(err)
	p.log.Warn("device connection failed", "device", d.Name, "quality", q, "error", err)
	for _, dp := range dps {
		p.onReading(p.badReading(d, dp, now, q))
	}
}

func (p *Poller) badReading(d device.Device, dp datapoint.DataPoint, eventTime time.Time, q modbus.Quality) Reading {
	return Reading{
		DeviceID:       d.ID,
		DeviceName:     d.Name,
		DatapointID:    dp.ID,
		Tag:            dp.TagName,
		Value:          nil,
		Quality:        q,
		Unit:           dp.Unit,
		Priority:       string(dp.Priority),
		EventTimestamp: eventTime,
	}
}

// BuildClient constructs a Modbus client for a connection's configured
// protocol. Exported so the API layer can build short-lived clients for
// "Test Connection" / "Test Read" without touching the Manager's
// long-lived polling connections. It does not set a slave/unit ID —
// callers call Client.SetUnitID for whichever device they're about to
// read, since one Connection can serve several devices at different IDs.
func BuildClient(c connection.Connection) (modbus.Client, error) {
	timeout := time.Duration(c.TimeoutMs) * time.Millisecond

	switch c.Protocol {
	case connection.TCP:
		if c.IPAddress == "" {
			return nil, fmt.Errorf("TCP connection %q has no ip_address configured", c.Name)
		}
		addr := fmt.Sprintf("%s:%d", c.IPAddress, c.Port)
		return modbus.NewTCPClient(modbus.TCPConfig{
			Address: addr,
			Timeout: timeout,
		}), nil

	case connection.RTU:
		if c.Interface == "" {
			return nil, fmt.Errorf("RTU connection %q has no interface configured", c.Name)
		}
		return modbus.NewRTUClient(modbus.RTUConfig{
			Interface: c.Interface,
			BaudRate:  c.BaudRate,
			DataBits:  c.DataBits,
			Parity:    c.Parity,
			StopBits:  c.StopBits,
			Timeout:   timeout,
		}), nil

	default:
		return nil, fmt.Errorf("unknown protocol %q", c.Protocol)
	}
}
