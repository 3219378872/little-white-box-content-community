package logic

import (
	"context"
	"errors"
	"testing"

	"esx/app/user/rpc/internal/model"
	"esx/app/user/rpc/pb/xiaobaihe/user/pb"
	"esx/pkg/errx"

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

func TestGetUserViewerFollowState(t *testing.T) {
	for _, tt := range []struct {
		name        string
		viewer      int64
		relationErr error
		following   bool
		wantErr     bool
	}{
		{"anonymous", 0, nil, false, false},
		{"self", 2, nil, false, false},
		{"followed", 1, nil, true, false},
		{"not followed", 3, model.ErrNotFound, false, false},
		{"lookup failure", 4, errors.New("database unavailable"), false, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			profiles := new(MockUserProfileModel)
			profiles.On("FindOne", mock.Anything, int64(2)).Return(sampleUser(2, "target"), nil).Once()
			follows := new(MockUserFollowStore)
			if tt.viewer > 0 && tt.viewer != 2 {
				var relation *model.UserFollow
				if tt.following {
					relation = &model.UserFollow{UserId: tt.viewer, TargetUserId: 2}
				}
				follows.On("FindOneByUserIdTargetUserId", mock.Anything, tt.viewer, int64(2)).Return(relation, tt.relationErr).Once()
			}
			response, err := NewGetUserLogic(context.Background(), newUnitSvcCtx(profiles, follows)).GetUser(&pb.GetUserReq{UserId: 2, ViewerId: tt.viewer})
			if tt.wantErr {
				require.True(t, errx.Is(err, errx.SystemError))
				require.Nil(t, response)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.following, response.IsFollowing)
			}
			profiles.AssertExpectations(t)
			follows.AssertExpectations(t)
		})
	}
}

func TestGetUserRequiresRelationStoreForAuthenticatedViewer(t *testing.T) {
	profiles := new(MockUserProfileModel)
	profiles.On("FindOne", mock.Anything, int64(2)).Return(sampleUser(2, "target"), nil).Once()
	response, err := NewGetUserLogic(context.Background(), newUnitSvcCtx(profiles, nil)).GetUser(&pb.GetUserReq{UserId: 2, ViewerId: 1})
	require.True(t, errx.Is(err, errx.ServiceUnavailable))
	require.Nil(t, response)
	profiles.AssertExpectations(t)
}
