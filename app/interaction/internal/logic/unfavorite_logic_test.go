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
		On("Update", mock.Anything, mock.MatchedBy(func(data *model.Favorite) bool {
			return data.Id == 1 && data.Status == model.StatusInactive
		})).
		Return(nil).
		Once()
	countModel.
		On("DecrFavoriteCount", mock.Anything, int64(100), int64(1)).
		Return(nil).
		Once()
	countModel.
		On("FindOneByTarget", mock.Anything, int64(100), int64(1)).
		Return(&model.ActionCount{Id: 10, TargetId: 100, TargetType: 1, FavoriteCount: 2}, nil).
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

func TestUnfavoriteLogic_DecrFavoriteCount_WritesCache(t *testing.T) {
	countModel := new(mockActionCountModel)
	redisStore := new(mockRedisStore)
	svcCtx := &svc.ServiceContext{
		ActionCountModel: countModel,
		RedisStore:       redisStore,
	}

	countModel.
		On("DecrFavoriteCount", mock.Anything, int64(100), int64(1)).
		Return(nil).
		Once()
	countModel.
		On("FindOneByTarget", mock.Anything, int64(100), int64(1)).
		Return(&model.ActionCount{Id: 10, TargetId: 100, TargetType: 1, FavoriteCount: 1}, nil).
		Once()
	redisStore.On("Hset", "interaction:action_count:100:1", "like_count", "0").Return(nil).Once()
	redisStore.On("Hset", "interaction:action_count:100:1", "favorite_count", "1").Return(nil).Once()
	redisStore.On("Hset", "interaction:action_count:100:1", "comment_count", "0").Return(nil).Once()
	redisStore.On("Hset", "interaction:action_count:100:1", "share_count", "0").Return(nil).Once()
	redisStore.On("Expire", "interaction:action_count:100:1", model.CacheLongTTL).Return(nil).Once()

	logic := NewUnfavoriteLogic(context.Background(), svcCtx)
	require.NoError(t, logic.decrFavoriteCount(100))
	countModel.AssertExpectations(t)
	redisStore.AssertExpectations(t)
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
		On("Update", mock.Anything, mock.Anything).
		Return(nil).
		Once()

	logic := NewUnfavoriteLogic(context.Background(), svcCtx)
	resp, err := logic.Unfavorite(&pb.UnfavoriteReq{UserId: 1, PostId: 100})
	require.NoError(t, err)
	require.NotNil(t, resp)
	favoriteModel.AssertExpectations(t)
}

func TestUnfavoriteLogic_DecrFavoriteCount_NotFound(t *testing.T) {
	countModel := new(mockActionCountModel)
	svcCtx := &svc.ServiceContext{
		ActionCountModel: countModel,
	}

	countModel.
		On("DecrFavoriteCount", mock.Anything, int64(100), int64(1)).
		Return(nil).
		Once()
	countModel.
		On("FindOneByTarget", mock.Anything, int64(100), int64(1)).
		Return((*model.ActionCount)(nil), model.ErrNotFound).
		Once()

	logic := NewUnfavoriteLogic(context.Background(), svcCtx)
	require.NoError(t, logic.decrFavoriteCount(100))
	countModel.AssertExpectations(t)
}

func TestUnfavoriteLogic_SyncFavoriteCountCache_NoStore(t *testing.T) {
	logic := NewUnfavoriteLogic(context.Background(), &svc.ServiceContext{})
	logic.syncFavoriteCountCache(&model.ActionCount{TargetId: 100, TargetType: 1})
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
		On("Update", mock.Anything, mock.Anything).
		Return(nil).
		Once()
	countModel.
		On("DecrFavoriteCount", mock.Anything, int64(100), int64(1)).
		Return(assert.AnError).
		Once()

	logic := NewUnfavoriteLogic(context.Background(), svcCtx)
	_, err := logic.Unfavorite(&pb.UnfavoriteReq{UserId: 1, PostId: 100})
	require.Error(t, err)
	favoriteModel.AssertExpectations(t)
	countModel.AssertExpectations(t)
}

func TestUnfavoriteLogic_DecrFavoriteCount_FindError(t *testing.T) {
	countModel := new(mockActionCountModel)
	svcCtx := &svc.ServiceContext{
		ActionCountModel: countModel,
	}

	countModel.
		On("DecrFavoriteCount", mock.Anything, int64(100), int64(1)).
		Return(nil).
		Once()
	countModel.
		On("FindOneByTarget", mock.Anything, int64(100), int64(1)).
		Return((*model.ActionCount)(nil), assert.AnError).
		Once()

	logic := NewUnfavoriteLogic(context.Background(), svcCtx)
	err := logic.decrFavoriteCount(100)
	require.Error(t, err)
	countModel.AssertExpectations(t)
}
