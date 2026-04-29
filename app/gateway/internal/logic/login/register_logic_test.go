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

func TestRegisterLogic(t *testing.T) {
	tests := []struct {
		name      string
		req       *types.RegisterReq
		setupMock func(*MockUserService)
		wantErr   bool
		errCode   int
		check     func(t *testing.T, resp *types.RegisterResp)
	}{
		{
			name: "用户名注册成功",
			req:  &types.RegisterReq{Username: "newuser", Password: "Strong@123"},
			setupMock: func(m *MockUserService) {
				m.On("Register", mock.Anything, mock.Anything).Return(
					&userservice.RegisterResp{UserId: 10, Token: "tok"}, nil,
				).Once()
			},
			check: func(t *testing.T, resp *types.RegisterResp) {
				assert.Equal(t, int64(10), resp.UserId)
				assert.Equal(t, "tok", resp.Token)
			},
		},
		{
			name:    "用户名只含空格校验失败",
			req:     &types.RegisterReq{Username: "  ", Password: "Strong@123"},
			wantErr: true,
			errCode: errx.ParamError,
		},
		{
			name:    "密码太短",
			req:     &types.RegisterReq{Username: "user", Password: "123"},
			wantErr: true,
			errCode: errx.ParamError,
		},
		{
			name:    "用户名和手机号都不提供",
			req:     &types.RegisterReq{},
			wantErr: true,
			errCode: errx.ParamError,
		},
		{
			name: "RPC 错误",
			req:  &types.RegisterReq{Username: "newuser", Password: "Strong@123"},
			setupMock: func(m *MockUserService) {
				m.On("Register", mock.Anything, mock.Anything).Return(
					(*userservice.RegisterResp)(nil), errors.New("rpc error"),
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
			logic := NewRegisterLogic(context.Background(), svcCtx)

			resp, err := logic.Register(tt.req)

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
