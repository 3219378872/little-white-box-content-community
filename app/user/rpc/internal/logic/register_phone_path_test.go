package logic

import (
	"context"
	"errors"
	"testing"

	"esx/app/user/rpc/internal/model"
	"esx/app/user/rpc/internal/svc"
	"esx/app/user/rpc/pb/xiaobaihe/user/pb"
	"esx/pkg/errx"
	"esx/pkg/jwtx"

	"github.com/go-sql-driver/mysql"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func registerPhoneSvcCtx(mem svc.RedisStore, profile *MockUserProfileModel) *svc.ServiceContext {
	svcCtx := newUnitSvcCtx(profile, nil)
	svcCtx.RedisClient = mem
	svcCtx.Config.JwtConfig = jwtx.JwtConfig{
		AccessSecret:  "test-secret-32bytes-long-key!!",
		AccessExpire:  3600,
		RefreshSecret: "test-refresh-secret-32bytes!!",
		RefreshExpire: 7 * 24 * 3600,
	}
	return svcCtx
}

func TestRegisterByPhone_Success(t *testing.T) {
	mem := &memoryRedis{values: map[string]string{verifyCodeRedisKey("13800110000"): "654321"}}
	profile := &MockUserProfileModel{}
	profile.On("FindOneByPhone", mock.Anything, mock.Anything).Return(nil, model.ErrNotFound).Once()
	profile.On("Insert", mock.Anything, mock.AnythingOfType("*model.UserProfile")).
		Return(nil, nil).Once()

	req := &pb.RegisterReq{Phone: "13800110000", VerifyCode: "654321"}
	resp, err := NewRegisterLogic(context.Background(), registerPhoneSvcCtx(mem, profile)).Register(req)
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Greater(t, resp.UserId, int64(0))
	require.NotEmpty(t, resp.Token)
	require.NotEmpty(t, resp.RefreshToken)

	// 成功消费后验证码被删除，且 refresh jti 进入白名单。
	code, err := mem.GetCtx(context.Background(), verifyCodeRedisKey("13800110000"))
	require.NoError(t, err)
	require.Empty(t, code)
}

func TestRegisterByPhone_PhoneLookupSystemError(t *testing.T) {
	mem := &memoryRedis{values: map[string]string{}}
	profile := &MockUserProfileModel{}
	profile.On("FindOneByPhone", mock.Anything, mock.Anything).
		Return(nil, errors.New("db down")).Once()

	req := &pb.RegisterReq{Phone: "13800110001", VerifyCode: "654321"}
	_, err := NewRegisterLogic(context.Background(), registerPhoneSvcCtx(mem, profile)).Register(req)
	require.Error(t, err)
	require.Equal(t, errx.SystemError, errx.GetCode(err))
	profile.AssertExpectations(t)
}

func TestRegisterByPhone_PhoneAlreadyExists(t *testing.T) {
	mem := &memoryRedis{values: map[string]string{}}
	profile := &MockUserProfileModel{}
	profile.On("FindOneByPhone", mock.Anything, mock.Anything).
		Return(sampleUser(9, "taken"), nil).Once()

	req := &pb.RegisterReq{Phone: "13800110002", VerifyCode: "654321"}
	_, err := NewRegisterLogic(context.Background(), registerPhoneSvcCtx(mem, profile)).Register(req)
	require.Error(t, err)
	require.Equal(t, errx.UserAlreadyExist, errx.GetCode(err))
	profile.AssertExpectations(t)
}

func TestRegisterByPhone_VerifyCodeLookupFailed(t *testing.T) {
	mem := &flakyRedis{
		memoryRedis: &memoryRedis{values: map[string]string{}},
		onGet: func(key string) error {
			return errInjectedRedis
		},
	}
	profile := &MockUserProfileModel{}
	profile.On("FindOneByPhone", mock.Anything, mock.Anything).Return(nil, model.ErrNotFound).Once()

	req := &pb.RegisterReq{Phone: "13800110003", VerifyCode: "654321"}
	_, err := NewRegisterLogic(context.Background(), registerPhoneSvcCtx(mem, profile)).Register(req)
	require.Error(t, err)
	require.Equal(t, errx.SystemError, errx.GetCode(err))
	profile.AssertExpectations(t)
}

func TestRegisterByPhone_InsertConflict(t *testing.T) {
	mem := &memoryRedis{values: map[string]string{verifyCodeRedisKey("13800110004"): "654321"}}
	profile := &MockUserProfileModel{}
	profile.On("FindOneByPhone", mock.Anything, mock.Anything).Return(nil, model.ErrNotFound).Once()
	profile.On("Insert", mock.Anything, mock.AnythingOfType("*model.UserProfile")).
		Return(nil, &mysql.MySQLError{Number: 1062, Message: "Duplicate entry"}).Once()

	req := &pb.RegisterReq{Phone: "13800110004", VerifyCode: "654321"}
	_, err := NewRegisterLogic(context.Background(), registerPhoneSvcCtx(mem, profile)).Register(req)
	require.Error(t, err)
	require.Equal(t, errx.UserAlreadyExist, errx.GetCode(err))
	profile.AssertExpectations(t)
}

func TestRegisterByPhone_ConsumeVerifyCodeFailed(t *testing.T) {
	mem := &flakyRedis{
		memoryRedis: &memoryRedis{values: map[string]string{verifyCodeRedisKey("13800110005"): "654321"}},
		onDel: func(keys ...string) error {
			return errInjectedRedis
		},
	}
	profile := &MockUserProfileModel{}
	profile.On("FindOneByPhone", mock.Anything, mock.Anything).Return(nil, model.ErrNotFound).Once()
	profile.On("Insert", mock.Anything, mock.AnythingOfType("*model.UserProfile")).
		Return(nil, nil).Once()

	req := &pb.RegisterReq{Phone: "13800110005", VerifyCode: "654321"}
	resp, err := NewRegisterLogic(context.Background(), registerPhoneSvcCtx(mem, profile)).Register(req)
	require.NoError(t, err)
	require.NotNil(t, resp)
	require.Greater(t, resp.UserId, int64(0))
	profile.AssertExpectations(t)
}

func TestRegisterByPhone_RefreshTokenGenerateFailed(t *testing.T) {
	mem := &memoryRedis{values: map[string]string{verifyCodeRedisKey("13800110006"): "654321"}}
	profile := &MockUserProfileModel{}
	profile.On("FindOneByPhone", mock.Anything, mock.Anything).Return(nil, model.ErrNotFound).Once()
	profile.On("Insert", mock.Anything, mock.AnythingOfType("*model.UserProfile")).
		Return(nil, nil).Once()

	svcCtx := registerPhoneSvcCtx(mem, profile)
	// 空 RefreshSecret 使 jwtx.GenerateRefreshToken 失败（访问令牌已签发成功）。
	svcCtx.Config.JwtConfig.RefreshSecret = ""

	req := &pb.RegisterReq{Phone: "13800110006", VerifyCode: "654321"}
	_, err := NewRegisterLogic(context.Background(), svcCtx).Register(req)
	require.Error(t, err)
	require.Equal(t, errx.SystemError, errx.GetCode(err))
	profile.AssertExpectations(t)
}
