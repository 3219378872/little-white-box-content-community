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

func TestUnfollowLogic_EquivalenceAndFailures(t *testing.T) {
	tests := []struct {
		name      string
		req       *pb.UnfollowReq
		setup     func(*MockUserProfileModel, *MockUserFollowStore)
		wantCode  int
		wantError bool
	}{
		{
			name: "valid distinct users",
			req:  &pb.UnfollowReq{UserId: 1, TargetUserId: 2},
			setup: func(profiles *MockUserProfileModel, follows *MockUserFollowStore) {
				profiles.On("FindOne", mock.Anything, int64(2)).Return(sampleUser(2, "target"), nil).Once()
				follows.On("Unfollow", mock.Anything, int64(1), int64(2)).Return(nil).Once()
			},
		},
		{name: "zero target id", req: &pb.UnfollowReq{UserId: 1}, wantCode: errx.ParamError, wantError: true},
		{name: "same user boundary", req: &pb.UnfollowReq{UserId: 2, TargetUserId: 2}, wantCode: errx.CannotFollowSelf, wantError: true},
		{
			name: "target missing",
			req:  &pb.UnfollowReq{UserId: 1, TargetUserId: 2},
			setup: func(profiles *MockUserProfileModel, _ *MockUserFollowStore) {
				profiles.On("FindOne", mock.Anything, int64(2)).Return((*model.UserProfile)(nil), model.ErrNotFound).Once()
			},
			wantCode: errx.UserNotFound, wantError: true,
		},
		{
			name: "unfollow transaction failure",
			req:  &pb.UnfollowReq{UserId: 1, TargetUserId: 2},
			setup: func(profiles *MockUserProfileModel, follows *MockUserFollowStore) {
				profiles.On("FindOne", mock.Anything, int64(2)).Return(sampleUser(2, "target"), nil).Once()
				follows.On("Unfollow", mock.Anything, int64(1), int64(2)).Return(errors.New("db down")).Once()
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
			resp, err := NewUnfollowLogic(context.Background(), newUnitSvcCtx(profiles, follows)).Unfollow(tt.req)
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
