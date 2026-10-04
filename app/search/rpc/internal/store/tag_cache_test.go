package store

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type countingTagStore struct {
	Store
	calls atomic.Int32
	fn    func(context.Context, string, int32) ([]Tag, error)
}

func (s *countingTagStore) SearchTags(ctx context.Context, keyword string, limit int32) ([]Tag, error) {
	s.calls.Add(1)
	return s.fn(ctx, keyword, limit)
}

func fixedClock(start time.Time) (func() time.Time, func(time.Duration)) {
	var mu sync.Mutex
	now := start
	return func() time.Time { mu.Lock(); defer mu.Unlock(); return now },
		func(d time.Duration) { mu.Lock(); defer mu.Unlock(); now = now.Add(d) }
}

func TestTagCacheServesCaseFoldedHitsUntilTTL(t *testing.T) {
	next := &countingTagStore{fn: func(context.Context, string, int32) ([]Tag, error) {
		return []Tag{{Name: "golang", PostCount: 9}}, nil
	}}
	cache := NewTagCache(next, time.Minute, 4)
	now, advance := fixedClock(time.Unix(0, 0))
	cache.now = now

	for _, keyword := range []string{"Go", "go", "GO"} {
		tags, err := cache.SearchTags(context.Background(), keyword, 5)
		require.NoError(t, err)
		assert.Equal(t, []Tag{{Name: "golang", PostCount: 9}}, tags)
	}
	assert.EqualValues(t, 1, next.calls.Load())

	_, err := cache.SearchTags(context.Background(), "go", 6)
	require.NoError(t, err)
	assert.EqualValues(t, 2, next.calls.Load(), "limit is part of the key")

	advance(time.Minute)
	_, err = cache.SearchTags(context.Background(), "go", 5)
	require.NoError(t, err)
	assert.EqualValues(t, 3, next.calls.Load(), "expired entries are never served")
}

func TestTagCacheDoesNotCacheErrors(t *testing.T) {
	failure := errors.New("es unavailable")
	next := &countingTagStore{fn: func(context.Context, string, int32) ([]Tag, error) { return nil, failure }}
	cache := NewTagCache(next, time.Minute, 4)
	for range 2 {
		_, err := cache.SearchTags(context.Background(), "go", 5)
		require.ErrorIs(t, err, failure)
	}
	assert.EqualValues(t, 2, next.calls.Load())
}

func TestTagCacheEvictsOldestWhenFull(t *testing.T) {
	next := &countingTagStore{fn: func(_ context.Context, keyword string, _ int32) ([]Tag, error) {
		return []Tag{{Name: keyword}}, nil
	}}
	cache := NewTagCache(next, time.Minute, 2)
	now, advance := fixedClock(time.Unix(0, 0))
	cache.now = now
	for _, keyword := range []string{"a", "b", "c"} {
		_, err := cache.SearchTags(context.Background(), keyword, 1)
		require.NoError(t, err)
		advance(time.Second)
	}
	assert.Len(t, cache.entries, 2)
	_, err := cache.SearchTags(context.Background(), "a", 1)
	require.NoError(t, err)
	assert.EqualValues(t, 4, next.calls.Load(), "the oldest entry was evicted")
	_, err = cache.SearchTags(context.Background(), "c", 1)
	require.NoError(t, err)
	assert.EqualValues(t, 4, next.calls.Load())
}

func TestTagCacheSharesConcurrentMisses(t *testing.T) {
	release := make(chan struct{})
	next := &countingTagStore{fn: func(context.Context, string, int32) ([]Tag, error) {
		<-release
		return []Tag{{Name: "golang"}}, nil
	}}
	cache := NewTagCache(next, time.Minute, 4)
	var wg sync.WaitGroup
	for range 8 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			tags, err := cache.SearchTags(context.Background(), "go", 5)
			assert.NoError(t, err)
			assert.Len(t, tags, 1)
		}()
	}
	time.Sleep(20 * time.Millisecond)
	close(release)
	wg.Wait()
	assert.EqualValues(t, 1, next.calls.Load())
}

func TestTagCacheRetriesWhenSharedLeaderWasCanceled(t *testing.T) {
	leaderStarted := make(chan struct{})
	next := &countingTagStore{}
	next.fn = func(ctx context.Context, _ string, _ int32) ([]Tag, error) {
		if next.calls.Load() == 1 {
			close(leaderStarted)
			<-ctx.Done()
			return nil, ctx.Err()
		}
		return []Tag{{Name: "golang"}}, nil
	}
	cache := NewTagCache(next, time.Minute, 4)
	leaderCtx, cancel := context.WithCancel(context.Background())
	leaderDone := make(chan error, 1)
	go func() {
		_, err := cache.SearchTags(leaderCtx, "go", 5)
		leaderDone <- err
	}()
	<-leaderStarted
	followerDone := make(chan []Tag, 1)
	go func() {
		tags, err := cache.SearchTags(context.Background(), "go", 5)
		assert.NoError(t, err)
		followerDone <- tags
	}()
	time.Sleep(20 * time.Millisecond)
	cancel()
	require.ErrorIs(t, <-leaderDone, context.Canceled)
	assert.Equal(t, []Tag{{Name: "golang"}}, <-followerDone)
}
