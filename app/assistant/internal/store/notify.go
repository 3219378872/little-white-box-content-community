package store

import (
	"context"
	"fmt"
	"strconv"

	"github.com/zeromicro/go-zero/core/stores/redis"
)

type RedisNotifier struct {
	rds *redis.Redis
}

func NewRedisNotifier(rds *redis.Redis) *RedisNotifier {
	return &RedisNotifier{rds: rds}
}

func wakeKey(runID int64) string {
	return fmt.Sprintf("assistant:run:%d:wake", runID)
}

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
