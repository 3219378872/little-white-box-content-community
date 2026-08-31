package retention

import (
	"context"
	"errors"
	"time"
)

const (
	AssistantMessageRetention = 365 * 24 * time.Hour
	WatchAuditRetention       = 90 * 24 * time.Hour
	DefaultBatchSize          = 500
	DefaultMaxBatches         = 20
)

type Store interface {
	PurgeExpiredMessages(ctx context.Context, cutoffMs int64, batchSize int) (int, error)
	PurgeExpiredWatchHits(ctx context.Context, cutoffMs int64, batchSize int) (int, error)
	PurgeExpiredWatchExecutions(ctx context.Context, cutoffMs int64, batchSize int) (int, error)
}

type Result struct {
	Messages        int
	WatchHits       int
	WatchExecutions int
}

type Cleaner struct {
	store      Store
	now        func() time.Time
	batchSize  int
	maxBatches int
}

func New(store Store) *Cleaner {
	return &Cleaner{
		store: store, now: time.Now,
		batchSize: DefaultBatchSize, maxBatches: DefaultMaxBatches,
	}
}

func (c *Cleaner) RunOnce(ctx context.Context) (Result, error) {
	if c == nil || c.store == nil {
		return Result{}, nil
	}
	now := c.now()
	messageCutoff := now.Add(-AssistantMessageRetention).UnixMilli()
	watchCutoff := now.Add(-WatchAuditRetention).UnixMilli()

	var result Result
	var errs []error
	result.Messages, errs = c.runBatches(ctx, messageCutoff, c.store.PurgeExpiredMessages, errs)
	result.WatchHits, errs = c.runBatches(ctx, watchCutoff, c.store.PurgeExpiredWatchHits, errs)
	result.WatchExecutions, errs = c.runBatches(ctx, watchCutoff, c.store.PurgeExpiredWatchExecutions, errs)
	return result, errors.Join(errs...)
}

func (c *Cleaner) runBatches(
	ctx context.Context,
	cutoffMs int64,
	purge func(context.Context, int64, int) (int, error),
	errs []error,
) (int, []error) {
	total := 0
	for range c.maxBatches {
		if err := ctx.Err(); err != nil {
			return total, append(errs, err)
		}
		deleted, err := purge(ctx, cutoffMs, c.batchSize)
		if err != nil {
			return total, append(errs, err)
		}
		total += deleted
		if deleted < c.batchSize {
			break
		}
	}
	return total, errs
}
