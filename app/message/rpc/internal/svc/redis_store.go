package svc

import (
	"context"
	"fmt"
	"strconv"
	"time"

	redis "esx/pkg/redisstore"
)

const unreadCacheTTL = time.Hour

// UnreadStore 是按用户缓存未读数的存储。当前读路径直接统计数据库，写路径只调用 DeleteUserUnread 做失效。
type UnreadStore interface {
	GetMessageUnread(ctx context.Context, userID int64) (int64, bool, error)
	SetMessageUnread(ctx context.Context, userID int64, count int64) error
	GetNotificationUnread(ctx context.Context, userID int64) (int64, bool, error)
	SetNotificationUnread(ctx context.Context, userID int64, count int64) error
	DeleteUserUnread(ctx context.Context, userID int64) error
}

// RedisUnreadStore 用 Redis 保存未读数，条目带 TTL。
type RedisUnreadStore struct {
	redis *redis.Redis
}

// NewRedisUnreadStore 创建基于 Redis 的未读数存储。
func NewRedisUnreadStore(redisClient *redis.Redis) *RedisUnreadStore {
	return &RedisUnreadStore{redis: redisClient}
}

// GetMessageUnread 读取缓存的未读私信数；未命中时 ok=false。
func (s *RedisUnreadStore) GetMessageUnread(ctx context.Context, userID int64) (int64, bool, error) {
	return s.get(ctx, messageUnreadKey(userID))
}

// SetMessageUnread 缓存未读私信数。
func (s *RedisUnreadStore) SetMessageUnread(ctx context.Context, userID int64, count int64) error {
	return s.set(ctx, messageUnreadKey(userID), count)
}

// GetNotificationUnread 读取缓存的未读通知数；未命中时 ok=false。
func (s *RedisUnreadStore) GetNotificationUnread(ctx context.Context, userID int64) (int64, bool, error) {
	return s.get(ctx, notificationUnreadKey(userID))
}

// SetNotificationUnread 缓存未读通知数。
func (s *RedisUnreadStore) SetNotificationUnread(ctx context.Context, userID int64, count int64) error {
	return s.set(ctx, notificationUnreadKey(userID), count)
}

// DeleteUserUnread 同时删除用户的私信与通知未读数缓存。
func (s *RedisUnreadStore) DeleteUserUnread(ctx context.Context, userID int64) error {
	if s == nil || s.redis == nil {
		return nil
	}
	_, err := s.redis.DelCtx(ctx, messageUnreadKey(userID), notificationUnreadKey(userID))
	return err
}

// get 读取并解析计数；未配置 Redis 时按未命中处理。
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

// set 写入带 TTL 的计数；未配置 Redis 时忽略。
func (s *RedisUnreadStore) set(ctx context.Context, key string, count int64) error {
	if s == nil || s.redis == nil {
		return nil
	}
	return s.redis.SetexCtx(ctx, key, strconv.FormatInt(count, 10), int(unreadCacheTTL.Seconds()))
}

// messageUnreadKey 是未读私信数的缓存键。
func messageUnreadKey(userID int64) string {
	return fmt.Sprintf("message:unread:%d", userID)
}

// notificationUnreadKey 是未读通知数的缓存键。
func notificationUnreadKey(userID int64) string {
	return fmt.Sprintf("notification:unread:%d", userID)
}
