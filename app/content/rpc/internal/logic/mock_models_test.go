package logic

import (
	"context"
	"database/sql"
	model2 "esx/app/content/rpc/internal/model"
	"esx/app/content/rpc/internal/svc"
	"esx/pkg/idempotencyx"
	"esx/pkg/outboxx"

	"github.com/stretchr/testify/mock"
	"github.com/zeromicro/go-zero/core/stores/sqlx"

	"esx/pkg/util"
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

func (m *MockPostModel) Insert(ctx context.Context, data *model2.Post) (sql.Result, error) {
	args := m.Called(ctx, data)
	return mockSQLResult{}, args.Error(0)
}

func (m *MockPostModel) FindOne(ctx context.Context, id int64) (*model2.Post, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model2.Post), args.Error(1)
}

func (m *MockPostModel) FindPostById(ctx context.Context, id int64) (*model2.Post, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model2.Post), args.Error(1)
}

func (m *MockPostModel) InsertPost(ctx context.Context, data *model2.Post) error {
	return m.Called(ctx, data).Error(0)
}

func (m *MockPostModel) InsertPostTx(ctx context.Context, tx *sql.Tx, data *model2.Post) error {
	return m.Called(ctx, tx, data).Error(0)
}

func (m *MockPostModel) Update(ctx context.Context, data *model2.Post) error {
	return m.Called(ctx, data).Error(0)
}

func (m *MockPostModel) Delete(ctx context.Context, id int64) error {
	return m.Called(ctx, id).Error(0)
}

func (m *MockPostModel) FindUserPostsByCursor(ctx context.Context, authorId int64, cur *model2.PostListCursor, pageSize int) ([]*model2.Post, bool, error) {
	args := m.Called(ctx, authorId, cur, pageSize)
	return args.Get(0).([]*model2.Post), args.Bool(1), args.Error(2)
}

func (m *MockPostModel) FindListByCursor(ctx context.Context, cur *model2.PostListCursor, pageSize int) ([]*model2.Post, bool, error) {
	args := m.Called(ctx, cur, pageSize)
	return args.Get(0).([]*model2.Post), args.Bool(1), args.Error(2)
}

func (m *MockPostModel) FindByIds(ctx context.Context, ids []int64) ([]*model2.Post, error) {
	args := m.Called(ctx, ids)
	return args.Get(0).([]*model2.Post), args.Error(1)
}

func (m *MockPostModel) UpdateStatus(ctx context.Context, id, status int64) error {
	return m.Called(ctx, id, status).Error(0)
}

func (m *MockPostModel) UpdateFields(ctx context.Context, id int64, fields map[string]any) error {
	return m.Called(ctx, id, fields).Error(0)
}

func (m *MockPostModel) InvalidatePostCache(context.Context, int64) error {
	return nil
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

func (m *MockCommentModel) Insert(ctx context.Context, data *model2.Comment) (sql.Result, error) {
	args := m.Called(ctx, data)
	return mockSQLResult{}, args.Error(0)
}

func (m *MockCommentModel) FindOne(ctx context.Context, id int64) (*model2.Comment, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model2.Comment), args.Error(1)
}

func (m *MockCommentModel) FindCommentById(ctx context.Context, id int64) (*model2.Comment, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model2.Comment), args.Error(1)
}

func (m *MockCommentModel) InsertComment(ctx context.Context, data *model2.Comment) error {
	return m.Called(ctx, data).Error(0)
}

func (m *MockCommentModel) Update(ctx context.Context, data *model2.Comment) error {
	return m.Called(ctx, data).Error(0)
}

func (m *MockCommentModel) Delete(ctx context.Context, id int64) error {
	return m.Called(ctx, id).Error(0)
}

func (m *MockCommentModel) FindByPostId(ctx context.Context, postId int64, page, pageSize, sortBy int) ([]*model2.Comment, int64, error) {
	args := m.Called(ctx, postId, page, pageSize, sortBy)
	return args.Get(0).([]*model2.Comment), args.Get(1).(int64), args.Error(2)
}

func (m *MockCommentModel) FindByParentId(ctx context.Context, parentId int64, page, pageSize int) ([]*model2.Comment, int64, error) {
	args := m.Called(ctx, parentId, page, pageSize)
	return args.Get(0).([]*model2.Comment), args.Get(1).(int64), args.Error(2)
}

func (m *MockCommentModel) FindByParentIds(ctx context.Context, postId int64, parentIds []int64) ([]*model2.Comment, error) {
	args := m.Called(ctx, postId, parentIds)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*model2.Comment), args.Error(1)
}

func (m *MockCommentModel) FindActiveByIds(ctx context.Context, postID int64, ids []int64) ([]*model2.Comment, error) {
	args := m.Called(ctx, postID, ids)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).([]*model2.Comment), args.Error(1)
}

func (m *MockCommentModel) UpdateStatus(ctx context.Context, id, status int64) error {
	return m.Called(ctx, id, status).Error(0)
}

func (m *MockCommentModel) InvalidateCommentCache(context.Context, int64) error {
	return nil
}

// ─── MockTagModel ─────────────────────────────────────────────────────────────

type MockTagModel struct {
	mock.Mock
}

func (m *MockTagModel) Insert(ctx context.Context, data *model2.Tag) (sql.Result, error) {
	args := m.Called(ctx, data)
	return mockSQLResult{}, args.Error(0)
}

func (m *MockTagModel) FindOne(ctx context.Context, id int64) (*model2.Tag, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model2.Tag), args.Error(1)
}

func (m *MockTagModel) FindOneByName(ctx context.Context, name string) (*model2.Tag, error) {
	args := m.Called(ctx, name)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model2.Tag), args.Error(1)
}

func (m *MockTagModel) Update(ctx context.Context, data *model2.Tag) error {
	return m.Called(ctx, data).Error(0)
}

func (m *MockTagModel) Delete(ctx context.Context, id int64) error {
	return m.Called(ctx, id).Error(0)
}

func (m *MockTagModel) FindList(ctx context.Context, limit int) ([]*model2.Tag, error) {
	args := m.Called(ctx, limit)
	return args.Get(0).([]*model2.Tag), args.Error(1)
}

// ─── MockPostTagModel ─────────────────────────────────────────────────────────

type MockPostTagModel struct {
	mock.Mock
}

func (m *MockPostTagModel) Insert(ctx context.Context, data *model2.PostTag) (sql.Result, error) {
	args := m.Called(ctx, data)
	return mockSQLResult{}, args.Error(0)
}

func (m *MockPostTagModel) FindOne(ctx context.Context, id int64) (*model2.PostTag, error) {
	args := m.Called(ctx, id)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model2.PostTag), args.Error(1)
}

func (m *MockPostTagModel) FindOneByPostIdTagName(ctx context.Context, postId int64, tagName string) (*model2.PostTag, error) {
	args := m.Called(ctx, postId, tagName)
	if args.Get(0) == nil {
		return nil, args.Error(1)
	}
	return args.Get(0).(*model2.PostTag), args.Error(1)
}

func (m *MockPostTagModel) Update(ctx context.Context, data *model2.PostTag) error {
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

func newUnitSvcCtx(pm model2.PostModel, cm model2.CommentModel, tm model2.TagModel, ptm model2.PostTagModel) *svc.ServiceContext {
	return &svc.ServiceContext{
		PostModel:           pm,
		CommentModel:        cm,
		TagModel:            tm,
		PostTagModel:        ptm,
		PostCommandModel:    legacyPostCommandModel{post: pm, tags: ptm},
		CommentCommandModel: legacyCommentCommandModel{post: pm, comments: cm},
	}
}

type legacyCommentCommandModel struct {
	post     model2.PostModel
	comments model2.CommentModel
}

func (m legacyCommentCommandModel) CreateComment(
	ctx context.Context,
	comment *model2.Comment,
	_ outboxx.Event,
	_ idempotencyx.IdempotencyRecord,
) (int64, bool, error) {
	if err := m.comments.InsertComment(ctx, comment); err != nil {
		return 0, false, err
	}
	if err := m.post.IncrCommentCount(ctx, comment.PostId); err != nil {
		return 0, false, err
	}
	return comment.Id, true, nil
}

func (m legacyCommentCommandModel) DeleteComment(ctx context.Context, comment *model2.Comment) error {
	if err := m.comments.UpdateStatus(ctx, comment.Id, 0); err != nil {
		return err
	}
	return m.post.DecrCommentCount(ctx, comment.PostId)
}

type legacyPostCommandModel struct {
	post model2.PostModel
	tags model2.PostTagModel
}

func (m legacyPostCommandModel) CreatePost(
	ctx context.Context,
	post *model2.Post,
	tags []string,
	tagIDs []int64,
	_ outboxx.Event,
	_ idempotencyx.IdempotencyRecord,
) (int64, bool, error) {
	if err := m.post.InsertPostTx(ctx, nil, post); err != nil {
		return 0, false, err
	}
	if err := m.tags.BatchInsertTagsByPostIdTx(ctx, nil, post.Id, tags, tagIDs); err != nil {
		return 0, false, err
	}
	return post.Id, true, nil
}

func (m legacyPostCommandModel) UpdatePost(
	ctx context.Context,
	postID int64,
	fields map[string]any,
	tags []string,
	tagIDs []int64,
	_ outboxx.Event,
	_ int64,
	replaceTags bool,
) error {
	if !replaceTags {
		return m.post.UpdateFields(ctx, postID, fields)
	}
	if err := m.post.UpdateFields(ctx, postID, fields); err != nil {
		return err
	}
	return m.tags.TransactReplaceTagsByPostId(ctx, nil, postID, tags, tagIDs)
}

func (m legacyPostCommandModel) DeletePost(ctx context.Context, postID int64, _ outboxx.Event, _ int64) error {
	return m.post.UpdateStatus(ctx, postID, 2)
}
