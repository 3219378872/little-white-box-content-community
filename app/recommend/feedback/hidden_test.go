package feedback

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeHashes struct {
	key    string
	values map[string]string
	err    error
}

func (f *fakeHashes) HgetallCtx(_ context.Context, key string) (map[string]string, error) {
	f.key = key
	return f.values, f.err
}

func TestHiddenPostReaderUsesExistingNegativeFeatureKeys(t *testing.T) {
	redis := &fakeHashes{values: map[string]string{"post:44": "1", "user:90": "1"}}
	reader := NewHiddenPostReader(redis, "")
	ids, err := reader.HiddenPosts(context.Background(), 7)
	require.NoError(t, err)
	assert.Equal(t, "feature:v1:u:7:negative", redis.key)
	assert.Equal(t, map[int64]struct{}{44: {}}, ids)
	redis.err = errors.New("redis unavailable")
	_, err = reader.HiddenPosts(context.Background(), 7)
	assert.Error(t, err)
	redis.err = nil
	redis.values = map[string]string{"post:bad": "1"}
	_, err = reader.HiddenPosts(context.Background(), 7)
	assert.Error(t, err)
	_, err = NewHiddenPostReader(nil, "v1").HiddenPosts(context.Background(), 7)
	assert.Error(t, err)
}

func TestAnonymousFallbackDoesNotReadOrCreateProfile(t *testing.T) {
	reader := NewHiddenPostReader(nil, "v1")
	ids, err := reader.HiddenPosts(context.Background(), 0)
	require.NoError(t, err)
	assert.Empty(t, ids)
}
