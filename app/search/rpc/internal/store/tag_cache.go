package store

import (
	"context"
	"errors"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

const (
	// TagCacheTTL bounds how long tag post counts and tag existence may lag
	// the index. ES only indexes published posts, so the cache adds no new
	// invisible content.
	TagCacheTTL = 60 * time.Second
	// TagCacheCapacity bounds memory; the oldest entry is evicted when full.
	TagCacheCapacity = 1024
)

// TagCache serves SearchTags from a per-process TTL cache keyed by the
// case-folded keyword and limit (matching is case-insensitive). Concurrent
// misses for one key share a single aggregation. Errors are never cached and
// an expired entry is never served.
type TagCache struct {
	Store
	ttl      time.Duration
	capacity int
	now      func() time.Time

	mu      sync.Mutex
	entries map[string]tagCacheEntry
	group   singleflight.Group
}

// tagCacheEntry stores a private copy of the tags and when they were cached.
type tagCacheEntry struct {
	tags     []Tag
	storedAt time.Time
}

// NewTagCache wraps next; a non-positive ttl or capacity disables caching.
func NewTagCache(next Store, ttl time.Duration, capacity int) *TagCache {
	return &TagCache{
		Store: next, ttl: ttl, capacity: capacity, now: time.Now,
		entries: make(map[string]tagCacheEntry),
	}
}

// SearchTags serves a fresh cached copy when present, otherwise loads through
// singleflight so concurrent misses issue one aggregation.
func (c *TagCache) SearchTags(ctx context.Context, keyword string, limit int32) ([]Tag, error) {
	key := strings.ToLower(keyword) + "\x00" + strconv.Itoa(int(limit))
	if tags, ok := c.lookup(key); ok {
		return tags, nil
	}
	value, err, _ := c.group.Do(key, func() (any, error) {
		tags, err := c.Store.SearchTags(ctx, keyword, limit)
		if err != nil {
			return nil, err
		}
		c.store(key, tags)
		return tags, nil
	})
	if err != nil {
		// A shared call can fail because the leader's request was canceled;
		// a caller whose own request is still live retries on its own.
		if ctx.Err() == nil && (errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded)) {
			return c.Store.SearchTags(ctx, keyword, limit)
		}
		return nil, err
	}
	return append([]Tag(nil), value.([]Tag)...), nil
}

// lookup returns a copy of a live entry and drops an expired one.
func (c *TagCache) lookup(key string) ([]Tag, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	entry, ok := c.entries[key]
	if !ok {
		return nil, false
	}
	if c.now().Sub(entry.storedAt) >= c.ttl {
		delete(c.entries, key)
		return nil, false
	}
	return append([]Tag(nil), entry.tags...), true
}

// store caches a copy of tags; when full it evicts an expired entry if one is
// found, otherwise the oldest entry.
func (c *TagCache) store(key string, tags []Tag) {
	if c.ttl <= 0 || c.capacity <= 0 {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	now := c.now()
	if _, exists := c.entries[key]; !exists && len(c.entries) >= c.capacity {
		oldestKey, oldest := "", now
		for k, e := range c.entries {
			if now.Sub(e.storedAt) >= c.ttl {
				oldestKey = k
				break
			}
			if oldestKey == "" || e.storedAt.Before(oldest) {
				oldestKey, oldest = k, e.storedAt
			}
		}
		delete(c.entries, oldestKey)
	}
	c.entries[key] = tagCacheEntry{tags: append([]Tag(nil), tags...), storedAt: now}
}
