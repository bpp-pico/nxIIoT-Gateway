package storage_test

import (
	"database/sql"
	"io"
	"log/slog"
	"path/filepath"
	"testing"

	"nxiiot-gateway/internal/storage"
)

// TestMigration0005DropsPollingIntervalAgainstSeededData proves
// 0005_scan_polling.sql (a plain ALTER TABLE ADD/DROP COLUMN, no table
// rebuild) is safe against a database that already has real connection/
// device/datapoint rows from the post-0004 schema, and that it doesn't
// lose any other column's data along the way.
func TestMigration0005DropsPollingIntervalAgainstSeededData(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "test.db")
	log := slog.New(slog.NewTextHandler(io.Discard, nil))

	// Step 1: apply only 0001-0004 (the post-connection-split schema, which
	// still has polling_interval_ms on both device and datapoint), then
	// seed real rows — this is what an already-deployed gateway's database
	// looks like right before it's upgraded to a binary bundling 0005.
	preMigrationsDir := t.TempDir()
	copyMigration(t, preMigrationsDir, "0001_init.sql")
	copyMigration(t, preMigrationsDir, "0002_device_rtu_params.sql")
	copyMigration(t, preMigrationsDir, "0003_data_queue_retry.sql")
	copyMigration(t, preMigrationsDir, "0004_connection_split.sql")

	db, err := storage.Open(dbPath, preMigrationsDir, log)
	if err != nil {
		t.Fatalf("open pre-0005 schema: %v", err)
	}

	mustExec(t, db, `INSERT INTO connection (id, name, protocol, interface, baud_rate, data_bits, parity, stop_bits, timeout_ms, retry, enabled)
		VALUES (1, 'RTU Bus 1', 'RTU', '/dev/ttyUSB0', 9600, 8, 'N', 1, 1000, 3, 1)`)
	mustExec(t, db, `INSERT INTO device (id, name, connection_id, slave_id, polling_interval_ms, enabled)
		VALUES (1, 'Temp-Humidity Sensor', 1, 1, 250, 1)`)
	mustExec(t, db, `INSERT INTO datapoint (id, device_id, tag_name, function_code, register_address, data_type, polling_interval_ms, enabled)
		VALUES (1, 1, 'temperature', 4, 1, 'UINT16', 250, 1)`)
	mustExec(t, db, `INSERT INTO datapoint (id, device_id, tag_name, function_code, register_address, data_type, polling_interval_ms, enabled)
		VALUES (2, 1, 'humidity', 4, 2, 'UINT16', 250, 1)`)

	if err := db.Close(); err != nil {
		t.Fatalf("close pre-0005 db: %v", err)
	}

	// Step 2: re-open the SAME db file through a migrations dir scoped to
	// just 0001-0005 (not the full "../../migrations", which will grow to
	// include migrations after 0005 too — this test is specifically about
	// 0005's own behavior in isolation, so later migrations must not run
	// here). schema_migrations already records 0001-0004 as applied, so
	// only 0005 runs, against real data.
	postMigrationsDir := t.TempDir()
	copyMigration(t, postMigrationsDir, "0001_init.sql")
	copyMigration(t, postMigrationsDir, "0002_device_rtu_params.sql")
	copyMigration(t, postMigrationsDir, "0003_data_queue_retry.sql")
	copyMigration(t, postMigrationsDir, "0004_connection_split.sql")
	copyMigration(t, postMigrationsDir, "0005_scan_polling.sql")

	db, err = storage.Open(dbPath, postMigrationsDir, log)
	if err != nil {
		t.Fatalf("apply 0005 against seeded db: %v", err)
	}
	defer db.Close()

	// connection gained next_device_delay_ms, defaulted to 250.
	var delayMs int
	if err := db.QueryRow(`SELECT next_device_delay_ms FROM connection WHERE id = 1`).Scan(&delayMs); err != nil {
		t.Fatalf("query next_device_delay_ms: %v", err)
	}
	if delayMs != 250 {
		t.Fatalf("expected next_device_delay_ms default 250, got %d", delayMs)
	}

	// device and datapoint no longer have polling_interval_ms.
	assertColumnDropped(t, db, "device", "polling_interval_ms")
	assertColumnDropped(t, db, "datapoint", "polling_interval_ms")

	// Every other column on the surviving rows is untouched.
	var deviceName string
	var slaveID int
	if err := db.QueryRow(`SELECT name, slave_id FROM device WHERE id = 1`).Scan(&deviceName, &slaveID); err != nil {
		t.Fatalf("query device: %v", err)
	}
	if deviceName != "Temp-Humidity Sensor" || slaveID != 1 {
		t.Fatalf("device row corrupted: name=%q slave_id=%d", deviceName, slaveID)
	}

	var datapointCount int
	if err := db.QueryRow(`SELECT COUNT(*) FROM datapoint`).Scan(&datapointCount); err != nil {
		t.Fatalf("count datapoints: %v", err)
	}
	if datapointCount != 2 {
		t.Fatalf("expected both datapoints to survive untouched, got %d", datapointCount)
	}

	var tag string
	var addr int
	if err := db.QueryRow(`SELECT tag_name, register_address FROM datapoint WHERE id = 2`).Scan(&tag, &addr); err != nil {
		t.Fatalf("query datapoint 2: %v", err)
	}
	if tag != "humidity" || addr != 2 {
		t.Fatalf("datapoint row corrupted: tag_name=%q register_address=%d", tag, addr)
	}

	violations, err := db.Query(`PRAGMA foreign_key_check`)
	if err != nil {
		t.Fatalf("foreign_key_check: %v", err)
	}
	defer violations.Close()
	if violations.Next() {
		t.Fatal("PRAGMA foreign_key_check reported a violation after migration 0005 — see test output above")
	}
}

// assertColumnDropped fails the test if table still has a column named col.
func assertColumnDropped(t *testing.T, db *sql.DB, table, col string) {
	t.Helper()
	rows, err := db.Query(`PRAGMA table_info(` + table + `)`)
	if err != nil {
		t.Fatalf("table_info(%s): %v", table, err)
	}
	defer rows.Close()

	for rows.Next() {
		var cid int
		var name, ctype string
		var notNull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notNull, &dflt, &pk); err != nil {
			t.Fatalf("scan table_info(%s): %v", table, err)
		}
		if name == col {
			t.Fatalf("expected column %s.%s to be dropped, but it's still present", table, col)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("table_info(%s) iteration: %v", table, err)
	}
}
