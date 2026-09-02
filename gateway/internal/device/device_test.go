package device_test

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"testing"

	"nxiiot-gateway/internal/connection"
	"nxiiot-gateway/internal/device"
	"nxiiot-gateway/internal/storage"
)

func openTestDB(t *testing.T) (*sql.DB, *device.Repository) {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "test.db")
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	db, err := storage.Open(dbPath, "../../migrations", log)
	if err != nil {
		t.Fatalf("open storage: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	return db, device.NewRepository(db)
}

// seedConnection inserts a Connection for device rows to reference —
// device.connection_id is a required FK, so every test needs one.
func seedConnection(t *testing.T, db *sql.DB) int64 {
	t.Helper()

	ctx := context.Background()
	connRepo := connection.NewRepository(db)
	id, err := connRepo.Create(ctx, connection.Connection{
		Name: "USB-RS485", Protocol: connection.RTU, Interface: "/dev/ttyUSB0",
		BaudRate: 9600, DataBits: 8, Parity: "N", StopBits: 1,
		TimeoutMs: 1000, Retry: 3, Enabled: true,
	})
	if err != nil {
		t.Fatalf("seed connection: %v", err)
	}
	return id
}

func sampleDevice(connID int64) device.Device {
	return device.Device{
		Name: "Temp-Humidity Sensor", ConnectionID: connID, SlaveID: 1,
		Enabled: true,
	}
}

func TestDeviceCreateAndGet(t *testing.T) {
	ctx := context.Background()
	db, repo := openTestDB(t)
	connID := seedConnection(t, db)

	id, err := repo.Create(ctx, sampleDevice(connID))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := repo.Get(ctx, id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Name != "Temp-Humidity Sensor" || got.ConnectionID != connID || got.SlaveID != 1 {
		t.Fatalf("unexpected device: %+v", got)
	}
}

func TestDeviceGetMissingReturnsErrNotFound(t *testing.T) {
	ctx := context.Background()
	_, repo := openTestDB(t)

	_, err := repo.Get(ctx, 999)
	if !errors.Is(err, device.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestDeviceListEnabledExcludesDisabled(t *testing.T) {
	ctx := context.Background()
	db, repo := openTestDB(t)
	connID := seedConnection(t, db)

	enabled := sampleDevice(connID)
	enabled.Name = "enabled-one"
	if _, err := repo.Create(ctx, enabled); err != nil {
		t.Fatalf("Create enabled: %v", err)
	}

	disabled := sampleDevice(connID)
	disabled.Name = "disabled-one"
	disabled.SlaveID = 2
	disabled.Enabled = false
	if _, err := repo.Create(ctx, disabled); err != nil {
		t.Fatalf("Create disabled: %v", err)
	}

	got, err := repo.ListEnabled(ctx)
	if err != nil {
		t.Fatalf("ListEnabled: %v", err)
	}
	if len(got) != 1 || got[0].Name != "enabled-one" {
		t.Fatalf("expected exactly the enabled device, got %+v", got)
	}
}

func TestDeviceTwoDevicesCanShareOneConnection(t *testing.T) {
	// The whole point of the split: two devices on the same RTU bus,
	// different slave IDs, one connection_id — this used to be
	// impossible (see MEMORY.md's shared-port TIMEOUT incident).
	ctx := context.Background()
	db, repo := openTestDB(t)
	connID := seedConnection(t, db)

	first := sampleDevice(connID)
	first.Name = "Temp-Humidity Sensor"
	first.SlaveID = 1
	if _, err := repo.Create(ctx, first); err != nil {
		t.Fatalf("Create first: %v", err)
	}

	second := sampleDevice(connID)
	second.Name = "PM"
	second.SlaveID = 2
	if _, err := repo.Create(ctx, second); err != nil {
		t.Fatalf("Create second: %v", err)
	}

	got, err := repo.ListEnabled(ctx)
	if err != nil {
		t.Fatalf("ListEnabled: %v", err)
	}
	if len(got) != 2 || got[0].ConnectionID != got[1].ConnectionID {
		t.Fatalf("expected both devices to share one connection_id, got %+v", got)
	}
}

func TestDeviceUpdate(t *testing.T) {
	ctx := context.Background()
	db, repo := openTestDB(t)
	connID := seedConnection(t, db)

	id, err := repo.Create(ctx, sampleDevice(connID))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	updated := sampleDevice(connID)
	updated.SlaveID = 7
	if err := repo.Update(ctx, id, updated); err != nil {
		t.Fatalf("Update: %v", err)
	}

	got, err := repo.Get(ctx, id)
	if err != nil {
		t.Fatalf("Get after update: %v", err)
	}
	if got.SlaveID != 7 {
		t.Fatalf("expected updated slave_id 7, got %d", got.SlaveID)
	}
}

func TestDeviceDelete(t *testing.T) {
	ctx := context.Background()
	db, repo := openTestDB(t)
	connID := seedConnection(t, db)

	id, err := repo.Create(ctx, sampleDevice(connID))
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := repo.Delete(ctx, id); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	if _, err := repo.Get(ctx, id); !errors.Is(err, device.ErrNotFound) {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}
}

func TestDeviceValidateRejectsMissingConnectionID(t *testing.T) {
	d := sampleDevice(0)
	if err := d.Validate(); err == nil {
		t.Fatal("expected Validate to reject a zero connection_id")
	}
}
