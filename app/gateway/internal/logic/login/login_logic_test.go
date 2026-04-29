package login

import (
	"context"
	"errors"
	"testing"

	"errx"
	"gateway/internal/types"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"user/userservice"
)

func TestLoginLogic(t *testing.T) {
	tests := []struct {
		name      string
		req       *types.LoginReq
		setupMock func(*MockUserService)
		wantErr   bool
		errCode   int
		check     func(t *testing.T, resp *types.LoginResp)
	}{
		{
			name: "密码登录成功",
			req:  &types.LoginReq{Username: "alice", Password: "correct", LoginType: 1},
			setupMock: func(m *MockUserService) {
				m.On("Login", mock.Anything, &userservice.LoginReq{
					Username: "alice", Password: "correct", LoginType: 1,
				}).Return(&userservice.LoginResp{UserId: 1, Token: "token123"}, nil).Once()
			},
			check: func(t *testing.T, resp *types.LoginResp) {
				assert.Equal(t, int64(1), resp.UserId)
				assert.Equal(t, "token123", resp.Token)
			},
		},
		{
			name:    "密码登录-用户名为空",
			req:     &types.LoginReq{Password: "x", LoginType: 1},
			wantErr: true,
			errCode: errx.ParamError,
		},
		{
			name:    "密码登录-密码为空",
			req:     &types.LoginReq{Username: "alice", LoginType: 1},
			wantErr: true,
			errCode: errx.ParamError,
		},
		{
			name:    "验证码登录-手机号非法",
			req:     &types.LoginReq{Phone: "123", LoginType: 2},
			wantErr: true,
			errCode: errx.ParamError,
		},
		{
			name:    "验证码登录-验证码为空",
			req:     &types.LoginReq{Phone: "13800138000", LoginType: 2},
			wantErr: true,
			errCode: errx.ParamError,
		},
		{
			name: "RPC 返回错误",
			req:  &types.LoginReq{Username: "alice", Password: "correct", LoginType: 1},
			setupMock: func(m *MockUserService) {
				m.On("Login", mock.Anything, mock.Anything).Return(
					(*userservice.LoginResp)(nil), errors.New("rpc timeout"),
				).Once()
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			userSvc := new(MockUserService)
			if tt.setupMock != nil {
				tt.setupMock(userSvc)
			}
			svcCtx := newUnitSvcCtx(userSvc)
			logic := NewLoginLogic(context.Background(), svcCtx)

			resp, err := logic.Login(tt.req)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errCode != 0 {
					assert.Equal(t, tt.errCode, errx.GetCode(err))
				}
			} else {
				require.NoError(t, err)
				if tt.check != nil {
					tt.check(t, resp)
				}
			}
			userSvc.AssertExpectations(t)
		})
	}
}
