package svc

import "github.com/zeromicro/go-zero/core/stores/redis"

type RedisStore interface {
	Hget(key, field string) (string, error)
	Hset(key, field, value string) error
	Expire(key string, seconds int) error
	Exists(key string) (bool, error)
	Hincrby(key, field string, increment int) (int, error)
}

type goZeroRedisStore struct {
	client *redis.Redis
}

func NewRedisStore(client *redis.Redis) RedisStore {
	if client == nil {
		return nil
	}

	return &goZeroRedisStore{client: client}
}

func (s *goZeroRedisStore) Hget(key, field string) (string, error) {
	return s.client.Hget(key, field)
}

func (s *goZeroRedisStore) Hset(key, field, value string) error {
	return s.client.Hset(key, field, value)
}

func (s *goZeroRedisStore) Expire(key string, seconds int) error {
	return s.client.Expire(key, seconds)
}

func (s *goZeroRedisStore) Exists(key string) (bool, error) {
	return s.client.Exists(key)
}

func (s *goZeroRedisStore) Hincrby(key, field string, increment int) (int, error) {
	return s.client.Hincrby(key, field, increment)
}
