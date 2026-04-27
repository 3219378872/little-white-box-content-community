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

func TestBatchCheckLikedLogic_BatchCheckLiked_Success(t *testing.T) {
	likeModel := new(mockLikeRecordModel)
	svcCtx := &svc.ServiceContext{
		LikeRecordModel: likeModel,
	}

	likeModel.
		On("FindStatusByUserAndTargets", mock.Anything, int64(1), []int64{10, 20, 30}, int64(1)).
		Return(map[int64]bool{10: true, 20: false}, nil).
		Once()

	logic := NewBatchCheckLikedLogic(context.Background(), svcCtx)
	resp, err := logic.BatchCheckLiked(&pb.BatchCheckLikedReq{UserId: 1, TargetIds: []int64{10, 20, 30}, TargetType: 1})
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.True(t, resp.Results[10])
	assert.False(t, resp.Results[20])
	assert.False(t, resp.Results[30])
	likeModel.AssertExpectations(t)
}

func TestBatchCheckLikedLogic_BatchCheckLiked_Empty(t *testing.T) {
	logic := NewBatchCheckLikedLogic(context.Background(), &svc.ServiceContext{})
	resp, err := logic.BatchCheckLiked(&pb.BatchCheckLikedReq{UserId: 1, TargetIds: []int64{}})
	require.NoError(t, err)
	require.NotNil(t, resp)
	assert.Empty(t, resp.Results)
}

func TestBatchCheckLikedLogic_BatchCheckLiked_TooMany(t *testing.T) {
	logic := NewBatchCheckLikedLogic(context.Background(), &svc.ServiceContext{})
	ids := make([]int64, validator.MaxBatchQueryIds+1)
	_, err := logic.BatchCheckLiked(&pb.BatchCheckLikedReq{UserId: 1, TargetIds: ids})
	require.Error(t, err)
	assert.True(t, errx.Is(err, errx.ParamError))
}

func TestBatchCheckLikedLogic_BatchCheckLiked_InvalidUser(t *testing.T) {
	logic := NewBatchCheckLikedLogic(context.Background(), &svc.ServiceContext{})
	_, err := logic.BatchCheckLiked(&pb.BatchCheckLikedReq{UserId: 0, TargetIds: []int64{10}})
	require.Error(t, err)
	assert.True(t, errx.Is(err, errx.ParamError))
}

func TestBatchCheckLikedLogic_BatchCheckLiked_QueryError(t *testing.T) {
	likeModel := new(mockLikeRecordModel)
	svcCtx := &svc.ServiceContext{
		LikeRecordModel: likeModel,
	}

	likeModel.
		On("FindStatusByUserAndTargets", mock.Anything, int64(1), []int64{10}, int64(1)).
		Return(nil, assert.AnError).
		Once()

	logic := NewBatchCheckLikedLogic(context.Background(), svcCtx)
	_, err := logic.BatchCheckLiked(&pb.BatchCheckLikedReq{UserId: 1, TargetIds: []int64{10}, TargetType: 1})
	require.Error(t, err)
	assert.True(t, errx.Is(err, errx.SystemError))
	likeModel.AssertExpectations(t)
}
