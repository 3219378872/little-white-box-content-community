package model

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	"github.com/zeromicro/go-zero/core/stores/cache"
	"github.com/zeromicro/go-zero/core/stores/sqlx"
)

var _ PostModel = (*customPostModel)(nil)

// allowedUpdateCols UpdateFields 允许的列白名单，防止 SQL 注入
var allowedUpdateCols = map[string]struct{}{
	"title":   {},
	"content": {},
	"images":  {},
	"status":  {},
}

type (
	// PostModel is an interface to be customized, add more methods here,
	// and implement the added methods in customPostModel.
	PostModel interface {
		postModel
		FindPostById(ctx context.Context, id int64) (*Post, error)
		InsertPost(ctx context.Context, post *Post) error
		FindByAuthorId(ctx context.Context, authorId int64, page, pageSize, sortBy int) ([]*Post, int64, error)
		FindList(ctx context.Context, page, pageSize int, sortBy int) ([]*Post, int64, error)
		FindByIds(ctx context.Context, ids []int64) ([]*Post, error)
		UpdateStatus(ctx context.Context, id int64, status int64) error
		UpdateFields(ctx context.Context, id int64, fields map[string]interface{}) error
		IncrCommentCount(ctx context.Context, postId int64) error
		DecrCommentCount(ctx context.Context, postId int64) error
	}

	customPostModel struct {
		*defaultPostModel
	}
)

// NewPostModel returns a model for the database table.
func NewPostModel(conn sqlx.SqlConn, c cache.CacheConf, opts ...cache.Option) PostModel {
	return &customPostModel{
		defaultPostModel: newPostModel(conn, c, opts...),
	}
}

// FindPostById 按主键查询帖子（业务专用，显式 SQL）
func (m *customPostModel) FindPostById(ctx context.Context, id int64) (*Post, error) {
	postIdKey := fmt.Sprintf("%s%v", cachePostIdPrefix, id)
	var post Post
	err := m.QueryRowCtx(ctx, &post, postIdKey, func(ctx context.Context, conn sqlx.SqlConn, v interface{}) error {
		query := fmt.Sprintf("select %s from %s where `id`=? limit 1", postRows, m.table)
		return conn.QueryRowCtx(ctx, v, query, id)
	})
	switch {
	case err == nil:
		if post.Images.Valid {
			// 处理images的Json格式 []
			var images []string
			err = json.Unmarshal([]byte(post.Images.String), &images)
			if err != nil {
				return nil, err
			}
			post.Images.String = strings.Join(images, ",")
		}
		return &post, err
	case errors.Is(err, sqlx.ErrNotFound):
		return nil, ErrNotFound
	default:
		return nil, err
	}
}

// InsertPost 插入帖子（显式字段列，避免依赖 gen 生成的通用 Insert）
func (m *customPostModel) InsertPost(ctx context.Context, post *Post) error {
	postIdKey := fmt.Sprintf("%s%v", cachePostIdPrefix, post.Id)
	// 校验post中images是否为空
	if !post.Images.Valid {
		_, err := m.ExecCtx(ctx, func(ctx context.Context, conn sqlx.SqlConn) (sql.Result, error) {
			query := fmt.Sprintf("insert into %s (`id`,`author_id`,`title`,`content`,`status`) values (?,?,?,?,?)", m.table)
			return conn.ExecCtx(ctx, query, post.Id, post.AuthorId, post.Title, post.Content, post.Status)
		}, postIdKey)
		return err
	}
	_, err := m.ExecCtx(ctx, func(ctx context.Context, conn sqlx.SqlConn) (sql.Result, error) {
		query := fmt.Sprintf("insert into %s (`id`,`author_id`,`title`,`content`,`images`,`status`) values (?,?,?,?,?,?)", m.table)
		return conn.ExecCtx(ctx, query, post.Id, post.AuthorId, post.Title, post.Content, post.Images.String, post.Status)
	}, postIdKey)
	return err
}

func (m *customPostModel) FindByAuthorId(ctx context.Context, authorId int64, page, pageSize, sortBy int) ([]*Post, int64, error) {
	offset := (page - 1) * pageSize

	orderBy := "`created_at` desc"
	switch sortBy {
	case SortByHot:
		orderBy = "`like_count` desc, `created_at` desc"
	}

	var posts []*Post
	query := fmt.Sprintf("select %s from %s where `author_id` = ? and `status` = 1 order by %s limit ?,?", postRows, m.table, orderBy)
	err := m.CachedConn.QueryRowsNoCacheCtx(ctx, &posts, query, authorId, offset, pageSize)
	if err != nil {
		return nil, 0, err
	}

	var total int64
	countQuery := fmt.Sprintf("select count(*) from %s where `author_id` = ? and `status` = 1", m.table)
	err = m.CachedConn.QueryRowNoCacheCtx(ctx, &total, countQuery, authorId)
	if err != nil {
		return nil, 0, err
	}

	return posts, total, nil
}

func (m *customPostModel) FindList(ctx context.Context, page, pageSize int, sortBy int) ([]*Post, int64, error) {
	offset := (page - 1) * pageSize

	orderBy := "`created_at` desc"
	switch sortBy {
	case SortByHot:
		orderBy = "`like_count` desc"
	case SortByViewed:
		orderBy = "`view_count` desc"
	}

	var posts []*Post
	query := fmt.Sprintf("select %s from %s where `status` = 1 order by %s limit ?,?", postRows, m.table, orderBy)
	err := m.CachedConn.QueryRowsNoCacheCtx(ctx, &posts, query, offset, pageSize)
	if err != nil {
		return nil, 0, err
	}

	var total int64
	countQuery := fmt.Sprintf("select count(*) from %s where `status` = 1", m.table)
	err = m.CachedConn.QueryRowNoCacheCtx(ctx, &total, countQuery)
	if err != nil {
		return nil, 0, err
	}

	return posts, total, nil
}

func (m *customPostModel) UpdateStatus(ctx context.Context, id int64, status int64) error {
	postIdKey := fmt.Sprintf("%s%v", cachePostIdPrefix, id)
	_, err := m.ExecCtx(ctx, func(ctx context.Context, conn sqlx.SqlConn) (sql.Result, error) {
		query := fmt.Sprintf("update %s set `status` = ? where `id` = ?", m.table)
		return conn.ExecCtx(ctx, query, status, id)
	}, postIdKey)
	return err
}

// UpdateFields 动态更新帖子字段（PATCH 语义），只更新 fields 中显式传入的字段，
// 避免覆盖计数等字段（防止 Lost Update），空 fields 直接返回。
func (m *customPostModel) UpdateFields(ctx context.Context, id int64, fields map[string]interface{}) error {
	if len(fields) == 0 {
		return nil
	}
	setClauses := make([]string, 0, len(fields))
	args := make([]interface{}, 0, len(fields)+1)
	for col, val := range fields {
		if _, ok := allowedUpdateCols[col]; !ok {
			return fmt.Errorf("UpdateFields: disallowed column %q", col)
		}
		setClauses = append(setClauses, fmt.Sprintf("`%s`=?", col))
		args = append(args, val)
	}
	args = append(args, id)
	postIdKey := fmt.Sprintf("%s%v", cachePostIdPrefix, id)
	_, err := m.ExecCtx(ctx, func(ctx context.Context, conn sqlx.SqlConn) (sql.Result, error) {
		query := fmt.Sprintf("update %s set %s where `id`=?", m.table, strings.Join(setClauses, ", "))
		return conn.ExecCtx(ctx, query, args...)
	}, postIdKey)
	return err
}

// IncrCommentCount 原子递增评论数，避免并发写丢失
func (m *customPostModel) IncrCommentCount(ctx context.Context, postId int64) error {
	postIdKey := fmt.Sprintf("%s%v", cachePostIdPrefix, postId)
	_, err := m.ExecCtx(ctx, func(ctx context.Context, conn sqlx.SqlConn) (sql.Result, error) {
		query := fmt.Sprintf("update %s set `comment_count`=`comment_count`+1 where `id`=?", m.table)
		return conn.ExecCtx(ctx, query, postId)
	}, postIdKey)
	return err
}

// DecrCommentCount 原子递减评论数，不低于 0
func (m *customPostModel) DecrCommentCount(ctx context.Context, postId int64) error {
	postIdKey := fmt.Sprintf("%s%v", cachePostIdPrefix, postId)
	_, err := m.ExecCtx(ctx, func(ctx context.Context, conn sqlx.SqlConn) (sql.Result, error) {
		query := fmt.Sprintf("update %s set `comment_count`=GREATEST(`comment_count`-1,0) where `id`=?", m.table)
		return conn.ExecCtx(ctx, query, postId)
	}, postIdKey)
	return err
}

// FindByIds 批量查询帖子，避免 N+1 查询
func (m *customPostModel) FindByIds(ctx context.Context, ids []int64) ([]*Post, error) {
	if len(ids) == 0 {
		return []*Post{}, nil
	}
	args := make([]interface{}, len(ids))
	for i, id := range ids {
		args[i] = id
	}
	placeholders := strings.Repeat("?,", len(ids))
	placeholders = placeholders[:len(placeholders)-1]
	var posts []*Post
	query := fmt.Sprintf("select %s from %s where `id` IN (%s)", postRows, m.table, placeholders)
	if err := m.CachedConn.QueryRowsNoCacheCtx(ctx, &posts, query, args...); err != nil {
		return nil, err
	}
	return posts, nil
}
