package logic

import (
	"context"
	"database/sql"
	"database/sql/driver"
	model2 "esx/app/interaction/rpc/internal/model"
	"esx/app/interaction/rpc/internal/svc"
	"esx/app/interaction/rpc/pb/xiaobaihe/interaction/pb"
	"testing"

	"errx"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"github.com/zeromicro/go-zero/core/stores/sqlx"
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

func (m *mockLikeRecordModel) Insert(ctx context.Context, data *model2.LikeRecord) (sql.Result, error) {
	args := m.Called(ctx, data)
	result, _ := args.Get(0).(sql.Result)
	return result, args.Error(1)
}

func (m *mockLikeRecordModel) FindOne(ctx context.Context, id int64) (*model2.LikeRecord, error) {
	args := m.Called(ctx, id)
	record, _ := args.Get(0).(*model2.LikeRecord)
	return record, args.Error(1)
}

func (m *mockLikeRecordModel) FindOneByUserIdTargetIdTargetType(ctx context.Context, userID int64, targetID int64, targetType int64) (*model2.LikeRecord, error) {
	args := m.Called(ctx, userID, targetID, targetType)
	record, _ := args.Get(0).(*model2.LikeRecord)
	return record, args.Error(1)
}

func (m *mockLikeRecordModel) Update(ctx context.Context, data *model2.LikeRecord) error {
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

func (m *mockLikeRecordModel) UpsertLikeStatusTx(ctx context.Context, conn sqlx.SqlConn, userId, targetId, targetType, status int64) (sql.Result, int64, error) {
	args := m.Called(ctx, conn, userId, targetId, targetType, status)
	result, _ := args.Get(0).(sql.Result)
	return result, args.Get(1).(int64), args.Error(2)
}

func (m *mockLikeRecordModel) InvalidateLikeRecordCache(ctx context.Context, id, userId, targetId, targetType int64) error {
	args := m.Called(ctx, id, userId, targetId, targetType)
	return args.Error(0)
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

func (m *mockActionCountModel) Insert(ctx context.Context, data *model2.ActionCount) (sql.Result, error) {
	args := m.Called(ctx, data)
	result, _ := args.Get(0).(sql.Result)
	return result, args.Error(1)
}

func (m *mockActionCountModel) FindOneByTarget(ctx context.Context, targetID, targetType int64) (*model2.ActionCount, error) {
	args := m.Called(ctx, targetID, targetType)
	record, _ := args.Get(0).(*model2.ActionCount)
	return record, args.Error(1)
}

func (m *mockActionCountModel) Update(ctx context.Context, data *model2.ActionCount) error {
	args := m.Called(ctx, data)
	return args.Error(0)
}

func (m *mockActionCountModel) IncrLikeCount(ctx context.Context, targetID, targetType int64) error {
	args := m.Called(ctx, targetID, targetType)
	return args.Error(0)
}

func (m *mockActionCountModel) IncrLikeCountTx(ctx context.Context, conn sqlx.SqlConn, targetID, targetType int64) error {
	args := m.Called(ctx, conn, targetID, targetType)
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

type fakeTxConn struct{}

func (fakeTxConn) Exec(query string, args ...any) (sql.Result, error) {
	return nil, assert.AnError
}

func (fakeTxConn) ExecCtx(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return nil, assert.AnError
}

func (fakeTxConn) Prepare(query string) (sqlx.StmtSession, error) {
	return nil, assert.AnError
}

func (fakeTxConn) PrepareCtx(ctx context.Context, query string) (sqlx.StmtSession, error) {
	return nil, assert.AnError
}

func (fakeTxConn) QueryRow(v any, query string, args ...any) error {
	return assert.AnError
}

func (fakeTxConn) QueryRowCtx(ctx context.Context, v any, query string, args ...any) error {
	return assert.AnError
}

func (fakeTxConn) QueryRowPartial(v any, query string, args ...any) error {
	return assert.AnError
}

func (fakeTxConn) QueryRowPartialCtx(ctx context.Context, v any, query string, args ...any) error {
	return assert.AnError
}

func (fakeTxConn) QueryRows(v any, query string, args ...any) error {
	return assert.AnError
}

func (fakeTxConn) QueryRowsCtx(ctx context.Context, v any, query string, args ...any) error {
	return assert.AnError
}

func (fakeTxConn) QueryRowsPartial(v any, query string, args ...any) error {
	return assert.AnError
}

func (fakeTxConn) QueryRowsPartialCtx(ctx context.Context, v any, query string, args ...any) error {
	return assert.AnError
}

func (fakeTxConn) RawDB() (*sql.DB, error) {
	return nil, nil
}

func (fakeTxConn) Transact(fn func(sqlx.Session) error) error {
	return fn(fakeTxSession{})
}

func (fakeTxConn) TransactCtx(ctx context.Context, fn func(context.Context, sqlx.Session) error) error {
	return fn(ctx, fakeTxSession{})
}

type fakeTxSession struct{}

func (fakeTxSession) Exec(query string, args ...any) (sql.Result, error) {
	return driver.RowsAffected(1), nil
}

func (fakeTxSession) ExecCtx(ctx context.Context, query string, args ...any) (sql.Result, error) {
	return driver.RowsAffected(1), nil
}

func (fakeTxSession) Prepare(query string) (sqlx.StmtSession, error) {
	return nil, assert.AnError
}

func (fakeTxSession) PrepareCtx(ctx context.Context, query string) (sqlx.StmtSession, error) {
	return nil, assert.AnError
}

func (fakeTxSession) QueryRow(v any, query string, args ...any) error {
	return assert.AnError
}

func (fakeTxSession) QueryRowCtx(ctx context.Context, v any, query string, args ...any) error {
	return assert.AnError
}

func (fakeTxSession) QueryRowPartial(v any, query string, args ...any) error {
	return assert.AnError
}

func (fakeTxSession) QueryRowPartialCtx(ctx context.Context, v any, query string, args ...any) error {
	return assert.AnError
}

func (fakeTxSession) QueryRows(v any, query string, args ...any) error {
	return assert.AnError
}

func (fakeTxSession) QueryRowsCtx(ctx context.Context, v any, query string, args ...any) error {
	return assert.AnError
}

func (fakeTxSession) QueryRowsPartial(v any, query string, args ...any) error {
	return assert.AnError
}

func (fakeTxSession) QueryRowsPartialCtx(ctx context.Context, v any, query string, args ...any) error {
	return assert.AnError
}

func TestLikeLogic_Like_FirstTime(t *testing.T) {
	likeModel := new(mockLikeRecordModel)
	countModel := new(mockActionCountModel)
	svcCtx := &svc.ServiceContext{
		Conn:             fakeTxConn{},
		LikeRecordModel:  likeModel,
		ActionCountModel: countModel,
	}

	likeModel.
		On("UpsertLikeStatusTx", mock.Anything, mock.Anything, int64(1), int64(100), int64(1), int64(model2.StatusActive)).
		Return(stubResult{lastInsertID: 10, rowsAffected: 1}, int64(10), nil).
		Once()
	countModel.
		On("IncrLikeCountTx", mock.Anything, mock.Anything, int64(100), int64(1)).
		Return(nil).
		Once()
	likeModel.
		On("InvalidateLikeRecordCache", mock.Anything, int64(10), int64(1), int64(100), int64(1)).
		Return(nil).
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
	countModel := new(mockActionCountModel)
	svcCtx := &svc.ServiceContext{
		Conn:             fakeTxConn{},
		LikeRecordModel:  likeModel,
		ActionCountModel: countModel,
	}

	likeModel.
		On("UpsertLikeStatusTx", mock.Anything, mock.Anything, int64(1), int64(100), int64(1), int64(model2.StatusActive)).
		Return(stubResult{lastInsertID: 10, rowsAffected: 0}, int64(10), nil).
		Once()

	logic := NewLikeLogic(context.Background(), svcCtx)
	_, err := logic.Like(&pb.LikeReq{UserId: 1, TargetId: 100, TargetType: 1})
	require.Error(t, err)
	assert.True(t, errx.Is(err, errx.AlreadyLiked))
	likeModel.AssertExpectations(t)
}

func TestLikeLogic_Like_ReviveCanceledRecord(t *testing.T) {
	likeModel := new(mockLikeRecordModel)
	countModel := new(mockActionCountModel)
	svcCtx := &svc.ServiceContext{
		Conn:             fakeTxConn{},
		LikeRecordModel:  likeModel,
		ActionCountModel: countModel,
	}

	likeModel.
		On("UpsertLikeStatusTx", mock.Anything, mock.Anything, int64(1), int64(100), int64(1), int64(model2.StatusActive)).
		Return(stubResult{lastInsertID: 10, rowsAffected: 2}, int64(10), nil).
		Once()
	countModel.
		On("IncrLikeCountTx", mock.Anything, mock.Anything, int64(100), int64(1)).
		Return(nil).
		Once()
	likeModel.
		On("InvalidateLikeRecordCache", mock.Anything, int64(10), int64(1), int64(100), int64(1)).
		Return(nil).
		Once()

	logic := NewLikeLogic(context.Background(), svcCtx)
	resp, err := logic.Like(&pb.LikeReq{UserId: 1, TargetId: 100, TargetType: 1})
	require.NoError(t, err)
	require.NotNil(t, resp)
	likeModel.AssertExpectations(t)
	countModel.AssertExpectations(t)
}

func TestLikeLogic_Like_UpsertError(t *testing.T) {
	likeModel := new(mockLikeRecordModel)
	countModel := new(mockActionCountModel)
	svcCtx := &svc.ServiceContext{
		Conn:             fakeTxConn{},
		LikeRecordModel:  likeModel,
		ActionCountModel: countModel,
	}

	likeModel.
		On("UpsertLikeStatusTx", mock.Anything, mock.Anything, int64(1), int64(100), int64(1), int64(model2.StatusActive)).
		Return(nil, int64(0), assert.AnError).
		Once()

	logic := NewLikeLogic(context.Background(), svcCtx)
	_, err := logic.Like(&pb.LikeReq{UserId: 1, TargetId: 100, TargetType: 1})
	require.Error(t, err)
	assert.True(t, errx.Is(err, errx.SystemError))
	likeModel.AssertExpectations(t)
}

func TestLikeLogic_Like_IncrCountError(t *testing.T) {
	likeModel := new(mockLikeRecordModel)
	countModel := new(mockActionCountModel)
	svcCtx := &svc.ServiceContext{
		Conn:             fakeTxConn{},
		LikeRecordModel:  likeModel,
		ActionCountModel: countModel,
	}

	likeModel.
		On("UpsertLikeStatusTx", mock.Anything, mock.Anything, int64(1), int64(100), int64(1), int64(model2.StatusActive)).
		Return(stubResult{lastInsertID: 10, rowsAffected: 1}, int64(10), nil).
		Once()
	countModel.
		On("IncrLikeCountTx", mock.Anything, mock.Anything, int64(100), int64(1)).
		Return(assert.AnError).
		Once()

	logic := NewLikeLogic(context.Background(), svcCtx)
	resp, err := logic.Like(&pb.LikeReq{UserId: 1, TargetId: 100, TargetType: 1})
	require.Nil(t, resp)
	require.Error(t, err)
	assert.True(t, errx.Is(err, errx.SystemError))
	likeModel.AssertExpectations(t)
	countModel.AssertExpectations(t)
}

func TestLikeLogic_Like_NilActionCountModel(t *testing.T) {
	likeModel := new(mockLikeRecordModel)
	svcCtx := &svc.ServiceContext{
		Conn:            fakeTxConn{},
		LikeRecordModel: likeModel,
	}

	logic := NewLikeLogic(context.Background(), svcCtx)
	resp, err := logic.Like(&pb.LikeReq{UserId: 1, TargetId: 100, TargetType: 1})
	require.Nil(t, resp)
	require.Error(t, err)
	assert.True(t, errx.Is(err, errx.SystemError))
	likeModel.AssertExpectations(t)
}

func TestLikeLogic_Like_CacheInvalidationError(t *testing.T) {
	likeModel := new(mockLikeRecordModel)
	countModel := new(mockActionCountModel)
	svcCtx := &svc.ServiceContext{
		Conn:             fakeTxConn{},
		LikeRecordModel:  likeModel,
		ActionCountModel: countModel,
	}

	likeModel.
		On("UpsertLikeStatusTx", mock.Anything, mock.Anything, int64(1), int64(100), int64(1), int64(model2.StatusActive)).
		Return(stubResult{lastInsertID: 10, rowsAffected: 1}, int64(10), nil).
		Once()
	countModel.
		On("IncrLikeCountTx", mock.Anything, mock.Anything, int64(100), int64(1)).
		Return(nil).
		Once()
	likeModel.
		On("InvalidateLikeRecordCache", mock.Anything, int64(10), int64(1), int64(100), int64(1)).
		Return(assert.AnError).
		Once()

	logic := NewLikeLogic(context.Background(), svcCtx)
	resp, err := logic.Like(&pb.LikeReq{UserId: 1, TargetId: 100, TargetType: 1})

	require.Nil(t, resp)
	require.Error(t, err)
	assert.True(t, errx.Is(err, errx.SystemError))
	likeModel.AssertExpectations(t)
	countModel.AssertExpectations(t)
}

func TestLikeLogic_Like_InvalidParam(t *testing.T) {
	logic := NewLikeLogic(context.Background(), &svc.ServiceContext{})

	cases := []*pb.LikeReq{
		{UserId: 0, TargetId: 100, TargetType: 1},
		{UserId: 1, TargetId: 0, TargetType: 1},
		{UserId: -1, TargetId: 100, TargetType: 1},
	}
	for _, req := range cases {
		_, err := logic.Like(req)
		require.Error(t, err)
		assert.True(t, errx.Is(err, errx.ParamError))
	}
}
