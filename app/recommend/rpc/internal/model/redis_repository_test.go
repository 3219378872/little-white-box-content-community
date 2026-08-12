package model

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/zeromicro/go-zero/core/stores/redis"
)

type fakeRedisClient struct {
	values  map[string]string
	hashes  map[string]map[string]string
	lists   map[string][]string
	sets    map[string][]string
	zsets   map[string][]redis.FloatPair
	setKeys []string
	setTTL  int
}

func (f *fakeRedisClient) GetCtx(_ context.Context, key string) (string, error) {
	value, ok := f.values[key]
	if !ok {
		return "", redis.Nil
	}
	return value, nil
}

func (f *fakeRedisClient) HgetallCtx(_ context.Context, key string) (map[string]string, error) {
	return f.hashes[key], nil
}

func (f *fakeRedisClient) LrangeCtx(_ context.Context, key string, _, _ int) ([]string, error) {
	return f.lists[key], nil
}

func (f *fakeRedisClient) SetexCtx(_ context.Context, key, value string, seconds int) error {
	if f.values == nil {
		f.values = make(map[string]string)
	}
	f.values[key] = value
	f.setKeys = append(f.setKeys, key)
	f.setTTL = seconds
	return nil
}

func (f *fakeRedisClient) SmembersCtx(_ context.Context, key string) ([]string, error) {
	return f.sets[key], nil
}

func (f *fakeRedisClient) ZrevrangeWithScoresByFloatCtx(_ context.Context, key string, _, _ int64) ([]redis.FloatPair, error) {
	return f.zsets[key], nil
}

func TestRedisFeatureRepositoryUsesVersionedFeatureKeys(t *testing.T) {
	client := &fakeRedisClient{
		hashes: map[string]map[string]string{
			"feature:v2:u:42:positive": {"post:1": "2"},
			"feature:v2:u:42:negative": {"post:2": "1"},
			"feature:v2:post:9": {
				"status": "published", "visibility": "public", "author_id": "5",
				"category": "tech", "quality_score": "0.8", "ctr": "0.2",
			},
		},
		lists: map[string][]string{
			"feature:v2:u:42:recent": {fmt.Sprintf(`{"action":"exposure","target_id":3,"target_type":"post","event_time":%d}`, time.Now().UnixMilli())},
		},
		sets: map[string][]string{"feature:v2:u:42:blocked_authors": {"7"}},
	}
	repository := NewRedisFeatureRepository(client, "v2")
	viewer, err := repository.LoadViewerFeatures(context.Background(), "u:42")
	if err != nil {
		t.Fatalf("LoadViewerFeatures() error = %v", err)
	}
	if _, ok := viewer.PositivePostIDs[1]; !ok {
		t.Fatal("positive post feature was not loaded")
	}
	if _, ok := viewer.NegativePostIDs[2]; !ok {
		t.Fatal("negative post feature was not loaded")
	}
	if _, ok := viewer.SeenPostIDs[3]; !ok {
		t.Fatal("exposure feature was not loaded")
	}
	if _, ok := viewer.BlockedAuthors[7]; !ok {
		t.Fatal("blocked author feature was not loaded")
	}
	posts, err := repository.LoadPostFeatures(context.Background(), []int64{9})
	if err != nil {
		t.Fatalf("LoadPostFeatures() error = %v", err)
	}
	if !posts[9].Known || posts[9].AuthorID != 5 || posts[9].Category != "tech" || posts[9].Quality != 0.8 {
		t.Fatalf("unexpected post features: %+v", posts[9])
	}
}

func TestRedisFeatureRepositorySeenWindowIsSevenDays(t *testing.T) {
	now := time.Now()
	client := &fakeRedisClient{
		lists: map[string][]string{
			"feature:v2:u:42:recent": {
				fmt.Sprintf(`{"action":"exposure","target_id":3,"target_type":"post","event_time":%d}`, now.Add(-6*24*time.Hour).UnixMilli()),
				fmt.Sprintf(`{"action":"exposure","target_id":4,"target_type":"post","event_time":%d}`, now.Add(-8*24*time.Hour).UnixMilli()),
			},
		},
	}
	repository := NewRedisFeatureRepository(client, "v2")
	repository.now = func() time.Time { return now }
	viewer, err := repository.LoadViewerFeatures(context.Background(), "u:42")
	if err != nil {
		t.Fatalf("LoadViewerFeatures() error = %v", err)
	}
	if _, ok := viewer.SeenPostIDs[3]; !ok {
		t.Fatal("exposure within 7 days should be seen")
	}
	if _, ok := viewer.SeenPostIDs[4]; ok {
		t.Fatal("exposure older than 7 days should not be seen")
	}
}

func TestRedisSnapshotStoreRoundTripAndMissing(t *testing.T) {
	client := &fakeRedisClient{values: make(map[string]string)}
	store := NewRedisSnapshotStore(client, "recommend:v2")
	want := PostSnapshot{RequestID: "request-1", IdentityHash: "identity", Scene: "home", ExpiresAt: 123, Posts: []RankedPost{{PostID: 9}}}
	if err := store.Save(context.Background(), "snapshot-1", want, 600); err != nil {
		t.Fatalf("Save() error = %v", err)
	}
	got, err := store.Load(context.Background(), "snapshot-1")
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if got.RequestID != want.RequestID || len(got.Posts) != 1 || got.Posts[0].PostID != 9 {
		t.Fatalf("Load() = %+v, want %+v", got, want)
	}
	if len(client.setKeys) != 1 || client.setKeys[0] != "recommend:v2:snapshot:snapshot-1" || client.setTTL != 600 {
		t.Fatalf("snapshot key/ttl = %v/%d", client.setKeys, client.setTTL)
	}
	if _, err := store.Load(context.Background(), "missing"); err != ErrSnapshotMissing {
		t.Fatalf("missing Load() error = %v, want ErrSnapshotMissing", err)
	}
}

func TestIdentityKeyDoesNotExposeAnonymousIdentifier(t *testing.T) {
	key := IdentityKey(0, "private-device-id")
	if key == "" || key == "a:private-device-id" {
		t.Fatalf("anonymous identity was not hashed: %q", key)
	}
	if key != IdentityKey(0, "private-device-id") {
		t.Fatal("anonymous identity hashing is not deterministic")
	}
}
