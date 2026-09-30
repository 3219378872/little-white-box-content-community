// Package cachedstore adds disposable JSON model caching to explicit SQL stores.
package cachedstore

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"esx/pkg/logging"
	"esx/pkg/modelcache"
	"esx/pkg/redisstore"
	"esx/pkg/sqlstore"
	"fmt"
)

var ErrNotFound = sql.ErrNoRows

// v3 isolates entries written before cache fills were fenced by reservations.
const Prefix = "cache:v3:"

type CachedConn struct {
	conn    sqlstore.SqlConn
	redis   *redisstore.Redis
	options modelcache.Options
}

func NewConn(conn sqlstore.SqlConn, c modelcache.CacheConf, opts ...modelcache.Option) CachedConn {
	options := modelcache.Options{TTLSeconds: 7 * 24 * 3600, NotFoundTTLSeconds: 60}
	for _, opt := range opts {
		opt(&options)
	}
	var redis *redisstore.Redis
	if len(c) > 1 {
		panic("model cache requires a single Redis endpoint or cluster")
	}
	if len(c) == 1 {
		redis = redisstore.MustNewRedis(c[0].RedisConf)
	}
	return CachedConn{conn: conn, redis: redis, options: options}
}
func (c CachedConn) RawDB() (*sql.DB, error) { return c.conn.RawDB() }
func (c CachedConn) QueryRowNoCacheCtx(ctx context.Context, v any, q string, args ...any) error {
	return c.conn.QueryRowCtx(ctx, v, q, args...)
}
func (c CachedConn) QueryRowsNoCacheCtx(ctx context.Context, v any, q string, args ...any) error {
	return c.conn.QueryRowsCtx(ctx, v, q, args...)
}
func (c CachedConn) ExecNoCacheCtx(ctx context.Context, q string, args ...any) (sql.Result, error) {
	return c.conn.ExecCtx(ctx, q, args...)
}
func (c CachedConn) TransactCtx(ctx context.Context, fn func(context.Context, sqlstore.Session) error) error {
	return c.conn.TransactCtx(ctx, fn)
}
func (c CachedConn) DelCacheCtx(ctx context.Context, keys ...string) error {
	if c.redis == nil || len(keys) == 0 {
		return nil
	}
	var errs []error
	for _, k := range keys {
		// Each DEL is single-key, like the fill script, so unrelated keys work
		// in Redis Cluster too. Attempt every invalidation on partial failure.
		if _, err := c.redis.DelCtx(ctx, Prefix+k); err != nil {
			errs = append(errs, err)
		}
	}
	return errors.Join(errs...)
}
func (c CachedConn) ExecCtx(ctx context.Context, fn func(context.Context, sqlstore.SqlConn) (sql.Result, error), keys ...string) (sql.Result, error) {
	result, err := fn(ctx, c.conn)
	if err != nil {
		return result, err
	}
	if err = c.DelCacheCtx(ctx, keys...); err != nil {
		logging.WithContext(ctx).Errorw("model cache invalidation failed after committed write")
	}
	return result, nil
}
func (c CachedConn) QueryRowCtx(ctx context.Context, v any, key string, query func(context.Context, sqlstore.SqlConn, any) error) error {
	var reservation string
	if c.redis != nil {
		value, err := c.redis.GetCtx(ctx, Prefix+key)
		if err == nil && value != "" {
			var cached struct {
				Missing bool
				Value   json.RawMessage
			}
			if json.Unmarshal([]byte(value), &cached) == nil {
				if cached.Missing {
					return ErrNotFound
				}
				if len(cached.Value) > 0 && json.Unmarshal(cached.Value, v) == nil {
					return nil
				}
			}
		}
		if err == nil && value == "" {
			// Acquire before the SQL read. An invalidation removes this marker,
			// preventing that read from repopulating the cache afterward.
			reservation = c.reserveFill(ctx, Prefix+key)
		}
	}
	err := query(ctx, c.conn, v)
	if reservation != "" {
		c.finishFill(ctx, Prefix+key, reservation, v, err)
	}
	return err
}

func (c CachedConn) finishFill(ctx context.Context, key, reservation string, v any, err error) {
	var value string
	var ttl int
	if err == nil || errors.Is(err, ErrNotFound) {
		cached := struct {
			Missing bool
			Value   any
		}{Missing: errors.Is(err, ErrNotFound)}
		ttl = c.options.NotFoundTTLSeconds
		if err == nil {
			cached.Value = v
			ttl = c.options.TTLSeconds
		}
		if b, e := json.Marshal(cached); e == nil {
			value = string(b)
		}
	}
	// Only the original reservation owner can fill or release the key. Redis
	// errors, eviction, expiry and invalidation all fail closed for cache fill.
	_, _ = c.redis.EvalCtx(ctx, finishFillScript, []string{key}, reservation, value, ttl)
}

// Index entries contain only the primary ID. Row updates invalidate the primary
// cache without needing to discover every secondary lookup that has been used.
func (c CachedConn) QueryRowIndexCtx(ctx context.Context, v any, key string, primaryKey func(any) string, index func(context.Context, sqlstore.SqlConn, any) (any, error), primary func(context.Context, sqlstore.SqlConn, any, any) error) error {
	var id string
	err := c.QueryRowCtx(ctx, &id, key, func(ctx context.Context, conn sqlstore.SqlConn, _ any) error {
		value, err := index(ctx, conn, v)
		if err != nil {
			return err
		}
		id = fmt.Sprint(value)
		return nil
	})
	if err != nil {
		return err
	}
	return c.QueryRowCtx(ctx, v, primaryKey(id), func(ctx context.Context, conn sqlstore.SqlConn, out any) error {
		// The index query may precede a primary invalidation. Never transfer
		// that earlier row into a reservation acquired after the invalidation.
		return primary(ctx, conn, out, id)
	})
}
func (c CachedConn) String() string { return fmt.Sprintf("model cache (enabled=%t)", c.redis != nil) }
