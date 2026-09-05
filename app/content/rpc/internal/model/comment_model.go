package model

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

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
		FindByParentId(ctx context.Context, parentId int64, page, pageSize int) ([]*Comment, int64, error)
		FindByParentIds(ctx context.Context, postId int64, parentIds []int64) ([]*Comment, error)
		FindActiveByIds(ctx context.Context, postID int64, ids []int64) ([]*Comment, error)
		UpdateStatus(ctx context.Context, id int64, status int64) error
		InvalidateCommentCache(ctx context.Context, id int64) error
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

// FindCommentById 按主键读取评论，不走缓存，避免删除后仍读到旧 status。
func (m *customCommentModel) FindCommentById(ctx context.Context, id int64) (*Comment, error) {
	var comment Comment
	query := fmt.Sprintf("select %s from %s where `id`=? limit 1", commentRows, m.table)
	err := m.QueryRowNoCacheCtx(ctx, &comment, query, id)
	switch {
	case err == nil:
		return &comment, nil
	case errors.Is(err, sqlx.ErrNotFound):
		return nil, ErrNotFound
	default:
		return nil, err
	}
}

func (m *customCommentModel) FindActiveByIds(ctx context.Context, postID int64, ids []int64) ([]*Comment, error) {
	out := []*Comment{}
	if len(ids) == 0 {
		return out, nil
	}
	args := make([]any, 0, len(ids)+1)
	args = append(args, postID)
	for _, id := range ids {
		args = append(args, id)
	}
	query := fmt.Sprintf("SELECT %s FROM %s WHERE post_id=? AND status=1 AND id IN (%s) ORDER BY id", commentRows, m.table, strings.TrimSuffix(strings.Repeat("?,", len(ids)), ","))
	err := m.QueryRowsNoCacheCtx(ctx, &out, query, args...)
	return out, err
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

	// CORE-060：评论分页同样需要确定性二级键。
	orderBy := "`created_at` desc, `id` desc"
	if sortBy == 2 {
		orderBy = "`like_count` desc, `id` desc"
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

// FindByParentId 分页读取某条评论的楼中楼回复（时间正序，CORE-060 确定性二级键）。
func (m *customCommentModel) FindByParentId(ctx context.Context, parentId int64, page, pageSize int) ([]*Comment, int64, error) {
	offset := (page - 1) * pageSize

	var comments []*Comment
	query := fmt.Sprintf("select %s from %s where `parent_id` = ? and `status` = 1 order by `created_at` asc, `id` asc limit ?,?", commentRows, m.table)
	err := m.QueryRowsNoCacheCtx(ctx, &comments, query, parentId, offset, pageSize)
	if err != nil {
		return nil, 0, err
	}

	var total int64
	countQuery := fmt.Sprintf("select count(*) from %s where `parent_id` = ? and `status` = 1", m.table)
	err = m.QueryRowNoCacheCtx(ctx, &total, countQuery, parentId)
	if err != nil {
		return nil, 0, err
	}

	return comments, total, nil
}

// FindByParentIds 批量读取多条一级评论的可见回复（时间正序），供评论列表内嵌预览。
// 返回行不按父分组，由调用方在内存中按 ParentId 归组截断。
func (m *customCommentModel) FindByParentIds(ctx context.Context, postId int64, parentIds []int64) ([]*Comment, error) {
	if len(parentIds) == 0 {
		return nil, nil
	}
	placeholders := make([]string, 0, len(parentIds))
	args := make([]any, 0, len(parentIds)+1)
	args = append(args, postId)
	for _, id := range parentIds {
		placeholders = append(placeholders, "?")
		args = append(args, id)
	}
	var comments []*Comment
	query := fmt.Sprintf(
		"select %s from %s where `post_id` = ? and `status` = 1 and `parent_id` in (%s) order by `created_at` asc, `id` asc",
		commentRows, m.table, strings.Join(placeholders, ","),
	)
	err := m.QueryRowsNoCacheCtx(ctx, &comments, query, args...)
	if err != nil {
		return nil, err
	}
	return comments, nil
}

func (m *customCommentModel) UpdateStatus(ctx context.Context, id int64, status int64) error {
	commentIdKey := fmt.Sprintf("%s%v", cacheCommentIdPrefix, id)
	_, err := m.ExecCtx(ctx, func(ctx context.Context, conn sqlx.SqlConn) (sql.Result, error) {
		query := fmt.Sprintf("update %s set `status` = ? where `id` = ?", m.table)
		return conn.ExecCtx(ctx, query, status, id)
	}, commentIdKey)
	return err
}

func (m *customCommentModel) InvalidateCommentCache(ctx context.Context, id int64) error {
	return m.DelCacheCtx(ctx, fmt.Sprintf("%s%v", cacheCommentIdPrefix, id))
}
