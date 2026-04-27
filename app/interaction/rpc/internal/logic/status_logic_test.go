package logic

import (
	"context"
	model2 "esx/app/interaction/rpc/internal/model"
	"esx/app/interaction/rpc/internal/svc"
	"esx/app/interaction/rpc/pb/xiaobaihe/interaction/pb"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestCheckLikedLogic_CheckLiked_Liked(t *testing.T) {
	likeModel := new(mockLikeRecordModel)
	likeModel.
		On("FindOneByUserIdTargetIdTargetType", mock.Anything, int64(1), int64(100), int64(1)).
		Return(&model2.LikeRecord{Id: 1, UserId: 1, TargetId: 100, TargetType: 1, Status: model2.StatusActive}, nil).
		Once()

	logic := NewCheckLikedLogic(context.Background(), &svc.ServiceContext{LikeRecordModel: likeModel})
	resp, err := logic.CheckLiked(&pb.CheckLikedReq{UserId: 1, TargetId: 100, TargetType: 1})
	require.NoError(t, err)
	assert.True(t, resp.IsLiked)
	likeModel.AssertExpectations(t)
}

func TestCheckLikedLogic_CheckLiked_NotFound(t *testing.T) {
	likeModel := new(mockLikeRecordModel)
	likeModel.
		On("FindOneByUserIdTargetIdTargetType", mock.Anything, int64(1), int64(100), int64(1)).
		Return((*model2.LikeRecord)(nil), model2.ErrNotFound).
		Once()

	logic := NewCheckLikedLogic(context.Background(), &svc.ServiceContext{LikeRecordModel: likeModel})
	resp, err := logic.CheckLiked(&pb.CheckLikedReq{UserId: 1, TargetId: 100, TargetType: 1})
	require.NoError(t, err)
	assert.False(t, resp.IsLiked)
	likeModel.AssertExpectations(t)
}

func TestCheckLikedLogic_CheckLiked_Error(t *testing.T) {
	likeModel := new(mockLikeRecordModel)
	likeModel.
		On("FindOneByUserIdTargetIdTargetType", mock.Anything, int64(1), int64(100), int64(1)).
		Return((*model2.LikeRecord)(nil), assert.AnError).
		Once()

	logic := NewCheckLikedLogic(context.Background(), &svc.ServiceContext{LikeRecordModel: likeModel})
	_, err := logic.CheckLiked(&pb.CheckLikedReq{UserId: 1, TargetId: 100, TargetType: 1})
	require.Error(t, err)
	likeModel.AssertExpectations(t)
}

func TestBatchCheckLikedLogic_BatchCheckLiked(t *testing.T) {
	likeModel := new(mockLikeRecordModel)
	likeModel.
		On("FindStatusByUserAndTargets", mock.Anything, int64(1), []int64{100, 200}, int64(1)).
		Return(map[int64]bool{100: true}, nil).
		Once()

	logic := NewBatchCheckLikedLogic(context.Background(), &svc.ServiceContext{LikeRecordModel: likeModel})
	resp, err := logic.BatchCheckLiked(&pb.BatchCheckLikedReq{UserId: 1, TargetIds: []int64{100, 200}, TargetType: 1})
	require.NoError(t, err)
	assert.Equal(t, map[int64]bool{100: true, 200: false}, resp.Results)
	likeModel.AssertExpectations(t)
}

func TestCheckFavoritedLogic_CheckFavorited_Favorited(t *testing.T) {
	favoriteModel := new(mockFavoriteModel)
	favoriteModel.
		On("FindOneByUserIdPostId", mock.Anything, int64(1), int64(100)).
		Return(&model2.Favorite{Id: 1, UserId: 1, PostId: 100, Status: model2.StatusActive}, nil).
		Once()

	logic := NewCheckFavoritedLogic(context.Background(), &svc.ServiceContext{FavoriteModel: favoriteModel})
	resp, err := logic.CheckFavorited(&pb.CheckFavoritedReq{UserId: 1, PostId: 100})
	require.NoError(t, err)
	assert.True(t, resp.IsFavorited)
	favoriteModel.AssertExpectations(t)
}

func TestCheckFavoritedLogic_CheckFavorited_NotFound(t *testing.T) {
	favoriteModel := new(mockFavoriteModel)
	favoriteModel.
		On("FindOneByUserIdPostId", mock.Anything, int64(1), int64(100)).
		Return((*model2.Favorite)(nil), model2.ErrNotFound).
		Once()

	logic := NewCheckFavoritedLogic(context.Background(), &svc.ServiceContext{FavoriteModel: favoriteModel})
	resp, err := logic.CheckFavorited(&pb.CheckFavoritedReq{UserId: 1, PostId: 100})
	require.NoError(t, err)
	assert.False(t, resp.IsFavorited)
	favoriteModel.AssertExpectations(t)
}

func TestCheckFavoritedLogic_CheckFavorited_Error(t *testing.T) {
	favoriteModel := new(mockFavoriteModel)
	favoriteModel.
		On("FindOneByUserIdPostId", mock.Anything, int64(1), int64(100)).
		Return((*model2.Favorite)(nil), assert.AnError).
		Once()

	logic := NewCheckFavoritedLogic(context.Background(), &svc.ServiceContext{FavoriteModel: favoriteModel})
	_, err := logic.CheckFavorited(&pb.CheckFavoritedReq{UserId: 1, PostId: 100})
	require.Error(t, err)
	favoriteModel.AssertExpectations(t)
}

func TestBatchCheckFavoritedLogic_BatchCheckFavorited(t *testing.T) {
	favoriteModel := new(mockFavoriteModel)
	favoriteModel.
		On("FindFavoriteStatusByUserAndPosts", mock.Anything, int64(1), []int64{100, 200}).
		Return(map[int64]bool{100: true}, nil).
		Once()

	logic := NewBatchCheckFavoritedLogic(context.Background(), &svc.ServiceContext{FavoriteModel: favoriteModel})
	resp, err := logic.BatchCheckFavorited(&pb.BatchCheckFavoritedReq{UserId: 1, PostIds: []int64{100, 200}})
	require.NoError(t, err)
	assert.Equal(t, map[int64]bool{100: true, 200: false}, resp.Results)
	favoriteModel.AssertExpectations(t)
}
