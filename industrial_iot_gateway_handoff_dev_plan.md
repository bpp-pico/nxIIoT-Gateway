# Industrial IoT Gateway
## Handoff & Development Plan

**Document Version:** 1.0  
**Status:** Development Baseline  
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

- [ ] Queue state machine
- [ ] PENDING
- [ ] SENDING
- [ ] SENT
- [ ] FAILED
- [ ] Retry
- [ ] Exponential backoff
- [ ] Batch processing
- [ ] Queue recovery
- [ ] Server connectivity detection
- [ ] Storage threshold
- [ ] Storage full policy
- [ ] Historical data forwarding
- [ ] Current data priority

Deliverable:

```text
Server can be disconnected without stopping data acquisition.
```

---

# Phase 5 — MQTT

Tasks:

- [ ] MQTT client
- [ ] Connect
- [ ] Reconnect
- [ ] Authentication
- [ ] TLS
- [ ] QoS 1
- [ ] Batch publishing
- [ ] Application-level ACK
- [ ] Duplicate handling
- [ ] Server status

Deliverable:

```text
Gateway reliably forwards data to MQTT Server.
```

---

# Phase 6 — Time Service

Tasks:

- [ ] RTC support
- [ ] System clock initialization
- [ ] NTP client
- [ ] Internal NTP configuration
- [ ] UTC storage
- [ ] Timezone
- [ ] Time quality
- [ ] Clock offset
- [ ] Last synchronization
- [ ] RTC fallback

Deliverable:

```text
Gateway maintains valid timestamps without Internet.
```

---

# Phase 7 — Web UI

Tasks:

- [ ] Dashboard
- [ ] Device management
- [ ] Data Point management
- [ ] Modbus test
- [ ] Store & Forward page
- [ ] Time page
- [ ] Diagnostics
- [ ] Logs
- [ ] System information
- [ ] Configuration backup
- [ ] Configuration restore

Deliverable:

```text
Gateway can be configured and diagnosed entirely from Web UI.
```

---

# Phase 8 — Reliability

Test:

- [ ] Modbus timeout
- [ ] Modbus CRC error
- [ ] Device offline
- [ ] Server offline
- [ ] Network disconnected
- [ ] MQTT disconnect
- [ ] ACK loss
- [ ] Duplicate message
- [ ] Gateway restart
- [ ] Power failure
- [ ] SQLite recovery
- [ ] NTP server unavailable
- [ ] RTC fallback
- [ ] Storage warning
- [ ] Storage critical
- [ ] Storage full
- [ ] Large backlog
- [ ] Batch recovery

Deliverable:

```text
Gateway recovers automatically from communication and power failures.
```

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

- [ ] Modbus RTU works
- [ ] Modbus TCP works
- [ ] Device configuration works
- [ ] Data Point configuration works
- [ ] Data is stored locally before transmission
- [ ] Store & Forward works
- [ ] Retry works
- [ ] Batch transmission works
- [ ] Duplicate handling works
- [ ] Gateway restart recovery works
- [ ] Internal NTP synchronization works
- [ ] RTC fallback works
- [ ] Web UI works
- [ ] Diagnostics work
- [ ] Configuration backup/restore works
- [ ] System works without Internet
- [ ] All critical failure scenarios pass

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
