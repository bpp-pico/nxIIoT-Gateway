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

- [x] Gateway starts cleanly on the Pi (native/systemd), `GET /api/system` responds, `uname -m`/`go version` noted for the record — done in Session 1, needs redoing on the new SD card (nothing here survives the swap).
- [ ] **RTU round-trip**: add the real device + a data point matching a known register on the meter, confirm `quality: "GOOD"` and a sane decoded value via Test Read or the Devices page — this is the single most important box on this list, since it's never been checked at all.
- [ ] RTU failure paths against the real device: unplug the adapter mid-poll (expect `DEVICE_OFFLINE`), and if you can force a bad response somehow, confirm `CRC_ERROR` shows up for real (the classification logic is tested, actually triggering it from real hardware is not).
- [ ] **RTC**: `GET /api/time` shows `rtc_status: true` (currently always `false` — no hardware exists to make it otherwise until now). Pull NTP connectivity and confirm `time_quality` degrades to `RTC` using the real chip, not the fake one from `service_test.go`. Power-cycle the Pi with NTP unreachable and confirm the RTC still has a sane time (not reset to epoch).
- [x]/[ ] **Network IP** (native/systemd path only, console access ready per §0): `Current()` confirmed in Session 1 (`"supported": true`, real `wlan0`/IP/gateway/DNS) — **still open**: actually `Apply` a *safe* test change (e.g. a static IP still on the same subnet you're already reachable at) before trying anything that changes subnets, and deliberately let one change time out unconfirmed to verify it actually reverts. Redo the `Current()` check too on the new card, don't just assume it still works.
- [ ] MQTT/Store & Forward against the real internal broker (not `mosquitto` dev container) — same verification style as Phase 5: stop reachability to the broker, confirm the backlog grows and Modbus polling is unaffected, restore it, confirm the backlog drains with no duplicates.
- [ ] Settings save + restart (§29) actually reaches back up under `systemctl`/`Restart=always` on the native path, not just Docker Compose's `restart: unless-stopped` (only the latter has been exercised).

## 7. Continuing development on the Pi

Two reasonable workflows, pick based on how tomorrow goes:

- **Pi as the primary dev machine going forward**: commit and push directly from the Pi (`git config user.name`/`user.email` will need setting there too, same as this session's Windows machine did). Natural if most remaining work is inherently hardware-dependent (RTU register mapping for real meters, RTC/network tuning).
- **Windows stays primary, Pi is a hardware test bench**: keep writing code on Windows, `git pull` on the Pi to test against real hardware, push fixes back from Windows. Natural if most remaining work is still general application logic (V2/V3 roadmap items — §25) that doesn't need the Pi at all.

Either way, `git status` before doing anything destructive, same discipline as the rest of this project.

## 8. Rollback / troubleshooting quick reference

- **Locked out after a network-IP change**: physical console (§0) → `nmcli con mod <connection> ipv4.method auto && nmcli con up <connection>` to force DHCP back, or fix the static values directly. If the auto-revert (45s) already fired, this shouldn't be needed — but verify it actually happened rather than assuming.
- **systemd service won't start**: `journalctl -u nxiiot-gateway -n 100 --no-pager` first. Common causes for a fresh Pi: wrong `configs/config.yaml` path in the unit's `ExecStart`, missing `migrations/` next to the binary, or a permissions issue on `/dev/rtc0`/`/dev/ttyUSB0`.
- **Docker on Pi behaving differently than Windows/Docker Desktop**: expected in some cases — no Windows-bind-mount-DNS-cache class of issues (those were specific to this project's Windows/Docker Desktop dev environment, see `HANDOFF.md`'s bugs list, item 4), but ARM image availability/build times differ; `docker compose build --progress=plain` if a build fails mysteriously.
