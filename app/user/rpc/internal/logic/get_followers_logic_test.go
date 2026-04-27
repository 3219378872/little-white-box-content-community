package logic

import (
	"context"
	"errors"
	"testing"
	"time"

	"errx"
	"user/internal/model"
	"user/internal/svc"
	"user/pb/xiaobaihe/user/pb"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type mockUserFollowModel struct{ mock.Mock }

func (m *mockUserFollowModel) FindFollowers(ctx context.Context, userID int64, offset, limit int64) ([]*model.UserProfile, error) {
	args := m.Called(ctx, userID, offset, limit)
	if v := args.Get(0); v != nil {
		return v.([]*model.UserProfile), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *mockUserFollowModel) FindFollowing(ctx context.Context, userID int64, offset, limit int64) ([]*model.UserProfile, error) {
	args := m.Called(ctx, userID, offset, limit)
	if v := args.Get(0); v != nil {
		return v.([]*model.UserProfile), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *mockUserFollowModel) CountFollowers(ctx context.Context, userID int64) (int64, error) {
	args := m.Called(ctx, userID)
	return args.Get(0).(int64), args.Error(1)
}

func (m *mockUserFollowModel) CountFollowing(ctx context.Context, userID int64) (int64, error) {
	args := m.Called(ctx, userID)
	return args.Get(0).(int64), args.Error(1)
}

func TestGetFollowersLogic_Success(t *testing.T) {
	now := time.Unix(1710000000, 0)
	followModel := new(mockUserFollowModel)
	followModel.On("FindFollowers", mock.Anything, int64(9), int64(0), int64(2)).Return([]*model.UserProfile{
		{Id: 101, Username: "u101", FollowerCount: 2, FollowingCount: 3, CreatedAt: now},
		{Id: 102, Username: "u102", FollowerCount: 4, FollowingCount: 5, CreatedAt: now},
	}, nil).Once()
	followModel.On("CountFollowers", mock.Anything, int64(9)).Return(int64(12), nil).Once()

	logic := NewGetFollowersLogic(context.Background(), &svc.ServiceContext{UserFollowModel: followModel})
	resp, err := logic.GetFollowers(&pb.GetFollowersReq{UserId: 9, Page: 1, PageSize: 2})

	require.NoError(t, err)
	require.Len(t, resp.Users, 2)
	assert.Equal(t, int64(12), resp.Total)
	assert.Equal(t, int64(101), resp.Users[0].Id)
	followModel.AssertExpectations(t)
}

func TestGetFollowersLogic_InvalidPage(t *testing.T) {
	logic := NewGetFollowersLogic(context.Background(), &svc.ServiceContext{})
	resp, err := logic.GetFollowers(&pb.GetFollowersReq{UserId: 9, Page: 0, PageSize: 2})

	require.Nil(t, resp)
	require.Error(t, err)
	assert.Equal(t, errx.ParamError, errx.GetCode(err))
}

func TestGetFollowersLogic_ModelError(t *testing.T) {
	followModel := new(mockUserFollowModel)
	followModel.On("FindFollowers", mock.Anything, int64(9), int64(0), int64(2)).Return(nil, errors.New("db down")).Once()

	logic := NewGetFollowersLogic(context.Background(), &svc.ServiceContext{UserFollowModel: followModel})
	resp, err := logic.GetFollowers(&pb.GetFollowersReq{UserId: 9, Page: 1, PageSize: 2})

	require.Nil(t, resp)
	require.Error(t, err)
	assert.Equal(t, errx.SystemError, errx.GetCode(err))
	followModel.AssertExpectations(t)
}
