package logic

import (
	"context"
	"testing"

	"esx/pkg/errx"
	"esx/pkg/jwtx"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIssueTokenPair_RefreshGenerateFailed(t *testing.T) {
	svcCtx, _ := refreshTestSvcCtx()
	svcCtx.Config.JwtConfig.RefreshSecret = ""
	_, _, err := issueTokenPair(context.Background(), svcCtx, 7, "alice")
	require.Error(t, err)
	assert.Equal(t, errx.SystemError, errx.GetCode(err))
}

func TestStoreRefreshJTI_ParseFailed(t *testing.T) {
	svcCtx, _ := refreshTestSvcCtx()
	err := storeRefreshJTI(context.Background(), svcCtx, "garbage-token", 7)
	require.Error(t, err)
	assert.Equal(t, errx.SystemError, errx.GetCode(err))
}

func TestStoreRefreshJTI_DefaultTTLWhenExpireUnset(t *testing.T) {
	svcCtx, mem := refreshTestSvcCtx()
	svcCtx.Config.JwtConfig.RefreshExpire = 0
	token, err := jwtx.GenerateRefreshToken(8, "bob", svcCtx.Config.JwtConfig)
	require.NoError(t, err)

	require.NoError(t, storeRefreshJTI(context.Background(), svcCtx, token, 8))
	claims, err := jwtx.ParseRefreshToken(token, svcCtx.Config.JwtConfig)
	require.NoError(t, err)
	stored, err := mem.GetCtx(context.Background(), refreshJTIKey(claims.ID))
	require.NoError(t, err)
	assert.Equal(t, "8", stored)
}

func TestStoreRefreshJTI_SetexFailed(t *testing.T) {
	svcCtx, _ := refreshTestSvcCtx()
	mem := &flakyRedis{
		memoryRedis: &memoryRedis{values: map[string]string{}},
		onSetex: func(key string) error {
			return errInjectedRedis
		},
	}
	svcCtx.RedisClient = mem
	token, err := jwtx.GenerateRefreshToken(9, "carol", svcCtx.Config.JwtConfig)
	require.NoError(t, err)

	err = storeRefreshJTI(context.Background(), svcCtx, token, 9)
	require.Error(t, err)
	assert.Equal(t, errx.SystemError, errx.GetCode(err))
}

func TestRotateRefreshToken_RedisMissing(t *testing.T) {
	svcCtxWithRedis, _ := refreshTestSvcCtx()
	_, oldRefresh, err := issueTokenPair(context.Background(), svcCtxWithRedis, 10, "dave")
	require.NoError(t, err)

	// 同一令牌、无 Redis 的环境：白名单不可用，按系统错误处理而不是放行。
	svcCtxWithoutRedis, _ := refreshTestSvcCtx()
	svcCtxWithoutRedis.RedisClient = nil
	_, _, err = rotateRefreshToken(context.Background(), svcCtxWithoutRedis, oldRefresh)
	require.Error(t, err)
	assert.Equal(t, errx.SystemError, errx.GetCode(err))
}

func TestRotateRefreshToken_ConsumeJTIFailed(t *testing.T) {
	svcCtx, _ := refreshTestSvcCtx()
	mem := &flakyRedis{
		memoryRedis: &memoryRedis{values: map[string]string{}},
		onEval: func(keys []string, args ...any) error {
			return errInjectedRedis
		},
	}
	svcCtx.RedisClient = mem
	_, oldRefresh, err := issueTokenPair(context.Background(), svcCtx, 11, "erin")
	require.NoError(t, err)

	_, _, err = rotateRefreshToken(context.Background(), svcCtx, oldRefresh)
	require.Error(t, err)
	assert.Equal(t, errx.SystemError, errx.GetCode(err))
}
