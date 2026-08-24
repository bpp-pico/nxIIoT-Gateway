// Package acquisition schedules polling of configured Modbus devices and
// emits Readings. It must never depend on Server/queue availability (Rule 1)
// — it only produces Readings via a callback; what happens to them
// downstream (Phase 3/4) is not this package's concern.
package acquisition

import (
	"context"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"nxiiot-gateway/internal/datapoint"
	"nxiiot-gateway/internal/device"
	"nxiiot-gateway/internal/modbus"
)

// OnReading is called for every acquired (or failed) Data Point read. It
// must return quickly; slow consumers should buffer internally.
type OnReading func(Reading)

type Poller struct {
	log       *slog.Logger
	onReading OnReading
}

func NewPoller(log *slog.Logger, onReading OnReading) *Poller {
	return &Poller{log: log, onReading: onReading}
}

// Run polls every device concurrently until ctx is cancelled, blocking
// until all device pollers have stopped.
func (p *Poller) Run(ctx context.Context, devices []device.Device, dataPoints map[int64][]datapoint.DataPoint) {
	var wg sync.WaitGroup
	for _, d := range devices {
		dps := dataPoints[d.ID]
		if len(dps) == 0 {
			continue
		}
		wg.Add(1)
		go func(d device.Device, dps []datapoint.DataPoint) {
			defer wg.Done()
			p.runDevice(ctx, d, dps)
		}(d, dps)
	}
	wg.Wait()
}

func (p *Poller) runDevice(ctx context.Context, d device.Device, dps []datapoint.DataPoint) {
	client, err := BuildClient(d)
	if err != nil {
		p.log.Error("invalid device configuration", "device", d.Name, "error", err)
		return
	}

	interval := time.Duration(d.PollingIntervalMs) * time.Millisecond
	if interval <= 0 {
		interval = time.Second
	}
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	connected := false
	lastPolled := make(map[int64]time.Time, len(dps))

	poll := func() {
		now := time.Now().UTC()

		if !connected {
			if err := client.Connect(); err != nil {
				p.emitDeviceOffline(d, dps, now, err)
				return
			}
			connected = true
		}

		for _, dp := range dps {
			dpInterval := time.Duration(dp.PollingIntervalMs) * time.Millisecond
			if last, ok := lastPolled[dp.ID]; ok && now.Sub(last) < dpInterval {
				continue
			}
			lastPolled[dp.ID] = now
			p.readOne(ctx, client, d, dp, now)
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

	readCtx, cancel := context.WithTimeout(ctx, time.Duration(d.TimeoutMs)*time.Millisecond)
	raw, err := modbus.ReadWithRetry(readCtx, client, modbus.FunctionCode(dp.FunctionCode), dp.RegisterAddress, qty, d.Retry)
	cancel()

	if err != nil {
		q := modbus.QualityFromError(err)
		p.log.Warn("modbus read failed", "device", d.Name, "tag", dp.TagName, "quality", q, "error", err)
		p.onReading(p.badReading(d, dp, eventTime, q))
		return
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

// BuildClient constructs a Modbus client for a device's configured
// protocol. Exported so the API layer can build short-lived clients for
// "Test Connection" / "Test Read" without touching the Manager's
// long-lived polling connections.
func BuildClient(d device.Device) (modbus.Client, error) {
	timeout := time.Duration(d.TimeoutMs) * time.Millisecond

	switch d.Protocol {
	case device.TCP:
		if d.IPAddress == "" {
			return nil, fmt.Errorf("TCP device %q has no ip_address configured", d.Name)
		}
		addr := fmt.Sprintf("%s:%d", d.IPAddress, d.Port)
		return modbus.NewTCPClient(modbus.TCPConfig{
			Address: addr,
			UnitID:  byte(d.SlaveID),
			Timeout: timeout,
		}), nil

	case device.RTU:
		if d.Interface == "" {
			return nil, fmt.Errorf("RTU device %q has no interface configured", d.Name)
		}
		return modbus.NewRTUClient(modbus.RTUConfig{
			Interface: d.Interface,
			BaudRate:  d.BaudRate,
			DataBits:  d.DataBits,
			Parity:    d.Parity,
			StopBits:  d.StopBits,
			SlaveID:   byte(d.SlaveID),
			Timeout:   timeout,
		}), nil

	default:
		return nil, fmt.Errorf("unknown protocol %q", d.Protocol)
	}
}
