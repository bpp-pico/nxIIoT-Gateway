package queue_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"nxiiot-gateway/internal/queue"
)

func insertReading(t *testing.T, ctx context.Context, repo *queue.Repository, priority string) queue.Entry {
	t.Helper()
	v := 1.0
	e, err := repo.Insert(ctx, queue.Entry{
		GatewayID:      "GW001",
		DeviceID:       1,
		DatapointID:    1,
		Value:          &v,
		Quality:        "GOOD",
		EventTimestamp: time.Now(),
		Priority:       priority,
	})
	if err != nil {
		t.Fatalf("insert: %v", err)
	}
	return e
}

func TestFetchBatchOrdersByPriorityThenSequence(t *testing.T) {
	ctx := context.Background()
	_, repo := openTestDB(t)
	if err := repo.EnsureGateway(ctx, "GW001", "Test Gateway"); err != nil {
		t.Fatalf("EnsureGateway: %v", err)
	}

	// Insert LOW, then CRITICAL, then LOW again: CRITICAL must come first,
	// and the two LOW rows must stay in insertion (sequence) order.
	low1 := insertReading(t, ctx, repo, "LOW")
	critical := insertReading(t, ctx, repo, "CRITICAL")
	low2 := insertReading(t, ctx, repo, "LOW")

	batch, err := repo.FetchBatch(ctx, 10)
	if err != nil {
		t.Fatalf("FetchBatch: %v", err)
	}
	if len(batch) != 3 {
		t.Fatalf("got %d rows, want 3", len(batch))
	}

	wantOrder := []int64{critical.ID, low1.ID, low2.ID}
	for i, id := range wantOrder {
		if batch[i].ID != id {
			t.Errorf("batch[%d].ID = %d, want %d", i, batch[i].ID, id)
		}
	}
}

func TestFetchBatchMarksRowsSendingAndExcludesThemFromNextFetch(t *testing.T) {
	ctx := context.Background()
	_, repo := openTestDB(t)
	if err := repo.EnsureGateway(ctx, "GW001", "Test Gateway"); err != nil {
		t.Fatalf("EnsureGateway: %v", err)
	}
	insertReading(t, ctx, repo, "NORMAL")

	batch, err := repo.FetchBatch(ctx, 10)
	if err != nil {
		t.Fatalf("FetchBatch: %v", err)
	}
	if len(batch) != 1 {
		t.Fatalf("got %d rows, want 1", len(batch))
	}
	if batch[0].Status != "SENDING" {
		t.Errorf("status = %q, want SENDING", batch[0].Status)
	}

	// A SENDING row must not be picked up again by a concurrent/subsequent
	// fetch — it's in flight.
	again, err := repo.FetchBatch(ctx, 10)
	if err != nil {
		t.Fatalf("FetchBatch (2nd): %v", err)
	}
	if len(again) != 0 {
		t.Fatalf("2nd fetch returned %d rows, want 0", len(again))
	}
}

func TestFetchBatchRespectsLimit(t *testing.T) {
	ctx := context.Background()
	_, repo := openTestDB(t)
	if err := repo.EnsureGateway(ctx, "GW001", "Test Gateway"); err != nil {
		t.Fatalf("EnsureGateway: %v", err)
	}
	for i := 0; i < 5; i++ {
		insertReading(t, ctx, repo, "NORMAL")
	}

	batch, err := repo.FetchBatch(ctx, 2)
	if err != nil {
		t.Fatalf("FetchBatch: %v", err)
	}
	if len(batch) != 2 {
		t.Fatalf("got %d rows, want 2", len(batch))
	}
}

func TestMarkSentTransitionsToSent(t *testing.T) {
	ctx := context.Background()
	db, repo := openTestDB(t)
	if err := repo.EnsureGateway(ctx, "GW001", "Test Gateway"); err != nil {
		t.Fatalf("EnsureGateway: %v", err)
	}
	e := insertReading(t, ctx, repo, "NORMAL")

	if _, err := repo.FetchBatch(ctx, 10); err != nil {
		t.Fatalf("FetchBatch: %v", err)
	}
	if err := repo.MarkSent(ctx, []int64{e.ID}); err != nil {
		t.Fatalf("MarkSent: %v", err)
	}

	var status string
	var sentAt *string
	if err := db.QueryRowContext(ctx, `SELECT status, sent_at FROM data_queue WHERE id = ?`, e.ID).Scan(&status, &sentAt); err != nil {
		t.Fatalf("query: %v", err)
	}
	if status != "SENT" {
		t.Errorf("status = %q, want SENT", status)
	}
	if sentAt == nil {
		t.Error("sent_at should be set")
	}
}

func TestMarkFailedReturnsToPendingWithBackoffAndIsNotImmediatelyRefetched(t *testing.T) {
	ctx := context.Background()
	db, repo := openTestDB(t)
	if err := repo.EnsureGateway(ctx, "GW001", "Test Gateway"); err != nil {
		t.Fatalf("EnsureGateway: %v", err)
	}
	e := insertReading(t, ctx, repo, "NORMAL")

	if _, err := repo.FetchBatch(ctx, 10); err != nil {
		t.Fatalf("FetchBatch: %v", err)
	}
	if err := repo.MarkFailed(ctx, []int64{e.ID}, "connection refused"); err != nil {
		t.Fatalf("MarkFailed: %v", err)
	}

	var status, lastError string
	var retryCount int
	if err := db.QueryRowContext(ctx, `SELECT status, retry_count, last_error FROM data_queue WHERE id = ?`, e.ID).
		Scan(&status, &retryCount, &lastError); err != nil {
		t.Fatalf("query: %v", err)
	}
	if status != "PENDING" {
		t.Errorf("status = %q, want PENDING", status)
	}
	if retryCount != 1 {
		t.Errorf("retry_count = %d, want 1", retryCount)
	}
	if lastError != "connection refused" {
		t.Errorf("last_error = %q, want %q", lastError, "connection refused")
	}

	// retry 1 backs off 1s into the future, so an immediate re-fetch must
	// not pick it back up.
	batch, err := repo.FetchBatch(ctx, 10)
	if err != nil {
		t.Fatalf("FetchBatch after failure: %v", err)
	}
	if len(batch) != 0 {
		t.Fatalf("got %d rows immediately after a failure, want 0 (backoff not yet elapsed)", len(batch))
	}
}

func TestRecoverSendingToPending(t *testing.T) {
	ctx := context.Background()
	db, repo := openTestDB(t)
	if err := repo.EnsureGateway(ctx, "GW001", "Test Gateway"); err != nil {
		t.Fatalf("EnsureGateway: %v", err)
	}
	e := insertReading(t, ctx, repo, "NORMAL")

	// Simulate a crash mid-send: row stuck in SENDING.
	mustExec(t, db, `UPDATE data_queue SET status = 'SENDING' WHERE id = ?`, e.ID)

	n, err := repo.RecoverSendingToPending(ctx)
	if err != nil {
		t.Fatalf("RecoverSendingToPending: %v", err)
	}
	if n != 1 {
		t.Fatalf("recovered %d rows, want 1", n)
	}

	var status string
	if err := db.QueryRowContext(ctx, `SELECT status FROM data_queue WHERE id = ?`, e.ID).Scan(&status); err != nil {
		t.Fatalf("query: %v", err)
	}
	if status != "PENDING" {
		t.Errorf("status = %q, want PENDING", status)
	}
}

// TestConcurrentInsertsAndFetchBatchDoNotHitBusySnapshot reproduces a real
// live-production incident (2026-09-02, see MEMORY.md): after raising
// storage.Open's connection pool above 1, FetchBatch's SELECT-then-UPDATE
// transaction started failing every single time with SQLITE_BUSY_SNAPSHOT
// (517) the moment a concurrent Insert() landed between its read and write —
// exactly this shape of interleaving. Fixed via _txlock=immediate (storage.go),
// which takes FetchBatch's write lock at BEGIN instead of after the SELECT.
// This test drives real concurrent Insert/FetchBatch/MarkSent traffic against
// an on-disk (not :memory:) database, matching production's storage.Open
// config, and fails on any error from any goroutine.
func TestConcurrentInsertsAndFetchBatchDoNotHitBusySnapshot(t *testing.T) {
	ctx := context.Background()
	_, repo := openTestDB(t)
	if err := repo.EnsureGateway(ctx, "GW001", "Test Gateway"); err != nil {
		t.Fatalf("EnsureGateway: %v", err)
	}

	const inserters = 4
	const insertsEach = 50
	const fetchers = 2
	const fetchesEach = 100

	var wg sync.WaitGroup
	errs := make(chan error, inserters*insertsEach+fetchers*fetchesEach)

	wg.Add(inserters)
	for i := 0; i < inserters; i++ {
		go func() {
			defer wg.Done()
			v := 1.0
			for j := 0; j < insertsEach; j++ {
				if _, err := repo.Insert(ctx, queue.Entry{
					GatewayID: "GW001", DeviceID: 1, DatapointID: 1,
					Value: &v, Quality: "GOOD", EventTimestamp: time.Now(), Priority: "NORMAL",
				}); err != nil {
					errs <- err
				}
			}
		}()
	}

	wg.Add(fetchers)
	for i := 0; i < fetchers; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < fetchesEach; j++ {
				batch, err := repo.FetchBatch(ctx, 5)
				if err != nil {
					errs <- err
					continue
				}
				if len(batch) == 0 {
					continue
				}
				ids := make([]int64, len(batch))
				for k, e := range batch {
					ids[k] = e.ID
				}
				if err := repo.MarkSent(ctx, ids); err != nil {
					errs <- err
				}
			}
		}()
	}

	wg.Wait()
	close(errs)

	for err := range errs {
		t.Errorf("concurrent DB operation failed: %v", err)
	}
}

func TestStatsCountsPendingAndSending(t *testing.T) {
	ctx := context.Background()
	_, repo := openTestDB(t)
	if err := repo.EnsureGateway(ctx, "GW001", "Test Gateway"); err != nil {
		t.Fatalf("EnsureGateway: %v", err)
	}
	insertReading(t, ctx, repo, "NORMAL")
	insertReading(t, ctx, repo, "NORMAL")
	insertReading(t, ctx, repo, "NORMAL")

	// Move one to SENDING via FetchBatch(limit=1).
	if _, err := repo.FetchBatch(ctx, 1); err != nil {
		t.Fatalf("FetchBatch: %v", err)
	}

	stats, err := repo.Stats(ctx)
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if stats.PendingCount != 2 {
		t.Errorf("PendingCount = %d, want 2", stats.PendingCount)
	}
	if stats.SendingCount != 1 {
		t.Errorf("SendingCount = %d, want 1", stats.SendingCount)
	}
	if stats.OldestPending == nil || stats.NewestPending == nil {
		t.Error("expected OldestPending/NewestPending to be set")
	}
}
