package svc

import (
	"context"
	redis "esx/pkg/redisstore"
)

// RedisStore 是互动计数缓存用到的 Redis 操作，便于测试替换。
type RedisStore interface {
	Hget(ctx context.Context, key, field string) (string, error)
	Hset(ctx context.Context, key, field, value string) error
	Expire(ctx context.Context, key string, seconds int) error
	Exists(ctx context.Context, key string) (bool, error)
	Hincrby(ctx context.Context, key, field string, increment int) (int, error)
	Del(ctx context.Context, key string) error
}

// redisStore 是基于 Redis 客户端的 RedisStore 实现。
type redisStore struct {
	client *redis.Redis
}

// NewRedisStore 包装 Redis 客户端；客户端为空时返回 nil，调用方据此跳过缓存。
func NewRedisStore(client *redis.Redis) RedisStore {
	if client == nil {
		return nil
	}

	return &redisStore{client: client}
}

// Hget 读取哈希字段。
func (s *redisStore) Hget(ctx context.Context, key, field string) (string, error) {
	return s.client.HgetCtx(ctx, key, field)
}

// Hset 写入哈希字段。
func (s *redisStore) Hset(ctx context.Context, key, field, value string) error {
	return s.client.HsetCtx(ctx, key, field, value)
}

// Expire 设置键的过期时间。
func (s *redisStore) Expire(ctx context.Context, key string, seconds int) error {
	return s.client.ExpireCtx(ctx, key, seconds)
}

// Exists 判断键是否存在。
func (s *redisStore) Exists(ctx context.Context, key string) (bool, error) {
	return s.client.ExistsCtx(ctx, key)
}

// Hincrby 原子增减哈希字段。
func (s *redisStore) Hincrby(ctx context.Context, key, field string, increment int) (int, error) {
	return s.client.HincrbyCtx(ctx, key, field, increment)
}

// Del 删除键。
func (s *redisStore) Del(ctx context.Context, key string) error {
	_, err := s.client.DelCtx(ctx, key)
	return err
}
