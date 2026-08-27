# Handoff — nxIIoT Gateway (MVP complete, Phases 0-8 + post-MVP Settings/Network IP + Internal Server)

Status as of this handoff. For the full design spec and per-phase task checklists (with implementation notes inline), see [industrial_iot_gateway_handoff_dev_plan.md](industrial_iot_gateway_handoff_dev_plan.md) — §24 for the Definition of Done, §29 for the post-MVP Settings/Network IP addition, §30 for the Internal Server. For day-to-day dev commands, see [README.md](README.md). For the Raspberry Pi deployment history (three sessions so far) see [DEPLOY_PLAN.md](DEPLOY_PLAN.md). This document is the orientation layer: what's true right now, what to watch out for, and where to pick up.

## Git state

Everything through the Internal Server addition and the current Raspberry Pi deployment is committed and pushed to `origin/master` (https://github.com/bpp-pico/nxIIoT-Gateway.git) — working tree is clean on the Windows dev machine as of this handoff. Run `git log --oneline` for the authoritative, current history rather than trusting a commit list copied into this doc (it goes stale fast — this project is now on its third Pi deployment session and its own separate `internal-server/` service). Git committer identity: `bpp-pico <banpot@pico.co.th>`.

## What's implemented

| Phase | What | Key packages |
|---|---|---|
| 0 | Go backend + React/TS frontend scaffold, SQLite+WAL, Docker Compose dev stack | `gateway/cmd/gateway`, `web/` |
| 1 | Modbus RTU/TCP master, data type/byte-order decode, quality classification, polling scheduler | `internal/modbus`, `internal/acquisition/poller.go` |
| 2 | Device/Data Point CRUD API + Web UI, live config reload (no restart), Test Connection/Read, COM port dropdown | `internal/api`, `internal/acquisition/manager.go`, `web/src/pages`, `internal/system/serial.go` |
| 3 | Every Reading persisted to `data_queue` (not just logged), per-gateway sequence ID, quality/priority, retention sweep | `internal/queue` (queue.go, retention.go), `internal/processor` |
| 4 | Queue state machine (PENDING→SENDING→SENT/retry), exponential backoff, batching, crash recovery, storage threshold/full policy | `internal/queue` (dispatch.go, backoff.go, storagepolicy.go), `internal/forwarder` |
| 5 | `MQTTAdapter` (QoS 1, application-level ack over a dedicated ack topic, TLS/auth, auto-reconnect) as a second `forwarder.Adapter`, selected via `forwarder.transport` | `internal/forwarder` (mqttadapter.go, wire.go), `cmd/server-sim` (MQTT consumer) |
| 6 | Time Service: hand-rolled SNTP client, Linux hardware RTC (`/dev/rtc0` ioctls) with cross-platform fallback, SYNCED/RTC/UNSYNCED/INVALID quality | `internal/time` (package `timeservice`) |
| 7 | Web UI: Dashboard (CPU/RAM/storage/network/queue/time widgets), Store & Forward/Time/Diagnostics/Logs/Config pages; backend diagnostics counters, in-memory log ring buffer, config export/import | `internal/diagnostics`, `internal/logger/ringbuffer.go`, `internal/api` (configio.go, dashboard.go, diagnostics.go, logs.go), `web/src/pages` |
| 8 | Storage WARNING/CRITICAL/FULL levels (§17); chaos-tested SIGKILL power failure and full network partition, which surfaced and fixed a DNS-classification bug | `internal/queue/storagepolicy.go`, `internal/modbus/quality.go` |
| post-MVP | Web UI Settings (Gateway/MQTT/Time → `config.yaml` + auto-restart), host network IP config (`nmcli`-backed, confirm-or-auto-revert safety net) | `internal/netconfig`, `internal/api` (settings.go, network.go) |
| post-MVP | **Internal Server** (§30): the real, always-on MQTT consumer `cmd/server-sim` was always a stand-in for — subscribes `gateway/+/data`, dedupes into Postgres on `gateway_id`+`sequence_id`, acks back, serves a live dashboard + JSON API | `internal-server/` (separate Go module, own Docker Compose stack — not part of `gateway/`) |

Three dev-only simulators exist purely to make the above testable without physical hardware or a real server — never deployed to production:
- `cmd/modbus-sim` — fake Modbus TCP slave (Phase 1)
- `cmd/server-sim` — fake "Internal Server" that dedupes on `gateway_id`+`sequence_id`, speaking both HTTP and MQTT (Phase 4/5) — now that `internal-server/` exists as the real thing, `server-sim`'s only remaining use is as a disposable stand-in to drain a backlog during testing (see DEPLOY_PLAN.md's Session 3 log)
- `mosquitto` (docker-compose service, not Go code) — dev-only MQTT broker, anonymous/unencrypted (Phase 5) — the real deployment points at `mqtt.nxge.co` instead, both from the Pi gateway and from `internal-server/`

## Architecture decisions worth knowing before touching this code

- **Acquisition never depends on the server.** `acquisition.Manager` and `forwarder.Forwarder` are independent goroutines that only share the database. A dead MQTT broker/server should never slow or block Modbus polling — this is Rule 1 in the design doc and is load-bearing for several other decisions below. Verified under a full `docker network disconnect` (Phase 8), not just a stopped downstream container.
- **`acquisition.Manager` diffs and hot-reloads.** Every device/data point CRUD call triggers `Manager.Reload(ctx)`, which diffs the desired device set against what's currently running and starts/stops/restarts only what changed. This is why the Web UI doesn't need a gateway restart — **except** the new Settings save (§29), which restarts the whole process deliberately, because it's changing MQTT/Time subsystem config, not device polling config.
- **Sequence IDs are assigned atomically with the insert.** `queue.Repository.Insert` does `UPDATE gateway SET last_sequence = last_sequence + 1 ... RETURNING last_sequence` inside the same transaction as the `data_queue` INSERT. Survived a real `SIGKILL` mid-write with no gaps or resets (Phase 8).
- **The forwarder's transport is behind an `Adapter` interface** (`internal/forwarder/adapter.go`). `HTTPAdapter` is the dev/test transport; `MQTTAdapter` (Phase 5) is the production one. Swapping is a one-line change in `cmd/gateway/adapter.go` driven by `forwarder.transport` in config.
- **`internal/queue` owns all `data_queue` SQL.** `internal/processor` and `internal/forwarder` call into it rather than querying the table directly.
- **Every long-running loop is a bare goroutine over the same top-level `context.Context`**, cancelled on SIGINT/SIGTERM: retention sweeper, storage pressure sweeper, `forwarder.Run`, `timeservice.Run`. `main.go` is the single place that wires all of them.
- **Platform-specific code is isolated behind a package-level interface, not build tags, when the underlying call is just `os/exec` or similarly portable** (`internal/netconfig`'s `nmcli` invocation) — build tags (`internal/time/rtc_linux.go` vs `rtc_other.go`) are reserved for genuinely platform-specific syscalls (`golang.org/x/sys/unix` ioctls) that literally won't compile elsewhere. This distinction mattered in practice: `netconfig` needed no build tags at all, so its real parsing logic is unit-tested directly on this Windows dev host; the RTC ioctl code needed them, so it's only verified via `GOOS=linux` cross-compilation, never actually run here.
- **Config the Web UI can write back (§29) is a full `yaml.Marshal` re-encode, not a comment-preserving patch.** Deliberate simplicity tradeoff — `config.Save` loses any hand-written comments in `config.yaml` the first time it's used. Documented in-app (the Settings card's own copy) and in the design doc.
- **A settings change that needs a process restart calls `os.Exit(0)` and relies on the process supervisor to restart it** (`docker-compose.yml`'s `restart: unless-stopped`, or `gateway/deploy/nxiiot-gateway.service`'s `Restart=always`) — rather than the app trying to re-exec or restart its own subsystems in place. Keeps the restart path identical to a crash-restart, which is already tested.

## Bugs found via live testing (not caught by unit tests alone)

Recurring lesson for this codebase: **unit tests run against fresh temp state and (mostly) on this Windows host; real bugs have consistently shown up only under live `docker compose` testing with real accumulated state, real network conditions, or a genuinely killed process.**

1. **`modbus.QualityFromError`** (Phase 3) originally matched `"connection refused"` (Linux wording) but not Windows's `"actively refused it"` for the identical condition — only visible testing the native Windows build against a real refused connection. Fixed with explicit per-platform substring matching; regression test dials a real closed OS socket.
2. **Migration `0003_data_queue_retry.sql`** (Phase 3) used a non-constant `DEFAULT (strftime(...))` on `ALTER TABLE ADD COLUMN`, which SQLite only rejects on a table that already has rows — every test's fresh temp DB has zero rows, so this only surfaced against the native gateway's real, populated database. Fixed with a constant epoch default.
3. **`modbus.QualityFromError`** again (Phase 8): a real `docker network disconnect` chaos test made Docker's embedded DNS resolver fail with `"server misbehaving"`, which matched none of the existing substring checks and fell through to `INVALID` instead of `DEVICE_OFFLINE`. Fixed with an explicit `errors.As(err, &dnsErr)` check for `*net.DNSError` (catches every DNS failure mode, not just this string) — the fix was found *because* the chaos test used a real total network partition, not a single stopped service.
4. **Docker-on-Windows file-watcher misses** (Phases 7 and 9/post-MVP, recurring): the `web` and `gateway` dev containers' hot-reload (Vite / air) has repeatedly failed to notice source changes written from outside the container (this environment's Windows host editing files bind-mounted into Linux containers). Not a code bug, but real enough to have caused actual confusion mid-session (a user-reported "HTTP 502" that was actually a stale DNS registration from an earlier manual `docker network disconnect`/`connect`, not a code issue). `docker compose restart <service>` is the reliable fix; `docker compose up -d --force-recreate <service>` if a restart alone doesn't clear it.

**Takeaway for future work on this repo**: after unit tests pass, do a live `docker compose` run — and for anything touching failure/recovery paths specifically, an actual chaos test (`docker network disconnect`, `docker compose kill -s SIGKILL`, stopping a real dependency) — before considering a change done. This has found real, distinct bugs four times now, not hypothetically.

## How to run

**Docker (default path):**
```bash
docker compose up --build
```
Frontend: http://localhost:5173 · API: http://localhost:8080 · modbus-sim: host port 1502 · server-sim: host port 9000 · mosquitto: host port 1883.

**Native Windows (only needed for RTU/COM-port testing — see README's "Testing RTU against real hardware" section):**
```powershell
$env:Path += ";C:\Program Files\Go\bin"   # go.exe is installed but not on PATH by default in this environment
go build -o gateway.exe ./cmd/gateway
```
Stop the Dockerized `gateway`/`web` containers first to free ports 8080/5173.

## Known gaps / untested paths (be honest about these, don't assume they work)

- **RTU**: full read round-trip verified for real (Raspberry Pi deployment, Session 2, 2026-08-25) — a CH340 USB-RS485 adapter plus a real RTU temp/humidity sensor (slave ID 1, function code 04, registers 1/2, `INT16`×0.1). **Session 3 (2026-08-26) found and documented a real hardware limit**: this specific sensor returns `quality: "GOOD"` at 500ms and 250ms polling intervals, but **times out completely** (`quality: "TIMEOUT"` on every read) at 200ms and 100ms — the sensor's own internal response latency, not a gateway bug. Currently deployed at 250ms (device 1 + both data points). The exact breaking point between 200-250ms was not pinned down further. **Still open**: the real-hardware CRC-error/`DEVICE_OFFLINE` failure paths (unplug mid-poll, forced bad response) — not yet attempted. The CRC *classification logic itself* was already tested for real (`TestQualityFromErrorCRCMismatch`, against goburrow/modbus's actual frame decoder), just not triggered from live hardware yet.
- **RTC hardware**: `internal/time/rtc_linux.go`'s `/dev/rtc0` ioctls cross-compile clean (`GOOS=linux GOARCH=amd64 go build`) but have never run against a physical RTC chip — no such hardware in this dev environment. The NTP-fails/RTC-available fallback transition is proven only via a fake RTC in unit tests.
- **Host network IP (`internal/netconfig`)**: `Current()` is now confirmed live against a real Pi's `nmcli`/NetworkManager (Session 1, re-confirmed on a fresh SD card in Session 2 — see `DEPLOY_PLAN.md`'s session logs), correctly reporting the real `wlan0` interface/IP/gateway/DNS. `ApplyStatic`/the confirm-or-auto-revert flow is still unverified — deliberately deferred until physical console access is arranged, since a bad static-IP apply could lock out the only remote path to the device.
- **MQTT reconnect can silently stop delivering acks without ever reporting "connection lost."** Session 3 (2026-08-26) found the Pi's gateway stuck for 4.5 hours with `server_connected: false` and every batch timing out waiting for an ack, despite paho never logging a disconnect — `onConnect`'s ack-topic re-subscribe (`gateway/internal/forwarder/mqttadapter.go`) could lose a race with a fast reconnect and, on failure, only logged and gave up (no retry). **Fixed in Session 4 (2026-08-27)**: ported `internal-server/consumer.go`'s `subscribeWithRetry` pattern (3 attempts, linear backoff) back into `mqttadapter.go`'s `onConnect`, built and deployed live to the Pi, verified with zero regressions. (Root cause of the original 4.5h incident was actually something else — no real Internal Server was running at all — but the missing subscribe-retry was a real, now-closed gap regardless.)
- **A Pi's LAN IP is not a stable fact.** Both `eth0` and `wlan0` are DHCP-assigned with no reservation — `wlan0` (`192.168.99.84`) dropped off Wi-Fi entirely mid-Session-4, and the Pi kept running unaffected, reachable the whole time via `eth0` (`192.168.99.93`) and Tailscale (`100.84.193.68`, the one address guaranteed not to change with LAN topology). Always check spec.md's "Live access points" table for the current address rather than assuming a previously-known IP still works.
- **`word_order`** is a stored-but-unused datapoint field — `byte_order` alone (e.g. `"ABCD"`/`"BADC"`) already fully specifies both byte and word order for 32/64-bit types. Documented in `internal/modbus/decode.go`.
- **Config save (§29) is a full-file re-marshal.** Editing `config.yaml` by hand and then saving once from the Web UI will silently discard those hand-written comments/formatting. Not a bug, but a footgun worth remembering.

## Next

MVP (Phases 0-8), the post-MVP Settings/Network IP addition, and the Internal Server (§30) are all done, per the honest caveats above. Session 4 (2026-08-27) closed two more open items — `internal-server/` now runs on a real always-on host (`192.168.99.200`, not the Windows dev laptop), and the MQTT ack-resubscribe retry gap is fixed and deployed live. There is no pending "Phase 9" in the design doc — remaining work is either:
- **V2/V3 roadmap** items from §25 of the design doc (HTTPS adapter, user management, alarm management, fleet management, OTA updates, ...), or
- **Closing the remaining RPi-only gaps**: a real RTC chip (never connected in any session so far) and the `netconfig.ApplyStatic`/confirm-revert flow (deliberately not attempted without physical console access — see `DEPLOY_PLAN.md` §0). RTU round-trip and real-broker MQTT are both closed as of Session 2/3 — see `DEPLOY_PLAN.md`'s session logs for details.
- **Rotating `internal-server/`'s dev-shaped Postgres credentials** (`internal_server`/`internal_server`) — lower urgency since Session 4's move, since Postgres has no host port mapping and isn't reachable from the vendor server's LAN, but still open before calling this fully production-ready.
- **Migrating historical readings** from before the Session 4 host move, if ever needed — they're sitting untouched in the old laptop's `internal-server_internal-server-db` Docker volume (stopped, not deleted), not on the new host.

Whoever picks this up next should decide which of those is the priority rather than assume — nothing in the current codebase blocks any of these paths. **Read `spec.md` first** — it's the actively-maintained save point (see `CLAUDE.md`'s SPEC-DRIVEN rule) and will be more current than this document on any given day; this file is the deeper orientation layer for how the codebase is structured, not the first thing to check for "what changed most recently."
