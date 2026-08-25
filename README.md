# nxIIoT Gateway

Industrial IoT Gateway for Modbus RTU/TCP acquisition, local store & forward, and MQTT delivery to an internal server. See [industrial_iot_gateway_handoff_dev_plan.md](industrial_iot_gateway_handoff_dev_plan.md) for the full design and phased development plan, and [HANDOFF.md](HANDOFF.md) for the current orientation snapshot (architecture decisions, known gaps, what's next). The real "internal server" the gateway forwards to lives in [internal-server/](internal-server/) (its own Go module and Docker Compose stack, separate from `gateway/`) — see its section below.

## Stack

- Backend: Go (`gateway/`)
- Frontend: React + TypeScript, Vite (`web/`)
- Database: SQLite (WAL mode)
- Protocols: Modbus RTU/TCP, MQTT (QoS 1 + application-level ack)
- Time: hand-rolled SNTP client, Linux hardware RTC support

## Development

Requires Docker.

```bash
docker compose up --build
```

- Backend API: http://localhost:8080/api/system
- Frontend: http://localhost:5173
- Modbus simulator: TCP port 1502 on the host (`502` inside the Docker network)
- Fake "Internal Server" (Store & Forward target): TCP port 9000 on the host, speaks both HTTP (`/ingest`) and MQTT
- MQTT broker (mosquitto, dev-only): TCP port 1883 on the host

Backend hot-reloads via [air](https://github.com/air-verse/air); frontend hot-reloads via Vite. Both containers mount the local source tree. **On Windows/Docker Desktop, the file watcher occasionally misses a change** (confirmed multiple times during development) — if the API or UI seems to be serving stale code, `docker compose restart gateway` / `docker compose restart web` forces a fresh pick-up.

The gateway defaults to MQTT as its forward transport (`FORWARDER_TRANSPORT=mqtt` in `docker-compose.yml`, pointed at the `mosquitto` service) — the production path. Override to the HTTP dev adapter with `docker compose run -e FORWARDER_TRANSPORT=http gateway` if you want to test against `server-sim`'s HTTP side instead.

With `SEED_DEMO_DEVICE=true` (set by default in `docker-compose.yml`), the gateway seeds one demo Modbus TCP device (`PM001`, pointed at `modbus-sim`) on first boot if the `device` table is empty. Watch it read live values:

```bash
docker compose logs -f gateway
```

The Web UI (http://localhost:5173) has seven tabs:

- **Dashboard** — gateway status, CPU/RAM/storage/network, device/data point counts, server connection, pending queue, time sync
- **Devices** — add/edit/delete devices and their data points, toggle enabled, Test Connection/Test Read, COM port dropdown for RTU. Changes take effect immediately — the acquisition engine reloads its polling set on every change, no restart needed.
- **Store & Forward** — pending/sending counts, storage usage with WARNING/CRITICAL/FULL level, server connectivity
- **Time** — NTP sync status, clock offset, RTC status, time quality
- **Diagnostics** — Modbus TX/RX, average response time, timeout/CRC error/retry counts
- **Logs** — the last 1000 in-memory log entries (not a log file — resets on restart)
- **Config** — Gateway/MQTT/Time settings (saves to `config.yaml` and restarts the gateway to apply — see below), host network IP (Linux/NetworkManager only, not this Docker dev setup — see below), and configuration export/import (devices + data points, secrets redacted)

Every acquired reading is persisted to `data_queue` and then forwarded in batches. Check `GET /api/store-forward/status` for pending/sending counts, retry count, disk usage %/level, and server connectivity — or watch `curl http://localhost:9000/received` to see what the "server" has accepted. Stop the `mosquitto` (or `server-sim`, if using the HTTP transport) container to see the backlog grow while acquisition keeps running (Rule 1); restart it to see the backlog drain with no duplicates.

For RTU devices, the "Serial Interface" field shows a dropdown of ports detected on the gateway host (`GET /api/system/serial-ports`). In the Docker dev setup the list is normally empty — a container can't see a host's serial ports unless one is explicitly passed through (see the commented `devices:` example on the `gateway` service in `docker-compose.yml`) — so the field falls back to manual entry (e.g. `/dev/ttyUSB0`).

### Saving settings from the Web UI

The Config page's "Gateway / MQTT / Time Settings" card writes straight to `configs/config.yaml` and restarts the gateway process to apply the change (a few seconds of downtime; the page polls `/api/system` and clears its "restarting" banner once it's back). This rewrites the whole file, so **any hand-edited comments in `config.yaml` are lost the first time you save from the UI** — the page says so, but it's easy to miss.

The Config page's "Network (Host IP)" card is for setting the gateway *host's* actual network interface IP (via NetworkManager's `nmcli`) — only meaningful when the gateway binary runs directly on a Linux host, never inside this Docker Compose dev setup (a container's network is not the host's real NIC). In this dev environment it correctly reports "Not available on this host" and does nothing further; live testing against real `nmcli`/hardware is deferred to Raspberry Pi deployment. Applying a static IP is protected by a 45-second confirm-or-auto-revert window (`internal/netconfig`) — a typo'd address self-heals instead of locking you out, but this hasn't been exercised against real hardware yet.

### Testing RTU against real hardware on Windows

USB-to-RS485 adapters attach as a native Windows COM port, which a Docker container (even on Linux-container/WSL2 backends) cannot see without extra passthrough tooling (`usbipd-win`). The simplest way to test the real port is to run the gateway natively instead of in Docker:

```powershell
go build -o gateway.exe ./cmd/gateway
./gateway.exe -config configs/config.yaml -migrations migrations
```

and the frontend natively too (`cd web && npm run dev` — its Vite proxy already defaults to `http://localhost:8080` when `GATEWAY_API_URL` isn't set, which is only set inside `docker-compose.yml`). Stop the Dockerized `gateway`/`web` containers first to free ports 8080/5173; `modbus-sim` can keep running in Docker (native gateway reaches it at `localhost:1502`). This is how RTU was validated against a real USB-to-RS485 (CH340) adapter — see the Status section below.

## Project layout

```
gateway/          Go backend (cmd/gateway, cmd/modbus-sim, cmd/server-sim, internal/, migrations/, configs/, deploy/)
web/               React + TypeScript frontend
internal-server/   The real MQTT consumer the gateway forwards to in production — separate Go module + Docker Compose stack
```

## Internal Server (`internal-server/`)

`cmd/server-sim` (inside `gateway/`) is a dev-only, in-memory stand-in for testing Store & Forward without a real backend — it was never meant to run continuously. `internal-server/` is the real thing: subscribes `gateway/+/data` on the MQTT broker, persists every reading to Postgres (deduped on `gateway_id`+`sequence_id`, the same idempotency key the gateway itself uses), and publishes the application-level ack back on `gateway/{id}/ack` so Store & Forward can retire the batch.

```bash
cd internal-server
docker compose up -d --build
```

- Dashboard: http://localhost:9100/dashboard (also served at `/`) — live stat tiles with sparklines per gateway/device/datapoint, per-gateway totals, and a filterable recent-readings table.
- JSON API: `GET /health`, `GET /latest`, `GET /readings?gateway_id=&device_id=&datapoint_id=&limit=`, `GET /stats`.
- Configure which broker it points at via the `MQTT_BROKER_URL` env var in `internal-server/docker-compose.yml` (defaults to the real `mqtt.nxge.co:1883`, not the dev `mosquitto` service in the root `docker-compose.yml` — these are two separate brokers for two separate purposes).

Not yet production-hardened: it currently runs on whatever machine you start it on (no always-on host assigned yet), and its Postgres credentials are dev-shaped defaults (`internal_server`/`internal_server`) — see HANDOFF.md's "Next" section.

## Status

MVP (Phases 0-8) is complete — see [industrial_iot_gateway_handoff_dev_plan.md](industrial_iot_gateway_handoff_dev_plan.md) §24 (Definition of Done) for the full, honestly-caveated checklist, and §29 for the post-MVP Web UI Settings / Host Network IP addition. Summary:

- **Phase 0-4** (project setup through Store & Forward): done, verified live including crash recovery and zero-duplicate delivery under a real backlog.
- **Phase 5 — MQTT**: `MQTTAdapter` (QoS 1, application-level ack, TLS/auth, auto-reconnect) alongside `HTTPAdapter`, selected via `forwarder.transport`. Verified live via `docker compose` including a broker outage and reconnect.
- **Phase 6 — Time Service**: hand-rolled SNTP client, Linux hardware RTC (`/dev/rtc0` ioctls, cross-compiled clean but untested against physical hardware), SYNCED/RTC/UNSYNCED/INVALID quality state machine.
- **Phase 7 — Web UI**: all seven tabs above, verified live in a real headless-Chromium browser session (zero console errors) plus a real config export/import round trip.
- **Phase 8 — Reliability**: chaos-tested the live stack — a `SIGKILL` power-failure simulation and a full `docker network disconnect` network partition (which surfaced and fixed a real DNS-classification bug). Cumulative duplicate check across all of this project's chaos testing: 4747/4747 unique `gateway_id+sequence_id` keys.
- **Post-MVP**: Web UI Settings (Gateway/MQTT/Time, save + auto-restart), host network IP configuration (`internal/netconfig`, Linux/NetworkManager only), and the real Internal Server (`internal-server/`, see its section above).
- **Raspberry Pi deployment** (three sessions so far, `DEPLOY_PLAN.md`): the gateway now runs natively on a real Pi (Debian 13 trixie, aarch64) under systemd, with a real CH340 USB-RS485 adapter and RTU temp/humidity sensor delivering `GOOD` readings, forwarding over a real MQTT broker (`mqtt.nxge.co`) to the real Internal Server above, and Settings-save-triggered restarts verified under `systemctl`.

**Known, honestly-documented gaps** (not silently assumed to work):
- Full Modbus RTU read round-trip: **now verified live** against a real sensor (see above) — including finding a real hardware limit (this sensor times out below ~250ms polling interval, see `HANDOFF.md`). Still open: the CRC-error/`DEVICE_OFFLINE` failure paths haven't been triggered from live hardware yet (unplug mid-poll, forced bad response).
- Host network IP (`internal/netconfig`): `Current()` verified live against a real Pi's `nmcli`. `ApplyStatic`/the confirm-or-auto-revert flow is still unverified — deliberately deferred until physical console access is arranged, since a bad apply could lock out the only remote path to the device.
- RTC: the Linux `/dev/rtc0` ioctl code cross-compiles/unit-tests clean but has never run against a physical RTC chip — no such hardware connected in any session so far.
- `internal-server/` isn't on an always-on host yet, and its Postgres credentials are dev-shaped defaults — see `HANDOFF.md`'s "Next" section.
