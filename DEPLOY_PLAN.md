# Deploy Plan — Raspberry Pi

Plan for installing on a real Raspberry Pi and continuing development there. Written before that session happens — treat every unchecked box below as genuinely unverified, not "should work." See [HANDOFF.md](HANDOFF.md) for what's been verified so far (all of it on Windows/Docker, none on real ARM hardware) and [industrial_iot_gateway_handoff_dev_plan.md](industrial_iot_gateway_handoff_dev_plan.md) §29 for why the RTC and network-IP code specifically has never run for real.

## Why this trip matters

Three things in this codebase are written and unit-tested but have **never executed against real hardware**, because none of it exists in the Windows/Docker dev environment:

1. **Modbus RTU full read round-trip** against a responding slave device (only the serial *connect* was verified, against a real USB-RS485 adapter, back in Phase 1 — no slave was available then).
2. **Hardware RTC** (`/dev/rtc0` ioctls, `internal/time/rtc_linux.go`) — cross-compiles clean, zero real runs.
3. **Host network IP config** (`internal/netconfig`, `nmcli`) — real parsing logic, tested against canned `nmcli` output, zero runs against an actual NetworkManager instance.

Tomorrow's job is closing those three gaps for real, plus getting a working dev loop running directly on the Pi.

## Session 1 Log (2026-08-25) — read this first if resuming

Deployed via SSH (Claude Code driving it directly, `nxge-admin@192.168.99.84`), Path B (native/systemd), on the Pi's **original SD card** — which is about to be swapped for a bigger one because it ran nearly out of space. **Everything below needs redoing on the new card**; nothing here persists across the swap except what's already pushed to GitHub (all of it — the running deployment used no uncommitted code).

**What got done and confirmed working:**
- Target machine: `NXGE-RPI`, Debian 13 "trixie" (aarch64), kernel `6.18.34+rpt-rpi-v8`. NetworkManager/`nmcli` 1.52.1 present by default (Bookworm+ requirement from §1 — trixie has it too).
- SSH key-based auth set up (a new keypair was generated on the Windows dev machine, `~/.ssh/id_ed25519`, and its public key appended to the Pi's `~/.ssh/authorized_keys` using the password once). **This is gone once the SD card is replaced** — either re-run `ssh-copy-id` (or the manual `authorized_keys` append) against the new card's fresh `authorized_keys`, or reuse the same already-generated Windows-side keypair (it's still on the Windows machine, nothing to regenerate there).
- Go 1.27.0 installed to `/usr/local/go` (official upstream tarball, not the apt package — apt's `golang-go` candidate was only 1.24, older than `go.mod`'s `go 1.25.0` requirement).
- Repo cloned to `~/nxIIoT-Gateway` at commit `8f86707`. `cd gateway && go build -o gateway ./cmd/gateway` succeeded — **first successful native ARM64 build**, ~18.6MB binary. First build was slow (several minutes, `modernc.org/sqlite`'s generated Go source is large) but not stuck — worth knowing so it isn't mistaken for a hang next time either.
- Deployed to `/opt/nxiiot-gateway` (binary + `configs/` + `migrations/`), `gateway/deploy/nxiiot-gateway.service` installed to `/etc/systemd/system/` and enabled — `systemctl status nxiiot-gateway` showed `active (running)`, migrations applied cleanly, `GET /api/system` responded normally.
- **`internal/netconfig` confirmed working against real `nmcli` for the first time ever** (gap #3 from the intro above, closed): `GET /api/system/network` correctly reported `{"supported":true,"interface":"wlan0","method":"auto","address":"192.168.99.84",...}` — real interface, real IP, matching what we were actually SSH'd into. Only `Current()` was exercised; `ApplyStatic`/the confirm-or-revert flow (§6's checklist item) is still untested — do that once the new card is stable, with console access ready per §0.
- Node.js 20.19.2 / npm 9.2.0 installed via apt, for running the Web UI with `npm run dev` (chosen over building static files, for the hot-reload dev loop).

**What's blocked, and why the SD card is being swapped:**
- Original card: 6.9GB total. Started at 87% used (already tight *before* any of this session's work — worth asking why, on a fresh-ish install). Installing Go pushed it further; installing Node.js/npm pushed it to **94% used, 400MB free**. `npm install` for the web frontend (React+Vite+TypeScript+oxlint, several hundred MB of `node_modules` on the Windows dev machine) was judged too risky to attempt on 400MB — a full disk mid-write is a real risk to the running SQLite database, not just an inconvenience. Stopped there, deliberately, rather than push through.
- The Web UI was never actually reached in a browser this session — only the backend API was verified (`curl`).
- RTU, RTC, and the netconfig `Apply`/revert flow — the other three checklist items in §6 — are all still open. No RTU hardware or RTC module was connected this session; today was entirely about getting the backend running and closing the netconfig `Current()` gap.

**Resume steps on the new SD card** (assuming Raspberry Pi OS/Debian trixie or similar again):
1. Re-flash, boot, SSH in — get a fresh IP/hostname noted (§0).
2. Either push the existing Windows-side public key again, or start over with §2 of this doc (`git clone`, then Go/Node install per this log). All of §1-§4 needs re-running; none of it survives the card swap.
3. Once storage isn't a concern: `cd ~/nxIIoT-Gateway/web && npm install && npm run dev` — Vite's proxy already defaults to `localhost:8080` (see `README.md`), so no `GATEWAY_API_URL` env var needed for the native-on-same-host setup.
4. Then resume the §6 checklist from where it left off: RTU round-trip, RTC, netconfig `Apply`/confirm/revert, real MQTT broker, Settings-save restart under systemd.

## Session 2 Log (2026-08-25) — new SD card, redoing §1-§4

Same Pi (`NXGE-RPI`, Debian 13 trixie aarch64, kernel `6.18.34+rpt-rpi-v8`), fresh SD card: **29GB total** vs. the old card's 6.9GB — this alone should prevent a repeat of Session 1's disk crunch. Confirmed throughout: disk stayed at 21% used / 22GB free after Go, git, Node.js, and `npm install` combined (peak usage nowhere close to a concern this time).

**What got done:**
- SSH key-based auth re-established: reused the existing Windows-side keypair (`~/.ssh/id_ed25519`, generated in Session 1, never lost) — appended its public key to the fresh card's `~/.ssh/authorized_keys` via a one-time password-authenticated connection (Python/paramiko, since neither `sshpass` nor `expect` were available in this Windows/Git-Bash environment; `ssh-copy-id` alone can't supply the password non-interactively here). No new keypair generated.
- Go 1.27.0 installed to `/usr/local/go` (same official arm64 tarball approach as Session 1 — apt's `golang-go` still only offers 1.24). `go version` confirmed `go1.27.0 linux/arm64`.
- **git was not preinstalled this time** (Session 1's card apparently had it already, or it went unnoted) — installed via `apt-get install git` (2.47.3), trivial disk cost.
- Repo cloned to `~/nxIIoT-Gateway` at commit `b153a66` (latest master, includes Session 1's own log commit).
- `cd gateway && go build -o gateway ./cmd/gateway` succeeded — 18MB binary, ~3 minutes (matches Session 1's note that the first build is slow but not stuck).
- Deployed to `/opt/nxiiot-gateway`, `nxiiot-gateway.service` installed and enabled. `systemctl status` → `active (running)`, migrations applied cleanly (`0001_init.sql`, `0002_device_rtu_params.sql`, `0003_data_queue_retry.sql`), `GET /api/system` responded normally.
- **`netconfig.Current()` re-verified on the new card** — `GET /api/system/network` again correctly reported real `wlan0`/`192.168.99.84`/gateway/DNS, confirming Session 1's result wasn't a fluke tied to the old card's state.
- Node.js 20.19.2 / npm 9.2.0 installed via apt — same versions as Session 1, no disk issue this time.
- **`cd web && npm install` completed in ~30s with no disk concern** (disk stayed at 21%/22GB free before and after) — this is exactly where Session 1 stopped. `npm run dev -- --host 0.0.0.0` started successfully; **the Web UI was reached over HTTP for the first time ever** (`curl http://192.168.99.84:5173/` → `200`, and `/api/system` proxied correctly through Vite to the backend on :8080). Not yet opened in an actual browser window — only curl-verified so far.
- **Settings-save-triggers-restart verified for real under systemd** (§6 checklist item, previously only proven under Docker Compose's `restart: unless-stopped`): `PUT /api/config/settings` with the same (unchanged) values → `{"restarting":true,"saved":true}` → process PID changed from `6912` to `13286`, `systemctl status` showed a fresh `active (running)` within ~8s, `GET /api/system` responded again. `Restart=always` confirmed working exactly as designed.

**What's still open (unchanged from Session 1, nothing new happened here):**
- No RTU adapter or RTC module was physically connected this session (`ls /dev/ttyUSB*`/`/dev/ttyACM*`/`/dev/rtc0`/`/dev/i2c*` all empty) — both remain completely untested, same as every prior session. These need a human to physically wire hardware to the Pi.
- `netconfig.ApplyStatic`/confirm-revert flow still untested — deliberately not attempted without physical console access lined up first, per §0.
- Real MQTT broker test still open — current config still points at `tcp://localhost:1883` (dev-shaped default), no real broker address available yet.

## Session 3 Log (2026-08-26) — real Internal Server, RTU polling-interval limits, Logs UI fix

Same Pi (`NXGE-RPI`), reached this time over Tailscale (`100.84.193.68`) rather than the LAN IP used in Sessions 1-2 — both reach the same machine, Tailscale was just what was convenient this session. No SD card change; picking up from Session 2's state (RTU + MQTT + Settings-restart already verified).

**MQTT stuck-connection incident, diagnosed and resolved:**
- Found the gateway `active (running)` for 4.5 hours but stuck: `server_connected: false`, ~3,700 pending records, retry_count over 115,000, `server_last_error: "mqtt: timed out waiting for ack of batch ..."` on every attempt. `journalctl` showed paho logging `"mqtt connected"` once after the last restart, then **no further "connection lost" event for the entire 4.5 hours** — the client believed it stayed connected the whole time.
- Confirmed the broker itself was fine: a raw `mosquitto_pub`/`mosquitto_sub` from the Pi worked instantly, and subscribing to `gateway/GW001/data` directly showed the gateway's own batches publishing successfully with real data.
- **Actual root cause: nothing was consuming `gateway/GW001/data` and publishing back to `gateway/GW001/ack`.** `cmd/server-sim` (the only prior MQTT consumer, used in Session 2 to close the loop for testing) was not running — exactly the state Session 2's own log already predicted ("mqtt.nxge.co needs a real consumer app before this gateway's data is actually useful in production"). A `systemctl restart nxiiot-gateway` was tried first and didn't fix it (confirmed fresh reconnect, ack subscribe apparently succeeded, batches still timed out) — proving it wasn't a stuck-client-state problem at all, just the absence of a real ack producer.
- **`mqttadapter.go`'s `onConnect` re-subscribe has no retry on failure** (`gateway/internal/forwarder/mqttadapter.go:118-126`) — real log evidence exists of this failing once mid-session ("connection lost before Subscribe completed") with no follow-up attempt. Not the cause of this particular incident, but a real gap worth fixing — see HANDOFF.md.
- Backlog drained by running `cmd/server-sim` temporarily on the Pi (`go build -o /tmp/server-sim ./cmd/server-sim && /tmp/server-sim -mqtt-broker tcp://mqtt.nxge.co:1883`) — 3,780 records drained to 0 within seconds, then stopped (`pkill -x server-sim`, **not** `pkill -f` — `-f` matches the invoking command line itself and killed the SSH session the first time this was tried).

**Real Internal Server built and deployed** (closing the gap above properly instead of leaving `server-sim` running as a permanent hack) — see §30 of the design doc and the new `internal-server/` top-level directory:
- Go service + Postgres, Dockerized, currently running via `docker compose up -d` **on the Windows dev laptop** (not the Pi, not an always-on host — see HANDOFF.md's "Next" section for why this still needs a real home).
- Subscribes `gateway/+/data` on `mqtt.nxge.co`, dedupes into Postgres on `gateway_id`+`sequence_id`, acks back on `gateway/{id}/ack`. Verified live: Pi's backlog drains to 0 and stays there with this running.
- Ships a dashboard (`/dashboard`, also the `/` redirect target) — stat tiles with sparklines per gateway/device/datapoint, per-gateway totals, a filterable recent-readings table — plus a JSON API (`/health`, `/latest`, `/readings`, `/stats`).

**RTU polling-interval limits found by direct experiment** (device 1, "Temp-Humidity Sensor", both data points) — see HANDOFF.md's Known Gaps for the summary:
- 5000ms (original): `GOOD`. 500ms: `GOOD`. 250ms: `GOOD`, confirmed stable, MQTT kept up. 200ms and 100ms: **every reading `TIMEOUT`**, device status `TIMEOUT` — the sensor's own response latency can't keep up, not a code issue. Reverted from 200ms back down to 250ms (verified `GOOD` again) after confirming the failure.
- Changed via `PUT /api/devices/1` and `PUT /api/datapoints/{1,2}` directly (not the Web UI) — **remember both the device-level `polling_interval_ms` (gates how often the poll loop ticks) and each data point's own `polling_interval_ms` (gates whether that specific tag is actually read on a given tick) need to match** — changing only the device-level value was tried first and had no effect on actual read cadence, since both data points were still gating themselves at the old 5000ms. See `gateway/internal/acquisition/poller.go:61-89`.
- Current deployed state: device + both data points all at 250ms.

**Web UI fix deployed with zero downtime**: a Logs-page CSS alignment fix (`web/src/styles.ts`, `web/src/pages/LogsPage.tsx` — the message column drifted per line because a variable-width level label like `DEBUG` vs `INFO` sat inline before it; fixed with a 3-column CSS grid) was pushed to GitHub, then `git pull`ed on the Pi. The Session 2-era `npm run dev` process (still running, untouched since Session 2) picked up the change via Vite's hot-reload alone — no restart, no redeploy step, confirmed via the HMR log line for `LogsPage.tsx`.

## 0. Before you unplug anything

- **Have a way back in that doesn't depend on the network working.** A monitor + keyboard on the Pi, or physical access to re-flash the SD card, before testing the network-IP feature specifically. The confirm-or-auto-revert safety net (`internal/netconfig`, 45s window) is real and unit-tested, but it has never run against real `nmcli` — don't bet the only way to reach the device on code that's never executed for real. Test that feature last, once everything else is confirmed working, and only with physical console access as a fallback.
- **Note the Pi's current IP/hostname** before doing anything, so there's a known-good address to fall back to.

## 1. Hardware / OS prerequisites

- Raspberry Pi 4 or 5 (§27's target). `uname -m` should report `aarch64` — a 64-bit OS image is assumed below; 32-bit (`armv7l`) likely still works (everything in `go.mod` is either pure Go or has ARM support) but hasn't been considered here.
- Raspberry Pi OS **Bookworm or newer** matters specifically for `internal/netconfig`: it's the version that ships NetworkManager (`nmcli`) by default. Older Raspbian (Bullseye and earlier) uses `dhcpcd` instead — `internal/netconfig` doesn't support that, and will just report `{"supported": false}` on it, same as it does on Windows now. Check before relying on the feature:
  ```bash
  nmcli --version && systemctl is-active NetworkManager
  ```
- A real Modbus RTU slave device (power meter or similar) and a USB-RS485 adapter, wired up and addressable — this is the whole point of tomorrow's RTU test.
- If testing RTC: a battery-backed RTC module (e.g. DS3231 HAT) wired to the Pi's I2C pins and enabled in `/boot/firmware/config.txt` (`dtoverlay=i2c-rtc,ds3231` or similar for your specific module) — this is Raspberry Pi OS/hardware setup, outside this repo.
- Docker + Docker Compose, if going the container route (`curl -fsSL https://get.docker.com | sh`, standard Raspberry Pi OS install).
- Go 1.25+, if going the native/systemd route (`internal/time/service_test.go` etc. were written against 1.25 — check `go version` matches or exceeds what `gateway/go.mod` declares).

## 2. Get the code onto the Pi

```bash
git clone https://github.com/bpp-pico/nxIIoT-Gateway.git
cd nxIIoT-Gateway
git log --oneline -3   # should show 66975b3 at the top
```

## 3. Path A — Docker Compose (fastest, lowest-risk first pass)

Reuses the exact dev stack already built and tested (Phases 0-8). Good for: confirming the whole system runs on ARM at all, testing RTU with a passed-through serial device, general MQTT/Store & Forward sanity. **Not sufficient for RTC or network-IP testing** — see Path B.

```bash
docker compose up --build
```

Before that, in `docker-compose.yml`, uncomment and adjust the `gateway` service's serial passthrough for your actual adapter:
```yaml
devices:
  - "/dev/ttyUSB0:/dev/ttyUSB0"
```
(check the real device name first: `ls /dev/ttyUSB* /dev/ttyACM*` after plugging the adapter in, or `dmesg | tail` right after plugging it in).

Then add the real RTU device from the Web UI (http://<pi-ip>:5173 → Devices → Add Device → Protocol: RTU, Interface: `/dev/ttyUSB0`, and the real baud rate/parity/slave ID for your meter) and watch `docker compose logs -f gateway` for real readings instead of `modbus-sim`'s canned values.

## 4. Path B — Native / systemd (needed for RTC and network-IP)

A container's device/network isolation gets in the way of both remaining gaps — RTC needs `/dev/rtc0` passed through *and* correct group permissions, and `nmcli` needs to reach the host's real NetworkManager over D-Bus, which means bind-mounting `/run/dbus` and `/var/run/NetworkManager` plus `--network host` even to attempt it in a container. Running the binary directly on the host sidesteps all of that, and matches `gateway/deploy/nxiiot-gateway.service` (already in the repo, written for exactly this path, `Restart=always` so a Settings-save-triggered restart — §29 — actually comes back up).

```bash
cd gateway
go build -o gateway ./cmd/gateway
sudo mkdir -p /opt/nxiiot-gateway
sudo cp gateway /opt/nxiiot-gateway/
sudo cp -r configs migrations /opt/nxiiot-gateway/
sudo cp deploy/nxiiot-gateway.service /etc/systemd/system/
```

Edit `/opt/nxiiot-gateway/configs/config.yaml` for a real deployment (see §5 below), then:

```bash
sudo systemctl daemon-reload
sudo systemctl enable --now nxiiot-gateway
sudo systemctl status nxiiot-gateway
journalctl -u nxiiot-gateway -f
```

The web frontend isn't part of this unit — either run it separately (`cd web && npm run dev`, or `npm run build` and serve `dist/` with any static file server) or just drive the API directly with `curl` for initial hardware verification and wire up the frontend once the backend is confirmed working.

**Privileges**: setting a static IP via `nmcli` needs `CAP_NET_ADMIN` or root, depending on your system's polkit policy for NetworkManager. The unit file has both options commented out — uncomment whichever your setup needs. Reading `/dev/rtc0` typically needs the process's user to be in the `dialout`/`i2c` group or run as root; check `ls -l /dev/rtc0` on your Pi to see what's actually required.

## 5. Config changes for a real deployment

`configs/config.yaml` as committed has dev-shaped defaults (points at `localhost`, Docker service names). For a real Pi deployment, at minimum:

- `gateway.id` / `gateway.name` — a real identifier, not `GW001` / `Development Gateway`.
- `forwarder.transport: "mqtt"`, `mqtt.broker_url` — the real internal MQTT broker's address (`tcp://<broker-ip>:1883`), not `mosquitto`/`localhost`. Set `mqtt.username`/`password` and `mqtt.tls` if the real broker requires them.
- `time.ntp_server` — the real internal network's NTP server (Rule 8: never a public NTP server in production — `pool.ntp.org` was used earlier only as a dev-environment reachability check, explicitly not a production pattern; see §29/HANDOFF.md).
- `database.path` — a path on durable storage if `/opt/nxiiot-gateway` isn't already (SD card wear matters for a long-running SQLite+WAL database; consider whether this Pi has an SSD via USB3, which is the generally-recommended setup for anything write-heavy on a Pi).
- Remove `SEED_DEMO_DEVICE=true` (that's a Docker Compose dev-only env var in `docker-compose.yml`; irrelevant to the native path, just don't carry the habit of seeding fake devices into a real deployment).

Everything above is also editable from the Web UI's Config page (§29) once the gateway is running — either edit the file directly before first start, or start it with defaults and use the UI afterward. The UI path restarts the process to apply (a few seconds of downtime); expected and fine for initial setup.

## 6. Verification checklist for tomorrow

Work through these in order — each depends on the previous one actually working. Update `industrial_iot_gateway_handoff_dev_plan.md` §29 (and the "Known gaps" sections in `README.md`/`HANDOFF.md`) with the real results once done, the same way every other phase in this project was closed out — "verified live" with actual command output, not "should work now."

- [x] Gateway starts cleanly on the Pi (native/systemd), `GET /api/system` responds, `uname -m`/`go version` noted for the record — done in Session 1, **redone on the new SD card in Session 2** (commit `b153a66`, Go 1.27.0, 18MB binary, migrations applied cleanly).
- [x] **RTU round-trip**: **verified for real in Session 2.** A CH340 USB-RS485 adapter (`/dev/ttyUSB0`, vendor `1a86`) was connected mid-session along with an RTU temp/humidity sensor (slave ID 1, 9600 8N1, function code 04, input registers 1=temperature/2=humidity, both `INT16` × scale `0.1`). `POST /devices/1/test` → `{"quality":"GOOD"}`. `Test Read` on both data points: `temperature: 27.1°C`, `humidity: 58.5%RH`, both `quality: "GOOD"` — user-confirmed as plausible for the room. Beyond Test Read, the live polling loop was also confirmed writing real decoded readings into `data_queue` every ~5s (`store-forward/status` showed `pending_records` climbing from creation). First real Modbus RTU full read round-trip (function code → response → decode → scale → persist) in the project's history.
- [ ] RTU failure paths against the real device (unplug mid-poll → expect `DEVICE_OFFLINE`; forced bad response → `CRC_ERROR`) — the adapter is working and available, but this specific test was deliberately deferred to a future session (asked, user said skip for now).
- [x] **RTU polling-interval limits — found in Session 3 (2026-08-26)**: this sensor holds `quality: "GOOD"` down to 250ms (20x faster than the original 5000ms default) but times out completely at 200ms/100ms. See the Session 3 log above for the exact numbers and the device-level-vs-datapoint-level `polling_interval_ms` gotcha. Currently deployed at 250ms.
- [ ] **RTC**: `GET /api/time` shows `rtc_status: true` (currently always `false` — no hardware exists to make it otherwise until now). Deferred to a future session — no RTC module available this session (asked; none to connect yet).
- [x]/[ ] **Network IP** (native/systemd path only, console access ready per §0): `Current()` confirmed in Session 1 **and re-confirmed in Session 2 on the fresh SD card** (`"supported": true`, real `wlan0`/IP/gateway/DNS, unchanged) — **still open**: actually `Apply` a *safe* test change and deliberately let one change time out unconfirmed to verify it actually reverts. Deliberately not attempted in Session 2 either — no physical console access was lined up beforehand, and this needs a genuine risk/confirm step with the user before trying it (locking out the only remote path in would mean a third SD-card-adjacent recovery trip).
- [x] **MQTT/Store & Forward against the real internal broker — verified in Session 2** (`mqtt.nxge.co:1883`, plain TCP, no auth). `configs/config.yaml` updated: `forwarder.transport: mqtt`, `mqtt.broker_url: tcp://mqtt.nxge.co:1883`. Ran the repo's `cmd/server-sim` (MQTT consumer mode) on the Pi itself as a stand-in "Internal Server" purely to close the loop — not a production component. Sequence:
  - Initial connect: broker TCP/CONNACK fine, but publishes timed out for a couple minutes after the preceding `systemctl restart`. Raw `mosquitto_pub -i GW001 ...` against the broker worked instantly, confirming the broker itself wasn't the problem — the gateway's own connection was in a stuck state, tied to the same `client_id: GW001` restarting without a clean prior disconnect. It recovered right after that raw publish, but the later partition test (below) showed the same class of stuck-connection state self-heals on its own via paho's keepalive-based detection within ~30-70s (bounded by `keepalive_seconds: 30`) — so this was very plausibly just normal recovery latency that got short-circuited by impatience, not a distinct bug. **Worth knowing for next time**: give MQTT reconnect up to ~1.5x `keepalive_seconds` before assuming something's actually wrong.
  - Once healthy: full backlog (107 records accumulated during the above) drained cleanly — `server-sim`'s `/received` showed 107 unique entries, **zero duplicates**.
  - Deliberate partition test (`iptables -A OUTPUT -d <broker-ip> -p tcp --dport 1883 -j DROP`): backlog grew as expected (`server_connected: false`, retry_count climbing), **RTU polling completely unaffected** (`Test Read` kept returning `GOOD` throughout) — Rule 1 (acquisition never depends on the server) holds against a real MQTT broker, not just the dev HTTP adapter.
  - Restore (`iptables -D ...`): client took ~30-70s to notice the dead connection (`"pingresp not received, disconnecting"` — bounded by `keepalive_seconds: 30`, not a bug) then auto-reconnected and drained the backlog to zero. **Final tally: 151 total entries received, all unique, sequence IDs 1-151 with no gaps** — at-least-once delivery + dedup confirmed correct through a real network partition, not just the dev Docker/mosquitto setup.
  - `server-sim` stopped after verification (it's explicitly dev-only per its own doc comment, never meant to stay running). **With it stopped, the backlog is growing again** (nothing real is consuming/acking on `mqtt.nxge.co` yet) — expected and harmless by design (Rule 1, storage still at ~21%), but `mqtt.nxge.co` needs a real consumer app before this gateway's data is actually useful in production, not just safely queued.
  - **Update from Session 3 (2026-08-26)**: exactly this gap — no real consumer — is what caused a real 4.5-hour stuck backlog next session (see the Session 3 log above). Closed properly this time with a real Internal Server (`internal-server/`, §30 of the design doc) rather than leaving `server-sim` running as a stand-in; still not on an always-on host, though (see HANDOFF.md's "Next").

**Unplanned mid-session reboot**: the Pi rebooted on its own partway through this session (observed as a ~60s "Destination host unreachable" from the LAN gateway, then back up with `uptime` showing a fresh boot). Cause unconfirmed — possibly a power blip while physically connecting the USB-RS485 adapter. `nxiiot-gateway.service` survived fine (`enable`d under systemd, came back automatically). **`npm run dev` did not** — it's a manually-backgrounded process, not a systemd unit, so it needed restarting by hand after the reboot. Worth deciding later whether the dev-loop frontend needs a systemd unit too, or whether that's acceptable to redo by hand each session (§7 already treats the frontend as "run separately").
- [x] Settings save + restart (§29) actually reaches back up under `systemctl`/`Restart=always` on the native path — **verified in Session 2**: `PUT /api/config/settings` → `{"restarting":true,"saved":true}` → PID changed `6912`→`13286`, service back `active (running)` within ~8s, API responding again.

## 7. Continuing development on the Pi

Two reasonable workflows, pick based on how tomorrow goes:

- **Pi as the primary dev machine going forward**: commit and push directly from the Pi (`git config user.name`/`user.email` will need setting there too, same as this session's Windows machine did). Natural if most remaining work is inherently hardware-dependent (RTU register mapping for real meters, RTC/network tuning).
- **Windows stays primary, Pi is a hardware test bench**: keep writing code on Windows, `git pull` on the Pi to test against real hardware, push fixes back from Windows. Natural if most remaining work is still general application logic (V2/V3 roadmap items — §25) that doesn't need the Pi at all.

Either way, `git status` before doing anything destructive, same discipline as the rest of this project.

## 8. Rollback / troubleshooting quick reference

- **Locked out after a network-IP change**: physical console (§0) → `nmcli con mod <connection> ipv4.method auto && nmcli con up <connection>` to force DHCP back, or fix the static values directly. If the auto-revert (45s) already fired, this shouldn't be needed — but verify it actually happened rather than assuming.
- **systemd service won't start**: `journalctl -u nxiiot-gateway -n 100 --no-pager` first. Common causes for a fresh Pi: wrong `configs/config.yaml` path in the unit's `ExecStart`, missing `migrations/` next to the binary, or a permissions issue on `/dev/rtc0`/`/dev/ttyUSB0`.
- **Docker on Pi behaving differently than Windows/Docker Desktop**: expected in some cases — no Windows-bind-mount-DNS-cache class of issues (those were specific to this project's Windows/Docker Desktop dev environment, see `HANDOFF.md`'s bugs list, item 4), but ARM image availability/build times differ; `docker compose build --progress=plain` if a build fails mysteriously.
