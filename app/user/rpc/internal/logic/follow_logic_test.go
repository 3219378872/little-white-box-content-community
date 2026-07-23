package logic

import (
	"context"
	"errors"
	"testing"

	"errx"
	"user/internal/model"
	"user/pb/xiaobaihe/user/pb"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestFollowLogic_EquivalenceAndFailures(t *testing.T) {
	tests := []struct {
		name      string
		req       *pb.FollowReq
		setup     func(*MockUserProfileModel, *MockUserFollowStore)
		wantCode  int
		wantError bool
	}{
		{
			name: "valid distinct users",
			req:  &pb.FollowReq{UserId: 1, TargetUserId: 2},
			setup: func(profiles *MockUserProfileModel, follows *MockUserFollowStore) {
				profiles.On("FindOne", mock.Anything, int64(2)).Return(sampleUser(2, "target"), nil).Once()
				follows.On("Follow", mock.Anything, int64(1), int64(2)).Return(nil).Once()
			},
		},
		{name: "zero user id", req: &pb.FollowReq{TargetUserId: 2}, wantCode: errx.ParamError, wantError: true},
		{name: "negative target id", req: &pb.FollowReq{UserId: 1, TargetUserId: -1}, wantCode: errx.ParamError, wantError: true},
		{name: "same user boundary", req: &pb.FollowReq{UserId: 2, TargetUserId: 2}, wantCode: errx.CannotFollowSelf, wantError: true},
		{
			name: "target missing",
			req:  &pb.FollowReq{UserId: 1, TargetUserId: 2},
			setup: func(profiles *MockUserProfileModel, _ *MockUserFollowStore) {
				profiles.On("FindOne", mock.Anything, int64(2)).Return((*model.UserProfile)(nil), model.ErrNotFound).Once()
			},
			wantCode: errx.UserNotFound, wantError: true,
		},
		{
			name: "profile dependency failure",
			req:  &pb.FollowReq{UserId: 1, TargetUserId: 2},
			setup: func(profiles *MockUserProfileModel, _ *MockUserFollowStore) {
				profiles.On("FindOne", mock.Anything, int64(2)).Return((*model.UserProfile)(nil), errors.New("db down")).Once()
			},
			wantCode: errx.SystemError, wantError: true,
		},
		{
			name: "follow transaction failure",
			req:  &pb.FollowReq{UserId: 1, TargetUserId: 2},
			setup: func(profiles *MockUserProfileModel, follows *MockUserFollowStore) {
				profiles.On("FindOne", mock.Anything, int64(2)).Return(sampleUser(2, "target"), nil).Once()
				follows.On("Follow", mock.Anything, int64(1), int64(2)).Return(errors.New("db down")).Once()
			},
			wantCode: errx.SystemError, wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			profiles := new(MockUserProfileModel)
			follows := new(MockUserFollowStore)
			if tt.setup != nil {
				tt.setup(profiles, follows)
			}
			resp, err := NewFollowLogic(context.Background(), newUnitSvcCtx(profiles, follows)).Follow(tt.req)
			if tt.wantError {
				require.Error(t, err)
				require.Equal(t, tt.wantCode, errx.GetCode(err))
				require.Nil(t, resp)
			} else {
				require.NoError(t, err)
				require.NotNil(t, resp)
			}
			profiles.AssertExpectations(t)
			follows.AssertExpectations(t)
		})
	}
}
