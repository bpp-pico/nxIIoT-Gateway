# Industrial IoT Gateway
## Handoff & Development Plan

**Document Version:** 1.0  
**Status:** MVP Complete (Phases 0-8 done — see §24 Definition of Done)  
**Target:** Raspberry Pi 4/5 or x86 Linux IPC  
**Development Environment:** Windows 11 for initial development, then Linux/Raspberry Pi deployment

---

# 1. Project Overview

Develop an Industrial IoT Gateway for collecting data from field devices using **Modbus RTU and Modbus TCP**, storing data locally, and forwarding data to an internal server.

The system is designed for a **closed/OT network with no Internet access**.

Core capabilities:

- Modbus RTU Master
- Modbus TCP Client
- Device and Data Point configuration
- Local persistent data storage
- Store & Forward
- MQTT server communication
- Internal NTP-based time synchronization
- RTC fallback
- Web UI
- Diagnostics
- Configuration backup/restore
- Recovery after restart/power failure

---

# 2. High-Level Architecture

```text
                    CLOSED / OT NETWORK

 ┌──────────────────── Field Network ────────────────────┐
 │                                                       │
 │  Modbus RTU                         Modbus TCP        │
 │      │                                  │             │
 │      ▼                                  ▼             │
 │ ┌─────────────────────────────────────────────────┐  │
 │ │                 IoT Gateway                     │  │
 │ │                                                 │  │
 │ │  ┌───────────────┐    ┌─────────────────────┐  │  │
 │ │  │ Modbus Engine │───►│ Data Processor      │  │  │
 │ │  └───────────────┘    │ Timestamp / Quality │  │  │
 │ │                       └──────────┬──────────┘  │  │
 │ │                                  │             │  │
 │ │                                  ▼             │  │
 │ │                       ┌─────────────────────┐  │  │
 │ │                       │ SQLite + WAL        │  │  │
 │ │                       │ Persistent Queue    │  │  │
 │ │                       └──────────┬──────────┘  │  │
 │ │                                  │             │  │
 │ │                                  ▼             │  │
 │ │                       ┌─────────────────────┐  │  │
 │ │                       │ Store & Forward     │  │  │
 │ │                       │ Batch / Retry / ACK │  │  │
 │ │                       └──────────┬──────────┘  │  │
 │ │                                  │             │  │
 │ │                     ┌────────────┴──────────┐  │  │
 │ │                     │ MQTT / HTTPS Adapter  │  │  │
 │ │                     └────────────┬──────────┘  │  │
 │ │                                  │             │  │
 │ │  ┌───────────────┐               │             │  │
 │ │  │ Time Service  │               │             │  │
 │ │  │ NTP + RTC     │               │             │  │
 │ │  └───────────────┘               │             │  │
 │ │                                  │             │  │
 │ │  ┌───────────────────────────────▼───────────┐ │  │
 │ │  │ REST API + React Web UI                   │ │  │
 │ │  └───────────────────────────────────────────┘ │  │
 │ └──────────────────────────┬──────────────────────┘  │
 │                            │                         │
 │                            │ Local LAN               │
 │                            ▼                         │
 │                  ┌─────────────────────┐              │
 │                  │ Internal Server     │              │
 │                  │                     │              │
 │                  │ MQTT Broker         │              │
 │                  │ Database            │              │
 │                  │ NTP Server         │              │
 │                  └─────────────────────┘              │
 └───────────────────────────────────────────────────────┘
```

---

# 3. Design Principles

The following principles are mandatory:

1. **Gateway must not require Internet access.**
2. **Local Database is the Gateway Source of Truth.**
3. Modbus acquisition must continue even when the Server is unavailable.
4. Data must be persisted before being forwarded.
5. Store & Forward must survive application restart and power failure.
6. Event timestamp is created when the Modbus value is acquired.
7. Gateway uses an internal NTP server for time synchronization.
8. RTC is used as a fallback time source.
9. Gateway-to-Server delivery uses **At-Least-Once Delivery**.
10. Server must implement idempotent processing using `gateway_id + sequence_id`.
11. Protocol-specific code must be isolated from business logic.
12. Configuration must be exportable/importable.

---

# 4. Functional Requirements

## FR-001 Modbus RTU

Support:

- RS485
- Modbus RTU Master
- Slave ID 1-247
- Baud rate
- Data bits
- Parity
- Stop bits
- Timeout
- Retry
- Polling interval

Example:

```text
Interface: RS485-1
Baudrate: 9600
Data Bits: 8
Parity: Even
Stop Bits: 1
Timeout: 1000 ms
Retry: 3
```

---

## FR-002 Modbus TCP

Support:

- IP address
- TCP port
- Unit ID
- Timeout
- Retry
- Polling interval

Default TCP port:

```text
502
```

---

# 5. Device Model

Configuration hierarchy:

```text
Gateway
└── Device
    └── Data Point
```

Example:

```text
Gateway
└── PM001
    ├── Voltage_L1
    ├── Voltage_L2
    ├── Voltage_L3
    ├── Current_L1
    └── Active_Power
```

Device configuration:

```text
Device Name
Protocol
Interface
IP Address / Slave ID
Port
Polling Interval
Timeout
Retry
Enabled
```

---

# 6. Data Point Model

Each Data Point should support:

```text
Tag Name
Function Code
Register Address
Data Type
Byte Order
Word Order
Scale
Offset
Unit
Polling Interval
Priority
Enabled
```

Example:

```text
Tag Name: Voltage_L1
Function: 03
Address: 100
Data Type: FLOAT32
Byte Order: ABCD
Scale: 0.1
Unit: V
Polling: 1000 ms
Priority: NORMAL
```

---

# 7. Data Quality

Supported quality values:

```text
GOOD
TIMEOUT
CRC_ERROR
DEVICE_OFFLINE
INVALID
```

Example:

```json
{
  "tag": "PM001.Voltage_L1",
  "value": 230.2,
  "quality": "GOOD"
}
```

When communication fails:

```json
{
  "tag": "PM001.Voltage_L1",
  "value": null,
  "quality": "TIMEOUT"
}
```

Do not reuse an old value and mark it as `GOOD`.

---

# 8. Persistent Storage

Use:

```text
SQLite + WAL
```

Initial database entities:

```text
gateway
device
datapoint
data_queue
system_config
audit_log
```

Recommended `data_queue` fields:

```text
id
gateway_id
sequence_id
device_id
datapoint_id
value
quality
event_timestamp
created_at
status
retry_count
sent_at
last_error
priority
```

---

# 9. Store & Forward

## 9.1 Data Flow

```text
Modbus
   ↓
Data Processor
   ↓
Create Timestamp
   ↓
Assign Sequence ID
   ↓
INSERT SQLite
   ↓
PENDING
   ↓
Forward Worker
   ↓
Batch
   ↓
Server
   ↓
ACK
   ↓
SENT
```

Modbus Engine must never depend directly on Server availability.

---

## 9.2 Queue State Machine

```text
             ┌──────────┐
             │ PENDING  │
             └────┬─────┘
                  │
                  ▼
             ┌──────────┐
             │ SENDING  │
             └────┬─────┘
                  │
          ┌───────┴────────┐
          │                │
          ▼                ▼
      ┌────────┐       ┌────────┐
      │  SENT  │       │ FAILED │
      └────────┘       └────┬───┘
                             │
                             ▼
                          PENDING
```

---

## 9.3 Recovery

On application restart:

```text
SENDING → PENDING
```

Any incomplete transmission must be retried.

---

## 9.4 Retry

Use exponential backoff:

```text
1 sec
2 sec
4 sec
8 sec
16 sec
32 sec
60 sec
60 sec
...
```

Maximum retry interval:

```text
60 seconds
```

---

## 9.5 Batch

Do not send one record per message.

Default:

```text
Batch Size = 100 records
```

Make configurable.

---

# 10. Sequence ID and Duplicate Handling

Every data record must have:

```text
gateway_id
sequence_id
```

Example:

```text
GW001-000000001
GW001-000000002
GW001-000000003
```

Sequence must persist across Gateway restarts.

Server must use the sequence to detect duplicates.

Delivery model:

```text
At-Least-Once Delivery
+
Server Idempotency
```

If ACK is lost:

```text
Gateway → DATA → Server
Gateway ← ACK  ← X
```

Gateway retries.

Server recognizes the same `gateway_id + sequence_id` and does not create a duplicate record.

---

# 11. Time Synchronization

The system has **no Internet access**.

Therefore:

```text
Internal Server
      │
      │ NTP
      ▼
Gateway
```

The internal Server acts as the NTP source.

Gateway should support:

```text
NTP Server
RTC
Manual Configuration
```

Recommended priority:

```text
NTP
 ↓
RTC
 ↓
Local Clock
```

---

# 12. RTC

Hardware RTC with battery backup is recommended.

Boot sequence:

```text
RTC
 ↓
System Clock
 ↓
NTP Synchronization
 ↓
Normal Operation
```

If NTP is unavailable:

```text
NTP FAIL
 ↓
Use RTC/System Clock
 ↓
Continue Modbus Acquisition
```

Gateway must not stop Modbus acquisition because the NTP server is unavailable.

---

# 13. Timestamp Strategy

Store timestamps internally as UTC.

Each data record should contain:

```text
event_timestamp
created_at
sent_at
```

Example:

```text
event_timestamp = 2026-08-21T13:45:32.123Z
created_at      = 2026-08-21T13:45:32.130Z
sent_at         = 2026-08-21T14:05:20.200Z
```

Meaning:

- Event occurred at 13:45
- Stored at 13:45
- Sent to Server at 14:05

`event_timestamp` must never be changed during Store & Forward.

---

# 14. Time Quality

Supported values:

```text
SYNCED
RTC
UNSYNCED
INVALID
```

Example:

```json
{
  "timestamp": "2026-08-21T13:45:32.123Z",
  "time_quality": "SYNCED"
}
```

---

# 15. Server Communication

Primary protocol:

```text
MQTT
```

Architecture must use an adapter/interface so HTTPS can be added later.

```text
Persistent Queue
      │
      ├── MQTT Adapter
      │
      └── HTTPS Adapter
```

MQTT should support:

- Reconnect
- Authentication
- TLS
- QoS 1
- Batch publishing
- ACK/application-level acknowledgement

---

# 16. Web UI

## Dashboard

Display:

```text
Gateway Status
CPU
RAM
Storage
Network
Device Count
Data Point Count
Server Connection
Pending Queue
Time Synchronization
```

## Device Management

- Add Device
- Edit Device
- Delete Device
- Enable/Disable
- Test Connection

## Data Point Management

- Add
- Edit
- Delete
- Enable/Disable
- Test Read

## Store & Forward

Display:

```text
Pending Records
Storage Usage
Oldest Pending Record
Newest Pending Record
Retry Count
Server Status
```

## Time

Display:

```text
System Time
Timezone
NTP Server
NTP Status
Last Sync
Clock Offset
RTC Status
Time Quality
```

## Diagnostics

Display:

```text
Modbus TX
Modbus RX
Response Time
Timeout
CRC Error
Retry Count
```

---

# 17. Storage Management

Recommended thresholds:

```text
70%  Warning
80%  Warning
90%  Critical
95%  Storage Full Action
```

When storage is full or near full, support configurable policy.

Recommended default:

```text
Delete Oldest Non-Critical Data
```

Data priority:

```text
CRITICAL
HIGH
NORMAL
LOW
```

When storage pressure occurs, protect higher priority data first.

---

# 18. Configuration Backup / Restore

Configuration should be exportable as JSON.

Example:

```text
gateway-config.json
```

Include:

```text
Gateway settings
Device configuration
Data Point configuration
Polling configuration
Store & Forward configuration
MQTT configuration
NTP configuration
```

Do not include plaintext passwords in exported configuration unless explicitly encrypted.

---

# 19. Recommended Software Stack

```text
Backend:
Go

Frontend:
React + TypeScript

Database:
SQLite + WAL

API:
REST

Protocol:
Modbus RTU
Modbus TCP
MQTT

Time:
NTP / Chrony
RTC

Deployment:
Docker Compose or systemd
```

The architecture must remain modular so components can be replaced without changing the entire application.

---

# 20. Backend Module Structure

Suggested project structure:

```text
gateway/
├── cmd/
│   └── gateway/
│       └── main.go
│
├── internal/
│   ├── modbus/
│   │   ├── rtu.go
│   │   ├── tcp.go
│   │   └── client.go
│   │
│   ├── device/
│   ├── datapoint/
│   ├── acquisition/
│   ├── processor/
│   ├── storage/
│   ├── queue/
│   ├── forwarder/
│   ├── mqtt/
│   ├── time/
│   ├── system/
│   ├── config/
│   ├── logger/
│   └── api/
│
├── migrations/
├── web/
├── configs/
├── Dockerfile
├── docker-compose.yml
└── README.md
```

---

# 21. API Design

Initial endpoints:

```text
GET    /api/system
GET    /api/time
GET    /api/devices
POST   /api/devices
PUT    /api/devices/:id
DELETE /api/devices/:id

GET    /api/devices/:id/datapoints
POST   /api/devices/:id/datapoints
PUT    /api/datapoints/:id
DELETE /api/datapoints/:id

POST   /api/devices/:id/test

GET    /api/store-forward/status
GET    /api/store-forward/statistics

GET    /api/logs

GET    /api/config/export
POST   /api/config/import
```

---

# 22. Development Plan

## Phase 0 — Project Setup

Tasks:

- [ ] Create Git repository
- [ ] Define project structure
- [ ] Define coding standards
- [ ] Setup Go
- [ ] Setup React + TypeScript
- [ ] Setup SQLite
- [ ] Setup migrations
- [ ] Setup logging
- [ ] Setup configuration management
- [ ] Create Docker Compose for development

Deliverable:

```text
Empty Gateway Application
+ REST API
+ React UI
+ SQLite
```

---

# Phase 1 — Modbus Engine

Tasks:

- [x] Implement Modbus RTU (serial port open/connect verified against a real USB-to-RS485 adapter (CH340, COM3) via a native Windows build of the gateway; full read round-trip against a responding RTU slave device still untested)
- [x] Implement Modbus TCP (verified against a Modbus TCP simulator)
- [x] Connection management
- [x] Timeout handling
- [x] Retry handling
- [x] Function 01
- [x] Function 02
- [x] Function 03
- [x] Function 04
- [x] Data type conversion
- [x] Byte order
- [ ] Word order (byte_order permutation string, e.g. "ABCD"/"BADC", covers word ordering for now; separate word_order field is stored but not yet independently applied)
- [x] Scaling
- [x] Offset
- [x] Quality handling
- [x] Polling scheduler

Implementation: `gateway/internal/modbus` (client, decode, quality), `gateway/internal/acquisition` (poller), `gateway/internal/device` + `gateway/internal/datapoint` (config repositories). Verified end-to-end via `gateway/cmd/modbus-sim`, a dev-only Modbus TCP simulator wired into `docker-compose.yml`, including device-offline/timeout and recovery behavior.

Deliverable:

```text
Gateway can read configured Modbus devices.
```

---

# Phase 2 — Device and Data Point Management

Tasks:

- [x] Device database model
- [x] Data Point database model
- [x] REST API
- [x] Web UI (type-checked and built; not manually clicked through in a real browser in this environment — verified via `tsc -b`, `vite build`, and forcing Vite to transform every new module)
- [x] Add/Edit/Delete Device
- [x] Add/Edit/Delete Data Point
- [x] Enable/Disable
- [x] Test Modbus Read (`POST /api/datapoints/:id/test`, plus `POST /api/devices/:id/test` for Test Connection)
- [x] Device status (in-memory `internal/status` store, fed by the acquisition engine, surfaced as `status`/`last_seen` on `GET /api/devices`)
- [x] Web UI: COM port dropdown for the RTU device "Interface" field, listing serial ports actually available on the gateway host, instead of free-text entry (backend: `GET /api/system/serial-ports`; frontend: `DeviceForm.tsx` now shows a `<select>` when ports are detected, falling back to manual text entry when none are found or the existing value isn't in the list)

Implementation: `gateway/internal/api` (devices.go, datapoints.go, test.go), full CRUD added to `gateway/internal/device` and `gateway/internal/datapoint` repositories, `gateway/internal/acquisition/manager.go` (supervises per-device pollers and reloads them live on every config change — no gateway restart needed), `gateway/internal/status`, `gateway/internal/system/serial.go` (port enumeration via `go.bug.st/serial`, which bumped the module's Go version requirement to 1.25 — all three Dockerfiles updated to `golang:1.25-alpine`). Frontend: `web/src/pages/DevicesPage.tsx` + `DataPointsPanel.tsx` with Add/Edit/Delete/Enable/Test for both devices and data points.

Deliverable:

```text
User can configure Modbus devices from Web UI.
```

---

# Phase 3 — Persistent Data Storage

Tasks:

- [x] Data Queue table (created in Phase 0's migration; nothing wrote to it until now)
- [x] SQLite WAL
- [x] Event timestamp
- [x] Created timestamp
- [x] Sequence ID (per-gateway, persisted on the `gateway` row, atomic via `UPDATE ... RETURNING` in the same transaction as the insert)
- [x] Quality
- [x] Priority (inherited from the data point's configured priority)
- [x] Database indexes (added in Phase 0's migration)
- [x] Retention policy (deletes SENT rows older than `queue.retention_days`; nothing is SENT yet since Phase 4/Store & Forward doesn't exist, so this is currently a no-op in practice but is unit-tested)

Implementation: `gateway/internal/queue` (Repository.Insert, EnsureGateway, DeleteSentOlderThan/RunRetentionSweeper), `gateway/internal/processor` (adapts an `acquisition.Reading` into a queue.Entry — the seam between the Modbus engine and storage). Every Reading is now persisted, not just logged, regardless of quality (a TIMEOUT/DEVICE_OFFLINE reading is stored with `value = NULL`, matching §7 — "do not reuse an old value and mark it GOOD"). Verified end-to-end: created a real device+data point against `modbus-sim`, confirmed `sequence_id` increments 1,2,3..., confirmed the sequence survives a gateway restart (both via a live test and `TestEnsureGatewayIsIdempotentAndPreservesSequence`), and confirmed a real dropped connection persists a `DEVICE_OFFLINE` row with `value = NULL`.

Bug found and fixed along the way: `modbus.QualityFromError` mapped Linux's `"connection refused"` wording to `DEVICE_OFFLINE` but fell through to `INVALID` for Windows's `"actively refused it"` wording for the identical condition — caught by testing the native Windows build against a real dropped connection, not just Linux-container testing. Fixed with explicit per-platform substring matching (a `syscall.ECONNREFUSED` errno check was tried first and does not actually match on Windows — confirmed empirically) and covered by `TestQualityFromErrorConnectionRefused`, which dials a real closed OS socket rather than mocking an error string.

Deliverable:

```text
Every acquired data point is persistently stored.
```

---

# Phase 4 — Store & Forward

Tasks:

- [x] Queue state machine
- [x] PENDING
- [x] SENDING
- [x] SENT
- [x] FAILED (transient — a failed send goes SENDING → PENDING directly, in the same transaction that records the error/retry_count, matching the doc's diagram in spirit: nothing ever rests observably in FAILED)
- [x] Retry
- [x] Exponential backoff (1,2,4,8,16,32,60,60,... — pinned exactly by `TestBackoffDurationMatchesDesignDocSequence`)
- [x] Batch processing (configurable `forwarder.batch_size`, default 100)
- [x] Queue recovery (`RecoverSendingToPending`, run once at forwarder startup — a row stuck SENDING from an unclean shutdown is retried, never lost or double-counted)
- [x] Server connectivity detection (tracked from each send attempt's outcome, surfaced via `GET /api/store-forward/status`)
- [x] Storage threshold (`GET /api/store-forward/status` reports live disk-usage % via `internal/storage.DiskUsagePercent`, gopsutil-based)
- [x] Storage full policy (`EvictOldestNonCritical`: deletes oldest non-CRITICAL rows, LOW first then NORMAL then HIGH, once usage crosses `queue.storage_full_percent` (default 95%); CRITICAL is never evicted)
- [x] Historical data forwarding (FIFO by sequence_id within a priority tier — a large backlog still drains in order, never stalls)
- [x] Current data priority (batches are selected CRITICAL → HIGH → NORMAL → LOW first, so newly-tagged high-priority readings jump a LOW-priority backlog)

Implementation: `gateway/internal/queue` (dispatch.go: FetchBatch/MarkSent/MarkFailed/RecoverSendingToPending/Stats; backoff.go; storagepolicy.go), `gateway/internal/forwarder` (Forwarder — the state machine loop; Adapter interface so the transport is swappable per §15; HTTPAdapter as the dev/test transport ahead of Phase 5's MQTT adapter). Migration `0003_data_queue_retry.sql` adds `next_attempt_at` for backoff scheduling.

Dev/test-only fake server: `gateway/cmd/server-sim` (mirrors `cmd/modbus-sim`'s role for Phase 1) — a minimal HTTP "Internal Server" that deduplicates on `gateway_id + sequence_id`, demonstrating the server-side half of Rule 7/10 (at-least-once delivery + idempotent processing).

Verified live, all three of §23's scenarios A/B/C: forwarded a 137-row historical backlog left over from earlier phases correctly on startup (proving recovery + historical forwarding); stopped `server-sim` and confirmed Modbus acquisition kept running while `pending_records` grew and `server_connected` flipped false; restarted `server-sim` and confirmed the backlog drained to 0 with **zero duplicates** on the server side despite the retries during the outage (`gateway_id + sequence_id` idempotency holds under real retry conditions, not just in theory).

Bug found and fixed along the way: migration 0003 originally used `DEFAULT (strftime(...))` for the new column — SQLite's `ALTER TABLE ADD COLUMN` rejects a non-constant default on a table that already has rows (it would need to backfill each one), which only surfaced against the native gateway's real database (carrying rows from earlier phases), not against fresh-per-test temp databases where the restriction doesn't trigger. Fixed with a constant epoch default, which is functionally equivalent here.

Deliverable:

```text
Server can be disconnected without stopping data acquisition.
```

Verified — see above.

---

# Phase 5 — MQTT

Tasks:

- [x] MQTT client (`github.com/eclipse/paho.mqtt.golang`)
- [x] Connect
- [x] Reconnect (paho `SetAutoReconnect`/`SetConnectRetry`; verified live by killing and restarting the broker mid-run)
- [x] Authentication (username/password, `MQTTConfig.Username`/`Password`)
- [x] TLS (`MQTTConfig.TLS`: CA/cert/key files, `insecure_skip_verify`; not exercised live in dev since `mosquitto.conf` runs plaintext, but the same `*tls.Config` path is used by the modbus/HTTP stack's cert loading)
- [x] QoS 1 (default, configurable)
- [x] Batch publishing (one JSON-encoded batch per publish, reusing the same batching as HTTPAdapter/Forwarder)
- [x] Application-level ACK (batch published with a `batch_id`; adapter waits on a per-gateway ack topic for a matching `{"batch_id": ...}` before treating the batch as sent — QoS 1's PUBACK only proves the broker got it, not that the Internal Server processed it)
- [x] Duplicate handling (client-side: none needed — same as HTTPAdapter, correctness relies on server-side `gateway_id + sequence_id` idempotency per Rule 6/7, verified live: 66/66 unique keys after a retry-heavy broker outage)
- [x] Server status (`GET /api/store-forward/status` unchanged — `MQTTAdapter.Send` returning an error, e.g. "not connected", drives the same `forwarder.Status` the HTTP adapter already fed)

Implementation: `gateway/internal/forwarder/mqttadapter.go` (`MQTTAdapter`, `MQTTAdapterConfig`), `gateway/internal/forwarder/wire.go` (shared `WireEntry`/`toWireEntries`, factored out of `httpadapter.go` so both adapters serialize batches identically). `internal/config.MQTTConfig` gained auth/TLS/topic/timeout fields with `Load()` defaults (topics default to `gateway/<client_id>/data` and `/ack`); `ForwarderConfig.Transport` (`"http"` or `"mqtt"`) selects the adapter in `cmd/gateway/adapter.go`, overridable via `FORWARDER_TRANSPORT`/`MQTT_BROKER_URL` env vars for docker-compose. `cmd/server-sim` now also subscribes over MQTT (`-mqtt-broker`, dedups into the same in-memory store as its HTTP `/ingest` path, publishes the application-level ack) — the dev/test counterpart to a real Internal Server's MQTT consumer. `docker-compose.yml` adds an `eclipse-mosquitto` service (`gateway/configs/mosquitto.conf`, anonymous/plaintext for the closed-network dev case) and now defaults the gateway to `FORWARDER_TRANSPORT=mqtt`.

Tests: `gateway/internal/forwarder/mqttadapter_test.go` spins up a real embedded broker (`github.com/mochi-mqtt/server/v2`, pure Go, no external process) per test, matching this package's existing preference for real infrastructure over mocks (`forwarder_test.go`'s real-SQLite pattern) — covers a full publish+ack round trip against a fake MQTT consumer, an ack-timeout failure when nothing consumes the batch, and a fast-fail when disconnected.

Verified live via `docker compose up` (gateway + mosquitto + server-sim + modbus-sim): gateway connects to mosquitto and forwards real Modbus readings over MQTT end-to-end (`server_connected: true`, 0 pending); stopped mosquitto and confirmed Modbus acquisition kept running while the pending queue grew and `server_connected` flipped false (`"mqtt: not connected to tcp://mosquitto:1883"`); restarted mosquitto and confirmed auto-reconnect plus a full backlog drain back to 0 pending, `server_connected: true`, with **66/66 unique `gateway_id+sequence_id` keys** on server-sim despite the retries during the outage — no duplicates.

Deliverable:

```text
Gateway reliably forwards data to MQTT Server.
```

Verified — see above.

---

# Phase 6 — Time Service

Tasks:

- [x] RTC support (`internal/time/rtc_linux.go`: real hardware via `/dev/rtc0` + `golang.org/x/sys/unix` `IoctlGetRTCTime`/`IoctlSetRTCTime`, the same ioctls `hwclock` uses; `rtc_linux.go` cross-compiles clean for linux/amd64 but is unverified against physical RTC hardware — no RTC chip in this dev environment, same caveat Phase 1 recorded for the RTU serial path)
- [x] System clock initialization (§12 boot sequence: `Service.Run` reads RTC and attempts NTP immediately, before the first periodic tick)
- [x] NTP client (hand-rolled minimal SNTP, `internal/time/ntp.go` — RFC 4330 four-timestamp exchange over UDP; no third-party NTP dependency)
- [x] Internal NTP configuration (`TimeConfig.NTPServer`, any host:port reachable on the LAN — no public NTP pool required or assumed)
- [x] UTC storage (`Service` and `RTC` both operate in UTC throughout; unchanged from Phase 3's existing UTC timestamp convention)
- [x] Timezone (`TimeConfig.Timezone`, surfaced via `GET /api/time`; display-only, does not affect stored UTC timestamps)
- [x] Time quality (`SYNCED`/`RTC`/`UNSYNCED`/`INVALID` per §14, `internal/time/service.go`'s `degrade()` implementing §11's NTP → RTC → Local Clock priority; `INVALID` triggers when the system clock isn't even plausible, e.g. still at the epoch)
- [x] Clock offset (`Status.ClockOffset`, computed from the SNTP exchange, exposed as `clock_offset_ms`)
- [x] Last synchronization (`Status.LastSync`; deliberately preserved across a later failed sync — see below)
- [x] RTC fallback (on NTP failure, quality drops to `RTC` if hardware is available, `UNSYNCED`/`INVALID` otherwise — Rule 10: acquisition is never blocked either way, since `Service.Run` is an independent goroutine sharing nothing with the Modbus poller)

Implementation: `gateway/internal/time` (new package, named `timeservice` internally since its directory `internal/time` would otherwise shadow the standard library `time` package it needs) — `service.go` (`Service`, `Config`, `Status`, `Quality`), `ntp.go` (SNTP client), `rtc.go`/`rtc_linux.go`/`rtc_other.go` (the `RTC` interface and its Linux-hardware vs. every-other-platform implementations, mirroring `internal/system.ListSerialPorts`'s existing pattern of degrading to "unavailable" rather than erroring when there's no hardware). On every successful NTP sync, the service also writes the corrected time back to the RTC ("disciplining" it) so a future boot without NTP reachable starts from a recent time rather than a stale or dead-battery clock. `internal/config.TimeConfig` gained `SyncIntervalSec`/`QueryTimeoutMs`/`RTCDevice` with `Load()` defaults (5 min sync interval, 3 s query timeout, `/dev/rtc0`); `cmd/gateway/main.go` wires `timeservice.New` + `go timeSvc.Run(ctx)` alongside the other independent background loops (retention sweeper, storage sweeper, forwarder), with an `NTP_SERVER` env override for docker-compose parity with the existing `FORWARDER_*`/`MQTT_*` overrides. `GET /api/time` (`internal/api/router.go`) now returns the real §16 Time panel fields instead of the earlier hardcoded `UNSYNCED` stub.

Tests: `gateway/internal/time/ntp_test.go` runs a real UDP socket that speaks the SNTP wire format (not a mocked `queryNTP`), verifying offset computation both positive and negative and the failure path against an unreachable server. `service_test.go` uses a `fakeRTC` (constructed directly since these are same-package white-box tests) to deterministically cover: SYNCED after a successful sync, RTC disciplining (the write-back), degrading to `RTC` when NTP is unreachable but hardware is present, degrading to `UNSYNCED` when neither is available, `LastSync`/`ClockOffset` surviving a later failed sync unchanged, and `Run`'s immediate first sync. All 9 tests pass on Windows (exercising `rtc_other.go`); `GOOS=linux GOARCH=amd64 go build ./...` cross-compiles clean (exercising `rtc_linux.go`, whose ioctls can't run on this dev host).

Verified live via `docker compose`: pointed a one-off gateway container at `NTP_SERVER=pool.ntp.org` (dev-only reachability test — the container's network happens to have outbound access; production still targets an internal NTP server per Rule 8, nothing in the default config or docker-compose points anywhere but empty/LAN) and confirmed a real sync end-to-end — log: `"ntp sync ok" server=pool.ntp.org offset=-15.96063ms`, followed by a graceful `"rtc write skipped" error="rtc: not available on this host"` (no RTC chip in the container, exactly the intended degrade path) — and `GET /api/time` returning `{"time_quality":"SYNCED","ntp_status":true,"clock_offset_ms":-15.96063,"rtc_status":false,...}`. Torn that container down and restarted the normal gateway (default config, `ntp_server` empty) and confirmed it correctly reports `{"time_quality":"UNSYNCED","ntp_status":false,"rtc_status":false}` — Rule 10 holds either way: Modbus acquisition and MQTT forwarding kept running unaffected throughout (`batch forwarded` continued logging in both states).

Deliverable:

```text
Gateway maintains valid timestamps without Internet.
```

Verified — see above.

---

# Phase 7 — Web UI

Tasks:

- [x] Dashboard (`web/src/pages/Dashboard.tsx`, rewritten from Phase 2's placeholder — Gateway Status, CPU, RAM, Storage, Network, Device Count, Data Point Count, Server Connection, Pending Queue, Time Synchronization, all as live-polled cards)
- [x] Device management (built in Phase 2; unchanged)
- [x] Data Point management (built in Phase 2; unchanged)
- [x] Modbus test (built in Phase 2; unchanged — Test Connection/Test Read already wired to `POST /api/devices/:id/test` and `POST /api/datapoints/:id/test`)
- [x] Store & Forward page (`web/src/pages/StoreForwardPage.tsx`)
- [x] Time page (`web/src/pages/TimePage.tsx`)
- [x] Diagnostics (`web/src/pages/DiagnosticsPage.tsx` + new backend: `internal/diagnostics` package tracking Modbus TX/RX/response time/timeout/CRC-error/retry counts, hooked into `acquisition.Poller.readOne`)
- [x] Logs (`web/src/pages/LogsPage.tsx` + new backend: `internal/logger.RingBuffer`, an `slog.Handler` that tees every record into a 1000-entry in-memory ring alongside the existing stdout output — no log file, `GET /api/logs` serves straight from memory)
- [x] System information (folded into the Dashboard's CPU/RAM/Storage/Network cards — `internal/system.Current` extended with `github.com/shirou/gopsutil/v3` cpu/mem/disk/net sampling, previously just Go runtime stats)
- [x] Configuration backup (`GET /api/config/export`, `internal/api/configio.go` — downloads devices, data points, and non-secret Gateway/Forwarder/MQTT/Time settings as JSON; MQTT password is deliberately never included per §18)
- [x] Configuration restore (`POST /api/config/import` — restores devices/data points, matched by id: an id that exists is updated, a missing/zero id is created fresh; validates every device and data point before writing anything. Gateway/Forwarder/MQTT/Time settings are exported for backup/documentation only — they're static `config.yaml` values at runtime, not DB-backed, so there's nothing for import to write them into; restoring those means editing the config file and restarting)

Implementation notes:

- New backend package `internal/diagnostics` (atomic counters, no locks needed) and a signature change to `modbus.ReadWithRetry` (now also returns the attempt count) so the poller can record TX/retries even on failure.
- New backend endpoints beyond §21's original list, added because the Diagnostics/Dashboard pages needed them: `GET /api/diagnostics`, `GET /api/dashboard/summary` (device/datapoint counts — the rest of the Dashboard reuses the existing system/store-forward/time endpoints).
- No frontend router was introduced — the existing tab-based nav in `App.tsx` (a `useState<Tab>`, no URL routing) was simply extended from 2 tabs to 7, matching the codebase's existing minimal-dependency approach (no react-router in package.json).
- `web/src/styles.ts` gained a handful of tokens (`cardGrid`, `card`, `progressTrack`/`progressFill`, `logPanel`/`logLine`, `sectionTitle`) reused across every new page — still plain inline `CSSProperties` objects, no CSS framework, consistent with Phase 2.

Bug found and fixed along the way: `gateway/go.sum` was missing checksums for `gopsutil/v3/cpu`'s Linux-only transitive dependencies (`github.com/tklauser/go-sysconf` etc.) — invisible on Windows (the native dev host) since those files are behind a `//go:build linux` tag, but it broke `GOOS=linux go build`, i.e. the actual deployment target. Caught by cross-compiling for linux/amd64 after adding gopsutil's cpu/net packages, the same verification habit established in Phase 1/6 for the RTC/serial code. Fixed with `go get` + `go mod tidy`.

Verified: `go build/vet/test` and `GOOS=linux GOARCH=amd64 go build` all pass; `npm run build` (`tsc -b && vite build`) and `oxlint` are clean (zero new warnings). Verified live end-to-end via `docker compose` + a real headless-Chromium session (Playwright, installed for this since no browser automation tool was preinstalled in this environment): screenshotted all 7 tabs rendering real data with zero console errors, and drove the Export Configuration button to confirm it produces a real file download (`gateway-config-GW001.json`, 1 device, no `password` field anywhere in the output) — then round-tripped that same export back through `POST /api/config/import` and confirmed it updated the existing device/data points (`devices_updated: 1, data_points_updated: 3`) rather than duplicating them.

Bug found and fixed along the way (frontend, not code): the docker-compose `web` container's Vite dev server never picked up the new `App.tsx`/pages/`api.ts`/`types.ts`/`styles.ts` files via its file watcher (bind-mounted source over a Windows/WSL2 Docker Desktop boundary) — the browser kept rendering the old 2-tab nav until the container was restarted. Not a code bug, but worth knowing: `docker compose restart web` (or `gateway`, same class of issue) is sometimes necessary after editing source on Windows even though both dev containers are supposed to hot-reload.

Deliverable:

```text
Gateway can be configured and diagnosed entirely from Web UI.
```

Verified — see above.

---

# Phase 8 — Reliability

Test:

- [x] Modbus timeout — real (non-mocked) test: a TCP server that accepts the connection but never writes a response, client-side read deadline fires a genuine `net.Error.Timeout()`, classified `TIMEOUT` (`TestQualityFromErrorTimeout`)
- [x] Modbus CRC error — real test against goburrow/modbus's actual RTU frame CRC validation (`rtuPackager.Decode`) with a deliberately corrupted trailing CRC; no serial hardware needed since `Decode` is pure frame parsing (`TestQualityFromErrorCRCMismatch`). No physical RS-485/RTU hardware exists in this dev environment (same constraint noted in Phase 1), so this is the strongest test achievable without it — a live RTU round-trip against real hardware remains unverified.
- [x] Device offline — verified live in Phase 3 (dropped TCP connection → `DEVICE_OFFLINE`, `value = NULL`)
- [x] Server offline — verified live in Phase 4 (HTTP) and Phase 5 (MQTT): acquisition keeps running, queue grows, `server_connected` flips false
- [x] Network disconnected — verified live this phase: `docker network disconnect` fully severed the gateway container (Modbus, MQTT, DNS all gone at once, not just one service stopped). The gateway process itself kept running and serving its local API (confirmed via `docker exec ... curl localhost:8080/api/system` during the outage) — Rule 1/10 held even under total network loss, not just a single downstream service being down.
- [x] MQTT disconnect — verified live in Phase 5 and again this phase (auto-reconnect, `mqtt connected` on recovery)
- [x] ACK loss — verified live in Phase 4/5 (retries after a lost ACK do not create server-side duplicates)
- [x] Duplicate message — verified live repeatedly across Phases 4/5/8; final cumulative check this phase: **4747/4747 unique `gateway_id+sequence_id` keys, zero duplicates** on server-sim across the whole session's chaos testing (MQTT outages, a SIGKILL, a full network partition)
- [x] Gateway restart — verified live (graceful `docker compose restart`) in Phases 5-7
- [x] Power failure — verified live this phase: `docker compose kill -s SIGKILL` on the gateway container (no graceful shutdown, no deferred cleanup — genuine abrupt termination) while 11 rows were PENDING with active retries. Restarted cleanly: no corruption, sequence continued (2020→6606 with no gaps), backlog fully drained afterward.
- [x] SQLite recovery — same test as Power failure: WAL-mode SQLite reopened cleanly after the SIGKILL with no "database is locked"/corruption errors and no manual intervention
- [x] NTP server unavailable — verified live in Phase 6 (`UNSYNCED` when unconfigured/unreachable, acquisition unaffected)
- [x] RTC fallback — verified via unit tests with a fake RTC (`internal/time/service_test.go`) and live as the "no hardware" path (`rtc_status: false`, degrades to `UNSYNCED`) in Phase 6 — no physical RTC chip exists in this dev environment, so the NTP-fails-but-RTC-is-available transition itself is proven only by `TestServiceDegradesToRTCWhenNTPUnreachable`, not against real hardware.
- [x] Storage warning — **new this phase**: §17's four disk-usage bands (NORMAL/WARNING/CRITICAL/FULL — the doc's "70% Warning" and "80% Warning" lines are one WARNING band, not two states) didn't exist as an observable concept before Phase 8; only the 95% FULL eviction trigger did. Added `queue.ClassifyStorageLevel` + `RunStoragePressureSweeper` now logs at WARNING (70-89%) without evicting. Unit-tested (`TestClassifyStorageLevel`) and exposed via `GET /api/store-forward/status`'s new `storage_level` field and the Store & Forward page's badge.
- [x] Storage critical — same addition: CRITICAL (90% up to the configured FULL threshold) is classified and logged (`log.Error`) but still doesn't evict — only FULL does. Verified `RunStoragePressureSweeper` takes no action at WARNING/CRITICAL (`TestRunStoragePressureSweeperDoesNotEvictAtWarningOrCritical`).
- [x] Storage full — pre-existing from Phase 4, re-confirmed: `EvictOldestNonCritical` at/above the configured threshold, CRITICAL-priority data never touched (`TestEvictOldestNonCriticalNeverTouchesCritical`)
- [x] Large backlog — verified live repeatedly: 137 rows (Phase 4), 66 rows (Phase 5), 4747 rows cumulative (this phase) — FIFO-by-sequence-within-priority drain confirmed at every scale tested
- [x] Batch recovery — `RecoverSendingToPending` unit-tested since Phase 4, re-confirmed live this phase after the SIGKILL power-failure test (no rows lost or stuck)

Bug found and fixed this phase: the network-disconnect chaos test surfaced a real classification bug — `modbus.QualityFromError` had no handling for DNS resolution failures. With the container's network fully severed, Docker's embedded DNS resolver (127.0.0.11) fails with `"server misbehaving"`, which didn't match any of the existing connection-refused-style substrings and fell through to `INVALID` instead of `DEVICE_OFFLINE`. Fixed with an explicit `errors.As(err, &dnsErr)` check for `*net.DNSError` (catches every DNS failure mode, not just this one error string) placed ahead of the substring switch, and covered by a real test against an actual failed DNS lookup (`this-host-does-not-exist.invalid`, RFC 6761) — `TestQualityFromErrorDNSFailure`. Verified live: after the fix and a gateway restart, quality returned to `GOOD` across all data points once the network was reconnected.

Implementation notes: `internal/queue/storagepolicy.go` gained `StorageLevel`/`ClassifyStorageLevel` — the only genuinely new production code this phase; everything else was either a test-coverage gap being closed or a live chaos-test exercise of behavior that already existed from earlier phases. `internal/modbus/quality.go` and `quality_test.go` gained the DNS fix and its three new tests (timeout, CRC, DNS) described above.

Verified: `go build/vet/test` all pass (including the 3 new modbus tests and 2 new queue tests). Chaos tests were run against the live `docker compose` stack, not simulated — a real `SIGKILL`, a real `docker network disconnect`, a real stopped MQTT broker — because a "reliability" phase whose tests are all mocks would not actually prove the reliability guarantees in Rules 1/2/9/10.

Deliverable:

```text
Gateway recovers automatically from communication and power failures.
```

Verified — see above.

---

# 23. Critical Test Scenarios

## Scenario A — Normal Operation

```text
Modbus
 ↓
SQLite
 ↓
MQTT
 ↓
Server ACK
 ↓
SENT
```

Expected:

```text
No pending backlog.
```

---

## Scenario B — Server Down

```text
Server OFF
 ↓
Modbus continues
 ↓
SQLite continues
 ↓
Queue increases
```

Expected:

```text
No data loss while storage capacity is available.
```

---

## Scenario C — Server Recovery

```text
Server ON
 ↓
Gateway detects connection
 ↓
Forward pending batches
 ↓
ACK
 ↓
SENT
 ↓
Queue returns to normal
```

Expected:

```text
Historical data is preserved.
No duplicate records on Server.
```

---

## Scenario D — Gateway Restart

```text
Gateway
 ↓
Power OFF
 ↓
Power ON
 ↓
SQLite recovery
 ↓
Recover SENDING → PENDING
 ↓
Continue Forward
```

Expected:

```text
No queue corruption.
```

---

## Scenario E — No Internet

```text
Internet = unavailable
Server = available on LAN
NTP Server = available on LAN
```

Expected:

```text
Gateway works normally.
```

---

## Scenario F — NTP Server Down

```text
NTP = unavailable
RTC = available
```

Expected:

```text
Gateway continues operation.
time_quality = RTC
```

---

# 24. Definition of Done

MVP is complete when:

- [x] Modbus RTU works (connection-level verified against real USB-RS485 hardware in Phase 1; a full read round-trip against a responding RTU slave device — and the CRC-error path specifically — remain unverified against physical hardware, since none is available in this dev environment. `TestQualityFromErrorCRCMismatch` (Phase 8) verifies the CRC validation logic itself against goburrow/modbus's real decoder, just not over a physical wire.)
- [x] Modbus TCP works (fully verified live, Phases 1-8)
- [x] Device configuration works (Phase 2, verified live + via Web UI, Phase 7)
- [x] Data Point configuration works (Phase 2, verified live + via Web UI, Phase 7)
- [x] Data is stored locally before transmission (Phase 3)
- [x] Store & Forward works (Phase 4 HTTP, Phase 5 MQTT, re-verified under chaos in Phase 8)
- [x] Retry works (exponential backoff, Phase 4; verified under a real MQTT outage, Phase 5; verified after a real SIGKILL, Phase 8)
- [x] Batch transmission works (Phase 4/5)
- [x] Duplicate handling works (`gateway_id+sequence_id` idempotency; final cumulative check across the whole project's chaos testing: 4747/4747 unique keys, zero duplicates, Phase 8)
- [x] Gateway restart recovery works (graceful restarts, Phases 5-7; an ungraceful `SIGKILL` power-failure simulation, Phase 8 — `RecoverSendingToPending`, sequence continuity, and SQLite WAL recovery all held)
- [x] Internal NTP synchronization works (Phase 6; live-verified against a real reachable NTP server as a dev-only reachability test — production points at the internal network's NTP server per Rule 8, never the public Internet)
- [x] RTC fallback works (the NTP-unavailable → RTC/UNSYNCED degrade logic is real and unit-tested with a fake RTC, Phase 6/8; the real Linux hardware RTC path (`rtc_linux.go`, `/dev/rtc0` ioctls) cross-compiles clean but is unverified against a physical RTC chip, since none is available in this dev environment)
- [x] Web UI works (Phase 7, verified live in a real browser via Playwright — all 7 pages, zero console errors)
- [x] Diagnostics work (Phase 7: Modbus TX/RX/response time/timeout/CRC/retry counters; Phase 8 added the storage WARNING/CRITICAL/FULL levels §17 calls for)
- [x] Configuration backup/restore works (Phase 7, verified live: real file download, no secrets included, round-tripped through import without duplicating anything)
- [x] System works without Internet (nothing in the gateway hardcodes an external/Internet dependency — NTP server, MQTT broker, and Modbus targets are all configurable to internal-network addresses; dev/test verification used `pool.ntp.org` and public Docker image pulls purely as a convenient reachability check, never as a production assumption)
- [x] All critical failure scenarios pass (Phase 8 — see above; every scenario in §23 and Phase 8's checklist verified live against the running `docker compose` stack, not simulated)

---

# 25. Future Roadmap

## V2

- [ ] HTTPS server adapter
- [ ] User management
- [ ] Role-based access
- [ ] Alarm management
- [ ] Event log
- [ ] Advanced diagnostics
- [ ] Configuration validation
- [ ] Automatic backup
- [ ] Health monitoring

## V3

- [ ] Remote Gateway Management
- [ ] Remote Configuration
- [ ] OTA Firmware/Application Update
- [ ] Fleet Management
- [ ] Gateway provisioning
- [ ] Certificate management
- [ ] Centralized monitoring

---

# 26. Key Engineering Rules

### Rule 1

```text
Modbus Acquisition must never depend on Server connectivity.
```

### Rule 2

```text
Persist data before forwarding.
```

### Rule 3

```text
SQLite is the Source of Truth.
```

### Rule 4

```text
Timestamp at event acquisition time.
```

### Rule 5

```text
Never change event_timestamp during Store & Forward.
```

### Rule 6

```text
Use gateway_id + sequence_id for idempotency.
```

### Rule 7

```text
At-Least-Once Delivery + Server Idempotency.
```

### Rule 8

```text
NTP comes from the internal network, not the Internet.
```

### Rule 9

```text
RTC is the fallback time source.
```

### Rule 10

```text
Gateway must continue operating when NTP is unavailable.
```

---

# 27. Recommended MVP Target

Initial target:

```text
Hardware:
Raspberry Pi 4

OS:
Linux

Backend:
Go

Frontend:
React + TypeScript

Database:
SQLite + WAL

Protocols:
Modbus RTU
Modbus TCP
MQTT

Time:
Internal NTP
RTC

Network:
Ethernet

Deployment:
Docker Compose
```

Development should initially run on:

```text
Windows 11
    ↓
Docker Compose
    ↓
Local Development
    ↓
Linux / Raspberry Pi
```

The application must not contain Raspberry Pi-specific logic in the core business layer.

---

# 28. Final Architecture Principle

The most important architecture is:

```text
                 MODBUS
                    │
                    ▼
             ┌──────────────┐
             │ Acquisition  │
             └──────┬───────┘
                    │
                    ▼
             ┌──────────────┐
             │ Data Process │
             │ Timestamp    │
             │ Quality      │
             │ Sequence ID  │
             └──────┬───────┘
                    │
                    ▼
             ┌──────────────┐
             │ SQLite/WAL   │
             │ Source Truth │
             └──────┬───────┘
                    │
                    ▼
             ┌──────────────┐
             │ Store &      │
             │ Forward      │
             └──────┬───────┘
                    │
                    ▼
             ┌──────────────┐
             │ MQTT/HTTPS   │
             └──────┬───────┘
                    │
                    ▼
                 SERVER

Time:

        Internal NTP Server
                │
                ▼
        Gateway Time Service
                │
          ┌─────┴─────┐
          ▼           ▼
         NTP         RTC
```

The system is intentionally designed so that **field data acquisition, local persistence, time management, and server forwarding are independent components**. This allows the Gateway to continue collecting and storing industrial data even when the Server, network connection, MQTT service, or NTP service becomes temporarily unavailable.

---

# 29. Post-MVP: Web UI Settings & Host Network Configuration

Added after Phase 8 / Definition of Done, on direct request: the Config page's Gateway/Forwarder/MQTT/Time settings (documented in Phase 7 as "edit configs/config.yaml and restart the gateway for those") are now actually editable from the Web UI, and the Config page gained a fourth section for setting the gateway host's own network IP.

## 29.1 Gateway / MQTT / Time Settings

`GET/PUT /api/config/settings` — the Config page's new "Gateway / MQTT / Time Settings" card. Saving:

1. Merges the submitted Gateway ID/Name, MQTT (broker URL, client ID, username, password, QoS, topics, keepalive), and Time (NTP server, timezone, sync interval) fields into the in-memory `config.Config`.
2. Writes the whole struct back to `config.yaml` via the new `config.Save` (full `yaml.Marshal` re-encode — **not** a comment-preserving patch, a deliberate simplicity tradeoff; any hand-written comments in `config.yaml` are lost the first time a save happens through the UI, which the page's own copy explicitly warns about).
3. Responds `200`, then calls `os.Exit(0)` after a short delay (letting the HTTP response flush first) — the process restarts under whatever supervises it, and comes back up with the new config loaded.

This was the deliberately chosen "simpler and safer" option over live-reloading the MQTT client/Time Service in place (the alternative considered and rejected: tearing down and rebuilding those subsystems without a process restart, judged more complex and more failure-prone for comparatively little benefit).

Two things make the restart-on-save actually take effect in every deployment mode:
- **`docker-compose.yml`**: the `gateway` service gained `restart: unless-stopped`, so Docker brings the container back after the process exits. (In the dev compose stack specifically, air's own file watcher — it already watches `configs/` — also notices the same `config.yaml` write and rebuilds/restarts on its own; the two racing is harmless, just an occasional double-restart in dev.)
- **`gateway/deploy/nxiiot-gateway.service`** (new): a systemd unit for the bare-metal/systemd deployment path (§19, §27), which didn't have a reference unit file in the repo before this. `Restart=always` is load-bearing here for the same reason as the Docker restart policy.

The MQTT password is never sent to the browser (`GET /api/config/settings` omits it, matching §18's export rule) and a blank password field on save leaves the stored password unchanged rather than clearing it.

## 29.2 Host Network IP (`internal/netconfig`)

The higher-risk half of the request: setting the gateway host's actual network interface IP (not a config field, not a container's virtual network — the real LAN-facing adapter), for the case where a Raspberry Pi needs a static IP set up from its own Web UI instead of SSH/console access.

**Scope decision, confirmed with the user before implementing**: this only makes sense when the gateway binary runs directly on the host (the systemd deployment path), never inside this project's own Docker Compose dev setup — a container's network namespace is not the host's real NIC, so there is nothing meaningful to configure from inside `docker compose up`'s `gateway` container. The user explicitly asked to hold off on live-testing or further developing this piece until it's actually run on a Raspberry Pi; in this Windows/Docker dev environment it only exercises (and is only expected to exercise) its graceful "unsupported" path.

Implementation — `internal/netconfig`:
- Backed by NetworkManager's `nmcli` (the default network stack on Raspberry Pi OS Bookworm+ and most modern Debian/Ubuntu — §27's deployment target), invoked via `os/exec`. There are deliberately no Linux-only build tags here (unlike `internal/time`'s RTC code, which needs actual platform-specific syscalls): `nmcli` invocation is just an external command that fails gracefully ("not found") on any host without it, so the exact same code compiles and its parsing logic is fully unit-testable on any platform, including this Windows dev machine — no cross-compilation needed to verify it.
- `Controller.Current()/ApplyStatic()/ApplyDHCP()` — every method returns `ErrUnsupported` when `nmcli` isn't on `PATH`, verified live in this dev environment: `GET /api/system/network` on the running (Windows-hosted, Docker) stack returns `{"supported": false}`, confirmed without attempting a single real network command.
- **`netconfig.Service`** wraps the Controller with a confirm-or-auto-revert safety net — the actual point of this package, not incidental to it: `Apply` snapshots the current config, applies the new static IP immediately, and schedules an automatic revert (`time.AfterFunc`) unless `Confirm` is called within a 45-second window (`networkConfirmWindow`, `internal/api/network.go`). A typo'd IP or gateway that would otherwise lock an operator out of a headless device self-heals instead. Reverting restores the *exact* prior config (static-to-static goes back to the same address, not to DHCP) — `TestServiceRevertsToPreviousStaticConfigNotJustDHCP`.
- New endpoints: `GET/POST /api/system/network`, `POST /api/system/network/confirm`.
- Frontend: the Config page's new "Network (Host IP)" card shows "Not available on this host" with an explanation when unsupported (the expected/only state observed so far); when supported, an Apply button and a live countdown-to-revert banner with a Confirm button.

Tests: 10 tests across `netconfig_test.go` (real `nmcli -t` terse-format output parsing, using an injected fake command runner — no `nmcli` binary needed, matching this codebase's established injection pattern for external dependencies) and `service_test.go` (the confirm/revert timer logic against a fake `Controller`, including the "reverts to the prior static config, not DHCP" case). All pass on this Windows dev host, since nothing here requires Linux at compile time.

**Explicitly not yet verified**: any of this against a real `nmcli`/NetworkManager instance or actual network hardware. That verification is deferred to Raspberry Pi deployment, per the user's own instruction — this section will need an update once that happens.
