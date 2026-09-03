//go:build integration

package logic

import (
	"context"
	"testing"

	"esx/app/user/rpc/pb/xiaobaihe/user/pb"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSendVerifyCodeIntegration(t *testing.T) {
	logic := NewSendVerifyCodeLogic(context.Background(), testSvcCtx)
	_, err := logic.SendVerifyCode(&pb.SendVerifyCodeReq{Phone: "13800138000"})
	require.NoError(t, err)

	// 验证 Redis 中写入了 6 位验证码
	code, err := testEnv.Redis.GetCtx(context.Background(), verifyCodeRedisKey("13800138000"))
	require.NoError(t, err)
	assert.Len(t, code, 6)
}
