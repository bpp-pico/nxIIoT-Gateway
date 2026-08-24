package acquisition

import (
	"context"
	"log/slog"
	"reflect"
	"sync"

	"nxiiot-gateway/internal/datapoint"
	"nxiiot-gateway/internal/device"
)

// Manager supervises one Poller goroutine per enabled device and keeps them
// in sync with the device/datapoint repositories. Call Reload after any
// device or data point is created, updated, deleted, or has its enabled
// flag flipped, so the Web UI takes effect without a gateway restart.
type Manager struct {
	log           *slog.Logger
	deviceRepo    *device.Repository
	datapointRepo *datapoint.Repository
	poller        *Poller
	parentCtx     context.Context

	mu      sync.Mutex
	running map[int64]*runningDevice
	wg      sync.WaitGroup
}

type runningDevice struct {
	cancel context.CancelFunc
	cfg    device.Device
	dps    []datapoint.DataPoint
}

func NewManager(ctx context.Context, log *slog.Logger, deviceRepo *device.Repository, datapointRepo *datapoint.Repository, onReading OnReading) *Manager {
	return &Manager{
		log:           log,
		deviceRepo:    deviceRepo,
		datapointRepo: datapointRepo,
		poller:        NewPoller(log, onReading),
		parentCtx:     ctx,
		running:       make(map[int64]*runningDevice),
	}
}

// Reload loads all enabled devices/data points and starts, stops, or
// restarts device pollers so the running set matches configuration. Devices
// whose configuration is unchanged since the last Reload are left running
// (their connection is not disturbed).
func (m *Manager) Reload(ctx context.Context) error {
	devices, err := m.deviceRepo.ListEnabled(ctx)
	if err != nil {
		return err
	}

	type desired struct {
		cfg device.Device
		dps []datapoint.DataPoint
	}
	wanted := make(map[int64]desired, len(devices))
	for _, d := range devices {
		dps, err := m.datapointRepo.ListEnabledByDevice(ctx, d.ID)
		if err != nil {
			return err
		}
		wanted[d.ID] = desired{cfg: d, dps: dps}
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	for id, rd := range m.running {
		w, ok := wanted[id]
		if !ok || !reflect.DeepEqual(rd.cfg, w.cfg) || !reflect.DeepEqual(rd.dps, w.dps) {
			rd.cancel()
			delete(m.running, id)
		}
	}

	for id, w := range wanted {
		if _, ok := m.running[id]; ok {
			continue
		}
		devCtx, cancel := context.WithCancel(m.parentCtx)
		m.running[id] = &runningDevice{cancel: cancel, cfg: w.cfg, dps: w.dps}

		m.wg.Add(1)
		go func(d device.Device, dps []datapoint.DataPoint) {
			defer m.wg.Done()
			m.poller.runDevice(devCtx, d, dps)
		}(w.cfg, w.dps)
	}

	m.log.Info("acquisition reloaded", "active_devices", len(m.running))
	return nil
}

// Wait blocks until every device poller has stopped. Devices stop when
// their context is cancelled — either individually during Reload, or all
// at once when the parent context passed to NewManager is cancelled.
func (m *Manager) Wait() {
	m.wg.Wait()
}
