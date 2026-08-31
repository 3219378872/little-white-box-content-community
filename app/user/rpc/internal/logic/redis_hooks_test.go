package logic

import (
	"context"
	"errors"
)

// errInjectedRedis 统一的 redis 故障注入错误。
var errInjectedRedis = errors.New("injected redis failure")

// flakyRedis 在 memoryRedis 基础上按 key 注入单点故障，
// 用于覆盖各 logic 的 Redis 失败降级分支。
type flakyRedis struct {
	*memoryRedis
	onGet     func(key string) error
	onDel     func(keys ...string) error
	onEval    func(keys []string, args ...any) error
	onSetex   func(key string) error
	onSetnxEx func(key string) error
	onIncr    func(key string) error
	onExpire  func(key string) error
}

func (r *flakyRedis) EvalCtx(ctx context.Context, script string, keys []string, args ...any) (any, error) {
	if r.onEval != nil {
		if err := r.onEval(keys, args...); err != nil {
			return nil, err
		}
	}
	return r.memoryRedis.EvalCtx(ctx, script, keys, args...)
}

func (r *flakyRedis) GetCtx(ctx context.Context, key string) (string, error) {
	if r.onGet != nil {
		if err := r.onGet(key); err != nil {
			return "", err
		}
	}
	return r.memoryRedis.GetCtx(ctx, key)
}

func (r *flakyRedis) DelCtx(ctx context.Context, keys ...string) (int, error) {
	if r.onDel != nil {
		if err := r.onDel(keys...); err != nil {
			return 0, err
		}
	}
	return r.memoryRedis.DelCtx(ctx, keys...)
}

func (r *flakyRedis) SetexCtx(ctx context.Context, key, value string, seconds int) error {
	if r.onSetex != nil {
		if err := r.onSetex(key); err != nil {
			return err
		}
	}
	return r.memoryRedis.SetexCtx(ctx, key, value, seconds)
}

func (r *flakyRedis) SetnxExCtx(ctx context.Context, key, value string, seconds int) (bool, error) {
	if r.onSetnxEx != nil {
		if err := r.onSetnxEx(key); err != nil {
			return false, err
		}
	}
	return r.memoryRedis.SetnxExCtx(ctx, key, value, seconds)
}

func (r *flakyRedis) IncrCtx(ctx context.Context, key string) (int64, error) {
	if r.onIncr != nil {
		if err := r.onIncr(key); err != nil {
			return 0, err
		}
	}
	return r.memoryRedis.IncrCtx(ctx, key)
}

func (r *flakyRedis) ExpireCtx(ctx context.Context, key string, seconds int) error {
	if r.onExpire != nil {
		if err := r.onExpire(key); err != nil {
			return err
		}
	}
	return r.memoryRedis.ExpireCtx(ctx, key, seconds)
}
