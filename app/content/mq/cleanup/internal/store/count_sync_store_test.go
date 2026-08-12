package store

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"esx/pkg/event"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeCountRedis struct {
	setnxResults map[string]bool
	deleted      []string
}

func (f *fakeCountRedis) SetnxExCtx(_ context.Context, key, _ string, _ int) (bool, error) {
	if result, ok := f.setnxResults[key]; ok {
		return result, nil
	}
	return true, nil
}

func (f *fakeCountRedis) DelCtx(_ context.Context, keys ...string) (int, error) {
	f.deleted = append(f.deleted, keys...)
	return len(keys), nil
}

type fakeCountDB struct {
	queries []string
	err     error
}

func (f *fakeCountDB) ExecContext(_ context.Context, query string, args ...any) (sql.Result, error) {
	if f.err != nil {
		return nil, f.err
	}
	f.queries = append(f.queries, query)
	return nil, nil
}

func likeEvent(id, targetID int64, action, targetType string) event.BehaviorEvent {
	return event.BehaviorEvent{
		EventID: id, ClientEventID: "client-" + action, SchemaVersion: event.BehaviorSchemaVersion,
		EventTime: 100, ReceivedAt: 100, UserID: 7, Action: action,
		TargetID: targetID, TargetType: targetType, Producer: "business-outbox",
	}
}

func TestCountSyncStoreAppliesLikeDelta(t *testing.T) {
	db := &fakeCountDB{}
	redisClient := &fakeCountRedis{setnxResults: map[string]bool{}}
	store := NewCountSyncStore(db, redisClient)

	require.NoError(t, store.ApplyBehaviorCount(context.Background(), likeEvent(1, 55, event.BehaviorActionLike, "post")))
	require.Len(t, db.queries, 1)
	assert.Contains(t, db.queries[0], "`like_count` = `like_count` + ?")
	assert.Equal(t, []string{"cache:post:id:55"}, redisClient.deleted)
}

func TestCountSyncStoreAppliesUnfavoriteDeltaClamped(t *testing.T) {
	db := &fakeCountDB{}
	store := NewCountSyncStore(db, &fakeCountRedis{setnxResults: map[string]bool{}})

	require.NoError(t, store.ApplyBehaviorCount(context.Background(), likeEvent(2, 55, event.BehaviorActionUnfavorite, "post")))
	require.Len(t, db.queries, 1)
	assert.Contains(t, db.queries[0], "GREATEST(`favorite_count` + ?, 0)")
}

func TestCountSyncStoreCommentTargetInvalidatesCommentCache(t *testing.T) {
	db := &fakeCountDB{}
	redisClient := &fakeCountRedis{setnxResults: map[string]bool{}}
	store := NewCountSyncStore(db, redisClient)

	require.NoError(t, store.ApplyBehaviorCount(context.Background(), likeEvent(3, 88, event.BehaviorActionLike, "comment")))
	require.Len(t, db.queries, 1)
	assert.Contains(t, db.queries[0], "`like_count` = `like_count` + ?")
	assert.Equal(t, []string{"cache:comment:id:88"}, redisClient.deleted)
}

func TestCountSyncStoreDedupsRedeliveredEvent(t *testing.T) {
	db := &fakeCountDB{}
	redisClient := &fakeCountRedis{setnxResults: map[string]bool{"content:countsync:dedup:9": false}}
	store := NewCountSyncStore(db, redisClient)

	require.NoError(t, store.ApplyBehaviorCount(context.Background(), likeEvent(9, 55, event.BehaviorActionLike, "post")))
	assert.Empty(t, db.queries, "duplicate event must not re-apply the count")
}

func TestCountSyncStoreNonCountActionsIgnored(t *testing.T) {
	db := &fakeCountDB{}
	store := NewCountSyncStore(db, &fakeCountRedis{setnxResults: map[string]bool{}})

	require.NoError(t, store.ApplyBehaviorCount(context.Background(), likeEvent(10, 55, event.BehaviorActionClick, "post")))
	assert.Empty(t, db.queries)
}

func TestCountSyncStoreDBFailurePropagates(t *testing.T) {
	store := NewCountSyncStore(&fakeCountDB{err: errors.New("db down")}, &fakeCountRedis{setnxResults: map[string]bool{}})
	err := store.ApplyBehaviorCount(context.Background(), likeEvent(11, 55, event.BehaviorActionLike, "post"))
	require.Error(t, err)
}
