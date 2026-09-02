-- Replaces the old ticker + two-level polling_interval_ms gating (device
-- AND datapoint each had their own, independently gated — a documented
-- footgun, see MEMORY.md) with a continuous round-robin scan per
-- connection: devices are polled back-to-back, each device's datapoints
-- batched into as few Modbus block reads as possible, with one shared
-- delay before moving to the next device (models RS-485 bus settle/
-- turnaround time, not a per-datapoint or per-device sample rate).
--
-- Real production evidence motivating this: a device with 4 datapoints
-- that are actually just 2 distinct registers read under 4 tag names
-- (temperature@1, humidity@2, Temp_02@1, Humidity_02@2, all function code
-- 4) cost 4 sequential round-trips (~1.1s) under the old one-read-per-
-- datapoint model — block reads collapse that to 1 request.
--
-- No table rebuild needed: neither `device` nor `datapoint` has a CHECK/
-- UNIQUE/FK constraint referencing polling_interval_ms (unlike 0004's
-- device.protocol, which forced a rebuild), so plain ALTER TABLE
-- ADD/DROP COLUMN is safe.

ALTER TABLE connection ADD COLUMN next_device_delay_ms INTEGER NOT NULL DEFAULT 250;
ALTER TABLE datapoint DROP COLUMN polling_interval_ms;
ALTER TABLE device DROP COLUMN polling_interval_ms;
