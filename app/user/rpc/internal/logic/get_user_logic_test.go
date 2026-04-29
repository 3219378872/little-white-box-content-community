package logic

import (
	"context"
	"errors"
	"testing"

	"errx"
	"user/internal/model"
	"user/pb/xiaobaihe/user/pb"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestGetUserLogic(t *testing.T) {
	tests := []struct {
		name      string
		req       *pb.GetUserReq
		setupMock func(*MockUserProfileModel)
		wantErr   bool
		errCode   int
		check     func(t *testing.T, resp *pb.GetUserResp)
	}{
		{
			name: "成功获取用户",
			req:  &pb.GetUserReq{UserId: 1},
			setupMock: func(m *MockUserProfileModel) {
				m.On("FindOne", mock.Anything, int64(1)).Return(sampleUser(1, "alice"), nil).Once()
			},
			check: func(t *testing.T, resp *pb.GetUserResp) {
				assert.Equal(t, int64(1), resp.User.Id)
				assert.Equal(t, "alice", resp.User.Username)
			},
		},
		{
			name: "用户不存在",
			req:  &pb.GetUserReq{UserId: 999},
			setupMock: func(m *MockUserProfileModel) {
				m.On("FindOne", mock.Anything, int64(999)).Return(
					(*model.UserProfile)(nil), model.ErrNotFound,
				).Once()
			},
			wantErr: true,
			errCode: errx.UserNotFound,
		},
		{
			name: "DB 错误",
			req:  &pb.GetUserReq{UserId: 1},
			setupMock: func(m *MockUserProfileModel) {
				m.On("FindOne", mock.Anything, int64(1)).Return(
					(*model.UserProfile)(nil), errors.New("connection refused"),
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
			logic := NewGetUserLogic(context.Background(), svcCtx)

			resp, err := logic.GetUser(tt.req)

			if tt.wantErr {
				require.Error(t, err)
				assert.Equal(t, tt.errCode, errx.GetCode(err))
				require.Nil(t, resp)
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
