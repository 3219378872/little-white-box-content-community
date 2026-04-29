package logic

import (
	"context"
	"errors"
	"testing"

	"errx"
	"jwtx"
	"user/internal/model"
	"user/pb/xiaobaihe/user/pb"
	"util"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestLoginLogic_Password(t *testing.T) {
	hashedPwd, _ := util.HashPassword("correct123")

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
				AccessSecret: "test-secret-32bytes-long-key!!",
				AccessExpire: 3600,
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
