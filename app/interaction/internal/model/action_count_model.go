package model

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/zeromicro/go-zero/core/stores/sqlx"
)

var _ ActionCountModel = (*customActionCountModel)(nil)

type (
	ActionCountModel interface {
		Insert(ctx context.Context, data *ActionCount) (sql.Result, error)
		FindOneByTarget(ctx context.Context, targetID, targetType int64) (*ActionCount, error)
		Update(ctx context.Context, data *ActionCount) error
	}

	customActionCountModel struct {
		conn  sqlx.SqlConn
		table string
	}

	ActionCount struct {
		Id            int64 `db:"id"`
		TargetId      int64 `db:"target_id"`
		TargetType    int64 `db:"target_type"`
		LikeCount     int64 `db:"like_count"`
		FavoriteCount int64 `db:"favorite_count"`
		CommentCount  int64 `db:"comment_count"`
		ShareCount    int64 `db:"share_count"`
	}
)

func NewActionCountModel(conn sqlx.SqlConn) ActionCountModel {
	return &customActionCountModel{
		conn:  conn,
		table: "`action_count`",
	}
}

func (m *customActionCountModel) Insert(ctx context.Context, data *ActionCount) (sql.Result, error) {
	query := fmt.Sprintf("insert into %s (`target_id`, `target_type`, `like_count`, `favorite_count`, `comment_count`, `share_count`) values (?, ?, ?, ?, ?, ?)", m.table)
	return m.conn.ExecCtx(ctx, query, data.TargetId, data.TargetType, data.LikeCount, data.FavoriteCount, data.CommentCount, data.ShareCount)
}

func (m *customActionCountModel) FindOneByTarget(ctx context.Context, targetID, targetType int64) (*ActionCount, error) {
	query := fmt.Sprintf("select `id`, `target_id`, `target_type`, `like_count`, `favorite_count`, `comment_count`, `share_count` from %s where `target_id` = ? and `target_type` = ? limit 1", m.table)
	var resp ActionCount
	err := m.conn.QueryRowCtx(ctx, &resp, query, targetID, targetType)
	switch err {
	case nil:
		return &resp, nil
	case sqlx.ErrNotFound:
		return nil, ErrNotFound
	default:
		return nil, err
	}
}

func (m *customActionCountModel) Update(ctx context.Context, data *ActionCount) error {
	query := fmt.Sprintf("update %s set `like_count` = ?, `favorite_count` = ?, `comment_count` = ?, `share_count` = ? where `id` = ?", m.table)
	_, err := m.conn.ExecCtx(ctx, query, data.LikeCount, data.FavoriteCount, data.CommentCount, data.ShareCount, data.Id)
	return err
}
