package logic

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"esx/app/user/rpc/internal/model"
	"esx/app/user/rpc/internal/password"
	"esx/app/user/rpc/internal/svc"
	"esx/app/user/rpc/pb/xiaobaihe/user/pb"
	"esx/pkg/errx"
	"esx/pkg/jwtx"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestLoginLogic_Password(t *testing.T) {
	hashedPwd, _ := password.Hash("correct123")

	tests := []struct {
		name      string
		req       *pb.LoginReq
		setupMock func(*MockUserProfileModel)
		wantErr   bool
		errCode   int
		check     func(t *testing.T, resp *pb.LoginResp)
	}{
		{
			name: "密码登录成功",
			req:  &pb.LoginReq{Username: "alice", Password: "correct123", LoginType: 1},
			setupMock: func(pm *MockUserProfileModel) {
				pm.On("FindOneByUsername", mock.Anything, "alice").Return(
					&model.UserProfile{Id: 1, Username: "alice", Password: hashedPwd}, nil,
				).Once()
			},
			check: func(t *testing.T, resp *pb.LoginResp) {
				assert.Equal(t, int64(1), resp.UserId)
				assert.NotEmpty(t, resp.Token)
			},
		},
		{
			name: "用户不存在",
			req:  &pb.LoginReq{Username: "nobody", Password: "x", LoginType: 1},
			setupMock: func(pm *MockUserProfileModel) {
				pm.On("FindOneByUsername", mock.Anything, "nobody").Return(
					(*model.UserProfile)(nil), model.ErrNotFound,
				).Once()
			},
			wantErr: true,
			errCode: errx.UserNotFound,
		},
		{
			name: "密码错误",
			req:  &pb.LoginReq{Username: "alice", Password: "wrong", LoginType: 1},
			setupMock: func(pm *MockUserProfileModel) {
				pm.On("FindOneByUsername", mock.Anything, "alice").Return(
					&model.UserProfile{Id: 1, Username: "alice", Password: hashedPwd}, nil,
				).Once()
			},
			wantErr: true,
			errCode: errx.PasswordError,
		},
		{
			name: "DB 错误",
			req:  &pb.LoginReq{Username: "alice", Password: "x", LoginType: 1},
			setupMock: func(pm *MockUserProfileModel) {
				pm.On("FindOneByUsername", mock.Anything, "alice").Return(
					(*model.UserProfile)(nil), errors.New("db down"),
				).Once()
			},
			wantErr: true,
			errCode: errx.SystemError,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pm := new(MockUserProfileModel)
			if tt.setupMock != nil {
				tt.setupMock(pm)
			}
			svcCtx := newUnitSvcCtx(pm, nil)
			svcCtx.Config.JwtConfig = jwtx.JwtConfig{
				AccessSecret:  "test-secret-32bytes-long-key!!",
				AccessExpire:  3600,
				RefreshSecret: "test-refresh-secret-32bytes!!",
				RefreshExpire: 7 * 24 * 3600,
			}

			logic := NewLoginLogic(context.Background(), svcCtx)
			resp, err := logic.Login(tt.req)

			if tt.wantErr {
				require.Error(t, err)
				assert.Equal(t, tt.errCode, errx.GetCode(err))
			} else {
				require.NoError(t, err)
				if tt.check != nil {
					tt.check(t, resp)
				}
			}
			pm.AssertExpectations(t)
		})
	}
}

func TestLoginVerifyCodeSharesAttemptLimitWithRegister(t *testing.T) {
	mem := &memoryRedis{values: map[string]string{verifyCodeRedisKey("13900000000"): "654321"}}
	profile := &MockUserProfileModel{}
	profile.On("FindOneByPhone", mock.Anything, mock.Anything).
		Return(&model.UserProfile{Id: 1, Username: "u1", Phone: sql.NullString{String: "13900000000", Valid: true}}, nil).Maybe()
	svcCtx := &svc.ServiceContext{RedisClient: mem, UserProfileModel: profile}
	req := &pb.LoginReq{LoginType: 2, Phone: "13900000000", VerifyCode: "000000"}

	for range verifyCodeMaxAttempts {
		_, err := NewLoginLogic(context.Background(), svcCtx).Login(req)
		require.Error(t, err)
		assert.True(t, errx.Is(err, errx.VerifyCodeError))
	}
	// 达到共享上限后验证码作废：注册/登录继续尝试均报过期。
	_, err := NewLoginLogic(context.Background(), svcCtx).Login(req)
	require.Error(t, err)
	assert.True(t, errx.Is(err, errx.VerifyCodeExpired), "verify code must be revoked, got %v", err)
}

func TestLoginPasswordFailureLockout(t *testing.T) {
	mem := &memoryRedis{values: map[string]string{}}
	profile := &MockUserProfileModel{}
	profile.On("FindOneByUsername", mock.Anything, "attacker").
		Return(&model.UserProfile{Id: 1, Username: "attacker", Password: "hash"}, nil).Maybe()
	svcCtx := &svc.ServiceContext{RedisClient: mem, UserProfileModel: profile}
	req := &pb.LoginReq{LoginType: 1, Username: "attacker", Password: "wrong"}

	for range loginLockMaxAttempts {
		_, err := NewLoginLogic(context.Background(), svcCtx).Login(req)
		require.Error(t, err)
		assert.True(t, errx.Is(err, errx.PasswordError), "allowed failures must return PasswordError, got %v", err)
	}
	_, err := NewLoginLogic(context.Background(), svcCtx).Login(req)
	require.Error(t, err)
	assert.True(t, errx.Is(err, errx.TooManyReq), "password login must lock out after max attempts, got %v", err)
}

func TestLoginPasswordLockoutRejectsCorrectPassword(t *testing.T) {
	const username = "attacker"
	const correct = "correct123"
	hashedPwd, err := password.Hash(correct)
	require.NoError(t, err)

	mem := &memoryRedis{values: map[string]string{}}
	profile := &MockUserProfileModel{}
	profile.On("FindOneByUsername", mock.Anything, username).
		Return(&model.UserProfile{Id: 1, Username: username, Password: hashedPwd}, nil).Maybe()
	svcCtx := &svc.ServiceContext{RedisClient: mem, UserProfileModel: profile}
	svcCtx.Config.JwtConfig = jwtx.JwtConfig{
		AccessSecret:  "test-secret-32bytes-long-key!!",
		AccessExpire:  3600,
		RefreshSecret: "test-refresh-secret-32bytes!!",
		RefreshExpire: 7 * 24 * 3600,
	}

	wrong := &pb.LoginReq{LoginType: 1, Username: username, Password: "wrong"}
	for range loginLockMaxAttempts {
		_, loginErr := NewLoginLogic(context.Background(), svcCtx).Login(wrong)
		require.Error(t, loginErr)
		assert.True(t, errx.Is(loginErr, errx.PasswordError))
	}

	resp, loginErr := NewLoginLogic(context.Background(), svcCtx).Login(&pb.LoginReq{
		LoginType: 1, Username: username, Password: correct,
	})
	require.Error(t, loginErr)
	assert.Nil(t, resp)
	assert.True(t, errx.Is(loginErr, errx.TooManyReq),
		"6th attempt with correct password is rejected while locked, got %v", loginErr)

	lockVal, getErr := mem.GetCtx(context.Background(), loginLockKey(username))
	require.NoError(t, getErr)
	assert.Equal(t, "5", lockVal, "successful guess must not clear the lock")
}
