package svc

import (
	"context"
	"fmt"

	"github.com/zeromicro/go-zero/core/stores/redis"
)

type UnreadStore interface {
	DeleteUserUnread(ctx context.Context, userID int64) error
}

type redisUnreadStore struct {
	redis *redis.Redis
}

func NewRedisUnreadStore(r *redis.Redis) UnreadStore {
	return &redisUnreadStore{redis: r}
}

func (s *redisUnreadStore) DeleteUserUnread(ctx context.Context, userID int64) error {
	key := fmt.Sprintf("cache:unread:notification:%d", userID)
	_, err := s.redis.DelCtx(ctx, key)
	return err
}
