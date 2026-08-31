package logic

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"

	"esx/app/user/rpc/internal/svc"
	"esx/pkg/errx"
	"esx/pkg/jwtx"

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

func TestRotateRefreshTokenConcurrentReplayOnlyOneSucceeds(t *testing.T) {
	svcCtx, _ := refreshTestSvcCtx()
	_, oldRefresh, err := issueTokenPair(context.Background(), svcCtx, 23, "parallel")
	require.NoError(t, err)

	const callers = 32
	start := make(chan struct{})
	var wg sync.WaitGroup
	var succeeded atomic.Int32
	var rejected atomic.Int32
	errs := make(chan error, callers)
	for range callers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, _, rotateErr := rotateRefreshToken(context.Background(), svcCtx, oldRefresh)
			switch {
			case rotateErr == nil:
				succeeded.Add(1)
			case errx.Is(rotateErr, errx.LoginRequired):
				rejected.Add(1)
			default:
				errs <- rotateErr
			}
		}()
	}
	close(start)
	wg.Wait()
	close(errs)
	for rotateErr := range errs {
		t.Errorf("unexpected rotation error: %v", rotateErr)
	}
	assert.Equal(t, int32(1), succeeded.Load())
	assert.Equal(t, int32(callers-1), rejected.Load())
}
