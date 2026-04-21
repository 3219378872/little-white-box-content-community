package model

import (
	"context"
	"fmt"

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
	if err := m.CachedConn.QueryRowsNoCacheCtx(ctx, &rows, query, userID, offset, pageSize); err != nil {
		return nil, 0, err
	}
	postIDs := make([]int64, 0, len(rows))
	for _, row := range rows {
		postIDs = append(postIDs, row.PostID)
	}

	var total int64
	countQuery := fmt.Sprintf("select count(*) from %s where `user_id` = ? and `status` = 1", m.table)
	if err := m.CachedConn.QueryRowNoCacheCtx(ctx, &total, countQuery, userID); err != nil {
		return nil, 0, err
	}

	return postIDs, total, nil
}
