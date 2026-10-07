//go:build integration

package store

import (
	"context"
	"encoding/json"
	"testing"

	"esx/pkg/event"
	"esx/pkg/testutil"

	sqlx "esx/pkg/sqlstore"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// 真实 MySQL 上验证：重复投递、乱序到达都收敛到最大序号对应的权威计数，
// 只有推进投影的快照才写 counted 事件，且 stats_seq 单调递增。
func TestCountSyncStoreIntegrationConvergesOnHighestSeq(t *testing.T) {
	ctx := context.Background()
	env := testutil.SetupTestEnv(t, "xbh_content", testutil.SchemaPath("xbh_content.sql"))
	defer env.Close()

	store := NewCountSyncStore(sqlx.NewSqlConnFromDB(env.DB), env.Redis)

	_, err := env.DB.ExecContext(ctx, "INSERT INTO `post` (`id`, `author_id`, `title`, `content`, `status`, `revision`, `like_count`) VALUES (9001, 1, 't', 'c', 1, 1, 42)")
	require.NoError(t, err)

	// postState 读取帖子的计数投影与序号。
	postState := func() (likeCount, favoriteCount, seq, statsSeq int64) {
		require.NoError(t, env.DB.QueryRowContext(ctx,
			"SELECT `like_count`, `favorite_count`, `interaction_seq`, `stats_seq` FROM `post` WHERE `id` = 9001").
			Scan(&likeCount, &favoriteCount, &seq, &statsSeq))
		return
	}
	// countedEvents 返回该帖子 counted 事件的数量。
	countedEvents := func() int {
		var n int
		require.NoError(t, env.DB.QueryRowContext(ctx,
			"SELECT COUNT(*) FROM `event_outbox` WHERE `message_key` = '9001'").Scan(&n))
		return n
	}
	behavior := func(id int64, action string, likes, favorites, seq int64) event.BehaviorEvent {
		return event.BehaviorEvent{
			EventID: id, ClientEventID: "business-1", SchemaVersion: event.BehaviorSchemaVersion,
			EventTime: 100, ReceivedAt: 100, UserID: 7, Action: action,
			TargetID: 9001, TargetType: "post", Producer: "business-outbox",
			CountSnapshot: &event.InteractionCountSnapshot{LikeCount: likes, FavoriteCount: favorites, Seq: seq},
		}
	}

	// 首个快照覆盖存量的漂移计数（42 → 1）。
	applied, err := store.ApplyBehaviorCount(ctx, behavior(777001, event.BehaviorActionLike, 1, 0, 1))
	require.NoError(t, err)
	assert.True(t, applied)
	likeCount, _, seq, firstStatsSeq := postState()
	assert.Equal(t, int64(1), likeCount)
	assert.Equal(t, int64(1), seq)
	assert.Positive(t, firstStatsSeq)
	assert.Equal(t, 1, countedEvents())

	// 同一事件重投：序号守卫让它成为空操作，不重复写 counted。
	applied, err = store.ApplyBehaviorCount(ctx, behavior(777001, event.BehaviorActionLike, 1, 0, 1))
	require.NoError(t, err)
	assert.False(t, applied)
	assert.Equal(t, 1, countedEvents())

	// 乱序：seq=3 的收藏先到，再到 seq=2 的取消点赞；最终是 seq=3 的快照。
	applied, err = store.ApplyBehaviorCount(ctx, behavior(777003, event.BehaviorActionFavorite, 1, 1, 3))
	require.NoError(t, err)
	assert.True(t, applied)
	applied, err = store.ApplyBehaviorCount(ctx, behavior(777002, event.BehaviorActionUnlike, 0, 0, 2))
	require.NoError(t, err)
	assert.False(t, applied, "older snapshot must not roll the projection back")

	likeCount, favoriteCount, seq, statsSeq := postState()
	assert.Equal(t, int64(1), likeCount)
	assert.Equal(t, int64(1), favoriteCount)
	assert.Equal(t, int64(3), seq)
	assert.Greater(t, statsSeq, firstStatsSeq, "stats_seq must advance on every applied snapshot")
	assert.Equal(t, 2, countedEvents())

	// 最新的 counted 事件携带当前计数与帖子 stats_seq，搜索据此单调覆盖。
	var payload []byte
	require.NoError(t, env.DB.QueryRowContext(ctx,
		"SELECT `payload` FROM `event_outbox` WHERE `id` = 777003").Scan(&payload))
	var counted event.PostEvent
	require.NoError(t, json.Unmarshal(payload, &counted))
	assert.Equal(t, event.PostEventCounted, counted.Type)
	assert.Equal(t, int64(1), counted.LikeCount)
	assert.Equal(t, statsSeq, counted.StatsSeq)
}

// 评论只投影点赞数，同样按序号守卫。
func TestCountSyncStoreIntegrationCommentLikes(t *testing.T) {
	ctx := context.Background()
	env := testutil.SetupTestEnv(t, "xbh_content", testutil.SchemaPath("xbh_content.sql"))
	defer env.Close()

	store := NewCountSyncStore(sqlx.NewSqlConnFromDB(env.DB), env.Redis)
	_, err := env.DB.ExecContext(ctx, "INSERT INTO `comment` (`id`, `post_id`, `user_id`, `content`) VALUES (8801, 9001, 1, 'c')")
	require.NoError(t, err)

	like := event.BehaviorEvent{
		EventID: 778001, ClientEventID: "business-2", SchemaVersion: event.BehaviorSchemaVersion,
		EventTime: 100, ReceivedAt: 100, UserID: 7, Action: event.BehaviorActionLike,
		TargetID: 8801, TargetType: "comment", Producer: "business-outbox",
		CountSnapshot: &event.InteractionCountSnapshot{LikeCount: 2, Seq: 5},
	}
	applied, err := store.ApplyBehaviorCount(ctx, like)
	require.NoError(t, err)
	assert.True(t, applied)

	stale := like
	stale.EventID = 778000
	stale.CountSnapshot = &event.InteractionCountSnapshot{LikeCount: 1, Seq: 4}
	applied, err = store.ApplyBehaviorCount(ctx, stale)
	require.NoError(t, err)
	assert.False(t, applied)

	var likeCount, seq int64
	require.NoError(t, env.DB.QueryRowContext(ctx,
		"SELECT `like_count`, `interaction_seq` FROM `comment` WHERE `id` = 8801").Scan(&likeCount, &seq))
	assert.Equal(t, int64(2), likeCount)
	assert.Equal(t, int64(5), seq)
}
