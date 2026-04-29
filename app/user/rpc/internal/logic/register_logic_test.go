package logic

import (
	"context"
	"errors"
	"testing"

	"errx"
	"jwtx"
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
