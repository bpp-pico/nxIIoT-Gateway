package connection_test

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"log/slog"
	"path/filepath"
	"testing"

	"nxiiot-gateway/internal/connection"
	"nxiiot-gateway/internal/storage"
)

func openTestDB(t *testing.T) (*sql.DB, *connection.Repository) {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "test.db")
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	db, err := storage.Open(dbPath, "../../migrations", log)
	if err != nil {
		t.Fatalf("open storage: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	return db, connection.NewRepository(db)
}

func sampleRTUConnection() connection.Connection {
	return connection.Connection{
		Name: "USB-RS485", Protocol: connection.RTU, Interface: "/dev/ttyUSB0",
		BaudRate: 9600, DataBits: 8, Parity: "N", StopBits: 1,
		TimeoutMs: 1000, Retry: 3, Enabled: true, NextDeviceDelayMs: 250,
	}
}

func TestConnectionCreateAndGet(t *testing.T) {
	ctx := context.Background()
	_, repo := openTestDB(t)

	id, err := repo.Create(ctx, sampleRTUConnection())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	got, err := repo.Get(ctx, id)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if got.Name != "USB-RS485" || got.Interface != "/dev/ttyUSB0" || got.BaudRate != 9600 || got.NextDeviceDelayMs != 250 {
		t.Fatalf("unexpected connection: %+v", got)
	}
}

func TestConnectionGetMissingReturnsErrNotFound(t *testing.T) {
	ctx := context.Background()
	_, repo := openTestDB(t)

	_, err := repo.Get(ctx, 999)
	if !errors.Is(err, connection.ErrNotFound) {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestConnectionListEnabledExcludesDisabled(t *testing.T) {
	ctx := context.Background()
	_, repo := openTestDB(t)

	enabled := sampleRTUConnection()
	enabled.Name = "enabled-one"
	if _, err := repo.Create(ctx, enabled); err != nil {
		t.Fatalf("Create enabled: %v", err)
	}

	disabled := sampleRTUConnection()
	disabled.Name = "disabled-one"
	disabled.Enabled = false
	if _, err := repo.Create(ctx, disabled); err != nil {
		t.Fatalf("Create disabled: %v", err)
	}

	got, err := repo.ListEnabled(ctx)
	if err != nil {
		t.Fatalf("ListEnabled: %v", err)
	}
	if len(got) != 1 || got[0].Name != "enabled-one" {
		t.Fatalf("expected exactly the enabled connection, got %+v", got)
	}
}

func TestConnectionUpdate(t *testing.T) {
	ctx := context.Background()
	_, repo := openTestDB(t)

	id, err := repo.Create(ctx, sampleRTUConnection())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	updated := sampleRTUConnection()
	updated.BaudRate = 19200
	updated.NextDeviceDelayMs = 500
	if err := repo.Update(ctx, id, updated); err != nil {
		t.Fatalf("Update: %v", err)
	}

	got, err := repo.Get(ctx, id)
	if err != nil {
		t.Fatalf("Get after update: %v", err)
	}
	if got.BaudRate != 19200 {
		t.Fatalf("expected updated baud_rate 19200, got %d", got.BaudRate)
	}
	if got.NextDeviceDelayMs != 500 {
		t.Fatalf("expected updated next_device_delay_ms 500, got %d", got.NextDeviceDelayMs)
	}
}

func TestConnectionDeleteBlockedWhileDeviceReferencesIt(t *testing.T) {
	ctx := context.Background()
	db, repo := openTestDB(t)

	id, err := repo.Create(ctx, sampleRTUConnection())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if _, err := db.ExecContext(ctx, `INSERT INTO device (name, connection_id, slave_id, enabled) VALUES (?, ?, ?, ?)`,
		"dependent device", id, 1, 1); err != nil {
		t.Fatalf("seed dependent device: %v", err)
	}

	if err := repo.Delete(ctx, id); !errors.Is(err, connection.ErrInUse) {
		t.Fatalf("expected ErrInUse while a device still references the connection, got %v", err)
	}

	// Confirm it's still there — the guard must reject before deleting.
	if _, err := repo.Get(ctx, id); err != nil {
		t.Fatalf("expected connection to still exist after blocked delete, Get failed: %v", err)
	}
}

func TestConnectionDeleteSucceedsOnceUnreferenced(t *testing.T) {
	ctx := context.Background()
	_, repo := openTestDB(t)

	id, err := repo.Create(ctx, sampleRTUConnection())
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if err := repo.Delete(ctx, id); err != nil {
		t.Fatalf("Delete: %v", err)
	}

	if _, err := repo.Get(ctx, id); !errors.Is(err, connection.ErrNotFound) {
		t.Fatalf("expected ErrNotFound after delete, got %v", err)
	}
}
