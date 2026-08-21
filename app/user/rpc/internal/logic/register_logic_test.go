package logic

import (
	"context"
	"errors"
	"testing"

	"errx"
	"jwtx"
	"user/internal/model"
	"user/internal/svc"
	"user/pb/xiaobaihe/user/pb"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestRegisterLogic_ByUsername(t *testing.T) {
	tests := []struct {
		name      string
		req       *pb.RegisterReq
		setupMock func(*MockUserProfileModel)
		wantErr   bool
		errCode   int
		check     func(t *testing.T, resp *pb.RegisterResp)
	}{
		{
			name: "用户名注册成功",
			req:  &pb.RegisterReq{Username: "newuser", Password: "Strong@123"},
			setupMock: func(m *MockUserProfileModel) {
				m.On("Insert", mock.Anything, mock.AnythingOfType("*model.UserProfile")).Return(nil, nil).Once()
			},
			check: func(t *testing.T, resp *pb.RegisterResp) {
				assert.Greater(t, resp.UserId, int64(0))
				assert.NotEmpty(t, resp.Token)
			},
		},
		{
			name:    "密码太弱",
			req:     &pb.RegisterReq{Username: "newuser", Password: "123"},
			wantErr: true,
			errCode: errx.ParamError,
		},
		{
			name: "用户名已存在",
			req:  &pb.RegisterReq{Username: "existing", Password: "Strong@123"},
			setupMock: func(m *MockUserProfileModel) {
				m.On("Insert", mock.Anything, mock.AnythingOfType("*model.UserProfile")).Return(nil, errors.New("duplicate")).Once()
			},
			wantErr: true,
			errCode: errx.UserAlreadyExist,
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
				AccessSecret: "test-secret-32bytes-long-key!!",
				AccessExpire: 3600,
			}

			logic := NewRegisterLogic(context.Background(), svcCtx)
			resp, err := logic.Register(tt.req)

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

func TestRegisterByPhonePasswordStrength(t *testing.T) {
	t.Run("弱密码在触达存储前被拒绝", func(t *testing.T) {
		profile := &MockUserProfileModel{}
		// 强度校验必须先于任何存储访问：不设置任何 mock 期望，
		// 若实现提前查询手机号，AssertExpectations 不会失败但此处用
		// 显式断言确保未调用。
		profile.On("FindOneByPhone", mock.Anything, mock.Anything).
			Return(nil, model.ErrNotFound).Maybe()
		svcCtx := &svc.ServiceContext{RedisClient: &memoryRedis{values: map[string]string{}}, UserProfileModel: profile}
		req := &pb.RegisterReq{Phone: "13800000003", VerifyCode: "123456", Password: "123"}
		_, err := NewRegisterLogic(context.Background(), svcCtx).Register(req)
		require.Error(t, err)
		assert.Equal(t, errx.ParamError, errx.GetCode(err))
	})

	t.Run("空密码与强密码均放行进入验证码校验", func(t *testing.T) {
		for _, password := range []string{"", "Passw0rd!"} {
			mem := &memoryRedis{values: map[string]string{}}
			profile := &MockUserProfileModel{}
			profile.On("FindOneByPhone", mock.Anything, mock.Anything).
				Return(nil, model.ErrNotFound).Once()
			svcCtx := &svc.ServiceContext{RedisClient: mem, UserProfileModel: profile}
			req := &pb.RegisterReq{Phone: "13800000004", VerifyCode: "123456", Password: password}
			_, err := NewRegisterLogic(context.Background(), svcCtx).Register(req)
			require.Error(t, err)
			assert.True(t, errx.Is(err, errx.VerifyCodeExpired),
				"password %q must pass strength check and reach verify-code validation, got %v", password, err)
			profile.AssertExpectations(t)
		}
	})
}

func TestRegisterByPhoneVerifyCodeCooldownAndAttemptLimit(t *testing.T) {
	t.Run("未消费验证码的连续重发被冷却拦截", func(t *testing.T) {
		mem := &memoryRedis{values: map[string]string{}}
		svcCtx := &svc.ServiceContext{RedisClient: mem}
		logic := NewSendVerifyCodeLogic(context.Background(), svcCtx)
		// 首次发送：无未消费验证码，放行并设置验证码。
		resp, err := logic.SendVerifyCode(&pb.SendVerifyCodeReq{Phone: "13800000000"})
		require.NoError(t, err)
		require.NotNil(t, resp)
		// 第二次发送：验证码未消费，冷却键首次设置，放行（允许一次重发）。
		_, err = logic.SendVerifyCode(&pb.SendVerifyCodeReq{Phone: "13800000000"})
		require.NoError(t, err)
		// 第三次发送：验证码仍未消费且冷却键已存在，被拦截。
		_, err = logic.SendVerifyCode(&pb.SendVerifyCodeReq{Phone: "13800000000"})
		require.Error(t, err)
		assert.True(t, errx.Is(err, errx.TooManyReq), "cooldown must return TooManyReq")
	})

	t.Run("验证码消费后允许立即重发", func(t *testing.T) {
		mem := &memoryRedis{values: map[string]string{}}
		svcCtx := &svc.ServiceContext{RedisClient: mem}
		logic := NewSendVerifyCodeLogic(context.Background(), svcCtx)
		_, err := logic.SendVerifyCode(&pb.SendVerifyCodeReq{Phone: "13800000002"})
		require.NoError(t, err)
		// 模拟验证码被成功消费（注册/登录删除）。
		_, err = mem.DelCtx(context.Background(), "13800000002")
		require.NoError(t, err)
		// 消费后立即重发（注册后验证码登录流程）不应被冷却阻断。
		_, err = logic.SendVerifyCode(&pb.SendVerifyCodeReq{Phone: "13800000002"})
		require.NoError(t, err)
	})

	t.Run("错误尝试达上限后验证码作废", func(t *testing.T) {
		mem := &memoryRedis{values: map[string]string{"13800000001": "123456"}}
		profile := &MockUserProfileModel{}
		profile.On("FindOneByPhone", mock.Anything, mock.Anything).
			Return(nil, model.ErrNotFound).Maybe()
		svcCtx := &svc.ServiceContext{RedisClient: mem, UserProfileModel: profile}
		req := &pb.RegisterReq{
			Phone: "13800000001", VerifyCode: "000000",
			Username: "u1", Password: "Passw0rd!",
		}
		for i := 0; i < verifyCodeMaxAttempts; i++ {
			_, err := NewRegisterLogic(context.Background(), svcCtx).Register(req)
			require.Error(t, err, "wrong code must fail")
			assert.True(t, errx.Is(err, errx.VerifyCodeError))
		}
		// 达到上限：验证码键已被删除，继续尝试应报过期而非错误码。
		_, err := NewRegisterLogic(context.Background(), svcCtx).Register(req)
		require.Error(t, err)
		assert.True(t, errx.Is(err, errx.VerifyCodeExpired), "verify code must be revoked after max attempts, got %v", err)
	})
}
