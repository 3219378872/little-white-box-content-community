//go:build integration

package store

import (
	"context"
	"database/sql"
	"testing"

	"esx/pkg/event"
	"esx/pkg/testutil"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCountSyncStoreIntegrationAppliesAndDedups(t *testing.T) {
	ctx := context.Background()
	env := testutil.SetupTestEnv(t, "xbh_content", testutil.SchemaPath("xbh_content.sql"))
	defer env.Close()

	store := NewCountSyncStore(env.DB, env.Redis)

	_, err := env.DB.ExecContext(ctx, "INSERT INTO `post` (`id`, `author_id`, `title`, `content`, `status`, `revision`) VALUES (9001, 1, 't', 'c', 1, 1)")
	require.NoError(t, err)

	like := event.BehaviorEvent{
		EventID: 777001, ClientEventID: "client-777001", SchemaVersion: event.BehaviorSchemaVersion,
		EventTime: 100, ReceivedAt: 100, UserID: 7, Action: event.BehaviorActionLike,
		TargetID: 9001, TargetType: "post", Producer: "business-outbox",
	}
	require.NoError(t, store.ApplyBehaviorCount(ctx, like))

	var likeCount int64
	require.NoError(t, env.DB.QueryRowContext(ctx, "SELECT `like_count` FROM `post` WHERE `id` = 9001").Scan(&likeCount))
	assert.Equal(t, int64(1), likeCount)

	// 同一事件重投不重复累计
	require.NoError(t, store.ApplyBehaviorCount(ctx, like))
	require.NoError(t, env.DB.QueryRowContext(ctx, "SELECT `like_count` FROM `post` WHERE `id` = 9001").Scan(&likeCount))
	assert.Equal(t, int64(1), likeCount)

	// unfavorite 对 favorite_count 生效
	unfavorite := event.BehaviorEvent{
		EventID: 777002, ClientEventID: "client-777002", SchemaVersion: event.BehaviorSchemaVersion,
		EventTime: 100, ReceivedAt: 100, UserID: 7, Action: event.BehaviorActionUnfavorite,
		TargetID: 9001, TargetType: "post", Producer: "business-outbox",
	}
	require.NoError(t, store.ApplyBehaviorCount(ctx, unfavorite))
	var favoriteCount int64
	require.NoError(t, env.DB.QueryRowContext(ctx, "SELECT `favorite_count` FROM `post` WHERE `id` = 9001").Scan(&favoriteCount))
	assert.Equal(t, int64(0), favoriteCount, "unfavorite must not go below zero")
}

var _ = sql.ErrNoRows
