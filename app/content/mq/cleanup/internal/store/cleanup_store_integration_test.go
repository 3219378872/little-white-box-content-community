//go:build integration

package store

import (
	"context"
	"os"
	"testing"

	"esx/pkg/testutil"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var rdsEnv *testutil.RedisEnv
var cleanupStore *RedisCleanupStore

func TestMain(m *testing.M) {
	rdsEnv = testutil.SetupRedisEnvM()
	cleanupStore = NewRedisCleanupStore(rdsEnv.Redis)

	code := m.Run()
	rdsEnv.Close()
	os.Exit(code)
}

func TestRedisCleanup_DeletePostState(t *testing.T) {
	ctx := context.Background()
	require.NoError(t, rdsEnv.Redis.SetCtx(ctx, "post:100:stats", "stats-data"))
	require.NoError(t, rdsEnv.Redis.SetCtx(ctx, "post:100:quality", "0.8"))
	require.NoError(t, cleanupStore.DeletePostState(ctx, 100))

	got, _ := rdsEnv.Redis.GetCtx(ctx, "post:100:stats")
	assert.Empty(t, got)
	got, _ = rdsEnv.Redis.GetCtx(ctx, "post:100:quality")
	assert.Empty(t, got)
}

func TestRedisCleanup_DeletePostState_Idempotent(t *testing.T) {
	require.NoError(t, cleanupStore.DeletePostState(context.Background(), 999999))
	require.NoError(t, cleanupStore.DeletePostState(context.Background(), 999999))
}

func TestRedisCleanup_RemoveFromHotZSets(t *testing.T) {
	ctx := context.Background()
	_, err := rdsEnv.Redis.ZaddCtx(ctx, "hot:posts:24h", 100, "200")
	require.NoError(t, err)
	_, err = rdsEnv.Redis.ZaddCtx(ctx, "hot:posts:7d", 50, "200")
	require.NoError(t, err)
	_, err = rdsEnv.Redis.ZaddCtx(ctx, "hot:posts:24h", 80, "201")
	require.NoError(t, err)

	require.NoError(t, cleanupStore.RemoveFromHotZSets(ctx, 200))

	// 已删除的成员：ZRANK 返回 nil 错误或不存在
	_, err = rdsEnv.Redis.ZrankCtx(ctx, "hot:posts:24h", "200")
	require.Error(t, err)
	_, err = rdsEnv.Redis.ZrankCtx(ctx, "hot:posts:7d", "200")
	require.Error(t, err)

	// 其他 post 不受影响
	score, err := rdsEnv.Redis.ZscoreCtx(ctx, "hot:posts:24h", "201")
	require.NoError(t, err)
	assert.Equal(t, int64(80), score)
}

func TestRedisCleanup_RemoveFromTagZSets(t *testing.T) {
	ctx := context.Background()
	_, err := rdsEnv.Redis.ZaddCtx(ctx, "tag:游戏:posts", 100, "300")
	require.NoError(t, err)
	_, err = rdsEnv.Redis.ZaddCtx(ctx, "tag:科技:posts", 100, "300")
	require.NoError(t, err)
	_, err = rdsEnv.Redis.ZaddCtx(ctx, "tag:游戏:posts", 50, "301")
	require.NoError(t, err)

	require.NoError(t, cleanupStore.RemoveFromTagZSets(ctx, 300, []string{"游戏", "科技"}))

	_, err = rdsEnv.Redis.ZrankCtx(ctx, "tag:游戏:posts", "300")
	require.Error(t, err)
	_, err = rdsEnv.Redis.ZrankCtx(ctx, "tag:科技:posts", "300")
	require.Error(t, err)

	// 其他 post 不受影响
	score, err := rdsEnv.Redis.ZscoreCtx(ctx, "tag:游戏:posts", "301")
	require.NoError(t, err)
	assert.Equal(t, int64(50), score)
}

func TestRedisCleanup_RemoveFromTagZSets_EmptyTags(t *testing.T) {
	require.NoError(t, cleanupStore.RemoveFromTagZSets(context.Background(), 400, nil))
}
