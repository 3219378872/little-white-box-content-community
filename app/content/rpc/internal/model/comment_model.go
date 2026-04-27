package model

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/zeromicro/go-zero/core/stores/cache"
	"github.com/zeromicro/go-zero/core/stores/sqlx"
)

var _ CommentModel = (*customCommentModel)(nil)

type (
	// CommentModel is an interface to be customized, add more methods here,
	// and implement the added methods in customCommentModel.
	CommentModel interface {
		commentModel
		FindCommentById(ctx context.Context, id int64) (*Comment, error)
		InsertComment(ctx context.Context, comment *Comment) error
		FindByPostId(ctx context.Context, postId int64, page, pageSize int, sortBy int) ([]*Comment, int64, error)
		UpdateStatus(ctx context.Context, id int64, status int64) error
	}

	customCommentModel struct {
		*defaultCommentModel
	}
)

// NewCommentModel returns a model for the database table.
func NewCommentModel(conn sqlx.SqlConn, c cache.CacheConf, opts ...cache.Option) CommentModel {
	return &customCommentModel{
		defaultCommentModel: newCommentModel(conn, c, opts...),
	}
}

// FindCommentById 按主键查询评论（业务专用，显式 SQL）
func (m *customCommentModel) FindCommentById(ctx context.Context, id int64) (*Comment, error) {
	commentIdKey := fmt.Sprintf("%s%v", cacheCommentIdPrefix, id)
	var comment Comment
	err := m.QueryRowCtx(ctx, &comment, commentIdKey, func(ctx context.Context, conn sqlx.SqlConn, v interface{}) error {
		query := fmt.Sprintf("select %s from %s where `id`=? limit 1", commentRows, m.table)
		return conn.QueryRowCtx(ctx, v, query, id)
	})
	switch err {
	case nil:
		return &comment, nil
	case sqlx.ErrNotFound:
		return nil, ErrNotFound
	default:
		return nil, err
	}
}

// InsertComment 插入评论（显式字段列，避免依赖 gen 生成的通用 Insert）
func (m *customCommentModel) InsertComment(ctx context.Context, comment *Comment) error {
	commentIdKey := fmt.Sprintf("%s%v", cacheCommentIdPrefix, comment.Id)
	_, err := m.ExecCtx(ctx, func(ctx context.Context, conn sqlx.SqlConn) (sql.Result, error) {
		query := fmt.Sprintf("insert into %s (`id`,`post_id`,`user_id`,`parent_id`,`reply_user_id`,`content`,`status`) values (?,?,?,?,?,?,?)", m.table)
		return conn.ExecCtx(ctx, query, comment.Id, comment.PostId, comment.UserId, comment.ParentId, comment.ReplyUserId, comment.Content, comment.Status)
	}, commentIdKey)
	return err
}

func (m *customCommentModel) FindByPostId(ctx context.Context, postId int64, page, pageSize int, sortBy int) ([]*Comment, int64, error) {
	offset := (page - 1) * pageSize

	orderBy := "`created_at` desc"
	if sortBy == 2 {
		orderBy = "`like_count` desc"
	}

	var comments []*Comment
	query := fmt.Sprintf("select %s from %s where `post_id` = ? and `status` = 1 and `parent_id` is null order by %s limit ?,?", commentRows, m.table, orderBy)
	err := m.QueryRowsNoCacheCtx(ctx, &comments, query, postId, offset, pageSize)
	if err != nil {
		return nil, 0, err
	}

	var total int64
	countQuery := fmt.Sprintf("select count(*) from %s where `post_id` = ? and `status` = 1 and `parent_id` is null", m.table)
	err = m.QueryRowNoCacheCtx(ctx, &total, countQuery, postId)
	if err != nil {
		return nil, 0, err
	}

	return comments, total, nil
}

func (m *customCommentModel) UpdateStatus(ctx context.Context, id int64, status int64) error {
	commentIdKey := fmt.Sprintf("%s%v", cacheCommentIdPrefix, id)
	_, err := m.ExecCtx(ctx, func(ctx context.Context, conn sqlx.SqlConn) (sql.Result, error) {
		query := fmt.Sprintf("update %s set `status` = ? where `id` = ?", m.table)
		return conn.ExecCtx(ctx, query, status, id)
	}, commentIdKey)
	return err
}
