-- Initial schema for nxIIoT Gateway
-- Entities: gateway, device, datapoint, data_queue, system_config, audit_log

CREATE TABLE IF NOT EXISTS gateway (
    id              TEXT PRIMARY KEY,
    name            TEXT NOT NULL,
    last_sequence   INTEGER NOT NULL DEFAULT 0,
    created_at      TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

CREATE TABLE IF NOT EXISTS device (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    name            TEXT NOT NULL,
    protocol        TEXT NOT NULL CHECK (protocol IN ('RTU','TCP')),
    interface       TEXT,
    ip_address      TEXT,
    slave_id        INTEGER,
    port            INTEGER,
    polling_interval_ms INTEGER NOT NULL DEFAULT 1000,
    timeout_ms      INTEGER NOT NULL DEFAULT 1000,
    retry           INTEGER NOT NULL DEFAULT 3,
    enabled         INTEGER NOT NULL DEFAULT 1,
    created_at      TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    updated_at      TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

CREATE TABLE IF NOT EXISTS datapoint (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    device_id       INTEGER NOT NULL REFERENCES device(id) ON DELETE CASCADE,
    tag_name        TEXT NOT NULL,
    function_code   INTEGER NOT NULL,
    register_address INTEGER NOT NULL,
    data_type       TEXT NOT NULL,
    byte_order      TEXT NOT NULL DEFAULT 'ABCD',
    word_order      TEXT NOT NULL DEFAULT 'AB',
    scale           REAL NOT NULL DEFAULT 1,
    offset          REAL NOT NULL DEFAULT 0,
    unit            TEXT,
    polling_interval_ms INTEGER NOT NULL DEFAULT 1000,
    priority        TEXT NOT NULL DEFAULT 'NORMAL' CHECK (priority IN ('CRITICAL','HIGH','NORMAL','LOW')),
    enabled         INTEGER NOT NULL DEFAULT 1,
    created_at      TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    updated_at      TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

CREATE INDEX IF NOT EXISTS idx_datapoint_device_id ON datapoint(device_id);

CREATE TABLE IF NOT EXISTS data_queue (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    gateway_id      TEXT NOT NULL,
    sequence_id     INTEGER NOT NULL,
    device_id       INTEGER NOT NULL,
    datapoint_id    INTEGER NOT NULL,
    value           REAL,
    quality         TEXT NOT NULL,
    event_timestamp TEXT NOT NULL,
    created_at      TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now')),
    status          TEXT NOT NULL DEFAULT 'PENDING' CHECK (status IN ('PENDING','SENDING','SENT','FAILED')),
    retry_count     INTEGER NOT NULL DEFAULT 0,
    sent_at         TEXT,
    last_error      TEXT,
    priority        TEXT NOT NULL DEFAULT 'NORMAL'
);

CREATE UNIQUE INDEX IF NOT EXISTS idx_data_queue_gateway_sequence ON data_queue(gateway_id, sequence_id);
CREATE INDEX IF NOT EXISTS idx_data_queue_status ON data_queue(status);

CREATE TABLE IF NOT EXISTS system_config (
    key             TEXT PRIMARY KEY,
    value           TEXT NOT NULL,
    updated_at      TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);

CREATE TABLE IF NOT EXISTS audit_log (
    id              INTEGER PRIMARY KEY AUTOINCREMENT,
    action          TEXT NOT NULL,
    detail          TEXT,
    created_at      TEXT NOT NULL DEFAULT (strftime('%Y-%m-%dT%H:%M:%fZ','now'))
);
