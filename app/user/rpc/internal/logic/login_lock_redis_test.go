//go:build integration

package logic

import (
	"context"
	"esx/app/user/rpc/internal/model"
	"esx/app/user/rpc/internal/password"
	"esx/app/user/rpc/internal/svc"
	"esx/app/user/rpc/pb/xiaobaihe/user/pb"
	"esx/pkg/errx"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestLoginLockAtomicWindowIntegration(t *testing.T) {
	ctx := context.Background()
	l := NewLoginLogic(ctx, &svc.ServiceContext{RedisClient: testEnv.Redis})
	const id int64 = 990041
	key := loginLockKey(id)
	_, err := testEnv.Redis.DelCtx(ctx, key)
	require.NoError(t, err)
	for _, err := range concurrentAuthCalls(func() error { return l.recordLoginFailure(id) }) {
		require.NoError(t, err)
	}
	count, err := testEnv.Redis.GetCtx(ctx, key)
	require.NoError(t, err)
	require.Equal(t, "32", count)
	ttl, err := testEnv.Redis.TtlCtx(ctx, key)
	require.NoError(t, err)
	require.Positive(t, ttl)
	require.LessOrEqual(t, ttl, loginLockWindowSeconds)
	// A further failure must not extend an existing window.
	require.NoError(t, testEnv.Redis.ExpireCtx(ctx, key, 60))
	require.NoError(t, l.recordLoginFailure(id))
	ttl, err = testEnv.Redis.TtlCtx(ctx, key)
	require.NoError(t, err)
	require.LessOrEqual(t, ttl, 60)
	// Repair missing TTL even when the counter is already locked and no new
	// password comparison or failure increment will be performed.
	require.NoError(t, testEnv.Redis.SetCtx(ctx, key, "5"))
	locked, err := l.loginLocked(id)
	require.NoError(t, err)
	require.True(t, locked)
	ttl, err = testEnv.Redis.TtlCtx(ctx, key)
	require.NoError(t, err)
	require.Positive(t, ttl)
	require.NoError(t, testEnv.Redis.SetCtx(ctx, key, "2"))
	require.NoError(t, l.recordLoginFailure(id))
	ttl, err = testEnv.Redis.TtlCtx(ctx, key)
	require.NoError(t, err)
	require.Positive(t, ttl)
}

func TestLoginLockCollationIdentityIntegration(t *testing.T) {
	ctx := context.Background()
	const id int64 = 990042
	hashed, err := password.Hash("correct123")
	require.NoError(t, err)
	_, err = testSvcCtx.UserProfileModel.Insert(ctx, &model.UserProfile{Id: id, Username: "LockAlice", Password: hashed, Status: 1})
	require.NoError(t, err)
	// Exercise the actual shipped MySQL collation, not a mock alias mapping.
	alias, err := testSvcCtx.UserProfileModel.FindOneByUsername(ctx, "LOCKALICE")
	require.NoError(t, err)
	require.Equal(t, id, alias.Id)
	l := NewLoginLogic(ctx, testSvcCtx)
	for range loginLockMaxAttempts {
		_, err = l.Login(&pb.LoginReq{Username: "LockAlice", Password: "wrong", LoginType: 1})
		require.True(t, errx.Is(err, errx.PasswordError), "%v", err)
	}
	for _, username := range []string{"LockAlice", "LOCKALICE", "lockalice"} {
		_, err = l.Login(&pb.LoginReq{Username: username, Password: "correct123", LoginType: 1})
		require.True(t, errx.Is(err, errx.TooManyReq), "same locked ID must block %q: %v", username, err)
	}
}
