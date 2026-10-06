package retention

import (
	"context"
	"errors"
	"time"
)

const (
	AssistantMessageRetention = 365 * 24 * time.Hour
	DefaultBatchSize          = 500
	DefaultMaxBatches         = 20
)

// Store 是清理所需的最小存储接口。
type Store interface {
	PurgeExpiredMessages(ctx context.Context, cutoffMs int64, batchSize int) (int, error)
}

// Result 是一次清理删除的行数。
type Result struct {
	SourceEvidence int
	Messages       int
}

// Cleaner 定期删除超过保留期的助手消息与来源证据。
type Cleaner struct {
	store      Store
	now        func() time.Time
	batchSize  int
	maxBatches int
}

// New 使用默认批大小与每次运行的批数上限。
func New(store Store) *Cleaner {
	return &Cleaner{
		store: store, now: time.Now,
		batchSize: DefaultBatchSize, maxBatches: DefaultMaxBatches,
	}
}

// RunOnce 执行一轮清理；来源证据清理仅在存储支持时进行，两类错误合并返回。
func (c *Cleaner) RunOnce(ctx context.Context) (Result, error) {
	if c == nil || c.store == nil {
		return Result{}, nil
	}
	now := c.now()
	messageCutoff := now.Add(-AssistantMessageRetention).UnixMilli()

	var result Result
	var errs []error
	result.Messages, errs = c.runBatches(ctx, messageCutoff, c.store.PurgeExpiredMessages, errs)
	if sourceStore, ok := c.store.(interface {
		PurgeExpiredSourceEvidence(context.Context, int64, int) (int, error)
	}); ok {
		result.SourceEvidence, errs = c.runBatches(ctx, messageCutoff, sourceStore.PurgeExpiredSourceEvidence, errs)
	}
	return result, errors.Join(errs...)
}

// runBatches 分批删除，直到某批不足批大小或达到批数上限，避免单次运行长时间占用数据库。
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
