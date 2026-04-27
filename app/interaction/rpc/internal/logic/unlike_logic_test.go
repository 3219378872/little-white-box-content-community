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

func TestUnlikeLogic_Unlike_Success(t *testing.T) {
	likeModel := new(mockLikeRecordModel)
	countModel := new(mockActionCountModel)
	svcCtx := &svc.ServiceContext{
		LikeRecordModel:  likeModel,
		ActionCountModel: countModel,
	}

	likeModel.
		On("FindOneByUserIdTargetIdTargetType", mock.Anything, int64(1), int64(100), int64(1)).
		Return(&model.LikeRecord{Id: 1, UserId: 1, TargetId: 100, TargetType: 1, Status: 1}, nil).
		Once()
	likeModel.
		On("UpdateStatusById", mock.Anything, int64(1), int64(model.StatusActive), int64(model.StatusInactive)).
		Return(stubResult{rowsAffected: 1}, nil).
		Once()
	countModel.
		On("DecrLikeCount", mock.Anything, int64(100), int64(1)).
		Return(nil).
		Once()

	logic := NewUnlikeLogic(context.Background(), svcCtx)
	resp, err := logic.Unlike(&pb.UnlikeReq{UserId: 1, TargetId: 100, TargetType: 1})
	require.NoError(t, err)
	require.NotNil(t, resp)
	likeModel.AssertExpectations(t)
	countModel.AssertExpectations(t)
}

func TestUnlikeLogic_Unlike_NotLiked(t *testing.T) {
	likeModel := new(mockLikeRecordModel)
	svcCtx := &svc.ServiceContext{
		LikeRecordModel: likeModel,
	}

	likeModel.
		On("FindOneByUserIdTargetIdTargetType", mock.Anything, int64(1), int64(100), int64(1)).
		Return((*model.LikeRecord)(nil), model.ErrNotFound).
		Once()

	logic := NewUnlikeLogic(context.Background(), svcCtx)
	_, err := logic.Unlike(&pb.UnlikeReq{UserId: 1, TargetId: 100, TargetType: 1})
	require.Error(t, err)
	assert.True(t, errx.Is(err, errx.NotLikedYet))
	likeModel.AssertExpectations(t)
}

func TestUnlikeLogic_Unlike_AlreadyUnliked(t *testing.T) {
	likeModel := new(mockLikeRecordModel)
	svcCtx := &svc.ServiceContext{
		LikeRecordModel: likeModel,
	}

	likeModel.
		On("FindOneByUserIdTargetIdTargetType", mock.Anything, int64(1), int64(100), int64(1)).
		Return(&model.LikeRecord{Id: 1, UserId: 1, TargetId: 100, TargetType: 1, Status: model.StatusInactive}, nil).
		Once()

	logic := NewUnlikeLogic(context.Background(), svcCtx)
	_, err := logic.Unlike(&pb.UnlikeReq{UserId: 1, TargetId: 100, TargetType: 1})
	require.Error(t, err)
	assert.True(t, errx.Is(err, errx.NotLikedYet))
	likeModel.AssertExpectations(t)
}

func TestUnlikeLogic_Unlike_NilActionCountModel(t *testing.T) {
	likeModel := new(mockLikeRecordModel)
	svcCtx := &svc.ServiceContext{
		LikeRecordModel: likeModel,
	}

	likeModel.
		On("FindOneByUserIdTargetIdTargetType", mock.Anything, int64(1), int64(100), int64(1)).
		Return(&model.LikeRecord{Id: 1, UserId: 1, TargetId: 100, TargetType: 1, Status: 1}, nil).
		Once()
	likeModel.
		On("UpdateStatusById", mock.Anything, int64(1), int64(model.StatusActive), int64(model.StatusInactive)).
		Return(stubResult{rowsAffected: 1}, nil).
		Once()

	logic := NewUnlikeLogic(context.Background(), svcCtx)
	resp, err := logic.Unlike(&pb.UnlikeReq{UserId: 1, TargetId: 100, TargetType: 1})
	require.NoError(t, err)
	require.NotNil(t, resp)
	likeModel.AssertExpectations(t)
}

func TestUnlikeLogic_Unlike_DecrCountError(t *testing.T) {
	likeModel := new(mockLikeRecordModel)
	countModel := new(mockActionCountModel)
	svcCtx := &svc.ServiceContext{
		LikeRecordModel:  likeModel,
		ActionCountModel: countModel,
	}

	likeModel.
		On("FindOneByUserIdTargetIdTargetType", mock.Anything, int64(1), int64(100), int64(1)).
		Return(&model.LikeRecord{Id: 1, UserId: 1, TargetId: 100, TargetType: 1, Status: 1}, nil).
		Once()
	likeModel.
		On("UpdateStatusById", mock.Anything, int64(1), int64(model.StatusActive), int64(model.StatusInactive)).
		Return(stubResult{rowsAffected: 1}, nil).
		Once()
	countModel.
		On("DecrLikeCount", mock.Anything, int64(100), int64(1)).
		Return(assert.AnError).
		Once()

	logic := NewUnlikeLogic(context.Background(), svcCtx)
	resp, err := logic.Unlike(&pb.UnlikeReq{UserId: 1, TargetId: 100, TargetType: 1})
	require.NoError(t, err)
	require.NotNil(t, resp)
	likeModel.AssertExpectations(t)
	countModel.AssertExpectations(t)
}

func TestUnlikeLogic_Unlike_UpdateStatusError(t *testing.T) {
	likeModel := new(mockLikeRecordModel)
	svcCtx := &svc.ServiceContext{
		LikeRecordModel: likeModel,
	}

	likeModel.
		On("FindOneByUserIdTargetIdTargetType", mock.Anything, int64(1), int64(100), int64(1)).
		Return(&model.LikeRecord{Id: 1, UserId: 1, TargetId: 100, TargetType: 1, Status: 1}, nil).
		Once()
	likeModel.
		On("UpdateStatusById", mock.Anything, int64(1), int64(model.StatusActive), int64(model.StatusInactive)).
		Return(nil, assert.AnError).
		Once()

	logic := NewUnlikeLogic(context.Background(), svcCtx)
	_, err := logic.Unlike(&pb.UnlikeReq{UserId: 1, TargetId: 100, TargetType: 1})
	require.Error(t, err)
	assert.True(t, errx.Is(err, errx.SystemError))
	likeModel.AssertExpectations(t)
}

func TestUnlikeLogic_Unlike_InvalidParam(t *testing.T) {
	logic := NewUnlikeLogic(context.Background(), &svc.ServiceContext{})

	cases := []*pb.UnlikeReq{
		{UserId: 0, TargetId: 100, TargetType: 1},
		{UserId: 1, TargetId: 0, TargetType: 1},
		{UserId: -1, TargetId: 100, TargetType: 1},
	}
	for _, req := range cases {
		_, err := logic.Unlike(req)
		require.Error(t, err)
		assert.True(t, errx.Is(err, errx.ParamError))
	}
}
