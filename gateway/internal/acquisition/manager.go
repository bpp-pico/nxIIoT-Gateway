package acquisition

import (
	"context"
	"log/slog"
	"reflect"
	"sync"

	"nxiiot-gateway/internal/connection"
	"nxiiot-gateway/internal/datapoint"
	"nxiiot-gateway/internal/device"
	"nxiiot-gateway/internal/diagnostics"
)

// Manager supervises one Poller goroutine per enabled Connection (not per
// device — see poller.go's runConnection doc comment for why) and keeps
// them in sync with the connection/device/datapoint repositories. Call
// Reload after any connection, device, or data point is created, updated,
// deleted, or has its enabled flag flipped, so the Web UI takes effect
// without a gateway restart.
type Manager struct {
	log           *slog.Logger
	connRepo      *connection.Repository
	deviceRepo    *device.Repository
	datapointRepo *datapoint.Repository
	poller        *Poller
	parentCtx     context.Context

	mu      sync.Mutex
	running map[int64]*runningConnection
	wg      sync.WaitGroup
}

type runningConnection struct {
	cancel  context.CancelFunc
	conn    connection.Connection
	devices []deviceWithPoints
}

func NewManager(ctx context.Context, log *slog.Logger, connRepo *connection.Repository, deviceRepo *device.Repository, datapointRepo *datapoint.Repository, onReading OnReading, diag *diagnostics.Store) *Manager {
	return &Manager{
		log:           log,
		connRepo:      connRepo,
		deviceRepo:    deviceRepo,
		datapointRepo: datapointRepo,
		poller:        NewPoller(log, onReading, diag),
		parentCtx:     ctx,
		running:       make(map[int64]*runningConnection),
	}
}

// Reload loads all enabled connections/devices/data points and starts,
// stops, or restarts connection pollers so the running set matches
// configuration. Connections whose device group is unchanged since the
// last Reload are left running (their client is not disturbed).
//
// Editing any one device on a shared connection restarts polling for
// every sibling device on that connection too, since they all run in one
// goroutine keyed by connection ID. Accepted tradeoff: RTU device edits
// are infrequent admin actions, and this is far simpler than diffing
// within a running connection's device set.
func (m *Manager) Reload(ctx context.Context) error {
	devices, err := m.deviceRepo.ListEnabled(ctx)
	if err != nil {
		return err
	}
	conns, err := m.connRepo.ListEnabled(ctx)
	if err != nil {
		return err
	}
	connByID := make(map[int64]connection.Connection, len(conns))
	for _, c := range conns {
		connByID[c.ID] = c
	}

	wanted := make(map[int64]*runningConnection)
	for _, d := range devices {
		conn, ok := connByID[d.ConnectionID]
		if !ok {
			// Connection is disabled or missing — device is paused too.
			continue
		}
		dps, err := m.datapointRepo.ListEnabledByDevice(ctx, d.ID)
		if err != nil {
			return err
		}
		rc, ok := wanted[conn.ID]
		if !ok {
			rc = &runningConnection{conn: conn}
			wanted[conn.ID] = rc
		}
		rc.devices = append(rc.devices, deviceWithPoints{device: d, dps: dps})
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	for id, rc := range m.running {
		w, ok := wanted[id]
		if !ok || !reflect.DeepEqual(rc.conn, w.conn) || !reflect.DeepEqual(rc.devices, w.devices) {
			rc.cancel()
			delete(m.running, id)
		}
	}

	for id, w := range wanted {
		if _, ok := m.running[id]; ok {
			continue
		}
		connCtx, cancel := context.WithCancel(m.parentCtx)
		m.running[id] = &runningConnection{cancel: cancel, conn: w.conn, devices: w.devices}

		m.wg.Add(1)
		go func(conn connection.Connection, devices []deviceWithPoints) {
			defer m.wg.Done()
			m.poller.runConnection(connCtx, conn, devices)
		}(w.conn, w.devices)
	}

	m.log.Info("acquisition reloaded", "active_connections", len(m.running))
	return nil
}

// Wait blocks until every connection poller has stopped. Connections stop
// when their context is cancelled — either individually during Reload, or
// all at once when the parent context passed to NewManager is cancelled.
func (m *Manager) Wait() {
	m.wg.Wait()
}
