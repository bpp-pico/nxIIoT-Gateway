-- FR-001 requires baud rate, data bits, parity, and stop bits to be
-- configurable per RTU device; these were missing from the initial schema.

ALTER TABLE device ADD COLUMN baud_rate INTEGER NOT NULL DEFAULT 9600;
ALTER TABLE device ADD COLUMN data_bits INTEGER NOT NULL DEFAULT 8;
ALTER TABLE device ADD COLUMN parity TEXT NOT NULL DEFAULT 'N';
ALTER TABLE device ADD COLUMN stop_bits INTEGER NOT NULL DEFAULT 1;
