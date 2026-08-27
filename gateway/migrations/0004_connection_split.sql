-- Splits `device` into a `connection` (the physical link: protocol,
-- interface/ip+port, baud/parity/stop-bits, timeout, retry) and a slimmer
-- `device` (a slave ID + its data points, referencing a connection).
--
-- Real gateways run real Modbus RTU multi-drop: many slave IDs sharing one
-- physical serial bus. The pre-split schema conflated "physical connection"
-- with "logical device" on one flat `device` row, so the gateway had no way
-- to know two device rows were on the same wire — each device's acquisition
-- goroutine opened its own independent serial handle, and two devices
-- configured with the same `interface` corrupted each other's traffic on
-- the wire (confirmed live: both showed TIMEOUT). See MEMORY.md.
--
-- This migration preserves current behavior exactly: every existing device
-- gets its own 1:1 auto-migrated connection, so nothing changes at the bus
-- level until an operator explicitly repoints a device at a shared
-- connection via the Web UI. The migration cannot know whether two
-- identical-looking device rows are intentionally the same bus or just
-- coincidentally identical config — that's a human decision, not something
-- to guess here.

CREATE TABLE connection (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    name            TEXT NOT NULL,
    protocol        TEXT NOT NULL CHECK (protocol IN ('RTU','TCP')),
    interface       TEXT,
    ip_address      TEXT,
    port            INTEGER,
    baud_rate       INTEGER NOT NULL DEFAULT 9600,
    data_bits       INTEGER NOT NULL DEFAULT 8,
    parity          TEXT NOT NULL DEFAULT 'N',
    stop_bits       INTEGER NOT NULL DEFAULT 1,
    timeout_ms      INTEGER NOT NULL DEFAULT 1000,
    retry           INTEGER NOT NULL DEFAULT 3,
    enabled         INTEGER NOT NULL DEFAULT 1,
    created_at      TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    updated_at      TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

-- One connection per existing device, named uniquely by the source
-- device's own id (device.name itself has no UNIQUE constraint, so the
-- name alone can't be trusted to correlate rows back below).
INSERT INTO connection (name, protocol, interface, ip_address, port, baud_rate, data_bits, parity, stop_bits, timeout_ms, retry)
SELECT
    name || ' (auto-migrated #' || id || ')',
    protocol, interface, ip_address, port, baud_rate, data_bits, parity, stop_bits, timeout_ms, retry
FROM device;

-- device.protocol carries a CHECK constraint, which SQLite refuses to
-- DROP COLUMN through directly — rebuild the table to its final shape
-- instead of a sequence of ALTER TABLE ... DROP COLUMN statements.
CREATE TABLE device_new (
    id                  INTEGER PRIMARY KEY,
    name                TEXT NOT NULL,
    connection_id       INTEGER NOT NULL REFERENCES connection(id),
    slave_id            INTEGER,
    polling_interval_ms INTEGER NOT NULL DEFAULT 1000,
    enabled             INTEGER NOT NULL DEFAULT 1,
    created_at          TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    updated_at          TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

INSERT INTO device_new (id, name, connection_id, slave_id, polling_interval_ms, enabled, created_at, updated_at)
SELECT d.id, d.name, c.id, d.slave_id, d.polling_interval_ms, d.enabled, d.created_at, d.updated_at
FROM device d
JOIN connection c ON c.name = d.name || ' (auto-migrated #' || d.id || ')';

DROP TABLE device;
ALTER TABLE device_new RENAME TO device;

CREATE INDEX idx_device_connection_id ON device(connection_id);
