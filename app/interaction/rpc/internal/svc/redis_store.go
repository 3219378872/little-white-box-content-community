package svc

import (
	"context"
	redis "esx/pkg/redisstore"
)

type RedisStore interface {
	Hget(ctx context.Context, key, field string) (string, error)
	Hset(ctx context.Context, key, field, value string) error
	Expire(ctx context.Context, key string, seconds int) error
	Exists(ctx context.Context, key string) (bool, error)
	Hincrby(ctx context.Context, key, field string, increment int) (int, error)
	Del(ctx context.Context, key string) error
}

type redisStore struct {
	client *redis.Redis
}

func NewRedisStore(client *redis.Redis) RedisStore {
	if client == nil {
		return nil
	}

	return &redisStore{client: client}
}

func (s *redisStore) Hget(ctx context.Context, key, field string) (string, error) {
	return s.client.HgetCtx(ctx, key, field)
}

func (s *redisStore) Hset(ctx context.Context, key, field, value string) error {
	return s.client.HsetCtx(ctx, key, field, value)
}

func (s *redisStore) Expire(ctx context.Context, key string, seconds int) error {
	return s.client.ExpireCtx(ctx, key, seconds)
}

func (s *redisStore) Exists(ctx context.Context, key string) (bool, error) {
	return s.client.ExistsCtx(ctx, key)
}

func (s *redisStore) Hincrby(ctx context.Context, key, field string, increment int) (int, error) {
	return s.client.HincrbyCtx(ctx, key, field, increment)
}

func (s *redisStore) Del(ctx context.Context, key string) error {
	_, err := s.client.DelCtx(ctx, key)
	return err
}
