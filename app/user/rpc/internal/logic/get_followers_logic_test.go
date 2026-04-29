package logic

import (
	"context"
	"errors"
	"testing"
	"time"

	"errx"
	"user/internal/model"

	"user/pb/xiaobaihe/user/pb"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestGetFollowersLogic_Success(t *testing.T) {
	now := time.Unix(1710000000, 0)
	followStore := new(MockUserFollowStore)
	followStore.On("FindFollowers", mock.Anything, int64(9), int64(0), int64(2)).Return([]*model.UserProfile{
		{Id: 101, Username: "u101", FollowerCount: 2, FollowingCount: 3, CreatedAt: now},
		{Id: 102, Username: "u102", FollowerCount: 4, FollowingCount: 5, CreatedAt: now},
	}, nil).Once()
	followStore.On("CountFollowers", mock.Anything, int64(9)).Return(int64(12), nil).Once()

	svcCtx := newUnitSvcCtx(nil, followStore)
	logic := NewGetFollowersLogic(context.Background(), svcCtx)
	resp, err := logic.GetFollowers(&pb.GetFollowersReq{UserId: 9, Page: 1, PageSize: 2})

	require.NoError(t, err)
	require.Len(t, resp.Users, 2)
	assert.Equal(t, int64(12), resp.Total)
	assert.Equal(t, int64(101), resp.Users[0].Id)
	followStore.AssertExpectations(t)
}

func TestGetFollowersLogic_InvalidPage(t *testing.T) {
	logic := NewGetFollowersLogic(context.Background(), newUnitSvcCtx(nil, nil))
	resp, err := logic.GetFollowers(&pb.GetFollowersReq{UserId: 9, Page: 0, PageSize: 2})

	require.Nil(t, resp)
	require.Error(t, err)
	assert.Equal(t, errx.ParamError, errx.GetCode(err))
}

func TestGetFollowersLogic_ModelError(t *testing.T) {
	followStore := new(MockUserFollowStore)
	followStore.On("FindFollowers", mock.Anything, int64(9), int64(0), int64(2)).Return(nil, errors.New("db down")).Once()

	svcCtx := newUnitSvcCtx(nil, followStore)
	logic := NewGetFollowersLogic(context.Background(), svcCtx)
	resp, err := logic.GetFollowers(&pb.GetFollowersReq{UserId: 9, Page: 1, PageSize: 2})

	require.Nil(t, resp)
	require.Error(t, err)
	assert.Equal(t, errx.SystemError, errx.GetCode(err))
	followStore.AssertExpectations(t)
}
