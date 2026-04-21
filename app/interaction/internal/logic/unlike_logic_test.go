package logic

import (
	"context"
	"testing"

	"esx/app/interaction/internal/model"
	"esx/app/interaction/internal/svc"
	"esx/app/interaction/pb/xiaobaihe/interaction/pb"
	"errx"

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
		On("Update", mock.Anything, mock.MatchedBy(func(data *model.LikeRecord) bool {
			return data.Id == 1 && data.Status == 0
		})).
		Return(nil).
		Once()
	countModel.
		On("FindOneByTarget", mock.Anything, int64(100), int64(1)).
		Return(&model.ActionCount{Id: 10, TargetId: 100, TargetType: 1, LikeCount: 5}, nil).
		Once()
	countModel.
		On("Update", mock.Anything, mock.MatchedBy(func(data *model.ActionCount) bool {
			return data.Id == 10 && data.LikeCount == 4
		})).
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
		Return(&model.LikeRecord{Id: 1, UserId: 1, TargetId: 100, TargetType: 1, Status: 0}, nil).
		Once()

	logic := NewUnlikeLogic(context.Background(), svcCtx)
	_, err := logic.Unlike(&pb.UnlikeReq{UserId: 1, TargetId: 100, TargetType: 1})
	require.Error(t, err)
	assert.True(t, errx.Is(err, errx.NotLikedYet))
	likeModel.AssertExpectations(t)
}

func TestUnlikeLogic_DecrLikeCount_WritesCache(t *testing.T) {
	countModel := new(mockActionCountModel)
	redisStore := new(mockRedisStore)
	svcCtx := &svc.ServiceContext{
		ActionCountModel: countModel,
		RedisStore:       redisStore,
	}

	countModel.
		On("FindOneByTarget", mock.Anything, int64(100), int64(1)).
		Return(&model.ActionCount{Id: 10, TargetId: 100, TargetType: 1, LikeCount: 2}, nil).
		Once()
	countModel.
		On("Update", mock.Anything, mock.MatchedBy(func(data *model.ActionCount) bool {
			return data.Id == 10 && data.LikeCount == 1
		})).
		Return(nil).
		Once()
	redisStore.On("Hset", "action_count:100:1", "like_count", "1").Return(nil).Once()
	redisStore.On("Hset", "action_count:100:1", "favorite_count", "0").Return(nil).Once()
	redisStore.On("Hset", "action_count:100:1", "comment_count", "0").Return(nil).Once()
	redisStore.On("Hset", "action_count:100:1", "share_count", "0").Return(nil).Once()
	redisStore.On("Expire", "action_count:100:1", 300).Return(nil).Once()

	logic := NewUnlikeLogic(context.Background(), svcCtx)
	require.NoError(t, logic.decrLikeCount(100, 1))
	countModel.AssertExpectations(t)
	redisStore.AssertExpectations(t)
}
