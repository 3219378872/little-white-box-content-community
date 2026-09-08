package model

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
)

var ErrFallbackStateMissing = errors.New("fallback state missing")

type FallbackBinding struct {
	IdentityHash string `json:"identity"`
	RequestID    string `json:"request"`
	Scene        string `json:"scene"`
	SessionID    string `json:"session"`
	ExperimentID string `json:"experiment"`
	PageSize     int32  `json:"page_size"`
}

type FallbackSource struct {
	Name            string  `json:"name"`
	Cursor          string  `json:"cursor,omitempty"`
	CursorCreatedAt int64   `json:"created_at,omitempty"`
	CursorPostID    int64   `json:"post_id,omitempty"`
	Exhausted       bool    `json:"exhausted"`
	Pending         []int64 `json:"pending,omitempty"`
}

type FallbackState struct {
	Binding    FallbackBinding  `json:"binding"`
	ExpiresAt  int64            `json:"expires_at"`
	Position   int32            `json:"position"`
	NextSource int              `json:"next_source"`
	Seen       []int64          `json:"seen"`
	Sources    []FallbackSource `json:"sources"`
}

type FallbackStateStore interface {
	Save(context.Context, string, FallbackState, int) error
	Load(context.Context, string) (FallbackState, error)
}

type fallbackRedis interface {
	GetCtx(context.Context, string) (string, error)
	SetexCtx(context.Context, string, string, int) error
}

type redisFallbackStateStore struct{ redis fallbackRedis }

func NewFallbackStateStore(client fallbackRedis) FallbackStateStore {
	return &redisFallbackStateStore{redis: client}
}

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
