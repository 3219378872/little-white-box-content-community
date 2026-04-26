package logic

import (
	"context"
	"database/sql"

	"github.com/stretchr/testify/mock"
	"github.com/zeromicro/go-zero/core/stores/sqlx"

	"esx/app/content/internal/model"
	"esx/app/content/internal/svc"
	"util"
)

func init() {
	// 单元测试初始化雪花算法（worker=1, datacenter=1）
	_ = util.InitSnowflake(1, 1)
}

// mockSQLResult 实现 sql.Result 接口
type mockSQLResult struct{}

func (mockSQLResult) LastInsertId() (int64, error) { return 1, nil }
func (mockSQLResult) RowsAffected() (int64, error) { return 1, nil }

// ─── MockPostModel ────────────────────────────────────────────────────────────

type MockPostModel struct {
	mock.Mock
}

func (m *MockPostModel) Insert(ctx context.Context, data *model.Post) (sql.Result, error) {
	args := m.Called(ctx, data)
	return mockSQLResult{}, args.Error(0)
}

func (m *MockPostModel) FindOne(ctx context.Context, id int64) (*model.Post, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.Post), args.Error(1)
}

func (m *MockPostModel) FindPostById(ctx context.Context, id int64) (*model.Post, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.Post), args.Error(1)
}

func (m *MockPostModel) InsertPost(ctx context.Context, data *model.Post) error {
	return m.Called(ctx, data).Error(0)
}

func (m *MockPostModel) InsertPostTx(ctx context.Context, tx *sql.Tx, data *model.Post) error {
	return m.Called(ctx, tx, data).Error(0)
}

func (m *MockPostModel) Update(ctx context.Context, data *model.Post) error {
	return m.Called(ctx, data).Error(0)
}

func (m *MockPostModel) Delete(ctx context.Context, id int64) error {
	return m.Called(ctx, id).Error(0)
}

func (m *MockPostModel) FindByAuthorId(ctx context.Context, authorId int64, page, pageSize, sortBy int) ([]*model.Post, int64, error) {
	args := m.Called(ctx, authorId, page, pageSize, sortBy)
	return args.Get(0).([]*model.Post), args.Get(1).(int64), args.Error(2)
}

func (m *MockPostModel) FindList(ctx context.Context, page, pageSize, sortBy int) ([]*model.Post, int64, error) {
	args := m.Called(ctx, page, pageSize, sortBy)
	return args.Get(0).([]*model.Post), args.Get(1).(int64), args.Error(2)
}

func (m *MockPostModel) FindByIds(ctx context.Context, ids []int64) ([]*model.Post, error) {
	args := m.Called(ctx, ids)
	return args.Get(0).([]*model.Post), args.Error(1)
}

func (m *MockPostModel) UpdateStatus(ctx context.Context, id, status int64) error {
	return m.Called(ctx, id, status).Error(0)
}

func (m *MockPostModel) UpdateFields(ctx context.Context, id int64, fields map[string]interface{}) error {
	return m.Called(ctx, id, fields).Error(0)
}

func (m *MockPostModel) IncrCommentCount(ctx context.Context, postId int64) error {
	return m.Called(ctx, postId).Error(0)
}

func (m *MockPostModel) DecrCommentCount(ctx context.Context, postId int64) error {
	return m.Called(ctx, postId).Error(0)
}

// ─── MockCommentModel ─────────────────────────────────────────────────────────

type MockCommentModel struct {
	mock.Mock
}

func (m *MockCommentModel) Insert(ctx context.Context, data *model.Comment) (sql.Result, error) {
	args := m.Called(ctx, data)
	return mockSQLResult{}, args.Error(0)
}

func (m *MockCommentModel) FindOne(ctx context.Context, id int64) (*model.Comment, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.Comment), args.Error(1)
}

func (m *MockCommentModel) FindCommentById(ctx context.Context, id int64) (*model.Comment, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.Comment), args.Error(1)
}

func (m *MockCommentModel) InsertComment(ctx context.Context, data *model.Comment) error {
	return m.Called(ctx, data).Error(0)
}

func (m *MockCommentModel) Update(ctx context.Context, data *model.Comment) error {
	return m.Called(ctx, data).Error(0)
}

func (m *MockCommentModel) Delete(ctx context.Context, id int64) error {
	return m.Called(ctx, id).Error(0)
}

func (m *MockCommentModel) FindByPostId(ctx context.Context, postId int64, page, pageSize, sortBy int) ([]*model.Comment, int64, error) {
	args := m.Called(ctx, postId, page, pageSize, sortBy)
	return args.Get(0).([]*model.Comment), args.Get(1).(int64), args.Error(2)
}

func (m *MockCommentModel) UpdateStatus(ctx context.Context, id, status int64) error {
	return m.Called(ctx, id, status).Error(0)
}

// ─── MockTagModel ─────────────────────────────────────────────────────────────

type MockTagModel struct {
	mock.Mock
}

func (m *MockTagModel) Insert(ctx context.Context, data *model.Tag) (sql.Result, error) {
	args := m.Called(ctx, data)
	return mockSQLResult{}, args.Error(0)
}

func (m *MockTagModel) FindOne(ctx context.Context, id int64) (*model.Tag, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.Tag), args.Error(1)
}

func (m *MockTagModel) FindOneByName(ctx context.Context, name string) (*model.Tag, error) {
	args := m.Called(ctx, name)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.Tag), args.Error(1)
}

func (m *MockTagModel) Update(ctx context.Context, data *model.Tag) error {
	return m.Called(ctx, data).Error(0)
}

func (m *MockTagModel) Delete(ctx context.Context, id int64) error {
	return m.Called(ctx, id).Error(0)
}

func (m *MockTagModel) FindList(ctx context.Context, limit int) ([]*model.Tag, error) {
	args := m.Called(ctx, limit)
	return args.Get(0).([]*model.Tag), args.Error(1)
}

// ─── MockPostTagModel ─────────────────────────────────────────────────────────

type MockPostTagModel struct {
	mock.Mock
}

func (m *MockPostTagModel) Insert(ctx context.Context, data *model.PostTag) (sql.Result, error) {
	args := m.Called(ctx, data)
	return mockSQLResult{}, args.Error(0)
}

func (m *MockPostTagModel) FindOne(ctx context.Context, id int64) (*model.PostTag, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.PostTag), args.Error(1)
}

func (m *MockPostTagModel) FindOneByPostIdTagName(ctx context.Context, postId int64, tagName string) (*model.PostTag, error) {
	args := m.Called(ctx, postId, tagName)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model.PostTag), args.Error(1)
}

func (m *MockPostTagModel) Update(ctx context.Context, data *model.PostTag) error {
	return m.Called(ctx, data).Error(0)
}

func (m *MockPostTagModel) Delete(ctx context.Context, id int64) error {
	return m.Called(ctx, id).Error(0)
}

func (m *MockPostTagModel) FindTagNamesByPostId(ctx context.Context, postId int64) ([]string, error) {
	args := m.Called(ctx, postId)
	return args.Get(0).([]string), args.Error(1)
}

func (m *MockPostTagModel) FindTagNamesByPostIds(ctx context.Context, postIds []int64) (map[int64][]string, error) {
	args := m.Called(ctx, postIds)
	if args.Get(0) == nil {
		return map[int64][]string{}, args.Error(1)
	}
	return args.Get(0).(map[int64][]string), args.Error(1)
}

func (m *MockPostTagModel) FindPostIdsByTagName(ctx context.Context, tagName string, page, pageSize int) ([]int64, int64, error) {
	args := m.Called(ctx, tagName, page, pageSize)
	return args.Get(0).([]int64), args.Get(1).(int64), args.Error(2)
}

func (m *MockPostTagModel) DeleteByPostId(ctx context.Context, postId int64) error {
	return m.Called(ctx, postId).Error(0)
}

func (m *MockPostTagModel) TransactReplaceTagsByPostId(ctx context.Context, conn sqlx.SqlConn, postId int64, tags []string, ids []int64) error {
	return m.Called(ctx, conn, postId, tags, ids).Error(0)
}

func (m *MockPostTagModel) BatchInsertTagsByPostId(ctx context.Context, conn sqlx.SqlConn, postId int64, tags []string, ids []int64) error {
	return m.Called(ctx, conn, postId, tags, ids).Error(0)
}

func (m *MockPostTagModel) BatchInsertTagsByPostIdTx(ctx context.Context, tx *sql.Tx, postId int64, tags []string, ids []int64) error {
	return m.Called(ctx, tx, postId, tags, ids).Error(0)
}

// ─── 辅助构造 ─────────────────────────────────────────────────────────────────

func newUnitSvcCtx(pm model.PostModel, cm model.CommentModel, tm model.TagModel, ptm model.PostTagModel) *svc.ServiceContext {
	return &svc.ServiceContext{
		PostModel:    pm,
		CommentModel: cm,
		TagModel:     tm,
		PostTagModel: ptm,
	}
}
