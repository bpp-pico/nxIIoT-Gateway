# Handoff — nxIIoT Gateway (Phases 0-4 complete)

Status as of this handoff. For the full design spec and per-phase task checklists (with implementation notes inline), see [industrial_iot_gateway_handoff_dev_plan.md](industrial_iot_gateway_handoff_dev_plan.md). For day-to-day dev commands, see [README.md](README.md). This document is the orientation layer: what's true right now, what to watch out for, and where to pick up.

## Git state

- **Committed** (`01a0614`): Phases 0-3 — project setup, Modbus engine, device management, persistent storage.
- **Uncommitted**: Phase 4 — Store & Forward (queue state machine, forwarder, storage policy, `cmd/server-sim`). Run `git status` and commit when ready; nothing destructive is pending.
- Git committer identity was auto-detected from the OS account (`Banpot BPP <banpot@pico.co.th>`), never explicitly configured. Set it if that's wrong.

## What's implemented

| Phase | What | Key packages |
|---|---|---|
| 0 | Go backend + React/TS frontend scaffold, SQLite+WAL, Docker Compose dev stack | `gateway/cmd/gateway`, `web/` |
| 1 | Modbus RTU/TCP master, data type/byte-order decode, quality classification, polling scheduler | `internal/modbus`, `internal/acquisition/poller.go` |
| 2 | Device/Data Point CRUD API + Web UI, live config reload (no restart), Test Connection/Read, COM port dropdown | `internal/api`, `internal/acquisition/manager.go`, `web/src/pages`, `internal/system/serial.go` |
| 3 | Every Reading persisted to `data_queue` (not just logged), per-gateway sequence ID, quality/priority, retention sweep | `internal/queue` (queue.go, retention.go), `internal/processor` |
| 4 | Queue state machine (PENDING→SENDING→SENT/retry), exponential backoff, batching, crash recovery, storage threshold/full policy | `internal/queue` (dispatch.go, backoff.go, storagepolicy.go), `internal/forwarder` |

Two dev-only simulators exist purely to make the above testable without physical hardware or a real server — never deployed to production:
- `cmd/modbus-sim` — fake Modbus TCP slave (Phase 1)
- `cmd/server-sim` — fake "Internal Server" that dedupes on `gateway_id`+`sequence_id` (Phase 4)

## Architecture decisions worth knowing before touching this code

- **Acquisition never depends on the server.** `acquisition.Manager` and `forwarder.Forwarder` are independent goroutines that only share the database. A dead MQTT broker/server should never slow or block Modbus polling — this is Rule 1 in the design doc and is load-bearing for several other decisions below.
- **`acquisition.Manager` diffs and hot-reloads.** Every device/data point CRUD call triggers `Manager.Reload(ctx)`, which diffs the desired device set against what's currently running and starts/stops/restarts only what changed. This is why the Web UI doesn't need a gateway restart.
- **Sequence IDs are assigned atomically with the insert.** `queue.Repository.Insert` does `UPDATE gateway SET last_sequence = last_sequence + 1 ... RETURNING last_sequence` inside the same transaction as the `data_queue` INSERT. Confirmed via `go.bug.st`-style empirical testing that SQLite's `RETURNING` works fine with `modernc.org/sqlite`.
- **The forwarder's transport is behind an `Adapter` interface** (`internal/forwarder/adapter.go`) specifically so Phase 5's MQTT adapter can be added without touching the state machine. `HTTPAdapter` is the dev/test stand-in, not a production HTTPS adapter (that's V2 roadmap, out of scope).
- **`internal/queue` owns all `data_queue` SQL.** `internal/processor` and `internal/forwarder` call into it rather than querying the table directly — keeps the schema/SQL in one place.
- **Backoff and retention/eviction sweepers are all `func(ctx) { ...; select { case <-ctx.Done(): return; case <-ticker.C: } }` loops** started as bare goroutines in `main.go`, tied to the same top-level `context.Context` that's cancelled on SIGINT/SIGTERM.

## Bugs found via live testing (not caught by unit tests alone)

Both of these are worth remembering as a general lesson for this codebase: **unit tests here run against fresh, empty temp databases, and against Linux inside Docker. Real bugs showed up only when testing against a database with real accumulated state, and/or on native Windows.**

1. **`modbus.QualityFromError`** originally matched `"connection refused"` (Linux wording) but not Windows's `"actively refused it"` wording for the identical condition, so a real dropped TCP connection misclassified as `INVALID` instead of `DEVICE_OFFLINE` — only visible testing the native Windows build against a real refused connection. Fixed with explicit per-platform substring matching (a `syscall.ECONNREFUSED` errno check was tried first and does **not** actually match on Windows — confirmed empirically, not assumed). Regression test dials a real closed OS socket rather than mocking an error string.
2. **Migration `0003_data_queue_retry.sql`** originally used `DEFAULT (strftime(...))` for a new column. SQLite's `ALTER TABLE ADD COLUMN` rejects a non-constant default on a table that already has rows (would need to backfill each one) — but a table with zero rows doesn't trigger the restriction, which is exactly the situation every test's fresh temp DB is in. Only surfaced against the native gateway's real database carrying rows from earlier phases. Fixed with a constant epoch default (functionally equivalent: "in the past" just means "immediately eligible").

**Takeaway for future work on this repo**: after unit tests pass, do at least one live run against the native Windows build with an already-populated database before considering a change done, especially anything touching migrations or OS-level error handling.

## How to run

**Docker (default path):**
```bash
docker compose up --build
```
Frontend: http://localhost:5173 · API: http://localhost:8080 · modbus-sim: host port 1502 · server-sim: host port 9000.

**Native Windows (only needed for RTU/COM-port testing — see README's "Testing RTU against real hardware" section):**
Go is installed portably at `C:\Users\banpot\go-sdk` (no admin rights used; a normal `winget`/MSI install would need them). Set `GOROOT`/`PATH` to that before `go build`/`go run`. Stop the Dockerized `gateway`/`web` containers first to free ports 8080/5173.

## Known gaps / untested paths (be honest about these, don't assume they work)

- **RTU**: only the serial *connect* was verified against a real USB-to-RS485 (CH340) adapter. No responding RTU slave device was available, so a full read round-trip (function code → response → decode) has never been exercised against real hardware.
- **Frontend**: type-checks (`tsc -b`), builds (`vite build`), and every module transforms cleanly through the Vite dev server — but nobody has clicked through the UI in an actual browser in this environment. Treat "the API contract is correct" as verified; treat "the UX actually works as designed" as unverified.
- **`word_order`** is a stored-but-unused datapoint field — `byte_order` alone (e.g. `"ABCD"`/`"BADC"`) already fully specifies both byte and word order for 32/64-bit types. Documented in `internal/modbus/decode.go`.
- **API stubs still returning `501`**: `GET /api/logs`, `GET /api/config/export`, `POST /api/config/import` — not yet scoped to any phase's task list explicitly beyond Phase 7 (Web UI: "Logs", "Configuration backup", "Configuration restore").
- **MQTT**: not implemented. `MQTTConfig` exists in `internal/config` but nothing reads it yet.

## Next: Phase 5 — MQTT

Per the plan doc: MQTT client with connect/reconnect/auth/TLS/QoS 1/batch publishing/application-level ACK, replacing (or running alongside) `HTTPAdapter` as a second `forwarder.Adapter` implementation. The state machine, batching, retry/backoff, and `Adapter` interface are already built and tested — Phase 5 should be mostly a new `internal/mqtt` package plus an `MQTTAdapter` implementing the existing `forwarder.Adapter` interface, without needing to touch `internal/forwarder/forwarder.go` itself.
