package logic

import (
	"context"
	"testing"

	"errx"
	"esx/app/interaction/internal/model"
	"esx/app/interaction/internal/svc"
	"esx/app/interaction/pb/xiaobaihe/interaction/pb"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestUnfavoriteLogic_Unfavorite_Success(t *testing.T) {
	favoriteModel := new(mockFavoriteModel)
	countModel := new(mockActionCountModel)
	svcCtx := &svc.ServiceContext{
		FavoriteModel:    favoriteModel,
		ActionCountModel: countModel,
	}

	favoriteModel.
		On("FindOneByUserIdPostId", mock.Anything, int64(1), int64(100)).
		Return(&model.Favorite{Id: 1, UserId: 1, PostId: 100, Status: 1}, nil).
		Once()
	favoriteModel.
		On("UpdateStatusById", mock.Anything, int64(1), int64(model.StatusActive), int64(model.StatusInactive)).
		Return(stubResult{rowsAffected: 1}, nil).
		Once()
	countModel.
		On("DecrFavoriteCount", mock.Anything, int64(100), int64(1)).
		Return(nil).
		Once()

	logic := NewUnfavoriteLogic(context.Background(), svcCtx)
	resp, err := logic.Unfavorite(&pb.UnfavoriteReq{UserId: 1, PostId: 100})
	require.NoError(t, err)
	require.NotNil(t, resp)
	favoriteModel.AssertExpectations(t)
	countModel.AssertExpectations(t)
}

func TestUnfavoriteLogic_Unfavorite_NotFavorited(t *testing.T) {
	favoriteModel := new(mockFavoriteModel)
	svcCtx := &svc.ServiceContext{
		FavoriteModel: favoriteModel,
	}

	favoriteModel.
		On("FindOneByUserIdPostId", mock.Anything, int64(1), int64(100)).
		Return((*model.Favorite)(nil), model.ErrNotFound).
		Once()

	logic := NewUnfavoriteLogic(context.Background(), svcCtx)
	_, err := logic.Unfavorite(&pb.UnfavoriteReq{UserId: 1, PostId: 100})
	require.Error(t, err)
	assert.True(t, errx.Is(err, errx.NotFavoritedYet))
	favoriteModel.AssertExpectations(t)
}

func TestUnfavoriteLogic_Unfavorite_AlreadyUnfavorited(t *testing.T) {
	favoriteModel := new(mockFavoriteModel)
	svcCtx := &svc.ServiceContext{
		FavoriteModel: favoriteModel,
	}

	favoriteModel.
		On("FindOneByUserIdPostId", mock.Anything, int64(1), int64(100)).
		Return(&model.Favorite{Id: 1, UserId: 1, PostId: 100, Status: model.StatusInactive}, nil).
		Once()

	logic := NewUnfavoriteLogic(context.Background(), svcCtx)
	_, err := logic.Unfavorite(&pb.UnfavoriteReq{UserId: 1, PostId: 100})
	require.Error(t, err)
	assert.True(t, errx.Is(err, errx.NotFavoritedYet))
	favoriteModel.AssertExpectations(t)
}

func TestUnfavoriteLogic_Unfavorite_NilActionCountModel(t *testing.T) {
	favoriteModel := new(mockFavoriteModel)
	svcCtx := &svc.ServiceContext{
		FavoriteModel: favoriteModel,
	}

	favoriteModel.
		On("FindOneByUserIdPostId", mock.Anything, int64(1), int64(100)).
		Return(&model.Favorite{Id: 1, UserId: 1, PostId: 100, Status: 1}, nil).
		Once()
	favoriteModel.
		On("UpdateStatusById", mock.Anything, int64(1), int64(model.StatusActive), int64(model.StatusInactive)).
		Return(stubResult{rowsAffected: 1}, nil).
		Once()

	logic := NewUnfavoriteLogic(context.Background(), svcCtx)
	resp, err := logic.Unfavorite(&pb.UnfavoriteReq{UserId: 1, PostId: 100})
	require.NoError(t, err)
	require.NotNil(t, resp)
	favoriteModel.AssertExpectations(t)
}

func TestUnfavoriteLogic_Unfavorite_DecrCountError(t *testing.T) {
	favoriteModel := new(mockFavoriteModel)
	countModel := new(mockActionCountModel)
	svcCtx := &svc.ServiceContext{
		FavoriteModel:    favoriteModel,
		ActionCountModel: countModel,
	}

	favoriteModel.
		On("FindOneByUserIdPostId", mock.Anything, int64(1), int64(100)).
		Return(&model.Favorite{Id: 1, UserId: 1, PostId: 100, Status: 1}, nil).
		Once()
	favoriteModel.
		On("UpdateStatusById", mock.Anything, int64(1), int64(model.StatusActive), int64(model.StatusInactive)).
		Return(stubResult{rowsAffected: 1}, nil).
		Once()
	countModel.
		On("DecrFavoriteCount", mock.Anything, int64(100), int64(1)).
		Return(assert.AnError).
		Once()

	logic := NewUnfavoriteLogic(context.Background(), svcCtx)
	resp, err := logic.Unfavorite(&pb.UnfavoriteReq{UserId: 1, PostId: 100})
	require.NoError(t, err)
	require.NotNil(t, resp)
	favoriteModel.AssertExpectations(t)
	countModel.AssertExpectations(t)
}

func TestUnfavoriteLogic_Unfavorite_UpdateStatusError(t *testing.T) {
	favoriteModel := new(mockFavoriteModel)
	svcCtx := &svc.ServiceContext{
		FavoriteModel: favoriteModel,
	}

	favoriteModel.
		On("FindOneByUserIdPostId", mock.Anything, int64(1), int64(100)).
		Return(&model.Favorite{Id: 1, UserId: 1, PostId: 100, Status: 1}, nil).
		Once()
	favoriteModel.
		On("UpdateStatusById", mock.Anything, int64(1), int64(model.StatusActive), int64(model.StatusInactive)).
		Return(nil, assert.AnError).
		Once()

	logic := NewUnfavoriteLogic(context.Background(), svcCtx)
	_, err := logic.Unfavorite(&pb.UnfavoriteReq{UserId: 1, PostId: 100})
	require.Error(t, err)
	assert.True(t, errx.Is(err, errx.SystemError))
	favoriteModel.AssertExpectations(t)
}

func TestUnfavoriteLogic_Unfavorite_InvalidParam(t *testing.T) {
	logic := NewUnfavoriteLogic(context.Background(), &svc.ServiceContext{})

	cases := []*pb.UnfavoriteReq{
		{UserId: 0, PostId: 100},
		{UserId: 1, PostId: 0},
		{UserId: -1, PostId: 100},
	}
	for _, req := range cases {
		_, err := logic.Unfavorite(req)
		require.Error(t, err)
		assert.True(t, errx.Is(err, errx.ParamError))
	}
}
