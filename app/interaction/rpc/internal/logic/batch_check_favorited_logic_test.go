package logic

import (
	"context"
	"testing"

	"errx"
	"esx/app/interaction/internal/svc"
	"esx/app/interaction/pb/xiaobaihe/interaction/pb"
	"esx/pkg/validator"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestBatchCheckFavoritedLogic_BatchCheckFavorited_Success(t *testing.T) {
	favoriteModel := new(mockFavoriteModel)
	svcCtx := &svc.ServiceContext{
		FavoriteModel: favoriteModel,
	}

	favoriteModel.
		On("FindFavoriteStatusByUserAndPosts", mock.Anything, int64(1), []int64{10, 20, 30}).
		Return(map[int64]bool{10: true, 20: false}, nil).
		Once()

	logic := NewBatchCheckFavoritedLogic(context.Background(), svcCtx)
	resp, err := logic.BatchCheckFavorited(
		&pb.BatchCheckFavoritedReq{UserId: 1, PostIds: []int64{10, 20, 30}})
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.True(t, resp.Results[10])
	assert.False(t, resp.Results[20])
	assert.False(t, resp.Results[30])
	favoriteModel.AssertExpectations(t)
}

func TestBatchCheckFavoritedLogic_BatchCheckFavorited_Empty(t *testing.T) {
	logic := NewBatchCheckFavoritedLogic(context.Background(), &svc.ServiceContext{})
	resp, err := logic.BatchCheckFavorited(
		&pb.BatchCheckFavoritedReq{UserId: 1, PostIds: []int64{}})
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Empty(t, resp.Results)
}

func TestBatchCheckFavoritedLogic_BatchCheckFavorited_TooMany(t *testing.T) {
	logic := NewBatchCheckFavoritedLogic(context.Background(), &svc.ServiceContext{})
	ids := make([]int64, validator.MaxBatchQueryIds+1)
	_, err := logic.BatchCheckFavorited(
		&pb.BatchCheckFavoritedReq{UserId: 1, PostIds: ids})
	require.Error(t, err)
	assert.True(t, errx.Is(err, errx.ParamError))
}

func TestBatchCheckFavoritedLogic_BatchCheckFavorited_InvalidUser(t *testing.T) {
	logic := NewBatchCheckFavoritedLogic(context.Background(), &svc.ServiceContext{})
	_, err := logic.BatchCheckFavorited(
		&pb.BatchCheckFavoritedReq{UserId: 0, PostIds: []int64{10}})
	require.Error(t, err)
	assert.True(t, errx.Is(err, errx.ParamError))
}

func TestBatchCheckFavoritedLogic_BatchCheckFavorited_QueryError(t *testing.T) {
	favoriteModel := new(mockFavoriteModel)
	svcCtx := &svc.ServiceContext{
		FavoriteModel: favoriteModel,
	}

	favoriteModel.
		On("FindFavoriteStatusByUserAndPosts", mock.Anything, int64(1), []int64{10}).
		Return(nil, assert.AnError).
		Once()

	logic := NewBatchCheckFavoritedLogic(context.Background(), svcCtx)
	_, err := logic.BatchCheckFavorited(
		&pb.BatchCheckFavoritedReq{UserId: 1, PostIds: []int64{10}})
	require.Error(t, err)
	assert.True(t, errx.Is(err, errx.SystemError))
	favoriteModel.AssertExpectations(t)
}
