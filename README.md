# nxIIoT Gateway

Industrial IoT Gateway for Modbus RTU/TCP acquisition, local store & forward, and MQTT delivery to an internal server. See [industrial_iot_gateway_handoff_dev_plan.md](industrial_iot_gateway_handoff_dev_plan.md) for the full design and phased development plan.

## Stack

- Backend: Go (`gateway/`)
- Frontend: React + TypeScript, Vite (`web/`)
- Database: SQLite (WAL mode)
- Protocols: Modbus RTU/TCP, MQTT

## Development

Requires Docker.

```bash
docker compose up --build
```

- Backend API: http://localhost:8080/api/system
- Frontend: http://localhost:5173
- Modbus simulator: TCP port 1502 on the host (`502` inside the Docker network)
- Fake "Internal Server" (Store & Forward target): TCP port 9000 on the host

Backend hot-reloads via [air](https://github.com/air-verse/air); frontend hot-reloads via Vite. Both containers mount the local source tree.

With `SEED_DEMO_DEVICE=true` (set by default in `docker-compose.yml`), the gateway seeds one demo Modbus TCP device (`PM001`, pointed at `modbus-sim`) on first boot if the `device` table is empty. Watch it read live values:

```bash
docker compose logs -f gateway
```

Manage devices and data points at http://localhost:5173 → **Devices** tab: add/edit/delete devices, toggle enabled, Test Connection, and click a device name to manage its data points (add/edit/delete, toggle enabled, Test Read). Changes take effect immediately — the acquisition engine reloads its polling set on every change, no restart needed.

Every acquired reading is persisted to `data_queue` and then forwarded to the fake server (`server-sim`) in batches. Check `GET /api/store-forward/status` for pending/sending counts, retry count, disk usage %, and server connectivity — or watch `curl http://localhost:9000/received` to see what the "server" has accepted. Stop the `server-sim` container to see the backlog grow while acquisition keeps running (Rule 1); restart it to see the backlog drain with no duplicates.

For RTU devices, the "Serial Interface" field shows a dropdown of ports detected on the gateway host (`GET /api/system/serial-ports`). In the Docker dev setup the list is normally empty — a container can't see a host's serial ports unless one is explicitly passed through (see the commented `devices:` example on the `gateway` service in `docker-compose.yml`) — so the field falls back to manual entry (e.g. `/dev/ttyUSB0`).

### Testing RTU against real hardware on Windows

USB-to-RS485 adapters attach as a native Windows COM port, which a Docker container (even on Linux-container/WSL2 backends) cannot see without extra passthrough tooling (`usbipd-win`). The simplest way to test the real port is to run the gateway natively instead of in Docker:

```powershell
go build -o gateway.exe ./cmd/gateway
./gateway.exe -config configs/config.yaml -migrations migrations
```

and the frontend natively too (`cd web && npm run dev` — its Vite proxy already defaults to `http://localhost:8080` when `GATEWAY_API_URL` isn't set, which is only set inside `docker-compose.yml`). Stop the Dockerized `gateway`/`web` containers first to free ports 8080/5173; `modbus-sim` can keep running in Docker (native gateway reaches it at `localhost:1502`). This is how RTU was validated against a real USB-to-RS485 (CH340) adapter — see Phase 1's status below.

## Project layout

```
gateway/    Go backend (cmd/gateway, cmd/modbus-sim, cmd/server-sim, internal/, migrations/, configs/)
web/        React + TypeScript frontend
```

## Status

- **Phase 0** — project setup: done.
- **Phase 1** — Modbus engine (RTU/TCP clients, data type/byte-order decoding, scaling, quality handling, polling scheduler): done. TCP verified against `cmd/modbus-sim`; RTU's serial connect verified against a real USB-to-RS485 (CH340) adapter via a native Windows build — full read round-trip against a responding RTU slave device still untested.
- **Phase 2** — Device/Data Point management (REST API, Web UI, live config reload, Test Connection/Read, device status, COM port dropdown): done. Backend verified end-to-end via curl; frontend type-checked and built cleanly.
- **Phase 3** — Persistent Data Storage (every Reading, regardless of quality, is written to `data_queue` with a per-gateway sequence ID, event/created timestamps, and priority; retention sweep for old SENT rows): done. Verified end-to-end against a live device, including sequence persistence across a restart and a real dropped-connection scenario (which surfaced and fixed a real Windows-vs-Linux error-message bug in quality classification).
- **Phase 4** — Store & Forward (queue state machine, exponential backoff, batching, crash recovery, server connectivity detection, storage threshold/full policy): done. Verified live against all three of §23's scenarios — forwarded a real historical backlog on startup, confirmed acquisition survives a dead server while the backlog grows, and confirmed the backlog drains with zero duplicates on recovery. `cmd/server-sim` is the dev/test fake server (Phase 5 adds the real MQTT adapter behind the same `forwarder.Adapter` interface).
- **Phase 5** onward — not started. See [industrial_iot_gateway_handoff_dev_plan.md](industrial_iot_gateway_handoff_dev_plan.md) for the full plan.
