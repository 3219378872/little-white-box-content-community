package model

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fallbackRedisStub struct {
	value string
	ttl   int
	err   error
}

func (s *fallbackRedisStub) GetCtx(context.Context, string) (string, error) { return s.value, s.err }
func (s *fallbackRedisStub) SetexCtx(_ context.Context, _, value string, ttl int) error {
	if s.err != nil {
		return s.err
	}
	s.value, s.ttl = value, ttl
	return nil
}

func TestFallbackStateStoreRoundTripAndFailures(t *testing.T) {
	ctx := context.Background()
	redis := new(fallbackRedisStub)
	store := NewFallbackStateStore(redis)
	state := FallbackState{Binding: FallbackBinding{IdentityHash: "identity", RequestID: "request", PageSize: 2}, ExpiresAt: 100, Seen: []int64{1}, Sources: []FallbackSource{{Name: "popular", Pending: []int64{2}, Exhausted: true}}}
	require.NoError(t, store.Save(ctx, "state", state, 60))
	actual, err := store.Load(ctx, "state")
	require.NoError(t, err)
	assert.Equal(t, state, actual)
	assert.Equal(t, 60, redis.ttl)
	assert.Error(t, store.Save(ctx, "state", state, 0))
	redis.value = ""
	_, err = store.Load(ctx, "state")
	assert.ErrorIs(t, err, ErrFallbackStateMissing)
	redis.value = "corrupt"
	_, err = store.Load(ctx, "state")
	assert.Error(t, err)
	redis.err = errors.New("unavailable")
	assert.Error(t, store.Save(ctx, "state", state, 60))
	_, err = store.Load(ctx, "state")
	assert.Error(t, err)
}
