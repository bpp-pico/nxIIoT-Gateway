package main

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

type store struct {
	pool *pgxpool.Pool
}

func newStore(ctx context.Context, dsn string) (*store, error) {
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		return nil, fmt.Errorf("pgxpool.New: %w", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, fmt.Errorf("ping: %w", err)
	}
	return &store{pool: pool}, nil
}

func (s *store) Close() { s.pool.Close() }

func (s *store) Ping(ctx context.Context) error { return s.pool.Ping(ctx) }

const schema = `
CREATE TABLE IF NOT EXISTS readings (
	id BIGSERIAL PRIMARY KEY,
	gateway_id TEXT NOT NULL,
	sequence_id BIGINT NOT NULL,
	device_id BIGINT NOT NULL,
	datapoint_id BIGINT NOT NULL,
	value DOUBLE PRECISION,
	quality TEXT NOT NULL,
	event_timestamp TIMESTAMPTZ NOT NULL,
	priority TEXT NOT NULL,
	received_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	UNIQUE (gateway_id, sequence_id)
);
CREATE INDEX IF NOT EXISTS idx_readings_gateway_time ON readings (gateway_id, event_timestamp DESC);
`

// idx_readings_latest supports the DISTINCT ON (gateway_id, device_id,
// datapoint_id) ... ORDER BY ..., received_at DESC query in latest() below.
// Without it, that query has no index to satisfy its ORDER BY and falls
// back to a full sort of the whole table, which on production data volume
// (millions of rows) blows past the 5s request timeout every time
// (observed live: "query latest failed" error="timeout: context deadline
// exceeded"). Built CONCURRENTLY, and as its own statement outside the
// `schema` multi-statement Exec, because CREATE INDEX CONCURRENTLY cannot
// run inside a transaction block — this avoids taking a write lock on the
// live readings table while the gateway is still ingesting.
const latestIndexSQL = `CREATE INDEX CONCURRENTLY IF NOT EXISTS idx_readings_latest ON readings (gateway_id, device_id, datapoint_id, received_at DESC)`

func (s *store) migrate(ctx context.Context) error {
	if _, err := s.pool.Exec(ctx, schema); err != nil {
		return err
	}
	_, err := s.pool.Exec(ctx, latestIndexSQL)
	return err
}

// ingest upserts entries with gateway_id+sequence_id as the idempotency
// key (mirrors the gateway's own dedup contract, §6/Rule 6 in the design
// doc) so a retried batch after a lost ack never double-counts.
func (s *store) ingest(ctx context.Context, entries []wireEntry) (accepted, duplicates int, err error) {
	for _, e := range entries {
		ts, perr := time.Parse(time.RFC3339Nano, e.EventTimestamp)
		if perr != nil {
			ts = time.Now().UTC()
		}
		tag, terr := s.pool.Exec(ctx, `
			INSERT INTO readings (gateway_id, sequence_id, device_id, datapoint_id, value, quality, event_timestamp, priority)
			VALUES ($1,$2,$3,$4,$5,$6,$7,$8)
			ON CONFLICT (gateway_id, sequence_id) DO NOTHING
		`, e.GatewayID, e.SequenceID, e.DeviceID, e.DatapointID, e.Value, e.Quality, ts, e.Priority)
		if terr != nil {
			return accepted, duplicates, terr
		}
		if tag.RowsAffected() > 0 {
			accepted++
		} else {
			duplicates++
		}
	}
	return accepted, duplicates, nil
}

type readingRow struct {
	GatewayID      string    `json:"gateway_id"`
	SequenceID     int64     `json:"sequence_id"`
	DeviceID       int64     `json:"device_id"`
	DatapointID    int64     `json:"datapoint_id"`
	Value          *float64  `json:"value"`
	Quality        string    `json:"quality"`
	EventTimestamp time.Time `json:"event_timestamp"`
	Priority       string    `json:"priority"`
	ReceivedAt     time.Time `json:"received_at"`
}

// recent returns readings newest-first, optionally narrowed by gateway and/or
// device+datapoint (nil deviceID/datapointID means "any") and by a minimum
// received_at (nil since means "no lower bound").
func (s *store) recent(ctx context.Context, gatewayID string, deviceID, datapointID *int64, since *time.Time, limit int) ([]readingRow, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT gateway_id, sequence_id, device_id, datapoint_id, value, quality, event_timestamp, priority, received_at
		FROM readings
		WHERE ($1 = '' OR gateway_id = $1)
		  AND ($2::bigint IS NULL OR device_id = $2)
		  AND ($3::bigint IS NULL OR datapoint_id = $3)
		  AND ($4::timestamptz IS NULL OR received_at >= $4)
		ORDER BY received_at DESC
		LIMIT $5
	`, gatewayID, deviceID, datapointID, since, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []readingRow
	for rows.Next() {
		var r readingRow
		if err := rows.Scan(&r.GatewayID, &r.SequenceID, &r.DeviceID, &r.DatapointID, &r.Value, &r.Quality, &r.EventTimestamp, &r.Priority, &r.ReceivedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

// latest returns the single newest reading for every (gateway_id, device_id,
// datapoint_id) combination seen so far — the dashboard's stat-tile feed.
func (s *store) latest(ctx context.Context) ([]readingRow, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT DISTINCT ON (gateway_id, device_id, datapoint_id)
			gateway_id, sequence_id, device_id, datapoint_id, value, quality, event_timestamp, priority, received_at
		FROM readings
		ORDER BY gateway_id, device_id, datapoint_id, received_at DESC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []readingRow
	for rows.Next() {
		var r readingRow
		if err := rows.Scan(&r.GatewayID, &r.SequenceID, &r.DeviceID, &r.DatapointID, &r.Value, &r.Quality, &r.EventTimestamp, &r.Priority, &r.ReceivedAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

type gatewayStat struct {
	GatewayID    string    `json:"gateway_id"`
	Count        int64     `json:"count"`
	LastReceived time.Time `json:"last_received"`
	// Online is a last-seen-data heuristic, not a live connection check:
	// true when LastReceived is within onlineThreshold of now. A gateway
	// that's genuinely connected but has nothing to report (e.g. every
	// device disabled) would show as offline here — accepted tradeoff for
	// not needing any gateway-side change (see spec.md's design note).
	Online bool `json:"online"`
}

func (s *store) stats(ctx context.Context, onlineThreshold time.Duration) ([]gatewayStat, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT gateway_id, count(*), max(received_at)
		FROM readings
		GROUP BY gateway_id
		ORDER BY gateway_id
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []gatewayStat
	for rows.Next() {
		var g gatewayStat
		if err := rows.Scan(&g.GatewayID, &g.Count, &g.LastReceived); err != nil {
			return nil, err
		}
		g.Online = time.Since(g.LastReceived) <= onlineThreshold
		out = append(out, g)
	}
	return out, rows.Err()
}
