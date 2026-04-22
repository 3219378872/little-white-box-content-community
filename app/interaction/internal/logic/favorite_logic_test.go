package logic

import (
	"context"
	"database/sql"
	"testing"

	"errx"
	"esx/app/interaction/internal/model"
	"esx/app/interaction/internal/svc"
	"esx/app/interaction/pb/xiaobaihe/interaction/pb"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
)

type mockFavoriteModel struct {
	mock.Mock
}

func (m *mockFavoriteModel) Insert(ctx context.Context, data *model.Favorite) (sql.Result, error) {
	args := m.Called(ctx, data)
	result, _ := args.Get(0).(sql.Result)
	return result, args.Error(1)
}

func (m *mockFavoriteModel) FindOne(ctx context.Context, id int64) (*model.Favorite, error) {
	args := m.Called(ctx, id)
	record, _ := args.Get(0).(*model.Favorite)
	return record, args.Error(1)
}

func (m *mockFavoriteModel) FindOneByUserIdPostId(ctx context.Context, userID int64, postID int64) (*model.Favorite, error) {
	args := m.Called(ctx, userID, postID)
	record, _ := args.Get(0).(*model.Favorite)
	return record, args.Error(1)
}

func (m *mockFavoriteModel) Update(ctx context.Context, data *model.Favorite) error {
	args := m.Called(ctx, data)
	return args.Error(0)
}

func (m *mockFavoriteModel) Delete(ctx context.Context, id int64) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func (m *mockFavoriteModel) FindActivePostIds(ctx context.Context, userID int64, page, pageSize int32) ([]int64, int64, error) {
	args := m.Called(ctx, userID, page, pageSize)
	postIDs, _ := args.Get(0).([]int64)
	return postIDs, args.Get(1).(int64), args.Error(2)
}

func TestFavoriteLogic_Favorite_FirstTime(t *testing.T) {
	favoriteModel := new(mockFavoriteModel)
	countModel := new(mockActionCountModel)
	svcCtx := &svc.ServiceContext{
		FavoriteModel:    favoriteModel,
		ActionCountModel: countModel,
	}

	favoriteModel.
		On("FindOneByUserIdPostId", mock.Anything, int64(1), int64(100)).
		Return((*model.Favorite)(nil), model.ErrNotFound).
		Once()
	favoriteModel.
		On("Insert", mock.Anything, mock.MatchedBy(func(data *model.Favorite) bool {
			return data.UserId == 1 && data.PostId == 100 && data.Status == model.StatusActive
		})).
		Return(stubResult{lastInsertID: 1, rowsAffected: 1}, nil).
		Once()
	countModel.
		On("IncrFavoriteCount", mock.Anything, int64(100), int64(1)).
		Return(nil).
		Once()
	countModel.
		On("FindOneByTarget", mock.Anything, int64(100), int64(1)).
		Return(&model.ActionCount{Id: 10, TargetId: 100, TargetType: 1, FavoriteCount: 4}, nil).
		Once()

	logic := NewFavoriteLogic(context.Background(), svcCtx)
	resp, err := logic.Favorite(&pb.FavoriteReq{UserId: 1, PostId: 100})
	require.NoError(t, err)
	require.NotNil(t, resp)
	favoriteModel.AssertExpectations(t)
	countModel.AssertExpectations(t)
}

func TestFavoriteLogic_Favorite_AlreadyFavorited(t *testing.T) {
	favoriteModel := new(mockFavoriteModel)
	svcCtx := &svc.ServiceContext{
		FavoriteModel: favoriteModel,
	}

	favoriteModel.
		On("FindOneByUserIdPostId", mock.Anything, int64(1), int64(100)).
		Return(&model.Favorite{Id: 1, UserId: 1, PostId: 100, Status: model.StatusActive}, nil).
		Once()

	logic := NewFavoriteLogic(context.Background(), svcCtx)
	_, err := logic.Favorite(&pb.FavoriteReq{UserId: 1, PostId: 100})
	require.Error(t, err)
	assert.True(t, errx.Is(err, errx.AlreadyFavorited))
	favoriteModel.AssertExpectations(t)
}

func TestFavoriteLogic_Favorite_ReviveCanceledRecord(t *testing.T) {
	favoriteModel := new(mockFavoriteModel)
	svcCtx := &svc.ServiceContext{
		FavoriteModel: favoriteModel,
	}

	favoriteModel.
		On("FindOneByUserIdPostId", mock.Anything, int64(1), int64(100)).
		Return(&model.Favorite{Id: 1, UserId: 1, PostId: 100, Status: model.StatusInactive}, nil).
		Once()
	favoriteModel.
		On("Update", mock.Anything, mock.MatchedBy(func(data *model.Favorite) bool {
			return data.Id == 1 && data.Status == model.StatusActive
		})).
		Return(nil).
		Once()

	logic := NewFavoriteLogic(context.Background(), svcCtx)
	resp, err := logic.Favorite(&pb.FavoriteReq{UserId: 1, PostId: 100})
	require.NoError(t, err)
	require.NotNil(t, resp)
	favoriteModel.AssertExpectations(t)
}

func TestFavoriteLogic_IncrFavoriteCount_InsertMissingCountWritesCache(t *testing.T) {
	countModel := new(mockActionCountModel)
	redisStore := new(mockRedisStore)
	svcCtx := &svc.ServiceContext{
		ActionCountModel: countModel,
		RedisStore:       redisStore,
	}

	countModel.
		On("IncrFavoriteCount", mock.Anything, int64(100), int64(1)).
		Return(nil).
		Once()
	countModel.
		On("FindOneByTarget", mock.Anything, int64(100), int64(1)).
		Return(&model.ActionCount{Id: 1, TargetId: 100, TargetType: 1, FavoriteCount: 1}, nil).
		Once()
	redisStore.On("Hset", "action_count:100:1", "like_count", "0").Return(nil).Once()
	redisStore.On("Hset", "action_count:100:1", "favorite_count", "1").Return(nil).Once()
	redisStore.On("Hset", "action_count:100:1", "comment_count", "0").Return(nil).Once()
	redisStore.On("Hset", "action_count:100:1", "share_count", "0").Return(nil).Once()
	redisStore.On("Expire", "action_count:100:1", model.CacheLongTTL).Return(nil).Once()

	logic := NewFavoriteLogic(context.Background(), svcCtx)
	require.NoError(t, logic.incrFavoriteCount(100))
	countModel.AssertExpectations(t)
	redisStore.AssertExpectations(t)
}
