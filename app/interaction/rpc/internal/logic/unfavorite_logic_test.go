package logic

import (
	"context"
	model2 "esx/app/interaction/rpc/internal/model"
	"esx/app/interaction/rpc/internal/svc"
	pb "esx/kitex_gen/interaction"
	"testing"

	"esx/pkg/errx"
	"esx/pkg/event"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestUnfavoriteLogic_Unfavorite_Success(t *testing.T) {
	favoriteModel := new(mockFavoriteModel)
	svcCtx := &svc.ServiceContext{
		FavoriteModel: favoriteModel,
	}

	favoriteModel.
		On("FindOneByUserIdPostId", mock.Anything, int64(1), int64(100)).
		Return(&model2.Favorite{Id: 1, UserId: 1, PostId: 100, Status: 1}, nil).
		Once()
	favoriteModel.
		On("UpdateStatusById", mock.Anything, int64(1), int64(model2.StatusActive), int64(model2.StatusInactive)).
		Return(stubResult{rowsAffected: 1}, nil).
		Once()

	commands := fakeInteractionCommandsFor(svcCtx)
	svcCtx.InteractionCommands = commands

	logic := NewUnfavoriteLogic(context.Background(), svcCtx)
	resp, err := logic.Unfavorite(&pb.UnfavoriteReq{UserId: 1, PostId: 100})
	require.NoError(t, err)
	require.NotNil(t, resp)
	favoriteModel.AssertExpectations(t)
	// 事件携带本次变化后的绝对计数与序号。
	assert.Equal(t, event.InteractionCountSnapshot{LikeCount: 0, FavoriteCount: 0, Seq: 1}, commands.lastSnapshot(t))
}

func TestUnfavoriteLogic_Unfavorite_NotFavorited(t *testing.T) {
	favoriteModel := new(mockFavoriteModel)
	svcCtx := &svc.ServiceContext{
		FavoriteModel: favoriteModel,
	}

	favoriteModel.
		On("FindOneByUserIdPostId", mock.Anything, int64(1), int64(100)).
		Return((*model2.Favorite)(nil), model2.ErrNotFound).
		Once()

	svcCtx.InteractionCommands = fakeInteractionCommandsFor(svcCtx)

	logic := NewUnfavoriteLogic(context.Background(), svcCtx)
	resp, err := logic.Unfavorite(&pb.UnfavoriteReq{UserId: 1, PostId: 100})
	require.NoError(t, err)
	require.NotNil(t, resp)
	favoriteModel.AssertExpectations(t)
}

func TestUnfavoriteLogic_Unfavorite_AlreadyUnfavorited(t *testing.T) {
	favoriteModel := new(mockFavoriteModel)
	svcCtx := &svc.ServiceContext{
		FavoriteModel: favoriteModel,
	}

	favoriteModel.
		On("FindOneByUserIdPostId", mock.Anything, int64(1), int64(100)).
		Return(&model2.Favorite{Id: 1, UserId: 1, PostId: 100, Status: model2.StatusInactive}, nil).
		Once()

	svcCtx.InteractionCommands = fakeInteractionCommandsFor(svcCtx)

	logic := NewUnfavoriteLogic(context.Background(), svcCtx)
	resp, err := logic.Unfavorite(&pb.UnfavoriteReq{UserId: 1, PostId: 100})
	require.NoError(t, err)
	require.NotNil(t, resp)
	favoriteModel.AssertExpectations(t)
}

func TestUnfavoriteLogic_Unfavorite_DecrCountError(t *testing.T) {
	favoriteModel := new(mockFavoriteModel)
	svcCtx := &svc.ServiceContext{
		FavoriteModel: favoriteModel,
	}

	favoriteModel.
		On("FindOneByUserIdPostId", mock.Anything, int64(1), int64(100)).
		Return(&model2.Favorite{Id: 1, UserId: 1, PostId: 100, Status: 1}, nil).
		Once()
	favoriteModel.
		On("UpdateStatusById", mock.Anything, int64(1), int64(model2.StatusActive), int64(model2.StatusInactive)).
		Return(stubResult{rowsAffected: 1}, nil).
		Once()

	commands := fakeInteractionCommandsFor(svcCtx)
	svcCtx.InteractionCommands = commands
	commands.countErr = assert.AnError

	logic := NewUnfavoriteLogic(context.Background(), svcCtx)
	resp, err := logic.Unfavorite(&pb.UnfavoriteReq{UserId: 1, PostId: 100})
	require.Error(t, err)
	require.Nil(t, resp)
	assert.True(t, errx.Is(err, errx.SystemError))
	favoriteModel.AssertExpectations(t)
	assert.Empty(t, commands.events, "failed count write must not emit an event")
}

func TestUnfavoriteLogic_Unfavorite_UpdateStatusError(t *testing.T) {
	favoriteModel := new(mockFavoriteModel)
	svcCtx := &svc.ServiceContext{
		FavoriteModel: favoriteModel,
	}

	favoriteModel.
		On("FindOneByUserIdPostId", mock.Anything, int64(1), int64(100)).
		Return(&model2.Favorite{Id: 1, UserId: 1, PostId: 100, Status: 1}, nil).
		Once()
	favoriteModel.
		On("UpdateStatusById", mock.Anything, int64(1), int64(model2.StatusActive), int64(model2.StatusInactive)).
		Return(nil, assert.AnError).
		Once()

	svcCtx.InteractionCommands = fakeInteractionCommandsFor(svcCtx)

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
