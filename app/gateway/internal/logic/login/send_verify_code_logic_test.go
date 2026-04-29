package login

import (
	"context"
	"errors"
	"testing"

	"errx"
	"gateway/internal/types"
	"user/pb/xiaobaihe/user/pb"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"user/userservice"
)

func TestSendVerifyCodeLogic(t *testing.T) {
	tests := []struct {
		name      string
		req       *types.SendVerifyCodeReq
		setupMock func(*MockUserService)
		wantErr   bool
		errCode   int
	}{
		{
			name: "发送成功",
			req:  &types.SendVerifyCodeReq{Phone: "13800138000", Type: 1},
			setupMock: func(m *MockUserService) {
				m.On("SendVerifyCode", mock.Anything, &userservice.SendVerifyCodeReq{
					Phone: "13800138000", Type: 1,
				}).Return(&userservice.SendVerifyCodeResp{}, nil).Once()
			},
		},
		{
			name:    "手机号非法",
			req:     &types.SendVerifyCodeReq{Phone: "123"},
			wantErr: true,
			errCode: errx.ParamError,
		},
		{
			name: "RPC 错误",
			req:  &types.SendVerifyCodeReq{Phone: "13800138000", Type: 2},
			setupMock: func(m *MockUserService) {
				m.On("SendVerifyCode", mock.Anything, mock.Anything).Return(
					(*userservice.SendVerifyCodeResp)(nil), errors.New("rpc error"),
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
			logic := NewSendVerifyCodeLogic(context.Background(), svcCtx)

			_, err := logic.SendVerifyCode(tt.req)

			if tt.wantErr {
				require.Error(t, err)
				if tt.errCode != 0 {
					assert.Equal(t, tt.errCode, errx.GetCode(err))
				}
			} else {
				require.NoError(t, err)
			}
			userSvc.AssertExpectations(t)
		})
	}
}

func TestRegisterReqConvert(t *testing.T) {
	req := &types.RegisterReq{
		Username: "alice", Password: "pass", Phone: "138", VerifyCode: "123456",
	}
	result := RegisterReqConvert(req)
	assert.Equal(t, "alice", result.Username)
	assert.Equal(t, "pass", result.Password)
	assert.Equal(t, "138", result.Phone)
	assert.Equal(t, "123456", result.VerifyCode)
}

func TestRegisterRespConvert(t *testing.T) {
	resp := &pb.RegisterResp{UserId: 1, Token: "tok"}
	result := RegisterRespConvert(resp)
	assert.Equal(t, int64(1), result.UserId)
	assert.Equal(t, "tok", result.Token)
}
