// Package queue persists acquired readings to the data_queue table — the
// gateway's local source of truth (Design Principle #2/#3). Data must be
// persisted here before being forwarded (Rule 2); the Store & Forward
// worker (Phase 4) will read PENDING rows from this table and mark them
// SENT/FAILED, but insertion is independent of any of that.
package queue

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

const timeLayout = "2006-01-02T15:04:05.000Z"

type Entry struct {
	ID             int64
	GatewayID      string
	SequenceID     int64
	DeviceID       int64
	DatapointID    int64
	Value          *float64
	Quality        string
	EventTimestamp time.Time
	Priority       string
}

type Repository struct {
	db *sql.DB
}

func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

// EnsureGateway upserts the gateway's own row, which holds the persistent
// sequence counter (Rule 6: "sequence must persist across Gateway
// restarts"). Call once at startup.
func (r *Repository) EnsureGateway(ctx context.Context, id, name string) error {
	_, err := r.db.ExecContext(ctx, `
		INSERT INTO gateway (id, name) VALUES (?, ?)
		ON CONFLICT(id) DO UPDATE SET name = excluded.name`,
		id, name)
	return err
}

// Insert assigns the next sequence_id for gatewayID and persists the
// reading as a PENDING data_queue row, atomically, in a single transaction.
// gateway_id + sequence_id is the idempotency key the server uses to detect
// duplicates under at-least-once delivery (Rule 6/7).
func (r *Repository) Insert(ctx context.Context, e Entry) (Entry, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return Entry{}, err
	}
	defer tx.Rollback()

	var seq int64
	err = tx.QueryRowContext(ctx,
		`UPDATE gateway SET last_sequence = last_sequence + 1 WHERE id = ? RETURNING last_sequence`,
		e.GatewayID,
	).Scan(&seq)
	if err != nil {
		return Entry{}, fmt.Errorf("assign sequence: %w", err)
	}
	e.SequenceID = seq

	res, err := tx.ExecContext(ctx, `
		INSERT INTO data_queue (gateway_id, sequence_id, device_id, datapoint_id, value, quality, event_timestamp, status, priority)
		VALUES (?, ?, ?, ?, ?, ?, ?, 'PENDING', ?)`,
		e.GatewayID, e.SequenceID, e.DeviceID, e.DatapointID, e.Value, e.Quality,
		e.EventTimestamp.UTC().Format(timeLayout), priorityOrDefault(e.Priority))
	if err != nil {
		return Entry{}, fmt.Errorf("insert data_queue row: %w", err)
	}
	e.ID, err = res.LastInsertId()
	if err != nil {
		return Entry{}, err
	}

	if err := tx.Commit(); err != nil {
		return Entry{}, err
	}
	return e, nil
}

// DeleteSentOlderThan removes SENT rows whose sent_at predates cutoff
// (retention policy, §9/§17). Rows still PENDING/SENDING/FAILED are never
// touched here regardless of age — only Store & Forward (Phase 4) storage
// pressure policy may evict those.
func (r *Repository) DeleteSentOlderThan(ctx context.Context, cutoff time.Time) (int64, error) {
	res, err := r.db.ExecContext(ctx,
		`DELETE FROM data_queue WHERE status = 'SENT' AND sent_at IS NOT NULL AND sent_at < ?`,
		cutoff.UTC().Format(timeLayout))
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

func priorityOrDefault(p string) string {
	if p == "" {
		return "NORMAL"
	}
	return p
}
