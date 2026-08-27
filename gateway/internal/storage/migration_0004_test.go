package storage_test

import (
	"database/sql"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"testing"

	"nxiiot-gateway/internal/storage"
)

// TestMigration0004SplitsDeviceIntoConnectionAgainstSeededData proves
// 0004_connection_split.sql is safe against a database that already has
// real device/datapoint rows — not just an empty schema. This project has
// already been burned once by a migration that passed against an empty
// test DB and failed against one with real rows (0003_data_queue_retry.sql,
// see MEMORY.md) — the risk here is sharper still, since 0004 does a real
// DROP TABLE on `device`, which `datapoint.device_id` references via a
// foreign key, inside the single transaction the migration runner wraps
// each file in (storage.go's migrate()). Reasoning about whether that's
// safe isn't good enough; this proves it against the actual production
// code path.
func TestMigration0004SplitsDeviceIntoConnectionAgainstSeededData(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	// Step 1: apply only the pre-0004 migrations (a temp dir holding just
	// 0001-0003), then seed real device/datapoint rows by hand — this is
	// what a real, already-deployed gateway's database looks like right
	// before it's upgraded to a binary that bundles 0004.
	preMigrationsDir := t.TempDir()
	copyMigration(t, preMigrationsDir, "0001_init.sql")
	copyMigration(t, preMigrationsDir, "0002_device_rtu_params.sql")
	copyMigration(t, preMigrationsDir, "0003_data_queue_retry.sql")

	db, err := storage.Open(dbPath, preMigrationsDir, log)
	if err != nil {
		t.Fatalf("open pre-0004 schema: %v", err)
	}

	// Two devices sharing a serial interface — exactly the real-world
	// shape that motivated this migration — plus a TCP device, to cover
	// both protocols' column sets.
	mustExec(t, db, `INSERT INTO device (id, name, protocol, interface, slave_id, baud_rate, data_bits, parity, stop_bits, polling_interval_ms, timeout_ms, retry, enabled)
		VALUES (1, 'Temp-Humidity Sensor', 'RTU', '/dev/ttyUSB0', 1, 9600, 8, 'N', 1, 1000, 1000, 3, 1)`)
	mustExec(t, db, `INSERT INTO device (id, name, protocol, interface, slave_id, baud_rate, data_bits, parity, stop_bits, polling_interval_ms, timeout_ms, retry, enabled)
		VALUES (2, 'PM', 'RTU', '/dev/ttyUSB0', 2, 9600, 8, 'N', 1, 1000, 1000, 3, 1)`)
	mustExec(t, db, `INSERT INTO device (id, name, protocol, ip_address, port, slave_id, polling_interval_ms, timeout_ms, retry, enabled)
		VALUES (3, 'TCP Meter', 'TCP', '10.0.0.5', 502, 1, 5000, 1000, 3, 1)`)

	mustExec(t, db, `INSERT INTO datapoint (id, device_id, tag_name, function_code, register_address, data_type, polling_interval_ms, enabled)
		VALUES (1, 1, 'temperature', 4, 1, 'INT16', 1000, 1)`)
	mustExec(t, db, `INSERT INTO datapoint (id, device_id, tag_name, function_code, register_address, data_type, polling_interval_ms, enabled)
		VALUES (2, 1, 'humidity', 4, 2, 'INT16', 1000, 1)`)
	mustExec(t, db, `INSERT INTO datapoint (id, device_id, tag_name, function_code, register_address, data_type, polling_interval_ms, enabled)
		VALUES (3, 2, 'pm25', 4, 1, 'INT16', 1000, 1)`)
	mustExec(t, db, `INSERT INTO datapoint (id, device_id, tag_name, function_code, register_address, data_type, polling_interval_ms, enabled)
		VALUES (4, 3, 'voltage', 3, 100, 'FLOAT32', 5000, 1)`)

	if err := db.Close(); err != nil {
		t.Fatalf("close pre-0004 db: %v", err)
	}

	// Step 2: re-open the SAME db file through the real migrations
	// directory (which now includes 0004) — schema_migrations already
	// records 0001-0003 as applied, so only 0004 runs, against real data.
	db, err = storage.Open(dbPath, "../../migrations", log)
	if err != nil {
		t.Fatalf("apply 0004 against seeded db: %v", err)
	}
	defer db.Close()

	// Every device kept its data points, gained exactly one connection,
	// and the two RTU devices' connections still show the shared
	// interface (not yet consolidated — that's a deliberate, separate,
	// human decision, not something this migration does automatically).
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM connection`).Scan(&count); err != nil {
		t.Fatalf("count connections: %v", err)
	}
	if count != 3 {
		t.Fatalf("expected 3 connections (one per pre-existing device), got %d", count)
	}

	type deviceRow struct {
		connID      int64
		slaveID     sql.NullInt64
		pollingMs   int
	}
	rows := map[int64]deviceRow{}
	r, err := db.Query(`SELECT id, connection_id, slave_id, polling_interval_ms FROM device ORDER BY id`)
	if err != nil {
		t.Fatalf("query device: %v", err)
	}
	for r.Next() {
		var id, connID int64
		var slaveID sql.NullInt64
		var pollingMs int
		if err := r.Scan(&id, &connID, &slaveID, &pollingMs); err != nil {
			t.Fatalf("scan device: %v", err)
		}
		rows[id] = deviceRow{connID: connID, slaveID: slaveID, pollingMs: pollingMs}
	}
	r.Close()

	if len(rows) != 3 {
		t.Fatalf("expected 3 devices to survive the migration, got %d", len(rows))
	}
	if rows[1].connID == rows[3].connID {
		t.Fatalf("RTU device 1 and TCP device 3 should not share a connection, both got %d", rows[1].connID)
	}
	if rows[1].connID == rows[2].connID {
		t.Fatalf("devices 1 and 2 should each have their OWN auto-migrated connection (not consolidated) immediately after migration, both got %d", rows[1].connID)
	}

	var iface1, iface2 sql.NullString
	if err := db.QueryRow(`SELECT interface FROM connection WHERE id = ?`, rows[1].connID).Scan(&iface1); err != nil {
		t.Fatalf("read connection 1's interface: %v", err)
	}
	if err := db.QueryRow(`SELECT interface FROM connection WHERE id = ?`, rows[2].connID).Scan(&iface2); err != nil {
		t.Fatalf("read connection 2's interface: %v", err)
	}
	if iface1.String != "/dev/ttyUSB0" || iface2.String != "/dev/ttyUSB0" {
		t.Fatalf("expected both RTU devices' auto-migrated connections to still show /dev/ttyUSB0, got %q and %q", iface1.String, iface2.String)
	}

	var datapointCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM datapoint`).Scan(&datapointCount); err != nil {
		t.Fatalf("count datapoints: %v", err)
	}
	if datapointCount != 4 {
		t.Fatalf("expected all 4 datapoints to survive untouched, got %d", datapointCount)
	}

	// The real proof this migration is safe under foreign_keys=ON: no
	// dangling references anywhere in the database after the DROP
	// TABLE/rebuild on device (an FK parent of datapoint).
	violations, err := db.Query(`PRAGMA foreign_key_check`)
	if err != nil {
		t.Fatalf("foreign_key_check: %v", err)
	}
	defer violations.Close()
	if violations.Next() {
		t.Fatal("PRAGMA foreign_key_check reported a violation after migration 0004 — see test output above")
	}
}

func copyMigration(t *testing.T, destDir, name string) {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("../../migrations", name))
	if err != nil {
		t.Fatalf("read migration %s: %v", name, err)
	}
	if err := os.WriteFile(filepath.Join(destDir, name), data, 0o644); err != nil {
		t.Fatalf("copy migration %s: %v", name, err)
	}
}

func mustExec(t *testing.T, db *sql.DB, query string, args ...any) {
	t.Helper()
	if _, err := db.Exec(query, args...); err != nil {
		t.Fatalf("exec %q: %v", query, err)
	}
}
