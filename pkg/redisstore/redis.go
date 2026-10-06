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

// RedisConf selects a single node or a cluster; Tls enables TLS 1.2+.
type RedisConf struct {
	Host       string
	Type       string `json:",default=node,options=node|cluster"`
	User, Pass string
	Tls        bool
}

// RedisKeyConf is a Redis connection plus the key the service registers under.
type RedisKeyConf struct {
	RedisConf
	Key string
}

// FloatPair is a sorted-set member with its score.
type FloatPair struct {
	Key   string
	Score float64
}

// Redis wraps a go-redis universal client with the Ctx-style API the services use.
type Redis struct {
	client    r.UniversalClient
	closeOnce sync.Once
	closeErr  error
}

// NewRedis builds a node or cluster client; the host is required.
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

// MustNewRedis panics when the client cannot be built (startup only).
func MustNewRedis(c RedisConf) *Redis {
	v, e := NewRedis(c)
	if e != nil {
		panic(e)
	}
	return v
}

// Close closes the client once; later calls return the first result.
func (s *Redis) Close() error {
	s.closeOnce.Do(func() { s.closeErr = s.client.Close() })
	return s.closeErr
}

// GetCtx returns "" with a nil error for a missing key, so callers test the value
// instead of matching redis.Nil.
func (s *Redis) GetCtx(ctx context.Context, k string) (string, error) {
	v, e := s.client.Get(ctx, k).Result()
	if errors.Is(e, r.Nil) {
		return "", nil
	}
	return v, e
}

// SetCtx stores a value without expiry.
func (s *Redis) SetCtx(ctx context.Context, k, v string) error {
	return s.client.Set(ctx, k, v, 0).Err()
}

// SetexCtx stores a value that expires after seconds.
func (s *Redis) SetexCtx(ctx context.Context, k, v string, seconds int) error {
	return s.client.Set(ctx, k, v, time.Duration(seconds)*time.Second).Err()
}

// SetnxExCtx stores the value only when absent and reports whether it did; used for locks and dedup markers.
func (s *Redis) SetnxExCtx(ctx context.Context, k, v string, seconds int) (bool, error) {
	return s.client.SetNX(ctx, k, v, time.Duration(seconds)*time.Second).Result()
}

// DelCtx deletes keys and returns how many existed.
func (s *Redis) DelCtx(ctx context.Context, k ...string) (int, error) {
	n, e := s.client.Del(ctx, k...).Result()
	return int(n), e
}

// EvalCtx runs a Lua script atomically.
func (s *Redis) EvalCtx(ctx context.Context, script string, keys []string, args ...any) (any, error) {
	return s.client.Eval(ctx, script, keys, args...).Result()
}

// GetManyCtx pipelines one single-key GET per key, so unrelated keys also work
// in Redis Cluster. Missing keys are returned as "".
func (s *Redis) GetManyCtx(ctx context.Context, keys []string) ([]string, error) {
	cmds := make([]*r.StringCmd, len(keys))
	_, err := s.client.Pipelined(ctx, func(p r.Pipeliner) error {
		for i, k := range keys {
			cmds[i] = p.Get(ctx, k)
		}
		return nil
	})
	if err != nil && !errors.Is(err, r.Nil) {
		return nil, err
	}
	values := make([]string, len(keys))
	for i, cmd := range cmds {
		v, e := cmd.Result()
		if e != nil && !errors.Is(e, r.Nil) {
			return nil, e
		}
		values[i] = v
	}
	return values, nil
}

// SetnxExManyCtx pipelines one single-key SET NX EX per key. acquired[i] is
// false when the key exists or that command failed.
func (s *Redis) SetnxExManyCtx(ctx context.Context, keys, values []string, seconds int) ([]bool, error) {
	if len(keys) != len(values) {
		return nil, fmt.Errorf("redis SETNX batch has %d keys and %d values", len(keys), len(values))
	}
	cmds := make([]*r.BoolCmd, len(keys))
	_, err := s.client.Pipelined(ctx, func(p r.Pipeliner) error {
		for i, k := range keys {
			cmds[i] = p.SetNX(ctx, k, values[i], time.Duration(seconds)*time.Second)
		}
		return nil
	})
	acquired := make([]bool, len(keys))
	for i, cmd := range cmds {
		acquired[i] = cmd.Err() == nil && cmd.Val()
	}
	return acquired, err
}

// EvalEachCtx pipelines one single-key EVAL of script per key with its own
// arguments, and joins the per-command errors.
func (s *Redis) EvalEachCtx(ctx context.Context, script string, keys []string, args [][]any) error {
	if len(keys) != len(args) {
		return fmt.Errorf("redis EVAL batch has %d keys and %d argument sets", len(keys), len(args))
	}
	cmds := make([]*r.Cmd, len(keys))
	_, err := s.client.Pipelined(ctx, func(p r.Pipeliner) error {
		for i, k := range keys {
			cmds[i] = p.Eval(ctx, script, []string{k}, args[i]...)
		}
		return nil
	})
	if err == nil {
		return nil
	}
	var errs []error
	for _, cmd := range cmds {
		if e := cmd.Err(); e != nil && !errors.Is(e, r.Nil) {
			errs = append(errs, e)
		}
	}
	return errors.Join(errs...)
}

// ExpireCtx sets a TTL in seconds.
func (s *Redis) ExpireCtx(ctx context.Context, k string, seconds int) error {
	return s.client.Expire(ctx, k, time.Duration(seconds)*time.Second).Err()
}

// IncrCtx increments a counter and returns the new value.
func (s *Redis) IncrCtx(ctx context.Context, k string) (int64, error) {
	return s.client.Incr(ctx, k).Result()
}

// ExistsCtx reports whether the key exists.
func (s *Redis) ExistsCtx(ctx context.Context, k string) (bool, error) {
	n, e := s.client.Exists(ctx, k).Result()
	return n > 0, e
}

// HgetallCtx returns all hash fields; a missing key yields an empty map.
func (s *Redis) HgetallCtx(ctx context.Context, k string) (map[string]string, error) {
	return s.client.HGetAll(ctx, k).Result()
}

// HsetCtx sets one hash field.
func (s *Redis) HsetCtx(ctx context.Context, k, field, v string) error {
	return s.client.HSet(ctx, k, field, v).Err()
}

// KeysCtx lists keys matching a pattern; reserved for bounded maintenance scans.
func (s *Redis) KeysCtx(ctx context.Context, p string) ([]string, error) {
	return s.client.Keys(ctx, p).Result()
}

// ScanCtx iterates keys incrementally.
func (s *Redis) ScanCtx(ctx context.Context, cursor uint64, match string, count int64) ([]string, uint64, error) {
	return s.client.Scan(ctx, cursor, match, count).Result()
}

// TtlCtx returns the remaining TTL in seconds; negative Redis sentinels are passed through.
func (s *Redis) TtlCtx(ctx context.Context, k string) (int, error) {
	v, e := s.client.TTL(ctx, k).Result()
	if v < 0 {
		return int(v), e
	}
	return int(v / time.Second), e
}

// LlenCtx returns the list length.
func (s *Redis) LlenCtx(ctx context.Context, k string) (int, error) {
	v, e := s.client.LLen(ctx, k).Result()
	return int(v), e
}

// LrangeCtx returns list elements between start and stop (inclusive).
func (s *Redis) LrangeCtx(ctx context.Context, k string, start, stop int) ([]string, error) {
	return s.client.LRange(ctx, k, int64(start), int64(stop)).Result()
}

// SmembersCtx returns all set members.
func (s *Redis) SmembersCtx(ctx context.Context, k string) ([]string, error) {
	return s.client.SMembers(ctx, k).Result()
}

// ZaddCtx adds a member with an integer score and reports whether it was new.
func (s *Redis) ZaddCtx(ctx context.Context, k string, score int64, v string) (bool, error) {
	n, e := s.client.ZAdd(ctx, k, r.Z{Score: float64(score), Member: v}).Result()
	return n > 0, e
}

// ZrankCtx returns the ascending rank of a member.
func (s *Redis) ZrankCtx(ctx context.Context, k, v string) (int64, error) {
	n, e := s.client.ZRank(ctx, k, v).Result()
	return n, e
}

// ZscoreCtx returns a member score truncated to an integer.
func (s *Redis) ZscoreCtx(ctx context.Context, k, v string) (int64, error) {
	n, e := s.client.ZScore(ctx, k, v).Result()
	return int64(n), e
}

// ZremCtx removes members and returns how many were present.
func (s *Redis) ZremCtx(ctx context.Context, k string, values ...any) (int, error) {
	n, e := s.client.ZRem(ctx, k, values...).Result()
	return int(n), e
}

// ZrevrangeWithScoresByFloatCtx returns members by descending score, keeping float scores.
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

// HgetCtx returns one hash field.
func (s *Redis) HgetCtx(ctx context.Context, k, field string) (string, error) {
	v, e := s.client.HGet(ctx, k, field).Result()
	return v, e
}

// HincrbyCtx increments a hash field and returns the new value.
func (s *Redis) HincrbyCtx(ctx context.Context, k, field string, n int) (int, error) {
	v, e := s.client.HIncrBy(ctx, k, field, int64(n)).Result()
	return int(v), e
}

// ZscoreByFloatCtx returns a member score without truncation.
func (s *Redis) ZscoreByFloatCtx(ctx context.Context, key, member string) (float64, error) {
	value, err := s.client.ZScore(ctx, key, member).Result()
	return value, err
}
