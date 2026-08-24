package queue

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
)

// DispatchEntry is a data_queue row as read back for forwarding — a
// superset of Entry with the forwarding-specific bookkeeping fields.
type DispatchEntry struct {
	Entry
	Status      string
	RetryCount  int
	LastError   string
	NextAttempt time.Time
}

// priority ordering: CRITICAL first, LOW last (§17 "protect higher
// priority data first"), then FIFO by sequence_id within a priority so
// historical backlog still drains in order (needed for server-side
// idempotency/ordering, not just fairness).
const priorityOrderSQL = `CASE priority
	WHEN 'CRITICAL' THEN 0
	WHEN 'HIGH' THEN 1
	WHEN 'NORMAL' THEN 2
	WHEN 'LOW' THEN 3
	ELSE 4 END`

// FetchBatch selects up to limit PENDING rows eligible for a retry attempt
// right now (§9.4 backoff), highest priority and oldest first, and
// atomically marks them SENDING so a concurrent caller won't pick them up
// twice.
func (r *Repository) FetchBatch(ctx context.Context, limit int) ([]DispatchEntry, error) {
	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	rows, err := tx.QueryContext(ctx, `
		SELECT id, gateway_id, sequence_id, device_id, datapoint_id, value, quality,
		       event_timestamp, priority, retry_count
		FROM data_queue
		WHERE status = 'PENDING' AND next_attempt_at <= ?
		ORDER BY `+priorityOrderSQL+`, sequence_id ASC
		LIMIT ?`,
		time.Now().UTC().Format(timeLayout), limit)
	if err != nil {
		return nil, fmt.Errorf("select batch: %w", err)
	}

	var batch []DispatchEntry
	var ids []int64
	for rows.Next() {
		var e DispatchEntry
		var eventTS string
		if err := rows.Scan(&e.ID, &e.GatewayID, &e.SequenceID, &e.DeviceID, &e.DatapointID,
			&e.Value, &e.Quality, &eventTS, &e.Priority, &e.RetryCount); err != nil {
			rows.Close()
			return nil, fmt.Errorf("scan batch row: %w", err)
		}
		e.EventTimestamp, _ = time.Parse(timeLayout, eventTS)
		batch = append(batch, e)
		ids = append(ids, e.ID)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if len(batch) == 0 {
		return nil, nil
	}

	if _, err := tx.ExecContext(ctx,
		`UPDATE data_queue SET status = 'SENDING' WHERE id IN (`+placeholders(len(ids))+`)`,
		toArgs(ids)...); err != nil {
		return nil, fmt.Errorf("mark SENDING: %w", err)
	}

	if err := tx.Commit(); err != nil {
		return nil, err
	}
	for i := range batch {
		batch[i].Status = "SENDING"
	}
	return batch, nil
}

// MarkSent transitions rows to SENT after a server ACK (§9.1, Rule 9).
func (r *Repository) MarkSent(ctx context.Context, ids []int64) error {
	if len(ids) == 0 {
		return nil
	}
	_, err := r.db.ExecContext(ctx,
		`UPDATE data_queue SET status = 'SENT', sent_at = ? WHERE id IN (`+placeholders(len(ids))+`)`,
		append([]any{time.Now().UTC().Format(timeLayout)}, toArgs(ids)...)...)
	return err
}

// MarkFailed transitions rows back to PENDING (§9.2: SENDING -> FAILED ->
// PENDING) after a failed send, incrementing retry_count and scheduling
// the next attempt per the exponential backoff in §9.4.
func (r *Repository) MarkFailed(ctx context.Context, ids []int64, sendErr string) error {
	if len(ids) == 0 {
		return nil
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()

	stmt, err := tx.PrepareContext(ctx, `
		UPDATE data_queue
		SET status = 'PENDING', retry_count = retry_count + 1, last_error = ?, next_attempt_at = ?
		WHERE id = ?`)
	if err != nil {
		return err
	}
	defer stmt.Close()

	for _, id := range ids {
		var retryCount int
		if err := tx.QueryRowContext(ctx, `SELECT retry_count FROM data_queue WHERE id = ?`, id).Scan(&retryCount); err != nil {
			return fmt.Errorf("read retry_count for row %d: %w", id, err)
		}
		nextAttempt := time.Now().UTC().Add(BackoffDuration(retryCount + 1))
		if _, err := stmt.ExecContext(ctx, sendErr, nextAttempt.Format(timeLayout), id); err != nil {
			return fmt.Errorf("mark row %d failed: %w", id, err)
		}
	}

	return tx.Commit()
}

// RecoverSendingToPending resets any row still SENDING back to PENDING —
// run once at startup (§9.3): a gateway restart mid-transmission must not
// leave rows stuck, since no ACK could possibly still arrive for them.
func (r *Repository) RecoverSendingToPending(ctx context.Context) (int64, error) {
	res, err := r.db.ExecContext(ctx, `UPDATE data_queue SET status = 'PENDING' WHERE status = 'SENDING'`)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// Stats summarizes queue state for the Store & Forward dashboard (§16).
type Stats struct {
	PendingCount  int64
	SendingCount  int64
	OldestPending *time.Time
	NewestPending *time.Time
	TotalRetries  int64
}

func (r *Repository) Stats(ctx context.Context) (Stats, error) {
	var s Stats
	var oldest, newest sql.NullString
	err := r.db.QueryRowContext(ctx, `
		SELECT
			COUNT(*) FILTER (WHERE status = 'PENDING'),
			COUNT(*) FILTER (WHERE status = 'SENDING'),
			MIN(event_timestamp) FILTER (WHERE status = 'PENDING'),
			MAX(event_timestamp) FILTER (WHERE status = 'PENDING'),
			COALESCE(SUM(retry_count) FILTER (WHERE status = 'PENDING'), 0)
		FROM data_queue`,
	).Scan(&s.PendingCount, &s.SendingCount, &oldest, &newest, &s.TotalRetries)
	if err != nil {
		return Stats{}, err
	}
	if oldest.Valid {
		if t, err := time.Parse(timeLayout, oldest.String); err == nil {
			s.OldestPending = &t
		}
	}
	if newest.Valid {
		if t, err := time.Parse(timeLayout, newest.String); err == nil {
			s.NewestPending = &t
		}
	}
	return s, nil
}

func placeholders(n int) string {
	return strings.TrimSuffix(strings.Repeat("?,", n), ",")
}

func toArgs(ids []int64) []any {
	args := make([]any, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	return args
}
