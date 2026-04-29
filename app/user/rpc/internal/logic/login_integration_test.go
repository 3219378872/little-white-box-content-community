//go:build integration

package logic

import (
	"context"
	"testing"

	"user/pb/xiaobaihe/user/pb"

	"github.com/stretchr/testify/require"
)

func TestLoginPasswordIntegration(t *testing.T) {
	testEnv.TruncateAll(t, "user_profile", "user_login_log")

	// 注册用户
	regLogic := NewRegisterLogic(context.Background(), testSvcCtx)
	regResp, err := regLogic.Register(&pb.RegisterReq{
		Username: "logintest",
		Password: "Strong@123",
	})
	require.NoError(t, err)

	// 密码登录
	loginLogic := NewLoginLogic(context.Background(), testSvcCtx)
	resp, err := loginLogic.Login(&pb.LoginReq{
		Username:  "logintest",
		Password:  "Strong@123",
		LoginType: 1,
	})
	require.NoError(t, err)
	require.Equal(t, regResp.UserId, resp.UserId)
	require.NotEmpty(t, resp.Token)
}

func TestLoginPhoneIntegration(t *testing.T) {
	testEnv.TruncateAll(t, "user_profile", "user_login_log")

	// 先发验证码
	svcLogic := NewSendVerifyCodeLogic(context.Background(), testSvcCtx)
	_, err := svcLogic.SendVerifyCode(&pb.SendVerifyCodeReq{Phone: "13800001111"})
	require.NoError(t, err)

	// 获取验证码值
	code, err := testEnv.Redis.GetCtx(context.Background(), "13800001111")
	require.NoError(t, err)
	require.NotEmpty(t, code)

	// 手机号注册
	regLogic := NewRegisterLogic(context.Background(), testSvcCtx)
	regResp, err := regLogic.Register(&pb.RegisterReq{
		Phone:      "13800001111",
		VerifyCode: code,
	})
	require.NoError(t, err)

	// 再发验证码用于登录
	_, err = svcLogic.SendVerifyCode(&pb.SendVerifyCodeReq{Phone: "13800001111"})
	require.NoError(t, err)
	code2, err := testEnv.Redis.GetCtx(context.Background(), "13800001111")
	require.NoError(t, err)

	// 验证码登录
	loginLogic := NewLoginLogic(context.Background(), testSvcCtx)
	resp, err := loginLogic.Login(&pb.LoginReq{
		Phone:      "13800001111",
		VerifyCode: code2,
		LoginType:  2,
	})
	require.NoError(t, err)
	require.Equal(t, regResp.UserId, resp.UserId)
	require.NotEmpty(t, resp.Token)
}
