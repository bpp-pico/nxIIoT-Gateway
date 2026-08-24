package forwarder

import (
	"context"
	"log/slog"
	"time"

	"nxiiot-gateway/internal/queue"
)

type Config struct {
	BatchSize    int
	PollInterval time.Duration
}

type Forwarder struct {
	repo    *queue.Repository
	adapter Adapter
	cfg     Config
	log     *slog.Logger
	status  statusTracker
}

func New(repo *queue.Repository, adapter Adapter, cfg Config, log *slog.Logger) *Forwarder {
	if cfg.BatchSize <= 0 {
		cfg.BatchSize = 100
	}
	if cfg.PollInterval <= 0 {
		cfg.PollInterval = time.Second
	}
	return &Forwarder{repo: repo, adapter: adapter, cfg: cfg, log: log}
}

// Status reports current server connectivity, for the Store & Forward API.
func (f *Forwarder) Status() Status {
	return f.status.get()
}

// Run recovers any row left SENDING by an unclean shutdown (§9.3), then
// dispatches batches until ctx is cancelled. It never stops acquisition —
// a down server just means the queue keeps growing (Scenario B).
func (f *Forwarder) Run(ctx context.Context) {
	if n, err := f.repo.RecoverSendingToPending(ctx); err != nil {
		f.log.Error("failed to recover SENDING rows on startup", "error", err)
	} else if n > 0 {
		f.log.Warn("recovered rows stuck in SENDING from an unclean shutdown", "count", n)
	}

	ticker := time.NewTicker(f.cfg.PollInterval)
	defer ticker.Stop()

	for {
		f.dispatchOnce(ctx)
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func (f *Forwarder) dispatchOnce(ctx context.Context) {
	batch, err := f.repo.FetchBatch(ctx, f.cfg.BatchSize)
	if err != nil {
		f.log.Error("failed to fetch batch for forwarding", "error", err)
		return
	}
	if len(batch) == 0 {
		return
	}

	ids := make([]int64, len(batch))
	for i, e := range batch {
		ids[i] = e.ID
	}

	if err := f.adapter.Send(ctx, batch); err != nil {
		f.status.recordFailure(err)
		f.log.Warn("batch send failed, will retry", "count", len(batch), "error", err)
		if markErr := f.repo.MarkFailed(ctx, ids, err.Error()); markErr != nil {
			f.log.Error("failed to mark batch failed", "error", markErr)
		}
		return
	}

	f.status.recordSuccess()
	if err := f.repo.MarkSent(ctx, ids); err != nil {
		f.log.Error("failed to mark batch sent", "error", err)
		return
	}
	f.log.Debug("batch forwarded", "count", len(batch))
}
