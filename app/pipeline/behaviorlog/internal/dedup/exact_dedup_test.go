package dedup

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeExactStore struct {
	values    map[string]string
	existsErr error
	setErr    error
	ttl       int
}

func (s *fakeExactStore) Exists(_ context.Context, key string) (bool, error) {
	if s.existsErr != nil {
		return false, s.existsErr
	}
	_, ok := s.values[key]
	return ok, nil
}

func (s *fakeExactStore) Set(_ context.Context, key, value string, seconds int) error {
	if s.setErr != nil {
		return s.setErr
	}
	s.values[key] = value
	s.ttl = seconds
	return nil
}

func TestExactDedup(t *testing.T) {
	store := &fakeExactStore{values: map[string]string{}}
	dedup := NewExactDedup(store, 3600)

	duplicate, err := dedup.IsDuplicate(context.Background(), "42")
	require.NoError(t, err)
	assert.False(t, duplicate)
	require.NoError(t, dedup.MarkProcessed(context.Background(), "42"))
	duplicate, err = dedup.IsDuplicate(context.Background(), "42")
	require.NoError(t, err)
	assert.True(t, duplicate)
	assert.Equal(t, 3600, store.ttl)
}

func TestExactDedupPropagatesStoreErrors(t *testing.T) {
	dedup := NewExactDedup(&fakeExactStore{values: map[string]string{}, existsErr: errors.New("redis down")}, 1)
	_, err := dedup.IsDuplicate(context.Background(), "42")
	assert.ErrorContains(t, err, "redis down")
}
