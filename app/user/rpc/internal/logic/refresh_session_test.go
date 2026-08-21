package logic

import (
	"context"
	"testing"

	"errx"
	"jwtx"
	"user/internal/svc"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func refreshTestSvcCtx() (*svc.ServiceContext, *memoryRedis) {
	mem := &memoryRedis{values: map[string]string{}}
	svcCtx := &svc.ServiceContext{RedisClient: mem}
	svcCtx.Config.JwtConfig = jwtx.JwtConfig{
		AccessSecret:  "test-access-secret",
		AccessExpire:  1800,
		RefreshSecret: "test-refresh-secret",
		RefreshExpire: 7 * 24 * 3600,
	}
	return svcCtx, mem
}

func TestIssueTokenPair(t *testing.T) {
	svcCtx, _ := refreshTestSvcCtx()
	access, refresh, err := issueTokenPair(context.Background(), svcCtx, 7, "alice")
	require.NoError(t, err)
	assert.NotEmpty(t, access)
	assert.NotEmpty(t, refresh)

	claims, err := jwtx.ParseToken(access, svcCtx.Config.JwtConfig)
	require.NoError(t, err)
	assert.Equal(t, int64(7), claims.UserId)

	rClaims, err := jwtx.ParseRefreshToken(refresh, svcCtx.Config.JwtConfig)
	require.NoError(t, err)
	stored, err := svcCtx.RedisClient.GetCtx(context.Background(), refreshJTIKey(rClaims.ID))
	require.NoError(t, err)
	assert.Equal(t, "7", stored, "jti whitelist must record owner userId")
}

func TestRotateRefreshToken(t *testing.T) {
	t.Run("正常轮换：旧令牌一次性失效", func(t *testing.T) {
		svcCtx, _ := refreshTestSvcCtx()
		_, oldRefresh, err := issueTokenPair(context.Background(), svcCtx, 7, "alice")
		require.NoError(t, err)

		access2, refresh2, err := rotateRefreshToken(context.Background(), svcCtx, oldRefresh)
		require.NoError(t, err)
		assert.NotEmpty(t, access2)
		assert.NotEqual(t, oldRefresh, refresh2, "rotation must mint a fresh refresh token")

		// 重放旧令牌必须被拒绝。
		_, _, err = rotateRefreshToken(context.Background(), svcCtx, oldRefresh)
		require.Error(t, err)
		assert.True(t, errx.Is(err, errx.LoginRequired), "replayed refresh token must be rejected, got %v", err)
	})

	t.Run("伪造或格式错误的令牌被拒绝", func(t *testing.T) {
		svcCtx, _ := refreshTestSvcCtx()
		for _, bad := range []string{"garbage", "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxIn0.sig"} {
			_, _, err := rotateRefreshToken(context.Background(), svcCtx, bad)
			require.Error(t, err)
			assert.True(t, errx.Is(err, errx.LoginRequired))
		}
	})

	t.Run("访问令牌不能用于刷新", func(t *testing.T) {
		svcCtx, _ := refreshTestSvcCtx()
		access, _, err := issueTokenPair(context.Background(), svcCtx, 7, "alice")
		require.NoError(t, err)
		_, _, err = rotateRefreshToken(context.Background(), svcCtx, access)
		require.Error(t, err)
		assert.True(t, errx.Is(err, errx.LoginRequired), "access token must not refresh, got %v", err)
	})
}
