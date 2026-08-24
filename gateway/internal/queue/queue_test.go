package queue_test

import (
	"context"
	"database/sql"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"nxiiot-gateway/internal/queue"
	"nxiiot-gateway/internal/storage"
)

func openTestDB(t *testing.T) (*sql.DB, *queue.Repository) {
	t.Helper()

	dbPath := filepath.Join(t.TempDir(), "test.db")
	log := slog.New(slog.NewTextHandler(testWriter{t}, nil))

	db, err := storage.Open(dbPath, "../../migrations", log)
	if err != nil {
		t.Fatalf("open storage: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	return db, queue.NewRepository(db)
}

func mustExec(t *testing.T, db *sql.DB, query string, args ...any) {
	t.Helper()
	if _, err := db.Exec(query, args...); err != nil {
		t.Fatalf("exec %q: %v", query, err)
	}
}

type testWriter struct{ t *testing.T }

func (w testWriter) Write(p []byte) (int, error) {
	w.t.Log(string(p))
	return len(p), nil
}

func TestInsertAssignsSequentialIDs(t *testing.T) {
	ctx := context.Background()
	_, repo := openTestDB(t)

	if err := repo.EnsureGateway(ctx, "GW001", "Test Gateway"); err != nil {
		t.Fatalf("EnsureGateway: %v", err)
	}

	v := 42.0
	for i, want := range []int64{1, 2, 3} {
		e, err := repo.Insert(ctx, queue.Entry{
			GatewayID:      "GW001",
			DeviceID:       1,
			DatapointID:    1,
			Value:          &v,
			Quality:        "GOOD",
			EventTimestamp: time.Now(),
		})
		if err != nil {
			t.Fatalf("insert %d: %v", i, err)
		}
		if e.SequenceID != want {
			t.Fatalf("insert %d: got sequence_id %d, want %d", i, e.SequenceID, want)
		}
	}
}

func TestEnsureGatewayIsIdempotentAndPreservesSequence(t *testing.T) {
	ctx := context.Background()
	_, repo := openTestDB(t)

	if err := repo.EnsureGateway(ctx, "GW001", "Test Gateway"); err != nil {
		t.Fatalf("EnsureGateway (1st): %v", err)
	}

	v := 1.0
	e, err := repo.Insert(ctx, queue.Entry{GatewayID: "GW001", DeviceID: 1, DatapointID: 1, Value: &v, Quality: "GOOD", EventTimestamp: time.Now()})
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
	if e.SequenceID != 1 {
		t.Fatalf("got sequence_id %d, want 1", e.SequenceID)
	}

	// Simulate a gateway restart: EnsureGateway runs again on boot. It must
	// not reset the persisted sequence counter (Rule 6).
	if err := repo.EnsureGateway(ctx, "GW001", "Test Gateway (renamed)"); err != nil {
		t.Fatalf("EnsureGateway (2nd): %v", err)
	}

	e2, err := repo.Insert(ctx, queue.Entry{GatewayID: "GW001", DeviceID: 1, DatapointID: 1, Value: &v, Quality: "GOOD", EventTimestamp: time.Now()})
	if err != nil {
		t.Fatalf("insert after re-ensure: %v", err)
	}
	if e2.SequenceID != 2 {
		t.Fatalf("got sequence_id %d after restart, want 2 (sequence must not reset)", e2.SequenceID)
	}
}

func TestInsertPersistsNullValueForBadQuality(t *testing.T) {
	ctx := context.Background()
	_, repo := openTestDB(t)
	if err := repo.EnsureGateway(ctx, "GW001", "Test Gateway"); err != nil {
		t.Fatalf("EnsureGateway: %v", err)
	}

	// A failed read (nil value) must still be persisted — never silently
	// dropped, and never backfilled with a stale value.
	e, err := repo.Insert(ctx, queue.Entry{
		GatewayID:      "GW001",
		DeviceID:       1,
		DatapointID:    1,
		Value:          nil,
		Quality:        "TIMEOUT",
		EventTimestamp: time.Now(),
	})
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
	if e.Value != nil {
		t.Fatalf("expected nil value to round-trip as nil")
	}
	if e.Quality != "TIMEOUT" {
		t.Fatalf("got quality %q, want TIMEOUT", e.Quality)
	}
}

func TestDeleteSentOlderThanOnlyRemovesOldSentRows(t *testing.T) {
	ctx := context.Background()
	db, repo := openTestDB(t)
	if err := repo.EnsureGateway(ctx, "GW001", "Test Gateway"); err != nil {
		t.Fatalf("EnsureGateway: %v", err)
	}

	v := 1.0
	old, err := repo.Insert(ctx, queue.Entry{GatewayID: "GW001", DeviceID: 1, DatapointID: 1, Value: &v, Quality: "GOOD", EventTimestamp: time.Now()})
	if err != nil {
		t.Fatalf("insert old: %v", err)
	}
	recent, err := repo.Insert(ctx, queue.Entry{GatewayID: "GW001", DeviceID: 1, DatapointID: 1, Value: &v, Quality: "GOOD", EventTimestamp: time.Now()})
	if err != nil {
		t.Fatalf("insert recent: %v", err)
	}
	pending, err := repo.Insert(ctx, queue.Entry{GatewayID: "GW001", DeviceID: 1, DatapointID: 1, Value: &v, Quality: "GOOD", EventTimestamp: time.Now()})
	if err != nil {
		t.Fatalf("insert pending: %v", err)
	}

	mustExec(t, db, `UPDATE data_queue SET status='SENT', sent_at='2000-01-01T00:00:00.000Z' WHERE id = ?`, old.ID)
	mustExec(t, db, `UPDATE data_queue SET status='SENT', sent_at=strftime('%Y-%m-%dT%H:%M:%fZ','now') WHERE id = ?`, recent.ID)
	// pending row stays PENDING, untouched.

	n, err := repo.DeleteSentOlderThan(ctx, time.Now().Add(-24*time.Hour))
	if err != nil {
		t.Fatalf("DeleteSentOlderThan: %v", err)
	}
	if n != 1 {
		t.Fatalf("deleted %d rows, want 1", n)
	}

	var count int
	if err := db.QueryRowContext(ctx, `SELECT COUNT(*) FROM data_queue WHERE id IN (?, ?)`, recent.ID, pending.ID).Scan(&count); err != nil {
		t.Fatalf("count remaining: %v", err)
	}
	if count != 2 {
		t.Fatalf("expected the recent SENT row and the PENDING row to survive, got count=%d", count)
	}
}
