//go:build integration

package logic

import (
	"context"
	"testing"

	"esx/app/user/rpc/pb/xiaobaihe/user/pb"

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

// 注册用户必须以正常状态（status=1）落库：SearchPublic 只检索 status=1，
// 零值落库会让新用户永久不可被搜索（黑盒 e2e 首轮发现的缺陷 B1）。
func TestRegisterPersistsActiveStatusIntegration(t *testing.T) {
	testEnv.TruncateAll(t, "user_profile")

	const username = "status_probe_user"
	logic := NewRegisterLogic(context.Background(), testSvcCtx)
	resp, err := logic.Register(&pb.RegisterReq{
		Username: username,
		Password: "Strong@123",
	})
	require.NoError(t, err)

	profile, err := testSvcCtx.UserProfileModel.FindOne(context.Background(), resp.UserId)
	require.NoError(t, err)
	assert.EqualValues(t, 1, profile.Status, "registered user must be active (status=1)")

	found, total, err := testSvcCtx.UserProfileModel.SearchPublic(context.Background(), username, 0, 10)
	require.NoError(t, err)
	assert.EqualValues(t, 1, total)
	require.Len(t, found, 1)
	assert.Equal(t, resp.UserId, found[0].Id)
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
