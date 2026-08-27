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

- Dashboard: http://localhost:9100/dashboard (also served at `/`, which 302-redirects there) — an in-page nav (Health / Stats / Latest / Readings) jumps to: a Health section with Database/MQTT status cards, live stat tiles with sparklines per gateway/device/datapoint, per-gateway totals, and a filterable recent-readings table. Everything renders from the JSON API below via client-side JS — no separate UI for the raw JSON.
- JSON API: `GET /health`, `GET /latest`, `GET /readings?gateway_id=&device_id=&datapoint_id=&limit=&since=`, `GET /stats`. `since` (RFC3339) filters to `received_at >= since`; when given without an explicit `limit` the cap rises to 5000 instead of the default 100 (both are hard-capped at 5000).
- Configure which broker it points at via the `MQTT_BROKER_URL` env var in `internal-server/docker-compose.yml` (defaults to the real `mqtt.nxge.co:1883`, not the dev `mosquitto` service in the root `docker-compose.yml` — these are two separate brokers for two separate purposes).

The commands above are for running it locally in dev. In production it runs on a real always-on host (`192.168.99.200`) — see spec.md's "Live access points" for the live dashboard URL. Its Postgres credentials are still dev-shaped defaults (`internal_server`/`internal_server`) — not exposed outside the compose network (no host port mapping on the `postgres` service), but rotate before calling this fully production-ready.

## Production deployment

Two separate live targets, updated independently — neither deploy touches the other.

### Gateway → Raspberry Pi (native/systemd, not Docker)

The Pi (`NXGE-RPI`) runs the gateway natively under `nxiiot-gateway.service` (`Restart=always`) — see `gateway/deploy/nxiiot-gateway.service` and `DEPLOY_PLAN.md` for the original install. To ship a code change:

```bash
# 1. On your dev machine: commit and push (the Pi pulls from GitHub)
git push

# 2. SSH to the Pi (check spec.md's "Live access points" for the current IP — it's DHCP and can change)
ssh nxge-admin@<pi-ip>

# 3. On the Pi: pull and build natively (cross-compiling from Windows isn't used — see HANDOFF.md)
cd ~/nxIIoT-Gateway
git pull
cd gateway
go build -o /tmp/gateway-new ./cmd/gateway   # ~3 min, 18-19MB binary — slow but not stuck

# 4. Back up the running binary, then hot-swap it — do NOT `cp` straight onto the target
sudo cp /opt/nxiiot-gateway/gateway /opt/nxiiot-gateway/gateway.bak-$(date +%Y%m%d%H%M%S)
sudo cp /tmp/gateway-new /opt/nxiiot-gateway/gateway.new
sudo mv /opt/nxiiot-gateway/gateway.new /opt/nxiiot-gateway/gateway
sudo systemctl restart nxiiot-gateway

# 5. Verify
sudo systemctl status nxiiot-gateway --no-pager
journalctl -u nxiiot-gateway --no-pager -n 20
```

**Why `mv`, not `cp`, in step 4**: `cp`ing directly onto `/opt/nxiiot-gateway/gateway` while the service is running it fails with `cannot create regular file: Text file busy` (`ETXTBSY`) — Linux won't let you open-for-write a file that's mapped as a running process's executable text. `mv` (a `rename()`) just repoints the directory entry to a new inode without touching the old one, so the running process is undisturbed until the restart picks up the new binary. See MEMORY.md.

The Web UI (`nxiiot-gateway-web.service`, Vite dev server, `web/deploy/nxiiot-gateway-web.service`) picks up frontend changes via `git pull` + Vite's own hot-reload — no rebuild or restart needed for `web/` changes alone.

### `internal-server/` → vendor server (Docker Compose)

Runs on a vendor-provided always-on host (`192.168.99.200`, user `vendor-app` — see spec.md's "Live access points"), not the Pi. Source is copied over SFTP rather than `git clone`, specifically to avoid needing git credentials on a third-party-managed machine — even though the host does have `git` installed.

```bash
# From your dev machine, for each changed file (or the whole internal-server/ tree):
scp internal-server/<changed-file> vendor-app@192.168.99.200:/home/vendor-app/nxiiot-internal-server/<same-relative-path>

# Then on the vendor server (or via ssh -- '<cmd>'):
cd /home/vendor-app/nxiiot-internal-server
docker compose -p nxiiot-internal-server up -d --build
```

Rebuilding is required even for a static-file-only change (`internal-server/static/dashboard.html`) — it's `go:embed`'d into the binary (`server.go`), not served from disk, so the Docker image must be rebuilt to pick it up.

Verify after redeploying:
```bash
curl -s http://192.168.99.200:9100/health   # {"database_ok":true,"mqtt_connected":true,"status":"ok"}
docker ps --filter name=nxiiot-internal-server
```

### Deploying via automation (Claude Code / any non-interactive environment)

The vendor server (`vendor-app@192.168.99.200`) used password-only auth until 2026-08-27 — a real limitation for any non-interactive tool (a CI job, a script, an AI coding agent), since a plain `ssh`/`scp` invocation with no TTY has nothing to type the password into and fails immediately with `Permission denied (publickey,password)`.

**Fixed by adding SSH key auth** (2026-08-27): a dedicated keypair's public half (comment `claude-code-nxiiot-deploy`) was appended to `vendor-app`'s `~/.ssh/authorized_keys` on the vendor server, with the user's explicit setup action (they ran the `ssh`/`authorized_keys` commands themselves, once, interactively — see DEPLOY_PLAN.md). Plain `ssh`/`scp` from this repo's dev machine now authenticates with no password involved at all, so the whole `internal-server/` deploy flow above is automatable end-to-end.

**Do not solve a missing-TTY problem by embedding a plaintext password in a script**, in any language or library (Python's `paramiko`, `sshpass`, `SSH_ASKPASS`, a `plink`/heredoc trick, etc.). A prior version of this section suggested exactly that (with a fabricated claim that it was "the exact mechanism used in Session 4" — it wasn't; DEPLOY_PLAN.md's actual Session 4 log has no Python/paramiko involvement anywhere). Switching the implementation language doesn't change what Claude Code's auto mode classifier is actually gating: an agent holding and transmitting a plaintext production credential unattended. That was correctly declined when proposed in this project's chat log — see MEMORY.md's 2026-08-27 entry on this. SSH keys are the sanctioned fix precisely because they remove the plaintext-secret-in-a-script problem entirely, rather than routing around the check that flags it.

**What Claude Code's auto mode classifier actually blocks** — narrower than it might first appear, and unaffected by the SSH-key fix above:
- ❌ **Overwriting the live binary a running production `systemd` service is executing** (`cp`/writing straight onto `/opt/nxiiot-gateway/gateway` while `nxiiot-gateway.service` is active) — blocked even after in-chat user confirmation, because a conversational "yes" isn't the same signal as an approved permission rule.
- ❌ **An agent editing its own `autoMode` permission rules** in `.claude/settings.json`/`settings.local.json` to grant itself an exception to the block above — blocked regardless of who asked, since an agent shouldn't be able to route around its own safety boundary.
- ✅ Everything else in a normal deploy goes through without issue: `git pull`, `go build`/`docker build` to a scratch path, backing up the old binary (`cp` to a *different* filename), SFTP/`scp`-style file uploads (now key-authenticated, see above), `docker compose up -d --build`, starting or restarting a *new* or *already-dormant* systemd unit, and `sudo` in general.

In practice this means: build the new artifact and stage it (all automatable), then either (a) do the final "swap the live binary and restart" step as a `mv` + `systemctl restart` that a human runs themselves in their own terminal — see the Pi section above for the exact commands — or (b) for a Docker Compose target like `internal-server/`, there is no live-binary-swap step at all (`docker compose up -d --build` recreates the container from a fresh image), so the whole deploy is automatable end-to-end now that key auth is set up.

**Known gotcha: `docker compose ... up -d --build` can silently reuse a stale cached layer** even when the only change is a `go:embed`'d static file (`internal-server/static/dashboard.html`) — seen live on 2026-08-27, where the `COPY . .` build layer reported `CACHED` despite the file content having actually changed on disk, and the container kept serving the old embedded HTML with no error of any kind. Always verify the *deployed, running* content directly (e.g. `curl` the live endpoint and `grep` for a string unique to the change) rather than trusting "Built"/cache-hit output in the build log. If verification fails, force it with `docker compose build --no-cache <service> && docker compose up -d --force-recreate <service>`. Also note: `internal-server/`'s `/` route is a `302` redirect to `/dashboard`, not the dashboard itself — `curl` it with `-L`, or hit `/dashboard` directly, or a "verification" can look empty/failed when the redeploy actually succeeded.

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
- `internal-server/` now runs on an always-on host (`192.168.99.200`, see spec.md); its Postgres credentials are still dev-shaped defaults — see `HANDOFF.md`'s "Next" section.
