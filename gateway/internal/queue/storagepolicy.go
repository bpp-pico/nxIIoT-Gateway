package queue

import (
	"context"
	"log/slog"
	"time"
)

// EvictOldestNonCritical deletes up to limit rows to relieve storage
// pressure (§17 "Storage Full Action", default policy "Delete Oldest
// Non-Critical Data"). CRITICAL rows are never touched. Among the rest,
// LOW priority is evicted first, then NORMAL, then HIGH — "protect higher
// priority data first" — and within a priority tier, oldest first.
func (r *Repository) EvictOldestNonCritical(ctx context.Context, limit int) (int64, error) {
	res, err := r.db.ExecContext(ctx, `
		DELETE FROM data_queue
		WHERE id IN (
			SELECT id FROM data_queue
			WHERE priority != 'CRITICAL'
			ORDER BY CASE priority
				WHEN 'LOW' THEN 0
				WHEN 'NORMAL' THEN 1
				WHEN 'HIGH' THEN 2
				ELSE 3 END,
				event_timestamp ASC
			LIMIT ?
		)`, limit)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// DiskUsageFunc reports used disk space as a percentage (0-100) of the
// volume backing the database. Injected rather than imported directly so
// this package doesn't need to depend on how disk usage is measured, and
// so tests can simulate storage pressure without touching a real disk.
type DiskUsageFunc func() (float64, error)

// RunStoragePressureSweeper evicts oldest non-critical rows whenever disk
// usage is at or above fullPercent (§17, default 95%), until ctx is
// cancelled. It runs once immediately, then every interval.
func RunStoragePressureSweeper(ctx context.Context, repo *Repository, diskUsage DiskUsageFunc, fullPercent float64, evictBatchSize int, interval time.Duration, log *slog.Logger) {
	check := func() {
		pct, err := diskUsage()
		if err != nil {
			log.Error("failed to check disk usage", "error", err)
			return
		}
		if pct < fullPercent {
			return
		}

		n, err := repo.EvictOldestNonCritical(ctx, evictBatchSize)
		if err != nil {
			log.Error("storage-full eviction failed", "error", err)
			return
		}
		if n > 0 {
			log.Warn("storage full: evicted oldest non-critical rows", "disk_usage_percent", pct, "evicted", n)
		} else {
			log.Error("storage full and nothing left to evict (only CRITICAL data remains)", "disk_usage_percent", pct)
		}
	}

	check()

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			check()
		}
	}
}
