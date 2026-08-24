package queue

import (
	"context"
	"log/slog"
	"time"
)

// RunRetentionSweeper periodically deletes SENT rows older than retention,
// until ctx is cancelled. It runs once immediately, then every interval.
func RunRetentionSweeper(ctx context.Context, repo *Repository, retention, interval time.Duration, log *slog.Logger) {
	sweep := func() {
		n, err := repo.DeleteSentOlderThan(ctx, time.Now().Add(-retention))
		if err != nil {
			log.Error("retention sweep failed", "error", err)
			return
		}
		if n > 0 {
			log.Info("retention sweep removed old SENT rows", "count", n)
		}
	}

	sweep()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			sweep()
		}
	}
}
