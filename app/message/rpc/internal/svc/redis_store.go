package svc

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"github.com/zeromicro/go-zero/core/stores/redis"
)

const unreadCacheTTL = time.Hour

type UnreadStore interface {
	GetMessageUnread(ctx context.Context, userID int64) (int64, bool, error)
	SetMessageUnread(ctx context.Context, userID int64, count int64) error
	GetNotificationUnread(ctx context.Context, userID int64) (int64, bool, error)
	SetNotificationUnread(ctx context.Context, userID int64, count int64) error
	DeleteUserUnread(ctx context.Context, userID int64) error
}

type RedisUnreadStore struct {
	redis *redis.Redis
}

func NewRedisUnreadStore(redisClient *redis.Redis) *RedisUnreadStore {
	return &RedisUnreadStore{redis: redisClient}
}

func (s *RedisUnreadStore) GetMessageUnread(ctx context.Context, userID int64) (int64, bool, error) {
	return s.get(ctx, messageUnreadKey(userID))
}

func (s *RedisUnreadStore) SetMessageUnread(ctx context.Context, userID int64, count int64) error {
	return s.set(ctx, messageUnreadKey(userID), count)
}

func (s *RedisUnreadStore) GetNotificationUnread(ctx context.Context, userID int64) (int64, bool, error) {
	return s.get(ctx, notificationUnreadKey(userID))
}

func (s *RedisUnreadStore) SetNotificationUnread(ctx context.Context, userID int64, count int64) error {
	return s.set(ctx, notificationUnreadKey(userID), count)
}

func (s *RedisUnreadStore) DeleteUserUnread(ctx context.Context, userID int64) error {
	if s == nil || s.redis == nil {
		return nil
	}
	_, err := s.redis.DelCtx(ctx, messageUnreadKey(userID), notificationUnreadKey(userID))
	return err
}

func (s *RedisUnreadStore) get(ctx context.Context, key string) (int64, bool, error) {
	if s == nil || s.redis == nil {
		return 0, false, nil
	}
	value, err := s.redis.GetCtx(ctx, key)
	if err != nil {
		return 0, false, err
	}
	if value == "" {
		return 0, false, nil
	}
	count, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, false, err
	}
	return count, true, nil
}

func (s *RedisUnreadStore) set(ctx context.Context, key string, count int64) error {
	if s == nil || s.redis == nil {
		return nil
	}
	return s.redis.SetexCtx(ctx, key, strconv.FormatInt(count, 10), int(unreadCacheTTL.Seconds()))
}

func messageUnreadKey(userID int64) string {
	return fmt.Sprintf("message:unread:%d", userID)
}

func notificationUnreadKey(userID int64) string {
	return fmt.Sprintf("notification:unread:%d", userID)
}
