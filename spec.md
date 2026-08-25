# nxIIoT Gateway — industrial IoT edge gateway with a real MQTT sink

Modbus RTU/TCP acquisition → local SQLite store & forward → MQTT delivery to a real Internal Server, running live on a Raspberry Pi. For the deep architecture rationale and per-phase history see [HANDOFF.md](HANDOFF.md) and [industrial_iot_gateway_handoff_dev_plan.md](industrial_iot_gateway_handoff_dev_plan.md); for the Pi deployment blow-by-blow see [DEPLOY_PLAN.md](DEPLOY_PLAN.md).

## Architecture

- **`gateway/`** (Go) — the edge device binary. `internal/acquisition` polls Modbus RTU/TCP devices on independent per-device goroutines; `internal/queue` persists every reading to SQLite (WAL) as `data_queue` with an atomic per-gateway `sequence_id`; `internal/forwarder` ticks on `forwarder.poll_interval_ms` and dispatches batches through an `Adapter` interface — `MQTTAdapter` in production (QoS 1 + application-level ack over a dedicated ack topic), `HTTPAdapter` for dev/test. Acquisition and forwarding never block each other (Rule 1) — a dead broker just grows the local queue.
- **`web/`** (React + TS + Vite) — the gateway's own local admin UI: Dashboard, Devices, Store & Forward, Time, Diagnostics, Logs, Config. Talks to the gateway's REST API only.
- **`internal-server/`** (Go, separate module) — the real downstream consumer. Subscribes `gateway/+/data` on the MQTT broker, dedupes into Postgres on `gateway_id`+`sequence_id`, acks back on `gateway/{id}/ack`, and serves its own dashboard (`/dashboard`) + JSON API (`/health`, `/latest`, `/readings`, `/stats`). Runs via its own `docker compose up -d` in `internal-server/`.
- **Deployment target**: a real Raspberry Pi (`NXGE-RPI`, Debian 13 trixie, aarch64), native/systemd (not Docker — needed for `/dev/rtc0` and real `nmcli`), reachable over Tailscale. `gateway/deploy/nxiiot-gateway.service` (`Restart=always`) is the reference unit.
- **Real MQTT broker**: `mqtt.nxge.co:1883` (plain TCP, no auth/TLS yet) — both the Pi's gateway and `internal-server/` point at this, not the dev-only `mosquitto` service in the repo root's `docker-compose.yml`.

## Done

- MVP Phases 0-8 (Modbus engine, device/datapoint CRUD, Store & Forward with crash recovery, MQTT adapter, Time Service, full Web UI, chaos-tested reliability) — see HANDOFF.md for per-phase detail.
- Post-MVP: Web UI Settings (Gateway/MQTT/Time → `config.yaml` + auto-restart) and host network IP config (`internal/netconfig`, `nmcli`-backed, confirm-or-auto-revert safety net).
- **Real Raspberry Pi deployment, three sessions**: native systemd install, real CH340 USB-RS485 adapter + RTU temp/humidity sensor delivering `GOOD` readings, real MQTT broker round-trip (including a real network-partition chaos test), Settings-save-restart verified under `systemctl`.
- **Internal Server** (`internal-server/`) — built because `cmd/server-sim` was explicitly dev-only and nothing was consuming the Pi's real MQTT traffic. Chose Postgres over reusing the gateway's SQLite pattern because this is meant to serve multiple readers/dashboards later, not just one writer. Chose a `go:embed`'d single-file dashboard over a separate frontend build — this is a small ops tool, not a second product surface.
- **RTU polling-interval tuning** — this specific sensor holds `GOOD` down to 250ms (20x the original 5000ms default) but times out completely below that. Deployed at 250ms.
- **Logs page alignment fix** — the message column drifted per line because a variable-width level label (`DEBUG` vs `INFO`) sat inline before it; fixed with a CSS grid layout.
- **CLAUDE.md / MEMORY.md / spec.md scaffold** (this file and its siblings) — added 2026-08-26 per the `claude-md-setup` skill, installed globally at `~/.claude/skills/claude-md-setup`.

## Todo / Out of scope

- **RTC hardware**: `internal/time/rtc_linux.go`'s `/dev/rtc0` ioctls cross-compile clean but have never run against a physical RTC chip — none connected in any session so far. Out of scope until hardware is available.
- **`netconfig.ApplyStatic`/confirm-revert flow**: `Current()` is verified live; the actual apply-and-auto-revert-on-timeout path is deliberately deferred until physical console access is arranged (a bad apply could lock out the only remote path to the Pi).
- **RTU failure paths**: unplug-mid-poll (`DEVICE_OFFLINE`) and forced-bad-response (`CRC_ERROR`) haven't been triggered from live hardware yet — deliberately deferred, not forgotten.
- **`internal-server/` isn't on an always-on host** — currently runs via Docker on the Windows dev laptop. If that stops, the Pi's backlog just grows again harmlessly (Rule 1) until it's restarted. Moving it to a real always-on host (ideally alongside the `mqtt.nxge.co` broker) is open.
- **`internal-server/`'s Postgres credentials are dev-shaped defaults** (`internal_server`/`internal_server`) — rotate before calling this production-ready.
- **MQTT ack-topic re-subscribe has no retry on failure** (`gateway/internal/forwarder/mqttadapter.go`'s `onConnect`) — real log evidence of one failure with no follow-up attempt exists. Not yet the proven cause of an incident, but a real gap; `internal-server/consumer.go`'s `subscribeWithRetry` is the pattern to port back if it ever is.
- **Out of scope for now**: V2/V3 roadmap items (§25 of the design doc) — HTTPS adapter, user management, alarm management, fleet management, OTA updates. Nothing currently blocks starting any of them; just not prioritized yet.
- **Layer 4 of the `claude-md-setup` skill** (a "your brain" personalization section — work patterns / voice / skill triggers extracted from a chat-history-rich session) was not run — it requires prompting a separate long-lived chat the user has used a lot, which isn't something this session can do on its own. Left out of CLAUDE.md deliberately rather than faked.

## Current state

Everything above is committed and pushed to `origin/master`. The Pi is live and stable: gateway `active (running)` under systemd, RTU sensor reading `GOOD` at 250ms, MQTT delivering to `internal-server/` (which is currently running via Docker on the Windows dev machine, not an always-on host). No task is mid-flight. Next concrete step is whoever's picking this up choosing one item from Todo above — RTC hardware, the netconfig apply/revert test, or giving `internal-server/` a permanent home are the three most load-bearing open items.

## Data Contracts

Interfaces between components — flag before changing any of these, since gateway, `internal-server/`, and (for the MQTT ones) the broker's other future consumers all depend on them matching exactly.

- **MQTT data topic** (gateway → broker): `gateway/{gateway_id}/data`, QoS 1, JSON:
  `{ "batch_id": string, "entries": [{ "gateway_id": string, "sequence_id": int64, "device_id": int64, "datapoint_id": int64, "value": float64|null, "quality": "GOOD"|"CRC_ERROR"|"DEVICE_OFFLINE"|"INVALID"|"TIMEOUT", "event_timestamp": RFC3339, "priority": string }] }`
  (`gateway/internal/forwarder/wire.go`'s `WireEntry`, mirrored in `internal-server/consumer.go`'s `wireEntry`.)
- **MQTT ack topic** (consumer → gateway): `gateway/{gateway_id}/ack`, QoS 1, JSON: `{ "batch_id": string, "error": string (optional) }`. Any consumer must derive this from the data topic it received (`strings.TrimSuffix(topic, "/data") + "/ack"`), not hardcode a gateway ID — `internal-server/` subscribes with a `gateway/+/data` wildcard specifically so this works across a fleet.
- **Gateway REST API → Web UI**: devices (`GET/POST/PUT/DELETE /api/devices`) and datapoints (`GET/POST/PUT/DELETE /api/datapoints`) both carry their own `polling_interval_ms` field — they are independent gates (see MEMORY.md's entry on this), not a device-level override of a datapoint default.
- **`internal-server/` JSON API**: `GET /readings` and `GET /latest` return the same row shape (`gateway_id`, `sequence_id`, `device_id`, `datapoint_id`, `value`, `quality`, `event_timestamp`, `priority`, `received_at`) — `/latest` is one row per `(gateway_id, device_id, datapoint_id)` via `DISTINCT ON`, `/readings` is the full filtered history.
