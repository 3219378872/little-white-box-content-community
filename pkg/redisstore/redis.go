// Package redisstore exposes the Redis operations used by the domain stores.
package redisstore

import (
	"context"
	"crypto/tls"
	"errors"
	"esx/pkg/lifecycle"
	"fmt"
	"strings"
	"sync"
	"time"

	r "github.com/redis/go-redis/v9"
)

const NodeType = "node"
const ClusterType = "cluster"

var Nil = r.Nil

type RedisConf struct {
	Host       string
	Type       string `json:",default=node,options=node|cluster"`
	User, Pass string
	Tls        bool
}
type RedisKeyConf struct {
	RedisConf
	Key string
}
type FloatPair struct {
	Key   string
	Score float64
}
type Redis struct {
	client    r.UniversalClient
	closeOnce sync.Once
	closeErr  error
}

func NewRedis(c RedisConf) (*Redis, error) {
	if c.Host == "" {
		return nil, fmt.Errorf("Redis host is required")
	}
	var tlsConfig *tls.Config
	if c.Tls {
		tlsConfig = &tls.Config{MinVersion: tls.VersionTLS12}
	}
	var client r.UniversalClient
	if c.Type == ClusterType {
		client = r.NewClusterClient(&r.ClusterOptions{Addrs: strings.Split(c.Host, ","), Username: c.User, Password: c.Pass, TLSConfig: tlsConfig, MaxRetries: -1})
	} else {
		client = r.NewClient(&r.Options{Addr: c.Host, Username: c.User, Password: c.Pass, TLSConfig: tlsConfig, MaxRetries: -1})
	}
	store := &Redis{client: client}
	lifecycle.TrackResource(store)
	return store, nil
}
func MustNewRedis(c RedisConf) *Redis {
	v, e := NewRedis(c)
	if e != nil {
		panic(e)
	}
	return v
}
func (s *Redis) Close() error {
	s.closeOnce.Do(func() { s.closeErr = s.client.Close() })
	return s.closeErr
}
func (s *Redis) GetCtx(ctx context.Context, k string) (string, error) {
	v, e := s.client.Get(ctx, k).Result()
	if errors.Is(e, r.Nil) {
		return "", nil
	}
	return v, e
}
func (s *Redis) SetCtx(ctx context.Context, k, v string) error {
	return s.client.Set(ctx, k, v, 0).Err()
}
func (s *Redis) SetexCtx(ctx context.Context, k, v string, seconds int) error {
	return s.client.Set(ctx, k, v, time.Duration(seconds)*time.Second).Err()
}
func (s *Redis) SetnxExCtx(ctx context.Context, k, v string, seconds int) (bool, error) {
	return s.client.SetNX(ctx, k, v, time.Duration(seconds)*time.Second).Result()
}
func (s *Redis) DelCtx(ctx context.Context, k ...string) (int, error) {
	n, e := s.client.Del(ctx, k...).Result()
	return int(n), e
}
func (s *Redis) EvalCtx(ctx context.Context, script string, keys []string, args ...any) (any, error) {
	return s.client.Eval(ctx, script, keys, args...).Result()
}
func (s *Redis) ExpireCtx(ctx context.Context, k string, seconds int) error {
	return s.client.Expire(ctx, k, time.Duration(seconds)*time.Second).Err()
}
func (s *Redis) IncrCtx(ctx context.Context, k string) (int64, error) {
	return s.client.Incr(ctx, k).Result()
}
func (s *Redis) ExistsCtx(ctx context.Context, k string) (bool, error) {
	n, e := s.client.Exists(ctx, k).Result()
	return n > 0, e
}
func (s *Redis) HgetallCtx(ctx context.Context, k string) (map[string]string, error) {
	return s.client.HGetAll(ctx, k).Result()
}
func (s *Redis) HsetCtx(ctx context.Context, k, field, v string) error {
	return s.client.HSet(ctx, k, field, v).Err()
}
func (s *Redis) KeysCtx(ctx context.Context, p string) ([]string, error) {
	return s.client.Keys(ctx, p).Result()
}
func (s *Redis) ScanCtx(ctx context.Context, cursor uint64, match string, count int64) ([]string, uint64, error) {
	return s.client.Scan(ctx, cursor, match, count).Result()
}
func (s *Redis) TtlCtx(ctx context.Context, k string) (int, error) {
	v, e := s.client.TTL(ctx, k).Result()
	if v < 0 {
		return int(v), e
	}
	return int(v / time.Second), e
}
func (s *Redis) LlenCtx(ctx context.Context, k string) (int, error) {
	v, e := s.client.LLen(ctx, k).Result()
	return int(v), e
}
func (s *Redis) LrangeCtx(ctx context.Context, k string, start, stop int) ([]string, error) {
	return s.client.LRange(ctx, k, int64(start), int64(stop)).Result()
}
func (s *Redis) SmembersCtx(ctx context.Context, k string) ([]string, error) {
	return s.client.SMembers(ctx, k).Result()
}
func (s *Redis) ZaddCtx(ctx context.Context, k string, score int64, v string) (bool, error) {
	n, e := s.client.ZAdd(ctx, k, r.Z{Score: float64(score), Member: v}).Result()
	return n > 0, e
}
func (s *Redis) ZrankCtx(ctx context.Context, k, v string) (int64, error) {
	n, e := s.client.ZRank(ctx, k, v).Result()
	return n, e
}
func (s *Redis) ZscoreCtx(ctx context.Context, k, v string) (int64, error) {
	n, e := s.client.ZScore(ctx, k, v).Result()
	return int64(n), e
}
func (s *Redis) ZremCtx(ctx context.Context, k string, values ...any) (int, error) {
	n, e := s.client.ZRem(ctx, k, values...).Result()
	return int(n), e
}
func (s *Redis) ZrevrangeWithScoresByFloatCtx(ctx context.Context, k string, start, stop int64) ([]FloatPair, error) {
	values, err := s.client.ZRevRangeWithScores(ctx, k, start, stop).Result()
	if err != nil {
		return nil, err
	}
	result := make([]FloatPair, 0, len(values))
	for _, v := range values {
		result = append(result, FloatPair{Key: fmt.Sprint(v.Member), Score: v.Score})
	}
	return result, nil
}

func (s *Redis) HgetCtx(ctx context.Context, k, field string) (string, error) {
	v, e := s.client.HGet(ctx, k, field).Result()
	return v, e
}
func (s *Redis) HincrbyCtx(ctx context.Context, k, field string, n int) (int, error) {
	v, e := s.client.HIncrBy(ctx, k, field, int64(n)).Result()
	return int(v), e
}

func (s *Redis) ZscoreByFloatCtx(ctx context.Context, key, member string) (float64, error) {
	value, err := s.client.ZScore(ctx, key, member).Result()
	return value, err
}
