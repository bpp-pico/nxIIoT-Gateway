# Deploy Plan — Raspberry Pi

Plan for installing on a real Raspberry Pi and continuing development there. Written before that session happens — treat every unchecked box below as genuinely unverified, not "should work." See [HANDOFF.md](HANDOFF.md) for what's been verified so far (all of it on Windows/Docker, none on real ARM hardware) and [industrial_iot_gateway_handoff_dev_plan.md](industrial_iot_gateway_handoff_dev_plan.md) §29 for why the RTC and network-IP code specifically has never run for real.

## Why this trip matters

Three things in this codebase are written and unit-tested but have **never executed against real hardware**, because none of it exists in the Windows/Docker dev environment:

1. **Modbus RTU full read round-trip** against a responding slave device (only the serial *connect* was verified, against a real USB-RS485 adapter, back in Phase 1 — no slave was available then).
2. **Hardware RTC** (`/dev/rtc0` ioctls, `internal/time/rtc_linux.go`) — cross-compiles clean, zero real runs.
3. **Host network IP config** (`internal/netconfig`, `nmcli`) — real parsing logic, tested against canned `nmcli` output, zero runs against an actual NetworkManager instance.

Tomorrow's job is closing those three gaps for real, plus getting a working dev loop running directly on the Pi.

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

- [ ] Gateway starts cleanly on the Pi (Docker or native), `GET /api/system` responds, `uname -m`/`go version` noted for the record.
- [ ] **RTU round-trip**: add the real device + a data point matching a known register on the meter, confirm `quality: "GOOD"` and a sane decoded value via Test Read or the Devices page — this is the single most important box on this list, since it's never been checked at all.
- [ ] RTU failure paths against the real device: unplug the adapter mid-poll (expect `DEVICE_OFFLINE`), and if you can force a bad response somehow, confirm `CRC_ERROR` shows up for real (the classification logic is tested, actually triggering it from real hardware is not).
- [ ] **RTC**: `GET /api/time` shows `rtc_status: true` (currently always `false` — no hardware exists to make it otherwise until now). Pull NTP connectivity and confirm `time_quality` degrades to `RTC` using the real chip, not the fake one from `service_test.go`. Power-cycle the Pi with NTP unreachable and confirm the RTC still has a sane time (not reset to epoch).
- [ ] **Network IP** (native/systemd path only, console access ready per §0): `GET /api/system/network` should now report `"supported": true` with real interface/address data instead of `{"supported": false}`. Apply a *safe* test change first (e.g. a static IP still on the same subnet you're already reachable at) before trying anything that changes subnets. Deliberately let one change time out unconfirmed and verify it actually reverts.
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
