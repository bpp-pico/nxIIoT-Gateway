# Memory — operational mistakes and corrections

Log entries here whenever a pattern failure or operational mistake is identified, per CLAUDE.md's LEARNING CAPTURE rule. Three fields per entry: what happened / root cause / correct behavior. Newest entries at the top.

---

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
