// Explicit SQL model. Maintained with its domain query extensions.

package model

import (
	"context"
	"database/sql"
	sqlc "esx/pkg/cachedstore"
	cache "esx/pkg/modelcache"
	sqlx "esx/pkg/sqlstore"
	"fmt"
	"time"
)

var (
	commentRows                = "`id`,`post_id`,`user_id`,`parent_id`,`reply_user_id`,`content`,`images`,`like_count`,`reply_count`,`status`,`created_at`,`updated_at`"
	commentRowsExpectAutoSet   = "`id`,`post_id`,`user_id`,`parent_id`,`reply_user_id`,`content`,`images`,`like_count`,`reply_count`,`status`"
	commentRowsWithPlaceHolder = "`post_id`=?,`user_id`=?,`parent_id`=?,`reply_user_id`=?,`content`=?,`images`=?,`like_count`=?,`reply_count`=?,`status`=?"

	cacheCommentIdPrefix = "cache:comment:id:"
)

type (
	commentModel interface {
		Insert(ctx context.Context, data *Comment) (sql.Result, error)
		FindOne(ctx context.Context, id int64) (*Comment, error)
		Update(ctx context.Context, data *Comment) error
		Delete(ctx context.Context, id int64) error
	}

	defaultCommentModel struct {
		sqlc.CachedConn
		table string
	}

	Comment struct {
		Id          int64          `db:"id"`            // 评论ID
		PostId      int64          `db:"post_id"`       // 帖子ID
		UserId      int64          `db:"user_id"`       // 用户ID
		ParentId    sql.NullInt64  `db:"parent_id"`     // 父评论ID NULL:一级评论
		ReplyUserId sql.NullInt64  `db:"reply_user_id"` // 回复用户ID
		Content     string         `db:"content"`       // 评论内容
		Images      sql.NullString `db:"images"`        // 图片列表
		LikeCount   int64          `db:"like_count"`    // 点赞数
		ReplyCount  int64          `db:"reply_count"`   // 回复数
		Status      int64          `db:"status"`        // 状态 0:删除 1:正常 2:审核中
		CreatedAt   time.Time      `db:"created_at"`    // 创建时间
		UpdatedAt   time.Time      `db:"updated_at"`    // 更新时间
	}
)

func newCommentModel(conn sqlx.SqlConn, c cache.CacheConf, opts ...cache.Option) *defaultCommentModel {
	return &defaultCommentModel{
		CachedConn: sqlc.NewConn(conn, c, opts...),
		table:      "`comment`",
	}
}

func (m *defaultCommentModel) Delete(ctx context.Context, id int64) error {
	commentIdKey := fmt.Sprintf("%s%v", cacheCommentIdPrefix, id)
	_, err := m.ExecCtx(ctx, func(ctx context.Context, conn sqlx.SqlConn) (result sql.Result, err error) {
		query := fmt.Sprintf("delete from %s where `id` = ?", m.table)
		return conn.ExecCtx(ctx, query, id)
	}, commentIdKey)
	return err
}

func (m *defaultCommentModel) FindOne(ctx context.Context, id int64) (*Comment, error) {
	commentIdKey := fmt.Sprintf("%s%v", cacheCommentIdPrefix, id)
	var resp Comment
	err := m.QueryRowCtx(ctx, &resp, commentIdKey, func(ctx context.Context, conn sqlx.SqlConn, v any) error {
		query := fmt.Sprintf("select %s from %s where `id` = ? limit 1", commentRows, m.table)
		return conn.QueryRowCtx(ctx, v, query, id)
	})
	switch err {
	case nil:
		return &resp, nil
	case sqlc.ErrNotFound:
		return nil, ErrNotFound
	default:
		return nil, err
	}
}

func (m *defaultCommentModel) Insert(ctx context.Context, data *Comment) (sql.Result, error) {
	commentIdKey := fmt.Sprintf("%s%v", cacheCommentIdPrefix, data.Id)
	ret, err := m.ExecCtx(ctx, func(ctx context.Context, conn sqlx.SqlConn) (result sql.Result, err error) {
		query := fmt.Sprintf("insert into %s (%s) values (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)", m.table, commentRowsExpectAutoSet)
		return conn.ExecCtx(ctx, query, data.Id, data.PostId, data.UserId, data.ParentId, data.ReplyUserId, data.Content, data.Images, data.LikeCount, data.ReplyCount, data.Status)
	}, commentIdKey)
	return ret, err
}

func (m *defaultCommentModel) Update(ctx context.Context, data *Comment) error {
	commentIdKey := fmt.Sprintf("%s%v", cacheCommentIdPrefix, data.Id)
	_, err := m.ExecCtx(ctx, func(ctx context.Context, conn sqlx.SqlConn) (result sql.Result, err error) {
		query := fmt.Sprintf("update %s set %s where `id` = ?", m.table, commentRowsWithPlaceHolder)
		return conn.ExecCtx(ctx, query, data.PostId, data.UserId, data.ParentId, data.ReplyUserId, data.Content, data.Images, data.LikeCount, data.ReplyCount, data.Status, data.Id)
	}, commentIdKey)
	return err
}
