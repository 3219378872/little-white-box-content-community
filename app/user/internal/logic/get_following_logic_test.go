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

func TestGetFollowingLogic_Success(t *testing.T) {
	now := time.Unix(1710000000, 0)
	followModel := new(mockUserFollowModel)
	followModel.On("FindFollowing", mock.Anything, int64(9), int64(2), int64(2)).Return([]*model.UserProfile{
		{Id: 201, Username: "u201", FollowerCount: 6, FollowingCount: 7, CreatedAt: now},
	}, nil).Once()
	followModel.On("CountFollowing", mock.Anything, int64(9)).Return(int64(5), nil).Once()

	logic := NewGetFollowingLogic(context.Background(), &svc.ServiceContext{UserFollowModel: followModel})
	resp, err := logic.GetFollowing(&pb.GetFollowingReq{UserId: 9, Page: 2, PageSize: 2})

	require.NoError(t, err)
	require.Len(t, resp.Users, 1)
	assert.Equal(t, int64(5), resp.Total)
	assert.Equal(t, int64(201), resp.Users[0].Id)
	followModel.AssertExpectations(t)
}

func TestGetFollowingLogic_InvalidPage(t *testing.T) {
	logic := NewGetFollowingLogic(context.Background(), &svc.ServiceContext{})
	resp, err := logic.GetFollowing(&pb.GetFollowingReq{UserId: 9, Page: 1, PageSize: 0})

	require.Nil(t, resp)
	require.Error(t, err)
	assert.Equal(t, errx.ParamError, errx.GetCode(err))
}

func TestGetFollowingLogic_ModelError(t *testing.T) {
	followModel := new(mockUserFollowModel)
	followModel.On("FindFollowing", mock.Anything, int64(9), int64(0), int64(2)).Return(nil, errors.New("db down")).Once()

	logic := NewGetFollowingLogic(context.Background(), &svc.ServiceContext{UserFollowModel: followModel})
	resp, err := logic.GetFollowing(&pb.GetFollowingReq{UserId: 9, Page: 1, PageSize: 2})

	require.Nil(t, resp)
	require.Error(t, err)
	assert.Equal(t, errx.SystemError, errx.GetCode(err))
	followModel.AssertExpectations(t)
}
