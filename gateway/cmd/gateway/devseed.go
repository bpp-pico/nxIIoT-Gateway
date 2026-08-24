package main

import (
	"database/sql"
	"log/slog"
)

// seedDemoDevice inserts a demo Modbus TCP device (pointing at the
// modbus-sim dev container) with two data points, mirroring the PM001
// example in the design doc. It only runs when SEED_DEMO_DEVICE=true and
// the device table is empty — this exists purely so Phase 1 (the
// acquisition engine) can be exercised end-to-end in local dev without
// waiting for Phase 2's device management UI/API.
func seedDemoDevice(db *sql.DB, log *slog.Logger) {
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM device`).Scan(&count); err != nil {
		log.Error("seed: failed to check device table", "error", err)
		return
	}
	if count > 0 {
		return
	}

	res, err := db.Exec(`
		INSERT INTO device (name, protocol, ip_address, slave_id, port, polling_interval_ms, timeout_ms, retry, enabled)
		VALUES ('PM001', 'TCP', 'modbus-sim', 1, 502, 2000, 1000, 3, 1)`)
	if err != nil {
		log.Error("seed: failed to insert demo device", "error", err)
		return
	}
	deviceID, err := res.LastInsertId()
	if err != nil {
		log.Error("seed: failed to read demo device id", "error", err)
		return
	}

	points := []struct {
		tag       string
		fc        int
		address   int
		dataType  string
		byteOrder string
		scale     float64
		unit      string
		pollingMs int
	}{
		{"Voltage_L1", 3, 100, "INT16", "", 0.1, "V", 2000},
		{"Active_Power", 3, 102, "FLOAT32", "ABCD", 1, "W", 2000},
		{"Current_L1", 3, 104, "UINT16", "", 0.1, "A", 2000},
	}

	for _, p := range points {
		if _, err := db.Exec(`
			INSERT INTO datapoint (device_id, tag_name, function_code, register_address, data_type, byte_order, scale, unit, polling_interval_ms, enabled)
			VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, 1)`,
			deviceID, p.tag, p.fc, p.address, p.dataType, p.byteOrder, p.scale, p.unit, p.pollingMs); err != nil {
			log.Error("seed: failed to insert demo datapoint", "tag", p.tag, "error", err)
		}
	}

	log.Info("seeded demo device", "device_id", deviceID, "name", "PM001")
}
