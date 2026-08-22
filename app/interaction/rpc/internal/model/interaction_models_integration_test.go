//go:build integration

package model

import (
	"context"
	"os"
	"testing"

	"esx/pkg/outboxx"
	"esx/pkg/testutil"
	"esx/pkg/util"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zeromicro/go-zero/core/stores/cache"
	"github.com/zeromicro/go-zero/core/stores/redis"
	"github.com/zeromicro/go-zero/core/stores/sqlx"
)

var testEnv *testutil.TestEnv

func TestMain(m *testing.M) {
	testEnv = testutil.SetupTestEnvM("xbh_interaction", testutil.SchemaPath("xbh_interaction.sql"))
	_ = util.InitSnowflake(1, 1)
	code := m.Run()
	testEnv.Close()
	os.Exit(code)
}

func newTestConn() sqlx.SqlConn {
	return sqlx.NewSqlConnFromDB(testEnv.DB)
}

func testCacheConf() cache.CacheConf {
	return cache.CacheConf{
		cache.NodeConf{
			RedisConf: redis.RedisConf{Host: testEnv.RedisAddr, Type: "node"},
			Weight:    100,
		},
	}
}

func nextID(t *testing.T) int64 {
	t.Helper()
	id, err := util.NextID()
	require.NoError(t, err)
	return id
}

func outboxEvent(t *testing.T, topic string) outboxx.Event {
	t.Helper()
	return outboxx.Event{
		ID:      nextID(t),
		Topic:   topic,
		Tag:     "test",
		Key:     "integration-test",
		Payload: []byte(`{"probe":true}`),
	}
}

func countOutboxRows(t *testing.T) int64 {
	t.Helper()
	var n int64
	err := newTestConn().QueryRowCtx(context.Background(), &n,
		"SELECT COUNT(*) FROM `event_outbox`")
	require.NoError(t, err)
	return n
}

func likeCountOf(t *testing.T, targetID int64) int64 {
	t.Helper()
	var n int64
	err := newTestConn().QueryRowCtx(context.Background(), &n,
		"SELECT like_count FROM `action_count` WHERE `target_id` = ? AND `target_type` = 2", targetID)
	require.NoError(t, err)
	return n
}

func favoriteCountOf(t *testing.T, postID int64) int64 {
	t.Helper()
	var n int64
	err := newTestConn().QueryRowCtx(context.Background(), &n,
		"SELECT favorite_count FROM `action_count` WHERE `target_id` = ? AND `target_type` = 1", postID)
	require.NoError(t, err)
	return n
}

func TestInteractionCommandModelLikeUnlikeFlow(t *testing.T) {
	testEnv.TruncateAll(t, "like_record", "favorite", "action_count")
	testEnv.DB.ExecContext(context.Background(), "TRUNCATE TABLE `event_outbox`")
	ctx := context.Background()

	conn := newTestConn()
	commandModel := NewInteractionCommandModel(conn, outboxx.NewSQLStore(conn))
	targetID := nextID(t)

	before := countOutboxRows(t)

	// 点赞：记录 + 计数 + outbox 同事务落库。
	recordID, err := commandModel.Like(ctx, 101, targetID, 2, outboxEvent(t, "interaction-behavior-v2"))
	require.NoError(t, err)
	require.Greater(t, recordID, int64(0))
	assert.Equal(t, int64(1), likeCountOf(t, targetID))
	assert.Greater(t, countOutboxRows(t), before)

	var recordStatus int64
	require.NoError(t, conn.QueryRowCtx(ctx, &recordStatus,
		"SELECT status FROM `like_record` WHERE `id` = ?", recordID))
	assert.Equal(t, int64(StatusActive), recordStatus)

	// 重复点赞：无状态变化，计数不重复累加。
	_, err = commandModel.Like(ctx, 101, targetID, 2, outboxEvent(t, "interaction-behavior-v2"))
	require.ErrorIs(t, err, ErrNoStateChange)
	assert.Equal(t, int64(1), likeCountOf(t, targetID))

	// 取消点赞：状态与计数同步回落。
	require.NoError(t, commandModel.Unlike(ctx, recordID, targetID, 2, outboxEvent(t, "interaction-behavior-v2")))
	assert.Equal(t, int64(0), likeCountOf(t, targetID))
	require.NoError(t, conn.QueryRowCtx(ctx, &recordStatus,
		"SELECT status FROM `like_record` WHERE `id` = ?", recordID))
	assert.Equal(t, int64(StatusInactive), recordStatus)

	// 重复取消：无状态变化。
	err = commandModel.Unlike(ctx, recordID, targetID, 2, outboxEvent(t, "interaction-behavior-v2"))
	require.ErrorIs(t, err, ErrNoStateChange)
}

func TestInteractionCommandModelFavoriteUnfavoriteFlow(t *testing.T) {
	testEnv.TruncateAll(t, "like_record", "favorite", "action_count")
	ctx := context.Background()

	conn := newTestConn()
	commandModel := NewInteractionCommandModel(conn, outboxx.NewSQLStore(conn))
	postID := nextID(t)

	recordID, err := commandModel.Favorite(ctx, 201, postID, outboxEvent(t, "interaction-behavior-v2"))
	require.NoError(t, err)
	require.Greater(t, recordID, int64(0))
	assert.Equal(t, int64(1), favoriteCountOf(t, postID))

	_, err = commandModel.Favorite(ctx, 201, postID, outboxEvent(t, "interaction-behavior-v2"))
	require.ErrorIs(t, err, ErrNoStateChange)
	assert.Equal(t, int64(1), favoriteCountOf(t, postID))

	require.NoError(t, commandModel.Unfavorite(ctx, recordID, postID, outboxEvent(t, "interaction-behavior-v2")))
	assert.Equal(t, int64(0), favoriteCountOf(t, postID))

	err = commandModel.Unfavorite(ctx, recordID, postID, outboxEvent(t, "interaction-behavior-v2"))
	require.ErrorIs(t, err, ErrNoStateChange)
}

func TestLikeRecordModelUpsertAndConditionalUpdate(t *testing.T) {
	testEnv.TruncateAll(t, "like_record", "action_count")
	ctx := context.Background()

	conn := newTestConn()
	likeModel := NewLikeRecordModel(conn, testCacheConf())

	// 首次 upsert 激活，二次 upsert 幂等更新状态。
	result, err := likeModel.UpsertLikeStatus(ctx, 301, nextID(t), 2, StatusActive)
	require.NoError(t, err)
	rows, err := result.RowsAffected()
	require.NoError(t, err)
	assert.Equal(t, int64(1), rows)

	targetB := nextID(t)
	_, err = likeModel.UpsertLikeStatus(ctx, 301, targetB, 2, StatusActive)
	require.NoError(t, err)

	statuses, err := likeModel.FindStatusByUserAndTargets(ctx, 301, []int64{targetB, 999}, 2)
	require.NoError(t, err)
	assert.Equal(t, map[int64]bool{targetB: true}, statuses)

	// 条件更新：期望状态不匹配时影响行数为 0。
	record, err := likeModel.FindOneByUserIdTargetIdTargetType(ctx, 301, targetB, 2)
	require.NoError(t, err)
	stale, err := likeModel.UpdateStatusById(ctx, record.Id, StatusInactive, StatusActive)
	require.NoError(t, err)
	staleRows, err := stale.RowsAffected()
	require.NoError(t, err)
	assert.Equal(t, int64(0), staleRows)

	fresh, err := likeModel.UpdateStatusById(ctx, record.Id, StatusActive, StatusInactive)
	require.NoError(t, err)
	freshRows, err := fresh.RowsAffected()
	require.NoError(t, err)
	assert.Equal(t, int64(1), freshRows)

	require.NoError(t, likeModel.InvalidateLikeRecordCache(ctx, record.Id, 301, targetB, 2))
}

func TestFavoriteModelQueriesAndConditionalUpdate(t *testing.T) {
	testEnv.TruncateAll(t, "favorite", "action_count")
	ctx := context.Background()

	conn := newTestConn()
	favoriteModel := NewFavoriteModel(conn, testCacheConf())

	postA, postB, postC := nextID(t), nextID(t), nextID(t)
	_, err := favoriteModel.UpsertFavoriteStatus(ctx, 401, postA, StatusActive)
	require.NoError(t, err)
	_, err = favoriteModel.UpsertFavoriteStatus(ctx, 401, postB, StatusActive)
	require.NoError(t, err)
	// 另一用户的收藏不得串到 401 的结果里。
	_, err = favoriteModel.UpsertFavoriteStatus(ctx, 402, postC, StatusActive)
	require.NoError(t, err)

	ids, total, err := favoriteModel.FindActivePostIds(ctx, 401, 1, 10)
	require.NoError(t, err)
	assert.Equal(t, int64(2), total)
	assert.ElementsMatch(t, []int64{postA, postB}, ids)

	statuses, err := favoriteModel.FindFavoriteStatusByUserAndPosts(ctx, 401, []int64{postA, postC})
	require.NoError(t, err)
	// 无记录的目标不出现在结果里（缺失即未收藏），且不得串入其他用户的数据。
	assert.Equal(t, map[int64]bool{postA: true}, statuses)

	// 二次收藏同一帖子：upsert 更新而非新增行。
	if _, err = favoriteModel.UpsertFavoriteStatus(ctx, 401, postA, StatusActive); err != nil {
		t.Fatal(err)
	}
	_, total, err = favoriteModel.FindActivePostIds(ctx, 401, 1, 10)
	require.NoError(t, err)
	assert.Equal(t, int64(2), total)

	record, err := favoriteModel.FindOneByUserIdPostId(ctx, 401, postA)
	require.NoError(t, err)
	stale, err := favoriteModel.UpdateStatusById(ctx, record.Id, StatusInactive, StatusActive)
	require.NoError(t, err)
	staleRows, err := stale.RowsAffected()
	require.NoError(t, err)
	assert.Equal(t, int64(0), staleRows)

	fresh, err := favoriteModel.UpdateStatusById(ctx, record.Id, StatusActive, StatusInactive)
	require.NoError(t, err)
	freshRows, err := fresh.RowsAffected()
	require.NoError(t, err)
	assert.Equal(t, int64(1), freshRows)
}

func TestActionCountModelIncrDecr(t *testing.T) {
	testEnv.TruncateAll(t, "action_count")
	ctx := context.Background()

	conn := newTestConn()
	actionModel := NewActionCountModel(conn)

	targetID := nextID(t)
	_, err := actionModel.Insert(ctx, &ActionCount{
		Id: nextID(t), TargetId: targetID, TargetType: 1,
	})
	require.NoError(t, err)

	found, err := actionModel.FindOneByTarget(ctx, targetID, 1)
	require.NoError(t, err)
	assert.Equal(t, int64(0), found.LikeCount)

	require.NoError(t, actionModel.IncrLikeCount(ctx, targetID, 1))
	require.NoError(t, actionModel.IncrFavoriteCount(ctx, targetID, 1))
	found, err = actionModel.FindOneByTarget(ctx, targetID, 1)
	require.NoError(t, err)
	assert.Equal(t, int64(1), found.LikeCount)
	assert.Equal(t, int64(1), found.FavoriteCount)

	// Tx 版本走独立连接，行为一致。
	require.NoError(t, actionModel.IncrLikeCountTx(ctx, conn, targetID, 1))
	found, err = actionModel.FindOneByTarget(ctx, targetID, 1)
	require.NoError(t, err)
	assert.Equal(t, int64(2), found.LikeCount)

	// 递减有下限保护，不会出现负数。
	require.NoError(t, actionModel.DecrFavoriteCount(ctx, targetID, 1))
	require.NoError(t, actionModel.DecrFavoriteCount(ctx, targetID, 1))
	found, err = actionModel.FindOneByTarget(ctx, targetID, 1)
	require.NoError(t, err)
	assert.Equal(t, int64(0), found.FavoriteCount)
}
