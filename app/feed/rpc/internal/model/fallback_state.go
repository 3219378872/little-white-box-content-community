package model

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
)

// ErrFallbackStateMissing 表示兜底游标对应的状态已过期或不存在。
var ErrFallbackStateMissing = errors.New("fallback state missing")

// FallbackBinding 把兜底游标绑定到发起请求的身份与参数，防止游标被换到其他请求上续读。
type FallbackBinding struct {
	IdentityHash string `json:"identity"`
	RequestID    string `json:"request"`
	Scene        string `json:"scene"`
	SessionID    string `json:"session"`
	ExperimentID string `json:"experiment"`
	PageSize     int32  `json:"page_size"`
}

// FallbackSource 是一路兜底候选（follow/popular/latest）的翻页进度与尚未下发的候选。
type FallbackSource struct {
	Name            string  `json:"name"`
	Cursor          string  `json:"cursor,omitempty"`
	CursorCreatedAt int64   `json:"created_at,omitempty"`
	CursorPostID    int64   `json:"post_id,omitempty"`
	Exhausted       bool    `json:"exhausted"`
	Pending         []int64 `json:"pending,omitempty"`
}

// FallbackState 是兜底流跨页的服务端状态：各路进度、轮询位置与已下发帖子（用于跨页去重）。
type FallbackState struct {
	Binding    FallbackBinding  `json:"binding"`
	ExpiresAt  int64            `json:"expires_at"`
	Position   int32            `json:"position"`
	NextSource int              `json:"next_source"`
	Seen       []int64          `json:"seen"`
	Sources    []FallbackSource `json:"sources"`
}

// FallbackStateStore 按 ID 保存和读取兜底状态。
type FallbackStateStore interface {
	Save(context.Context, string, FallbackState, int) error
	Load(context.Context, string) (FallbackState, error)
}

// fallbackRedis 是状态存储用到的 Redis 子集，便于测试替换。
type fallbackRedis interface {
	GetCtx(context.Context, string) (string, error)
	SetexCtx(context.Context, string, string, int) error
}

// redisFallbackStateStore 把状态以 JSON 存在 Redis，随游标过期自动清理。
type redisFallbackStateStore struct{ redis fallbackRedis }

// NewFallbackStateStore 创建基于 Redis 的状态存储。
func NewFallbackStateStore(client fallbackRedis) FallbackStateStore {
	return &redisFallbackStateStore{redis: client}
}

// Save 写入状态并设置 TTL；拒绝无过期时间的写入，避免状态永久残留。
func (s *redisFallbackStateStore) Save(ctx context.Context, id string, state FallbackState, ttl int) error {
	if id == "" || ttl <= 0 {
		return fmt.Errorf("invalid fallback state expiry")
	}
	encoded, err := json.Marshal(state)
	if err != nil {
		return err
	}
	return s.redis.SetexCtx(ctx, "feed:fallback:"+id, string(encoded), ttl)
}

// Load 读取状态；键不存在时返回 ErrFallbackStateMissing。
func (s *redisFallbackStateStore) Load(ctx context.Context, id string) (FallbackState, error) {
	raw, err := s.redis.GetCtx(ctx, "feed:fallback:"+id)
	if err != nil {
		return FallbackState{}, err
	}
	if raw == "" {
		return FallbackState{}, ErrFallbackStateMissing
	}
	var state FallbackState
	if err := json.Unmarshal([]byte(raw), &state); err != nil {
		return FallbackState{}, err
	}
	return state, nil
}
