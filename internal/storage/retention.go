package storage

import (
	"context"
	"log/slog"
	"time"
)

// RetentionWorker periodically purges old data.
type RetentionWorker struct {
	store         Store
	retentionDays int
	period        time.Duration
	logger        *slog.Logger
}

func NewRetentionWorker(store Store, retentionDays int, period time.Duration, logger *slog.Logger) *RetentionWorker {
	return &RetentionWorker{
		store:         store,
		retentionDays: retentionDays,
		period:        period,
		logger:        logger,
	}
}

func (w *RetentionWorker) Run(ctx context.Context) {
	ticker := time.NewTicker(w.period)
	defer ticker.Stop()

	// Run once on startup
	w.purge(ctx)

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			w.purge(ctx)
		}
	}
}

func (w *RetentionWorker) purge(ctx context.Context) {
	before := time.Now().AddDate(0, 0, -w.retentionDays)
	deleted, err := w.store.PurgeOldData(ctx, before)
	if err != nil {
		w.logger.Error("retention purge failed", "error", err)
		return
	}
	if deleted > 0 {
		w.logger.Info("retention purge completed", "deleted", deleted, "before", before.Format(time.RFC3339))
	}
}
