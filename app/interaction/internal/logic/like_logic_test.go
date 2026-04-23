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

type stubResult struct {
	lastInsertID int64
	rowsAffected int64
}

func (r stubResult) LastInsertId() (int64, error) {
	return r.lastInsertID, nil
}

func (r stubResult) RowsAffected() (int64, error) {
	return r.rowsAffected, nil
}

type mockLikeRecordModel struct {
	mock.Mock
}

func (m *mockLikeRecordModel) Insert(ctx context.Context, data *model.LikeRecord) (sql.Result, error) {
	args := m.Called(ctx, data)
	result, _ := args.Get(0).(sql.Result)
	return result, args.Error(1)
}

func (m *mockLikeRecordModel) FindOne(ctx context.Context, id int64) (*model.LikeRecord, error) {
	args := m.Called(ctx, id)
	record, _ := args.Get(0).(*model.LikeRecord)
	return record, args.Error(1)
}

func (m *mockLikeRecordModel) FindOneByUserIdTargetIdTargetType(ctx context.Context, userID int64, targetID int64, targetType int64) (*model.LikeRecord, error) {
	args := m.Called(ctx, userID, targetID, targetType)
	record, _ := args.Get(0).(*model.LikeRecord)
	return record, args.Error(1)
}

func (m *mockLikeRecordModel) Update(ctx context.Context, data *model.LikeRecord) error {
	args := m.Called(ctx, data)
	return args.Error(0)
}

func (m *mockLikeRecordModel) Delete(ctx context.Context, id int64) error {
	args := m.Called(ctx, id)
	return args.Error(0)
}

func (m *mockLikeRecordModel) UpsertLikeStatus(ctx context.Context, userId, targetId, targetType, status int64) (sql.Result, error) {
	args := m.Called(ctx, userId, targetId, targetType, status)
	result, _ := args.Get(0).(sql.Result)
	return result, args.Error(1)
}

func (m *mockLikeRecordModel) FindStatusByUserAndTargets(ctx context.Context, userId int64, targetIds []int64, targetType int64) (map[int64]bool, error) {
	args := m.Called(ctx, userId, targetIds, targetType)
	result, _ := args.Get(0).(map[int64]bool)
	return result, args.Error(1)
}

func (m *mockLikeRecordModel) UpdateStatusById(ctx context.Context, id, expectedStatus, newStatus int64) (sql.Result, error) {
	args := m.Called(ctx, id, expectedStatus, newStatus)
	result, _ := args.Get(0).(sql.Result)
	return result, args.Error(1)
}

type mockActionCountModel struct {
	mock.Mock
}

func (m *mockActionCountModel) Insert(ctx context.Context, data *model.ActionCount) (sql.Result, error) {
	args := m.Called(ctx, data)
	result, _ := args.Get(0).(sql.Result)
	return result, args.Error(1)
}

func (m *mockActionCountModel) FindOneByTarget(ctx context.Context, targetID, targetType int64) (*model.ActionCount, error) {
	args := m.Called(ctx, targetID, targetType)
	record, _ := args.Get(0).(*model.ActionCount)
	return record, args.Error(1)
}

func (m *mockActionCountModel) Update(ctx context.Context, data *model.ActionCount) error {
	args := m.Called(ctx, data)
	return args.Error(0)
}

func (m *mockActionCountModel) IncrLikeCount(ctx context.Context, targetID, targetType int64) error {
	args := m.Called(ctx, targetID, targetType)
	return args.Error(0)
}

func (m *mockActionCountModel) IncrFavoriteCount(ctx context.Context, targetID, targetType int64) error {
	args := m.Called(ctx, targetID, targetType)
	return args.Error(0)
}

func (m *mockActionCountModel) DecrLikeCount(ctx context.Context, targetID, targetType int64) error {
	args := m.Called(ctx, targetID, targetType)
	return args.Error(0)
}

func (m *mockActionCountModel) DecrFavoriteCount(ctx context.Context, targetID, targetType int64) error {
	args := m.Called(ctx, targetID, targetType)
	return args.Error(0)
}

func TestLikeLogic_Like_FirstTime(t *testing.T) {
	likeModel := new(mockLikeRecordModel)
	countModel := new(mockActionCountModel)
	svcCtx := &svc.ServiceContext{
		LikeRecordModel:  likeModel,
		ActionCountModel: countModel,
	}

	likeModel.
		On("FindOneByUserIdTargetIdTargetType", mock.Anything, int64(1), int64(100), int64(1)).
		Return((*model.LikeRecord)(nil), model.ErrNotFound).
		Once()
	likeModel.
		On("Insert", mock.Anything, mock.MatchedBy(func(data *model.LikeRecord) bool {
			return data.UserId == 1 && data.TargetId == 100 && data.TargetType == 1 && data.Status == model.StatusActive
		})).
		Return(stubResult{lastInsertID: 1, rowsAffected: 1}, nil).
		Once()
	countModel.
		On("IncrLikeCount", mock.Anything, int64(100), int64(1)).
		Return(nil).
		Once()
	countModel.
		On("FindOneByTarget", mock.Anything, int64(100), int64(1)).
		Return(&model.ActionCount{Id: 10, TargetId: 100, TargetType: 1, LikeCount: 6}, nil).
		Once()

	logic := NewLikeLogic(context.Background(), svcCtx)
	resp, err := logic.Like(&pb.LikeReq{UserId: 1, TargetId: 100, TargetType: 1})
	require.NoError(t, err)
	require.NotNil(t, resp)
	likeModel.AssertExpectations(t)
	countModel.AssertExpectations(t)
}

func TestLikeLogic_Like_AlreadyLiked(t *testing.T) {
	likeModel := new(mockLikeRecordModel)
	svcCtx := &svc.ServiceContext{
		LikeRecordModel: likeModel,
	}

	likeModel.
		On("FindOneByUserIdTargetIdTargetType", mock.Anything, int64(1), int64(100), int64(1)).
		Return(&model.LikeRecord{Id: 1, UserId: 1, TargetId: 100, TargetType: 1, Status: 1}, nil).
		Once()

	logic := NewLikeLogic(context.Background(), svcCtx)
	_, err := logic.Like(&pb.LikeReq{UserId: 1, TargetId: 100, TargetType: 1})
	require.Error(t, err)
	assert.True(t, errx.Is(err, errx.AlreadyLiked))
	likeModel.AssertExpectations(t)
}

func TestLikeLogic_Like_ReviveCanceledRecord(t *testing.T) {
	likeModel := new(mockLikeRecordModel)
	svcCtx := &svc.ServiceContext{
		LikeRecordModel: likeModel,
	}

	likeModel.
		On("FindOneByUserIdTargetIdTargetType", mock.Anything, int64(1), int64(100), int64(1)).
		Return(&model.LikeRecord{Id: 1, UserId: 1, TargetId: 100, TargetType: 1, Status: model.StatusInactive}, nil).
		Once()
	likeModel.
		On("Update", mock.Anything, mock.MatchedBy(func(data *model.LikeRecord) bool {
			return data.Id == 1 && data.Status == model.StatusActive
		})).
		Return(nil).
		Once()

	logic := NewLikeLogic(context.Background(), svcCtx)
	resp, err := logic.Like(&pb.LikeReq{UserId: 1, TargetId: 100, TargetType: 1})
	require.NoError(t, err)
	require.NotNil(t, resp)
	likeModel.AssertExpectations(t)
}

func TestLikeLogic_IncrLikeCount_InsertMissingCountWritesCache(t *testing.T) {
	countModel := new(mockActionCountModel)
	redisStore := new(mockRedisStore)
	svcCtx := &svc.ServiceContext{
		ActionCountModel: countModel,
		RedisStore:       redisStore,
	}

	countModel.
		On("IncrLikeCount", mock.Anything, int64(100), int64(1)).
		Return(nil).
		Once()
	countModel.
		On("FindOneByTarget", mock.Anything, int64(100), int64(1)).
		Return(&model.ActionCount{Id: 1, TargetId: 100, TargetType: 1, LikeCount: 1}, nil).
		Once()
	redisStore.On("Hset", "interaction:action_count:100:1", "like_count", "1").Return(nil).Once()
	redisStore.On("Hset", "interaction:action_count:100:1", "favorite_count", "0").Return(nil).Once()
	redisStore.On("Hset", "interaction:action_count:100:1", "comment_count", "0").Return(nil).Once()
	redisStore.On("Hset", "interaction:action_count:100:1", "share_count", "0").Return(nil).Once()
	redisStore.On("Expire", "interaction:action_count:100:1", model.CacheLongTTL).Return(nil).Once()

	logic := NewLikeLogic(context.Background(), svcCtx)
	require.NoError(t, logic.incrLikeCount(100, 1))
	countModel.AssertExpectations(t)
	redisStore.AssertExpectations(t)
}

func TestLikeLogic_Like_NilActionCountModel(t *testing.T) {
	likeModel := new(mockLikeRecordModel)
	svcCtx := &svc.ServiceContext{
		LikeRecordModel: likeModel,
	}

	likeModel.
		On("FindOneByUserIdTargetIdTargetType", mock.Anything, int64(1), int64(100), int64(1)).
		Return((*model.LikeRecord)(nil), model.ErrNotFound).
		Once()
	likeModel.
		On("Insert", mock.Anything, mock.Anything).
		Return(stubResult{lastInsertID: 1, rowsAffected: 1}, nil).
		Once()

	logic := NewLikeLogic(context.Background(), svcCtx)
	resp, err := logic.Like(&pb.LikeReq{UserId: 1, TargetId: 100, TargetType: 1})
	require.NoError(t, err)
	require.NotNil(t, resp)
	likeModel.AssertExpectations(t)
}

func TestLikeLogic_IncrLikeCount_FindError(t *testing.T) {
	countModel := new(mockActionCountModel)
	svcCtx := &svc.ServiceContext{
		ActionCountModel: countModel,
	}

	countModel.
		On("IncrLikeCount", mock.Anything, int64(100), int64(1)).
		Return(nil).
		Once()
	countModel.
		On("FindOneByTarget", mock.Anything, int64(100), int64(1)).
		Return((*model.ActionCount)(nil), assert.AnError).
		Once()

	logic := NewLikeLogic(context.Background(), svcCtx)
	err := logic.incrLikeCount(100, 1)
	require.Error(t, err)
	countModel.AssertExpectations(t)
}

func TestLikeLogic_SyncLikeCountCache_NoStore(t *testing.T) {
	logic := NewLikeLogic(context.Background(), &svc.ServiceContext{})
	logic.syncLikeCountCache(&model.ActionCount{TargetId: 100, TargetType: 1})
}

func TestLikeLogic_SyncLikeCountCache_HsetError(t *testing.T) {
	redisStore := new(mockRedisStore)
	svcCtx := &svc.ServiceContext{RedisStore: redisStore}

	redisStore.On("Hset", "interaction:action_count:100:1", "like_count", "5").Return(assert.AnError).Once()
	redisStore.On("Hset", "interaction:action_count:100:1", "favorite_count", "3").Return(nil).Once()
	redisStore.On("Hset", "interaction:action_count:100:1", "comment_count", "1").Return(nil).Once()
	redisStore.On("Hset", "interaction:action_count:100:1", "share_count", "0").Return(nil).Once()
	redisStore.On("Expire", "interaction:action_count:100:1", model.CacheLongTTL).Return(nil).Once()

	logic := NewLikeLogic(context.Background(), svcCtx)
	logic.syncLikeCountCache(&model.ActionCount{TargetId: 100, TargetType: 1, LikeCount: 5, FavoriteCount: 3, CommentCount: 1})
	redisStore.AssertExpectations(t)
}

func TestLikeLogic_Like_IncrCountError(t *testing.T) {
	likeModel := new(mockLikeRecordModel)
	countModel := new(mockActionCountModel)
	svcCtx := &svc.ServiceContext{
		LikeRecordModel:  likeModel,
		ActionCountModel: countModel,
	}

	likeModel.
		On("FindOneByUserIdTargetIdTargetType", mock.Anything, int64(1), int64(100), int64(1)).
		Return((*model.LikeRecord)(nil), model.ErrNotFound).
		Once()
	likeModel.
		On("Insert", mock.Anything, mock.Anything).
		Return(stubResult{lastInsertID: 1, rowsAffected: 1}, nil).
		Once()
	countModel.
		On("IncrLikeCount", mock.Anything, int64(100), int64(1)).
		Return(assert.AnError).
		Once()

	logic := NewLikeLogic(context.Background(), svcCtx)
	_, err := logic.Like(&pb.LikeReq{UserId: 1, TargetId: 100, TargetType: 1})
	require.Error(t, err)
	likeModel.AssertExpectations(t)
	countModel.AssertExpectations(t)
}
