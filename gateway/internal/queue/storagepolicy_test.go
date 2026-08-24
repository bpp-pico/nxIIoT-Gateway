package queue_test

import (
	"context"
	"log/slog"
	"testing"
	"time"

	"nxiiot-gateway/internal/queue"
)

func TestEvictOldestNonCriticalNeverTouchesCritical(t *testing.T) {
	ctx := context.Background()
	_, repo := openTestDB(t)
	if err := repo.EnsureGateway(ctx, "GW001", "Test Gateway"); err != nil {
		t.Fatalf("EnsureGateway: %v", err)
	}

	critical := insertReading(t, ctx, repo, "CRITICAL")
	low := insertReading(t, ctx, repo, "LOW")
	normal := insertReading(t, ctx, repo, "NORMAL")

	n, err := repo.EvictOldestNonCritical(ctx, 10)
	if err != nil {
		t.Fatalf("EvictOldestNonCritical: %v", err)
	}
	if n != 2 {
		t.Fatalf("evicted %d rows, want 2 (low + normal, never critical)", n)
	}

	_ = low
	_ = normal
	remaining, err := repo.FetchBatch(ctx, 10)
	if err != nil {
		t.Fatalf("FetchBatch: %v", err)
	}
	if len(remaining) != 1 || remaining[0].ID != critical.ID {
		t.Fatalf("expected only the CRITICAL row to survive, got %+v", remaining)
	}
}

func TestEvictOldestNonCriticalPrefersLowestPriorityThenOldest(t *testing.T) {
	ctx := context.Background()
	_, repo := openTestDB(t)
	if err := repo.EnsureGateway(ctx, "GW001", "Test Gateway"); err != nil {
		t.Fatalf("EnsureGateway: %v", err)
	}

	high := insertReading(t, ctx, repo, "HIGH")
	lowOld := insertReading(t, ctx, repo, "LOW")
	lowNew := insertReading(t, ctx, repo, "LOW")

	// Evict exactly one: it must be the oldest LOW row, not HIGH.
	n, err := repo.EvictOldestNonCritical(ctx, 1)
	if err != nil {
		t.Fatalf("EvictOldestNonCritical: %v", err)
	}
	if n != 1 {
		t.Fatalf("evicted %d rows, want 1", n)
	}

	remaining, err := repo.FetchBatch(ctx, 10)
	if err != nil {
		t.Fatalf("FetchBatch: %v", err)
	}
	ids := map[int64]bool{}
	for _, e := range remaining {
		ids[e.ID] = true
	}
	if !ids[high.ID] || !ids[lowNew.ID] {
		t.Fatalf("expected HIGH and the newer LOW row to survive, got %+v", remaining)
	}
	if ids[lowOld.ID] {
		t.Fatal("expected the oldest LOW row to be evicted, but it survived")
	}
}

func TestRunStoragePressureSweeperOnlyEvictsWhenOverThreshold(t *testing.T) {
	ctx := context.Background()
	_, repo := openTestDB(t)
	if err := repo.EnsureGateway(ctx, "GW001", "Test Gateway"); err != nil {
		t.Fatalf("EnsureGateway: %v", err)
	}
	insertReading(t, ctx, repo, "LOW")

	log := slog.New(slog.NewTextHandler(testWriter{t}, nil))

	// Below threshold: must not evict.
	belowThreshold := func() (float64, error) { return 50, nil }
	runCtx, cancel := context.WithTimeout(ctx, 60*time.Millisecond)
	queue.RunStoragePressureSweeper(runCtx, repo, belowThreshold, 95, 10, 20*time.Millisecond, log)
	cancel()

	stats, err := repo.Stats(ctx)
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if stats.PendingCount != 1 {
		t.Fatalf("PendingCount = %d, want 1 (nothing should be evicted below threshold)", stats.PendingCount)
	}

	// At/above threshold: must evict.
	atThreshold := func() (float64, error) { return 96, nil }
	runCtx2, cancel2 := context.WithTimeout(ctx, 60*time.Millisecond)
	queue.RunStoragePressureSweeper(runCtx2, repo, atThreshold, 95, 10, 20*time.Millisecond, log)
	cancel2()

	stats2, err := repo.Stats(ctx)
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if stats2.PendingCount != 0 {
		t.Fatalf("PendingCount = %d, want 0 (should be evicted at/above threshold)", stats2.PendingCount)
	}
}
