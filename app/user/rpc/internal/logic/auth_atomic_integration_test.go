//go:build integration

package logic

import (
	"context"
	"sync"
	"testing"

	"esx/app/user/rpc/internal/svc"
	"esx/app/user/rpc/pb/xiaobaihe/user/pb"
	"esx/pkg/errx"
	"esx/pkg/jwtx"

	"github.com/stretchr/testify/require"
)

func concurrentAuthCalls(call func() error) []error {
	const workers = 32
	start := make(chan struct{})
	errs := make([]error, workers)
	var wg sync.WaitGroup
	for i := range workers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			errs[i] = call()
		}()
	}
	close(start)
	wg.Wait()
	return errs
}

func TestVerifyCodeConcurrentLoginIntegration(t *testing.T) {
	ctx := context.Background()
	const phone = "13800440001"
	require.NoError(t, testEnv.Redis.SetexCtx(ctx, verifyCodeRedisKey(phone), "123456", 120))
	_, err := NewRegisterLogic(ctx, testSvcCtx).Register(&pb.RegisterReq{Phone: phone, VerifyCode: "123456"})
	require.NoError(t, err)
	require.NoError(t, testEnv.Redis.SetexCtx(ctx, verifyCodeRedisKey(phone), "654321", 120))
	errs := concurrentAuthCalls(func() error {
		_, err := NewLoginLogic(ctx, testSvcCtx).Login(&pb.LoginReq{Phone: phone, VerifyCode: "654321", LoginType: 2})
		return err
	})
	winners := 0
	for _, err := range errs {
		if err == nil {
			winners++
		} else {
			require.True(t, errx.Is(err, errx.VerifyCodeExpired), "unexpected error: %v", err)
		}
	}
	require.Equal(t, 1, winners, "a verification code must issue at most one login session")
}

func TestVerifyCodeAttemptLimitIntegration(t *testing.T) {
	ctx := context.Background()
	const phone = "13800440002"
	key, attempts := verifyCodeRedisKey(phone), "verify:attempts:"+phone
	require.NoError(t, testEnv.Redis.SetexCtx(ctx, key, "654321", 120))
	for range verifyCodeMaxAttempts {
		require.True(t, errx.Is(consumeVerifyCode(ctx, testEnv.Redis, phone, "000000"), errx.VerifyCodeError))
	}
	require.True(t, errx.Is(consumeVerifyCode(ctx, testEnv.Redis, phone, "654321"), errx.VerifyCodeExpired))
	ttl, err := testEnv.Redis.TtlCtx(ctx, attempts)
	require.NoError(t, err)
	require.Positive(t, ttl)
	require.LessOrEqual(t, ttl, verifyCodeAttemptWindowSeconds)
	require.NoError(t, testEnv.Redis.SetexCtx(ctx, key, "654321", 120))
	require.NoError(t, consumeVerifyCode(ctx, testEnv.Redis, phone, "654321"))
	remaining, err := testEnv.Redis.ExistsCtx(ctx, attempts)
	require.NoError(t, err)
	require.False(t, remaining)
}

// Execute the real Lua with an invalid successor TTL. Redis does not roll back
// writes before a script error, so this also checks the ordering inside the script.
type invalidRotationTTL struct{ svc.RedisStore }

func (r invalidRotationTTL) EvalCtx(ctx context.Context, script string, keys []string, args ...any) (any, error) {
	if script == rotateRefreshJTIScript {
		args[1] = 0
	}
	return r.RedisStore.EvalCtx(ctx, script, keys, args...)
}

func TestRefreshRotationWriteFailureAndReplayIntegration(t *testing.T) {
	ctx := context.Background()
	svcCtx := *testSvcCtx
	_, old, err := issueTokenPair(ctx, &svcCtx, 44003, "rotation")
	require.NoError(t, err)
	svcCtx.RedisClient = invalidRotationTTL{testEnv.Redis}
	_, _, err = rotateRefreshToken(ctx, &svcCtx, old)
	require.True(t, errx.Is(err, errx.SystemError))
	claims, err := jwtx.ParseRefreshToken(old, svcCtx.Config.JwtConfig)
	require.NoError(t, err)
	owner, err := testEnv.Redis.GetCtx(ctx, refreshJTIKey(claims.ID))
	require.NoError(t, err)
	require.Equal(t, "44003", owner)

	svcCtx.RedisClient = testEnv.Redis
	winners := 0
	nextTokens := make(chan string, 32)
	for _, err := range concurrentAuthCalls(func() error {
		_, next, err := rotateRefreshToken(ctx, &svcCtx, old)
		if err == nil {
			nextTokens <- next
		}
		return err
	}) {
		if err == nil {
			winners++
		} else {
			require.True(t, errx.Is(err, errx.LoginRequired), "unexpected error: %v", err)
		}
	}
	require.Equal(t, 1, winners)
	_, _, err = rotateRefreshToken(ctx, &svcCtx, old)
	require.True(t, errx.Is(err, errx.LoginRequired))
	_, _, err = rotateRefreshToken(ctx, &svcCtx, <-nextTokens)
	require.NoError(t, err, "the winning response must carry a usable successor")
}

func TestRefreshRotationOwnerCollisionAndTTLIntegration(t *testing.T) {
	ctx := context.Background()
	old, next := refreshJTIKey("test-old"), refreshJTIKey("test-next")
	require.NoError(t, testEnv.Redis.SetexCtx(ctx, old, "44004", 120))
	result, err := testEnv.Redis.EvalCtx(ctx, rotateRefreshJTIScript, []string{old, next}, "other-owner", 120)
	require.NoError(t, err)
	require.Equal(t, int64(-1), result)
	require.NoError(t, testEnv.Redis.SetexCtx(ctx, next, "existing-owner", 120))
	result, err = testEnv.Redis.EvalCtx(ctx, rotateRefreshJTIScript, []string{old, next}, "44004", 120)
	require.NoError(t, err)
	require.Equal(t, int64(-2), result)
	value, err := testEnv.Redis.GetCtx(ctx, next)
	require.NoError(t, err)
	require.Equal(t, "existing-owner", value)
	_, err = testEnv.Redis.DelCtx(ctx, next)
	require.NoError(t, err)
	result, err = testEnv.Redis.EvalCtx(ctx, rotateRefreshJTIScript, []string{old, next}, "44004", 120)
	require.NoError(t, err)
	require.Equal(t, int64(1), result)
	ttl, err := testEnv.Redis.TtlCtx(ctx, next)
	require.NoError(t, err)
	require.Positive(t, ttl)
	require.LessOrEqual(t, ttl, 120)
	value, err = testEnv.Redis.GetCtx(ctx, old)
	require.NoError(t, err)
	require.Empty(t, value)
}
