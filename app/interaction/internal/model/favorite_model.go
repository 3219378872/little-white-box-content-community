package model

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/zeromicro/go-zero/core/stores/cache"
	"github.com/zeromicro/go-zero/core/stores/sqlx"
)

var _ FavoriteModel = (*customFavoriteModel)(nil)

type (
	// FavoriteModel is an interface to be customized, add more methods here,
	// and implement the added methods in customFavoriteModel.
	FavoriteModel interface {
		favoriteModel
		FindActivePostIds(ctx context.Context, userID int64, page, pageSize int32) ([]int64, int64, error)
		UpsertFavoriteStatus(ctx context.Context, userId, postId, status int64) (sql.Result, error)
		FindFavoriteStatusByUserAndPosts(ctx context.Context, userId int64, postIds []int64) (map[int64]bool, error)
		UpdateStatusById(ctx context.Context, id, expectedStatus, newStatus int64) (sql.Result, error)
	}

	customFavoriteModel struct {
		*defaultFavoriteModel
	}
)

// NewFavoriteModel returns a model for the database table.
func NewFavoriteModel(conn sqlx.SqlConn, c cache.CacheConf, opts ...cache.Option) FavoriteModel {
	return &customFavoriteModel{
		defaultFavoriteModel: newFavoriteModel(conn, c, opts...),
	}
}

func (m *customFavoriteModel) FindActivePostIds(ctx context.Context, userID int64, page, pageSize int32) ([]int64, int64, error) {
	offset := (page - 1) * pageSize

	var rows []struct {
		PostID int64 `db:"post_id"`
	}
	query := fmt.Sprintf("select `post_id` from %s where `user_id` = ? and `status` = 1 order by `created_at` desc limit ?, ?", m.table)
	if err := m.QueryRowsNoCacheCtx(ctx, &rows, query, userID, offset, pageSize); err != nil {
		return nil, 0, err
	}
	postIDs := make([]int64, 0, len(rows))
	for _, row := range rows {
		postIDs = append(postIDs, row.PostID)
	}

	var total int64
	countQuery := fmt.Sprintf("select count(*) from %s where `user_id` = ? and `status` = 1", m.table)
	if err := m.QueryRowNoCacheCtx(ctx, &total, countQuery, userID); err != nil {
		return nil, 0, err
	}

	return postIDs, total, nil
}

func (m *customFavoriteModel) UpsertFavoriteStatus(ctx context.Context, userId, postId, status int64) (sql.Result, error) {
	query := fmt.Sprintf(
		"insert into %s (`user_id`,`post_id`,`status`) values (?,?,?) on duplicate key update `status`=values(`status`)",
		m.table,
	)
	return m.ExecCtx(ctx, func(ctx context.Context, conn sqlx.SqlConn) (sql.Result, error) {
		return conn.ExecCtx(ctx, query, userId, postId, status)
	})
}

func (m *customFavoriteModel) FindFavoriteStatusByUserAndPosts(ctx context.Context, userId int64, postIds []int64) (map[int64]bool, error) {
	if len(postIds) == 0 {
		return map[int64]bool{}, nil
	}
	placeholders := strings.Repeat("?,", len(postIds))
	placeholders = placeholders[:len(placeholders)-1]
	args := make([]interface{}, 0, len(postIds)+1)
	args = append(args, userId)
	for _, id := range postIds {
		args = append(args, id)
	}

	var rows []struct {
		PostId int64 `db:"post_id"`
		Status int64 `db:"status"`
	}
	query := fmt.Sprintf("select `post_id`,`status` from %s where `user_id`=? and `post_id` in (%s)", m.table, placeholders)
	if err := m.QueryRowsNoCacheCtx(ctx, &rows, query, args...); err != nil {
		return nil, err
	}

	results := make(map[int64]bool, len(rows))
	for _, r := range rows {
		results[r.PostId] = r.Status == StatusActive
	}
	return results, nil
}

func (m *customFavoriteModel) UpdateStatusById(ctx context.Context, id, expectedStatus, newStatus int64) (sql.Result, error) {
	query := fmt.Sprintf("update %s set `status`=? where `id`=? and `status`=?", m.table)
	return m.ExecCtx(ctx, func(ctx context.Context, conn sqlx.SqlConn) (sql.Result, error) {
		return conn.ExecCtx(ctx, query, newStatus, id, expectedStatus)
	})
}
