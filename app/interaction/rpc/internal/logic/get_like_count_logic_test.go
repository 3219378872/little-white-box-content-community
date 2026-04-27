package logic

import (
	"context"
	"esx/app/interaction/rpc/internal/model"
	"esx/app/interaction/rpc/internal/svc"
	"esx/app/interaction/rpc/pb/xiaobaihe/interaction/pb"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

func TestGetLikeCountLogic_GetLikeCount(t *testing.T) {
	redisStore := new(mockRedisStore)
	redisStore.On("Hget", "interaction:action_count:100:1", "like_count").Return("9", nil).Once()
	redisStore.On("Hget", "interaction:action_count:100:1", "favorite_count").Return("4", nil).Once()
	redisStore.On("Hget", "interaction:action_count:100:1", "comment_count").Return("1", nil).Once()

	svcCtx := &svc.ServiceContext{
		RedisStore: redisStore,
	}

	logic := NewGetLikeCountLogic(context.Background(), svcCtx)
	resp, err := logic.GetLikeCount(&pb.GetLikeCountReq{TargetId: 100, TargetType: 1})
	require.NoError(t, err)
	assert.Equal(t, int64(9), resp.Count)
	redisStore.AssertExpectations(t)
}

func TestGetLikeCountLogic_GetLikeCount_Error(t *testing.T) {
	countModel := new(mockActionCountModel)
	countModel.On("FindOneByTarget", mock.Anything, int64(100), int64(1)).Return((*model.ActionCount)(nil), assert.AnError).Once()

	logic := NewGetLikeCountLogic(context.Background(), &svc.ServiceContext{ActionCountModel: countModel})
	_, err := logic.GetLikeCount(&pb.GetLikeCountReq{TargetId: 100, TargetType: 1})
	require.Error(t, err)
	countModel.AssertExpectations(t)
}
