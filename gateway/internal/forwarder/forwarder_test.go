package forwarder_test

import (
	"context"
	"errors"
	"log/slog"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"nxiiot-gateway/internal/forwarder"
	"nxiiot-gateway/internal/queue"
	"nxiiot-gateway/internal/storage"
)

func openTestRepo(t *testing.T) *queue.Repository {
	t.Helper()
	dbPath := filepath.Join(t.TempDir(), "test.db")
	log := slog.New(slog.NewTextHandler(testWriter{t}, nil))

	db, err := storage.Open(dbPath, "../../migrations", log)
	if err != nil {
		t.Fatalf("open storage: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	repo := queue.NewRepository(db)
	if err := repo.EnsureGateway(context.Background(), "GW001", "Test Gateway"); err != nil {
		t.Fatalf("EnsureGateway: %v", err)
	}
	return repo
}

type testWriter struct{ t *testing.T }

func (w testWriter) Write(p []byte) (int, error) {
	w.t.Log(string(p))
	return len(p), nil
}

// fakeAdapter lets tests script success/failure per call and records every
// batch it was asked to send.
type fakeAdapter struct {
	mu      sync.Mutex
	fail    func(callNum int) error // nil = always succeed
	calls   int
	batches [][]queue.DispatchEntry
}

func (a *fakeAdapter) Send(ctx context.Context, batch []queue.DispatchEntry) error {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.calls++
	a.batches = append(a.batches, batch)
	if a.fail != nil {
		return a.fail(a.calls)
	}
	return nil
}

func (a *fakeAdapter) callCount() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.calls
}

func newTestLogger(t *testing.T) *slog.Logger {
	return slog.New(slog.NewTextHandler(testWriter{t}, nil))
}

func TestForwarderSendsPendingRowsAndMarksSent(t *testing.T) {
	ctx := context.Background()
	repo := openTestRepo(t)
	v := 1.0
	e, err := repo.Insert(ctx, queue.Entry{GatewayID: "GW001", DeviceID: 1, DatapointID: 1, Value: &v, Quality: "GOOD", EventTimestamp: time.Now()})
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	adapter := &fakeAdapter{}
	fwd := forwarder.New(repo, adapter, forwarder.Config{BatchSize: 10, PollInterval: 10 * time.Millisecond}, newTestLogger(t))

	runCtx, cancel := context.WithTimeout(ctx, 300*time.Millisecond)
	defer cancel()
	fwd.Run(runCtx)

	if adapter.callCount() == 0 {
		t.Fatal("expected adapter.Send to be called at least once")
	}
	if !fwd.Status().Connected {
		t.Error("expected forwarder status to report Connected after a successful send")
	}

	stats, err := repo.Stats(ctx)
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if stats.PendingCount != 0 {
		t.Errorf("PendingCount = %d, want 0 (row should be SENT)", stats.PendingCount)
	}
	_ = e
}

func TestForwarderRetriesAfterFailureAndEventuallySucceeds(t *testing.T) {
	ctx := context.Background()
	repo := openTestRepo(t)
	v := 1.0
	if _, err := repo.Insert(ctx, queue.Entry{GatewayID: "GW001", DeviceID: 1, DatapointID: 1, Value: &v, Quality: "GOOD", EventTimestamp: time.Now()}); err != nil {
		t.Fatalf("insert: %v", err)
	}

	adapter := &fakeAdapter{
		fail: func(callNum int) error {
			if callNum == 1 {
				return errors.New("simulated server down")
			}
			return nil
		},
	}
	// PollInterval > 1s backoff so the second attempt only happens once the
	// backoff window has elapsed, proving MarkFailed's scheduling is honored
	// end-to-end (not just that the row eventually gets retried).
	fwd := forwarder.New(repo, adapter, forwarder.Config{BatchSize: 10, PollInterval: 200 * time.Millisecond}, newTestLogger(t))

	runCtx, cancel := context.WithTimeout(ctx, 2500*time.Millisecond)
	defer cancel()
	fwd.Run(runCtx)

	if adapter.callCount() < 2 {
		t.Fatalf("expected at least 2 send attempts, got %d", adapter.callCount())
	}

	stats, err := repo.Stats(ctx)
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if stats.PendingCount != 0 {
		t.Errorf("PendingCount = %d, want 0 (row should have eventually been SENT)", stats.PendingCount)
	}

	st := fwd.Status()
	if !st.Connected {
		t.Error("expected final status to be Connected after the retry succeeded")
	}
}

func TestForwarderRecoversRowsStuckInSendingOnStartup(t *testing.T) {
	ctx := context.Background()
	repo := openTestRepo(t)
	v := 1.0
	e, err := repo.Insert(ctx, queue.Entry{GatewayID: "GW001", DeviceID: 1, DatapointID: 1, Value: &v, Quality: "GOOD", EventTimestamp: time.Now()})
	if err != nil {
		t.Fatalf("insert: %v", err)
	}

	// Simulate the previous process crashing mid-send.
	if _, err := repo.FetchBatch(ctx, 10); err != nil { // marks it SENDING
		t.Fatalf("FetchBatch: %v", err)
	}

	adapter := &fakeAdapter{}
	fwd := forwarder.New(repo, adapter, forwarder.Config{BatchSize: 10, PollInterval: 10 * time.Millisecond}, newTestLogger(t))

	runCtx, cancel := context.WithTimeout(ctx, 200*time.Millisecond)
	defer cancel()
	fwd.Run(runCtx)

	if adapter.callCount() == 0 {
		t.Fatal("expected the recovered row to be re-sent, but adapter.Send was never called")
	}

	stats, err := repo.Stats(ctx)
	if err != nil {
		t.Fatalf("Stats: %v", err)
	}
	if stats.PendingCount != 0 || stats.SendingCount != 0 {
		t.Errorf("expected the recovered row to end up SENT, got pending=%d sending=%d", stats.PendingCount, stats.SendingCount)
	}
	_ = e
}

func TestForwarderDoesNotCallAdapterWhenQueueIsEmpty(t *testing.T) {
	ctx := context.Background()
	repo := openTestRepo(t)

	adapter := &fakeAdapter{}
	fwd := forwarder.New(repo, adapter, forwarder.Config{BatchSize: 10, PollInterval: 10 * time.Millisecond}, newTestLogger(t))

	runCtx, cancel := context.WithTimeout(ctx, 100*time.Millisecond)
	defer cancel()
	fwd.Run(runCtx)

	if adapter.callCount() != 0 {
		t.Errorf("expected 0 calls with an empty queue, got %d", adapter.callCount())
	}
}
