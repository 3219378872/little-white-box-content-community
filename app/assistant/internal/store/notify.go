package store

import (
	"context"
	"fmt"
	"strconv"

	redis "esx/pkg/redisstore"
)

// RedisNotifier signals run wakeups through a per-run Redis counter.
type RedisNotifier struct {
	rds *redis.Redis
}

// NewRedisNotifier wraps a Redis client; a nil client turns Wake into a no-op.
func NewRedisNotifier(rds *redis.Redis) *RedisNotifier {
	return &RedisNotifier{rds: rds}
}

// wakeKey is the Redis counter key for one run.
func wakeKey(runID int64) string {
	return fmt.Sprintf("assistant:run:%d:wake", runID)
}

// Wake bumps the run's counter; the key expires after an hour of silence.
func (n *RedisNotifier) Wake(ctx context.Context, runID int64) error {
	if n == nil || n.rds == nil {
		return nil
	}
	_, err := n.rds.IncrCtx(ctx, wakeKey(runID))
	if err != nil {
		return err
	}
	_ = n.rds.ExpireCtx(ctx, wakeKey(runID), 3600)
	return nil
}

// WakeToken returns the current counter; a reader can compare tokens to detect a wake in between.
func (n *RedisNotifier) WakeToken(ctx context.Context, runID int64) (string, error) {
	if n == nil || n.rds == nil {
		return "", fmt.Errorf("redis notifier unavailable")
	}
	val, err := n.rds.GetCtx(ctx, wakeKey(runID))
	if err != nil {
		return "", err
	}
	if val == "" {
		return "0", nil
	}
	if _, err := strconv.ParseInt(val, 10, 64); err != nil {
		return val, nil
	}
	return val, nil
}
