package store

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"esx/pkg/event"
	"esx/pkg/mqx"

	sqlx "esx/pkg/sqlstore"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeCountRedis 记录被删除的缓存键。
type fakeCountRedis struct {
	deleted []string
}

func (f *fakeCountRedis) DelCtx(_ context.Context, keys ...string) (int, error) {
	f.deleted = append(f.deleted, keys...)
	return len(keys), nil
}

// execCall 是一次记录下来的写语句。
type execCall struct {
	query string
	args  []any
}

// fakeCountConn 模拟连接与事务：affected 决定 UPDATE 影响行数，postCounts 是事务内读回的帖子计数，
// execErr 模拟数据库故障；事务回滚时丢弃事务内记录的写入。
type fakeCountConn struct {
	affected   int64
	postCounts postCounts
	execErr    error
	execs      []execCall
}

func (f *fakeCountConn) ExecCtx(_ context.Context, query string, args ...any) (sql.Result, error) {
	if f.execErr != nil {
		return nil, f.execErr
	}
	f.execs = append(f.execs, execCall{query: query, args: args})
	// outbox 插入总是成功；计数 UPDATE 返回配置的影响行数。
	if strings.Contains(query, "event_outbox") {
		return driver.RowsAffected(1), nil
	}
	return driver.RowsAffected(f.affected), nil
}

func (f *fakeCountConn) QueryRowCtx(_ context.Context, v any, _ string, _ ...any) error {
	reflect.ValueOf(v).Elem().Set(reflect.ValueOf(f.postCounts))
	return nil
}

func (f *fakeCountConn) QueryRowsCtx(context.Context, any, string, ...any) error {
	return errors.New("not supported")
}

func (f *fakeCountConn) PrepareCtx(context.Context, string) (sqlx.StmtSession, error) {
	return nil, errors.New("not supported")
}

func (f *fakeCountConn) TransactCtx(ctx context.Context, fn func(context.Context, sqlx.Session) error) error {
	before := len(f.execs)
	if err := fn(ctx, f); err != nil {
		f.execs = f.execs[:before]
		return err
	}
	return nil
}

// countEvent 构造携带计数快照的权威互动事件。
func countEvent(id, targetID int64, action, targetType string, snapshot *event.InteractionCountSnapshot) event.BehaviorEvent {
	return event.BehaviorEvent{
		EventID: id, ClientEventID: "client-" + action, SchemaVersion: event.BehaviorSchemaVersion,
		EventTime: 100, ReceivedAt: 100, UserID: 7, Action: action,
		TargetID: targetID, TargetType: targetType, Producer: "business-outbox",
		CountSnapshot: snapshot,
	}
}

// newTestCountSyncStore 创建时间固定的存储，便于断言 stats_seq 的时间下限。
func newTestCountSyncStore(conn CountSyncConn, redis RedisCmdable) *countSyncStore {
	return &countSyncStore{conn: conn, redis: redis, now: func() time.Time { return time.UnixMilli(5000) }}
}

func TestCountSyncStoreOverwritesPostCountsAndEmitsCounted(t *testing.T) {
	conn := &fakeCountConn{affected: 1, postCounts: postCounts{LikeCount: 3, CommentCount: 2, StatsSeq: 5000}}
	redisClient := &fakeCountRedis{}
	store := newTestCountSyncStore(conn, redisClient)

	applied, err := store.ApplyBehaviorCount(context.Background(), countEvent(1, 55, event.BehaviorActionLike, "post",
		&event.InteractionCountSnapshot{LikeCount: 3, FavoriteCount: 1, Seq: 7}))

	require.NoError(t, err)
	assert.True(t, applied)
	require.Len(t, conn.execs, 2)
	// 绝对值覆盖并以序号守卫，不再做增量。
	assert.Equal(t, applyPostSnapshotSQL, conn.execs[0].query)
	assert.Equal(t, []any{int64(3), int64(1), int64(7), int64(5000), int64(55), int64(7)}, conn.execs[0].args)
	// counted 事件与计数同事务入队，序号来自帖子 stats_seq。
	assert.Contains(t, conn.execs[1].query, "event_outbox")
	assert.Contains(t, string(conn.execs[1].args[4].([]byte)), `"stats_seq":5000`)
	assert.Equal(t, []string{"cache:post:id:55"}, redisClient.deleted)
}

func TestCountSyncStoreStaleSnapshotIsNoop(t *testing.T) {
	// 序号不大于已投影序号：UPDATE 影响 0 行，不发 counted、不失效缓存。
	conn := &fakeCountConn{affected: 0}
	redisClient := &fakeCountRedis{}
	store := newTestCountSyncStore(conn, redisClient)

	applied, err := store.ApplyBehaviorCount(context.Background(), countEvent(2, 55, event.BehaviorActionUnlike, "post",
		&event.InteractionCountSnapshot{LikeCount: 0, Seq: 3}))

	require.NoError(t, err)
	assert.False(t, applied)
	require.Len(t, conn.execs, 1)
	assert.Empty(t, redisClient.deleted)
}

func TestCountSyncStoreCommentProjectsLikeCountOnly(t *testing.T) {
	conn := &fakeCountConn{affected: 1}
	redisClient := &fakeCountRedis{}
	store := newTestCountSyncStore(conn, redisClient)

	applied, err := store.ApplyBehaviorCount(context.Background(), countEvent(3, 88, event.BehaviorActionLike, "comment",
		&event.InteractionCountSnapshot{LikeCount: 4, Seq: 9}))

	require.NoError(t, err)
	assert.True(t, applied)
	require.Len(t, conn.execs, 1)
	assert.Equal(t, applyCommentSnapshotSQL, conn.execs[0].query)
	assert.Equal(t, []any{int64(4), int64(9), int64(88), int64(9)}, conn.execs[0].args)
	assert.Equal(t, []string{"cache:comment:id:88"}, redisClient.deleted)
}

func TestCountSyncStoreMissingSnapshotIsPermanent(t *testing.T) {
	// 旧格式事件只有动作没有绝对计数，无法安全覆盖：永久错误，消费者确认丢弃。
	conn := &fakeCountConn{affected: 1}
	store := newTestCountSyncStore(conn, &fakeCountRedis{})

	_, err := store.ApplyBehaviorCount(context.Background(), countEvent(4, 55, event.BehaviorActionLike, "post", nil))

	require.Error(t, err)
	assert.True(t, mqx.IsPermanentEvent(err))
	assert.Empty(t, conn.execs)
}

func TestCountSyncStoreUnknownTargetTypeIsPermanent(t *testing.T) {
	store := newTestCountSyncStore(&fakeCountConn{affected: 1}, &fakeCountRedis{})

	_, err := store.ApplyBehaviorCount(context.Background(), countEvent(5, 55, event.BehaviorActionLike, "user",
		&event.InteractionCountSnapshot{LikeCount: 1, Seq: 1}))

	assert.True(t, mqx.IsPermanentEvent(err))
}

func TestCountSyncStoreNonCountActionsIgnored(t *testing.T) {
	conn := &fakeCountConn{affected: 1}
	store := newTestCountSyncStore(conn, &fakeCountRedis{})

	applied, err := store.ApplyBehaviorCount(context.Background(), countEvent(6, 55, event.BehaviorActionClick, "post", nil))

	require.NoError(t, err)
	assert.False(t, applied)
	assert.Empty(t, conn.execs)
}

func TestCountSyncStoreDBFailureIsRetryable(t *testing.T) {
	// 数据库故障是临时错误：交给 MQ 重试；重试时序号守卫保证不会重复推进。
	conn := &fakeCountConn{execErr: errors.New("db down")}
	redisClient := &fakeCountRedis{}
	store := newTestCountSyncStore(conn, redisClient)

	_, err := store.ApplyBehaviorCount(context.Background(), countEvent(7, 55, event.BehaviorActionLike, "post",
		&event.InteractionCountSnapshot{LikeCount: 1, Seq: 1}))

	require.Error(t, err)
	assert.False(t, mqx.IsPermanentEvent(err))
	assert.Empty(t, redisClient.deleted)
}
