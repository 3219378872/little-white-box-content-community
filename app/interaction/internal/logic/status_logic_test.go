package logic

import (
	"context"
	"testing"

	"esx/app/interaction/internal/model"
	"esx/app/interaction/internal/svc"
	"esx/app/interaction/pb/xiaobaihe/interaction/pb"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestCheckLikedLogic_CheckLiked_Liked(t *testing.T) {
	likeModel := new(mockLikeRecordModel)
	likeModel.
		On("FindOneByUserIdTargetIdTargetType", mock.Anything, int64(1), int64(100), int64(1)).
		Return(&model.LikeRecord{Id: 1, UserId: 1, TargetId: 100, TargetType: 1, Status: 1}, nil).
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
		Return((*model.LikeRecord)(nil), model.ErrNotFound).
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
		Return((*model.LikeRecord)(nil), assert.AnError).
		Once()

	logic := NewCheckLikedLogic(context.Background(), &svc.ServiceContext{LikeRecordModel: likeModel})
	_, err := logic.CheckLiked(&pb.CheckLikedReq{UserId: 1, TargetId: 100, TargetType: 1})
	require.ErrorIs(t, err, assert.AnError)
	likeModel.AssertExpectations(t)
}

func TestBatchCheckLikedLogic_BatchCheckLiked(t *testing.T) {
	likeModel := new(mockLikeRecordModel)
	likeModel.
		On("FindOneByUserIdTargetIdTargetType", mock.Anything, int64(1), int64(100), int64(1)).
		Return(&model.LikeRecord{Id: 1, UserId: 1, TargetId: 100, TargetType: 1, Status: 1}, nil).
		Once()
	likeModel.
		On("FindOneByUserIdTargetIdTargetType", mock.Anything, int64(1), int64(200), int64(1)).
		Return((*model.LikeRecord)(nil), model.ErrNotFound).
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
		Return(&model.Favorite{Id: 1, UserId: 1, PostId: 100, Status: 1}, nil).
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
		Return((*model.Favorite)(nil), model.ErrNotFound).
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
		Return((*model.Favorite)(nil), assert.AnError).
		Once()

	logic := NewCheckFavoritedLogic(context.Background(), &svc.ServiceContext{FavoriteModel: favoriteModel})
	_, err := logic.CheckFavorited(&pb.CheckFavoritedReq{UserId: 1, PostId: 100})
	require.ErrorIs(t, err, assert.AnError)
	favoriteModel.AssertExpectations(t)
}

func TestBatchCheckFavoritedLogic_BatchCheckFavorited(t *testing.T) {
	favoriteModel := new(mockFavoriteModel)
	favoriteModel.
		On("FindOneByUserIdPostId", mock.Anything, int64(1), int64(100)).
		Return(&model.Favorite{Id: 1, UserId: 1, PostId: 100, Status: 1}, nil).
		Once()
	favoriteModel.
		On("FindOneByUserIdPostId", mock.Anything, int64(1), int64(200)).
		Return((*model.Favorite)(nil), model.ErrNotFound).
		Once()

	logic := NewBatchCheckFavoritedLogic(context.Background(), &svc.ServiceContext{FavoriteModel: favoriteModel})
	resp, err := logic.BatchCheckFavorited(&pb.BatchCheckFavoritedReq{UserId: 1, PostIds: []int64{100, 200}})
	require.NoError(t, err)
	assert.Equal(t, map[int64]bool{100: true, 200: false}, resp.Results)
	favoriteModel.AssertExpectations(t)
}
