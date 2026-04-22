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

type mockRedisStore struct {
	mock.Mock
}

func (m *mockRedisStore) Hget(key, field string) (string, error) {
	args := m.Called(key, field)
	return args.String(0), args.Error(1)
}

func (m *mockRedisStore) Hset(key, field, value string) error {
	args := m.Called(key, field, value)
	return args.Error(0)
}

func (m *mockRedisStore) Expire(key string, seconds int) error {
	args := m.Called(key, seconds)
	return args.Error(0)
}

func (m *mockRedisStore) Exists(key string) (bool, error) {
	args := m.Called(key)
	return args.Bool(0), args.Error(1)
}

func (m *mockRedisStore) Hincrby(key, field string, increment int) (int, error) {
	args := m.Called(key, field, increment)
	return args.Int(0), args.Error(1)
}

func TestGetCountsLogic_GetCounts_RedisHit(t *testing.T) {
	redisStore := new(mockRedisStore)
	redisStore.On("Hget", "interaction:action_count:100:1", "like_count").Return("10", nil).Once()
	redisStore.On("Hget", "interaction:action_count:100:1", "favorite_count").Return("5", nil).Once()
	redisStore.On("Hget", "interaction:action_count:100:1", "comment_count").Return("2", nil).Once()

	svcCtx := &svc.ServiceContext{
		RedisStore: redisStore,
	}

	logic := NewGetCountsLogic(context.Background(), svcCtx)
	resp, err := logic.GetCounts(&pb.GetCountsReq{TargetId: 100, TargetType: 1})
	require.NoError(t, err)
	assert.Equal(t, int64(10), resp.LikeCount)
	assert.Equal(t, int64(5), resp.FavoriteCount)
	assert.Equal(t, int64(2), resp.CommentCount)
	redisStore.AssertExpectations(t)
}

func TestGetCountsLogic_GetCounts_RedisMiss(t *testing.T) {
	countModel := new(mockActionCountModel)
	redisStore := new(mockRedisStore)
	redisStore.On("Hget", "interaction:action_count:100:1", "like_count").Return("", assert.AnError).Twice()
	redisStore.On("Hset", "interaction:action_count:100:1", "like_count", "7").Return(nil).Once()
	redisStore.On("Hset", "interaction:action_count:100:1", "favorite_count", "3").Return(nil).Once()
	redisStore.On("Hset", "interaction:action_count:100:1", "comment_count", "1").Return(nil).Once()
	redisStore.On("Hset", "interaction:action_count:100:1", "share_count", "0").Return(nil).Once()
	redisStore.On("Expire", "interaction:action_count:100:1", model.CacheLongTTL).Return(nil).Once()

	svcCtx := &svc.ServiceContext{
		ActionCountModel: countModel,
		RedisStore:       redisStore,
	}

	countModel.
		On("FindOneByTarget", mock.Anything, int64(100), int64(1)).
		Return(&model.ActionCount{Id: 1, TargetId: 100, TargetType: 1, LikeCount: 7, FavoriteCount: 3, CommentCount: 1}, nil).
		Once()

	logic := NewGetCountsLogic(context.Background(), svcCtx)
	resp, err := logic.GetCounts(&pb.GetCountsReq{TargetId: 100, TargetType: 1})
	require.NoError(t, err)
	assert.Equal(t, int64(7), resp.LikeCount)
	assert.Equal(t, int64(3), resp.FavoriteCount)
	assert.Equal(t, int64(1), resp.CommentCount)
	countModel.AssertExpectations(t)
	redisStore.AssertExpectations(t)
}

func TestGetCountsLogic_GetCounts_NotFound(t *testing.T) {
	countModel := new(mockActionCountModel)
	redisStore := new(mockRedisStore)
	redisStore.On("Hget", "interaction:action_count:999:1", "like_count").Return("", assert.AnError).Twice()
	redisStore.On("Hset", "interaction:action_count:999:1", "like_count", "0").Return(nil).Once()
	redisStore.On("Hset", "interaction:action_count:999:1", "favorite_count", "0").Return(nil).Once()
	redisStore.On("Hset", "interaction:action_count:999:1", "comment_count", "0").Return(nil).Once()
	redisStore.On("Hset", "interaction:action_count:999:1", "share_count", "0").Return(nil).Once()
	redisStore.On("Expire", "interaction:action_count:999:1", model.CacheShortTTL).Return(nil).Once()

	svcCtx := &svc.ServiceContext{
		ActionCountModel: countModel,
		RedisStore:       redisStore,
	}

	countModel.
		On("FindOneByTarget", mock.Anything, int64(999), int64(1)).
		Return((*model.ActionCount)(nil), model.ErrNotFound).
		Once()

	logic := NewGetCountsLogic(context.Background(), svcCtx)
	resp, err := logic.GetCounts(&pb.GetCountsReq{TargetId: 999, TargetType: 1})
	require.NoError(t, err)
	assert.Equal(t, int64(0), resp.LikeCount)
	assert.Equal(t, int64(0), resp.FavoriteCount)
	assert.Equal(t, int64(0), resp.CommentCount)
	countModel.AssertExpectations(t)
	redisStore.AssertExpectations(t)
}

func TestGetCountsLogic_ReadCountsFromCache_NoStore(t *testing.T) {
	logic := NewGetCountsLogic(context.Background(), &svc.ServiceContext{})
	resp, ok := logic.readCountsFromCache("interaction:action_count:100:1")
	assert.False(t, ok)
	assert.Nil(t, resp)
}
