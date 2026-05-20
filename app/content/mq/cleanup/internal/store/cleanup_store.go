package store

import (
	"context"
	"fmt"

	"github.com/zeromicro/go-zero/core/stores/redis"
)

// CleanupStore 抽象帖子删除后需要执行的 Redis 清理操作，
// 便于单测 mock，避免直接耦合 go-zero redis.Redis。
type CleanupStore interface {
	DeletePostState(ctx context.Context, postID int64) error
	RemoveFromHotZSets(ctx context.Context, postID int64) error
	RemoveFromTagZSets(ctx context.Context, postID int64, tags []string) error
}

// RedisCleanupStore 是默认的 Redis 实现。
type RedisCleanupStore struct {
	rds *redis.Redis
}

func NewRedisCleanupStore(rds *redis.Redis) *RedisCleanupStore {
	return &RedisCleanupStore{rds: rds}
}

// DeletePostState 删除 post:{pid}:stats / post:{pid}:quality 等帖子级 Redis 状态。
func (s *RedisCleanupStore) DeletePostState(ctx context.Context, postID int64) error {
	keys := []string{
		fmt.Sprintf("post:%d:stats", postID),
		fmt.Sprintf("post:%d:quality", postID),
	}
	if _, err := s.rds.DelCtx(ctx, keys...); err != nil {
		return fmt.Errorf("delete post state: %w", err)
	}
	return nil
}

// RemoveFromHotZSets 从所有热榜 ZSET 中移除该 postID。
func (s *RedisCleanupStore) RemoveFromHotZSets(ctx context.Context, postID int64) error {
	hotKeys := []string{"hot:posts:24h", "hot:posts:7d"}
	pidStr := fmt.Sprintf("%d", postID)
	for _, k := range hotKeys {
		if _, err := s.rds.ZremCtx(ctx, k, pidStr); err != nil {
			return fmt.Errorf("zrem %s: %w", k, err)
		}
	}
	return nil
}

// RemoveFromTagZSets 从每个 tag 的 ZSET 中移除该 postID。
func (s *RedisCleanupStore) RemoveFromTagZSets(ctx context.Context, postID int64, tags []string) error {
	pidStr := fmt.Sprintf("%d", postID)
	for _, tag := range tags {
		key := fmt.Sprintf("tag:%s:posts", tag)
		if _, err := s.rds.ZremCtx(ctx, key, pidStr); err != nil {
			return fmt.Errorf("zrem %s: %w", key, err)
		}
	}
	return nil
}
