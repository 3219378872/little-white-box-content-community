//go:build integration

package logic

import (
	"context"
	"testing"

	"user/pb/xiaobaihe/user/pb"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRegisterByUsernameIntegration(t *testing.T) {
	testEnv.TruncateAll(t, "user_profile")

	logic := NewRegisterLogic(context.Background(), testSvcCtx)
	resp, err := logic.Register(&pb.RegisterReq{
		Username: "integ_user",
		Password: "Strong@123",
	})
	require.NoError(t, err)
	assert.Greater(t, resp.UserId, int64(0))
	assert.NotEmpty(t, resp.Token)
}

func TestRegisterDuplicateIntegration(t *testing.T) {
	testEnv.TruncateAll(t, "user_profile")

	logic := NewRegisterLogic(context.Background(), testSvcCtx)
	_, err := logic.Register(&pb.RegisterReq{Username: "dup_user", Password: "Strong@123"})
	require.NoError(t, err)

	_, err = logic.Register(&pb.RegisterReq{Username: "dup_user", Password: "Strong@123"})
	require.Error(t, err)
}

func TestRegisterByPhoneIntegration(t *testing.T) {
	testEnv.TruncateAll(t, "user_profile")

	// 发送验证码
	svcLogic := NewSendVerifyCodeLogic(context.Background(), testSvcCtx)
	_, err := svcLogic.SendVerifyCode(&pb.SendVerifyCodeReq{Phone: "13900001111"})
	require.NoError(t, err)

	// 获取验证码值
	code, err := testEnv.Redis.GetCtx(context.Background(), "13900001111")
	require.NoError(t, err)
	require.NotEmpty(t, code)

	// 手机号注册
	logic := NewRegisterLogic(context.Background(), testSvcCtx)
	resp, err := logic.Register(&pb.RegisterReq{
		Phone:      "13900001111",
		VerifyCode: code,
	})
	require.NoError(t, err)
	assert.Greater(t, resp.UserId, int64(0))
	assert.NotEmpty(t, resp.Token)
}
