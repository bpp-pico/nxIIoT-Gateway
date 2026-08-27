# Memory — operational mistakes and corrections

Log entries here whenever a pattern failure or operational mistake is identified, per CLAUDE.md's LEARNING CAPTURE rule. Three fields per entry: what happened / root cause / correct behavior. Newest entries at the top.

---

**Web UI was only ever started manually over SSH, so it died with the session and stayed dead (2026-08-27):**
- what: `http://192.168.99.84:5173` was unreachable — port not listening at all — even though DEPLOY_PLAN.md documented it working in an earlier session
- root cause: every prior session started the Vite dev server as a foreground/backgrounded shell command (`npm run dev -- --host 0.0.0.0`) inside an interactive SSH session, with no systemd unit; once that SSH session ended, the process was gone, and nothing was watching for it
- correct: any process meant to survive between sessions on the Pi needs a systemd unit (mirror `gateway/deploy/nxiiot-gateway.service`'s pattern), not a manually-launched shell command — added `web/deploy/nxiiot-gateway-web.service` to close this specific gap. Before assuming a previously-working dev process is still running, check `systemctl status`/`ss -tlnp` rather than trusting old session notes.

**`internal-server` Docker containers silently exited despite `restart: unless-stopped`, causing a ~20h MQTT ack outage (2026-08-27):**
- what: routine SSH health check on the Pi found 131 `batch send failed, will retry` / `timed out waiting for ack` warnings in just the preceding 6 hours, continuous and back-to-back — same symptom as the 2026-08-26 "stuck MQTT client" incident in the entry below
- root cause: `docker ps -a` on the Windows dev host showed `internal-server-internal-server-1` and `internal-server-postgres-1` both `Exited` ~20h earlier, even though `internal-server/docker-compose.yml` sets `restart: unless-stopped` on both — something (leading suspect: a Docker Desktop restart, which drops `unless-stopped` containers because Docker Desktop itself doesn't survive a host reboot as a running daemon) stopped them and nothing brought them back since there's no host-level supervisor for Docker Desktop itself. Confirmed via the direct-probe pattern from the entry below (`docker ps -a` first, not client-side log archaeology).
- correct: `restart: unless-stopped` only protects against the *container* dying — it does not survive the Docker *daemon*/Desktop app itself not being running. On a dev-laptop host (not systemd-managed like the Pi), always verify the daemon/containers are actually up after any host restart, sleep, or Docker Desktop update. This is the concrete argument for the open Todo item "give `internal-server/` a permanent always-on host" in spec.md — until then, this will recur silently (Rule 1 means the Pi just queues harmlessly, so nothing pages anyone).

**`pkill -f` killed its own SSH session (2026-08-26):**
- what: ran `pkill -f '/tmp/server-sim'` over SSH to stop a background process; the SSH connection itself died (exit 255) instead of just the target process
- root cause: `pkill -f` matches against the full command line of every process, including the invoking shell's own command line — which, since it literally contained the string `/tmp/server-sim` as a `pkill` argument, matched and killed itself (and the shell running it)
- correct: use `pkill -x <exact-process-name>` (matches only the process name, not the full command line) when the target's path/args might overlap with the command used to kill it — or `pgrep -f ... | grep -v $$` style exclusion if `-f` is required

**Diagnosed a "stuck MQTT client" that was actually "no consumer running" (2026-08-26):**
- what: spent significant effort suspecting a paho/gateway client-side bug (stuck connection state, missing subscribe-retry) for a 4.5-hour-stuck backlog, before confirming the real cause: nothing was subscribed to the data topic to publish acks back at all
- root cause: jumped to a plausible-sounding client-bug hypothesis from log evidence (one real subscribe failure was in the logs) without first checking the simplest explanation — was anything actually consuming and acking on the broker at all
- correct: before diagnosing a "client won't receive X" symptom, verify with a direct, minimal probe (`mosquitto_sub` on the exact topic) whether the data is even being produced/consumed by anyone — cheaper than reasoning about client internals, and rules out the trivial explanation first

**RTU `polling_interval_ms` exists at two levels that must both be changed (2026-08-26):**
- what: changed only the device-level `polling_interval_ms` (100ms) expecting faster reads; actual read cadence stayed at 5000ms with no effect
- root cause: `gateway/internal/acquisition/poller.go` gates actual reads per-datapoint (`dpInterval`) independently of the device-level ticker — the device value only controls how often the poll loop *checks*, not how often each datapoint is actually *read*
- correct: when tuning RTU polling speed, change `polling_interval_ms` on both the device (`PUT /api/devices/{id}`) and every one of its data points (`PUT /api/datapoints/{id}`) — changing only one has no visible effect and looks like the change silently failed

**Pushed a config change to real hardware without a step-down test (2026-08-26):**
- what: jumped straight to 200ms/100ms RTU polling interval on a live production sensor; every reading came back `TIMEOUT` until reverted
- root cause: didn't test incrementally (5000 → 1000 → 500 → 250 → 200) against real, response-time-limited hardware before picking an aggressive target value
- correct: on real hardware with unknown response-time limits, step down gradually and check `quality`/device status after each step, rather than jumping to the target value directly — cheap to revert, but still wastes a cycle of bad data and a diagnosis detour every time this is skipped
