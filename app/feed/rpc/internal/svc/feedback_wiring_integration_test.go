//go:build integration

package svc

import (
	"context"
	"testing"

	"esx/app/feed/rpc/internal/config"
	"esx/pkg/testutil"

	"github.com/stretchr/testify/require"
	"github.com/zeromicro/go-zero/core/conf"
	"github.com/zeromicro/go-zero/core/stores/redis"
	"github.com/zeromicro/go-zero/zrpc"
)

func TestFeedServiceReadsCurrentNegativeFeaturesFromConfiguredRedis(t *testing.T) {
	t.Setenv("RPC_INTERNAL_SECRET", "test-internal-secret")
	t.Setenv("FEED_CURSOR_SECRET", "configured-feed-cursor-secret")
	t.Setenv("DB_FEED", "test:test@tcp(127.0.0.1:1)/feed")
	t.Setenv("REDIS_PASSWORD", "")
	env := testutil.SetupRedisEnv(t)
	t.Cleanup(env.Close)

	var c config.Config
	require.NoError(t, conf.Load("../../etc/feed.yaml", &c, conf.UseEnv()))
	c.Redis.RedisConf = redis.RedisConf{Host: env.Addr, Type: redis.NodeType}
	// Only Redis is exercised; nonblocking clients avoid starting other services.
	client := zrpc.RpcClientConf{Endpoints: []string{"127.0.0.1:1"}, NonBlock: true}
	c.UserRpc, c.ContentRpc, c.RecommendRpc = client, client, client
	service := NewServiceContext(c)

	ctx := context.Background()
	require.NoError(t, env.Redis.HsetCtx(ctx, "feature:v1:u:7:negative", "post:11", "1"))
	require.NoError(t, env.Redis.HsetCtx(ctx, "feature:v2:u:7:negative", "post:44", "1"))
	posts, err := service.NegativeFeedback.HiddenPosts(ctx, 7)
	require.NoError(t, err)
	require.Equal(t, map[int64]struct{}{44: {}}, posts)

	require.NoError(t, env.Redis.HsetCtx(ctx, "feature:v2:u:7:negative", "post:45", "1"))
	posts, err = service.NegativeFeedback.HiddenPosts(ctx, 7)
	require.NoError(t, err)
	require.Equal(t, map[int64]struct{}{44: {}, 45: {}}, posts)
}
