package model

import (
	"context"
	"database/sql"
	"fmt"

	"esx/pkg/util"

	sqlx "esx/pkg/sqlstore"
)

var _ ActionCountModel = (*customActionCountModel)(nil)

type (
	ActionCountModel interface {
		Insert(ctx context.Context, data *ActionCount) (sql.Result, error)
		FindOneByTarget(ctx context.Context, targetID, targetType int64) (*ActionCount, error)
		Update(ctx context.Context, data *ActionCount) error
		IncrLikeCount(ctx context.Context, targetID, targetType int64) error
		IncrLikeCountTx(ctx context.Context, conn sqlx.SqlConn, targetID, targetType int64) error
		IncrFavoriteCount(ctx context.Context, targetID, targetType int64) error
		DecrLikeCount(ctx context.Context, targetID, targetType int64) error
		DecrFavoriteCount(ctx context.Context, targetID, targetType int64) error
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

// NewActionCountModel 创建互动计数模型。
func NewActionCountModel(conn sqlx.SqlConn) ActionCountModel {
	return &customActionCountModel{
		conn:  conn,
		table: "`action_count`",
	}
}

// Insert 写入一行计数；未指定 ID 时用雪花 ID。
func (m *customActionCountModel) Insert(ctx context.Context, data *ActionCount) (sql.Result, error) {
	if data.Id == 0 {
		id, err := util.NextID()
		if err != nil {
			return nil, err
		}
		data.Id = id
	}
	query := fmt.Sprintf("insert into %s (`id`, `target_id`, `target_type`, `like_count`, `favorite_count`, `comment_count`, `share_count`) values (?, ?, ?, ?, ?, ?, ?)", m.table)
	return m.conn.ExecCtx(ctx, query, data.Id, data.TargetId, data.TargetType, data.LikeCount, data.FavoriteCount, data.CommentCount, data.ShareCount)
}

// FindOneByTarget 读取目标的计数行，不存在时返回 ErrNotFound。
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

// Update 按主键覆盖全部计数。
func (m *customActionCountModel) Update(ctx context.Context, data *ActionCount) error {
	query := fmt.Sprintf("update %s set `like_count` = ?, `favorite_count` = ?, `comment_count` = ?, `share_count` = ? where `id` = ?", m.table)
	_, err := m.conn.ExecCtx(ctx, query, data.LikeCount, data.FavoriteCount, data.CommentCount, data.ShareCount, data.Id)
	return err
}

// IncrLikeCount 原子增加点赞数，计数行不存在时创建。
func (m *customActionCountModel) IncrLikeCount(ctx context.Context, targetID, targetType int64) error {
	query := fmt.Sprintf("insert into %s (`target_id`, `target_type`, `like_count`, `favorite_count`, `comment_count`, `share_count`) values (?, ?, 1, 0, 0, 0) on duplicate key update `like_count` = `like_count` + 1", m.table)
	_, err := m.conn.ExecCtx(ctx, query, targetID, targetType)
	return err
}

// IncrLikeCountTx 与 IncrLikeCount 相同，但在调用方的事务连接上执行。
func (m *customActionCountModel) IncrLikeCountTx(ctx context.Context, conn sqlx.SqlConn, targetID, targetType int64) error {
	query := fmt.Sprintf("insert into %s (`target_id`, `target_type`, `like_count`, `favorite_count`, `comment_count`, `share_count`) values (?, ?, 1, 0, 0, 0) on duplicate key update `like_count` = `like_count` + 1", m.table)
	_, err := conn.ExecCtx(ctx, query, targetID, targetType)
	return err
}

// IncrFavoriteCount 原子增加收藏数，计数行不存在时创建。
func (m *customActionCountModel) IncrFavoriteCount(ctx context.Context, targetID, targetType int64) error {
	query := fmt.Sprintf("insert into %s (`target_id`, `target_type`, `like_count`, `favorite_count`, `comment_count`, `share_count`) values (?, ?, 0, 1, 0, 0) on duplicate key update `favorite_count` = `favorite_count` + 1", m.table)
	_, err := m.conn.ExecCtx(ctx, query, targetID, targetType)
	return err
}

// DecrLikeCount 原子减少点赞数，不会减到负数。
func (m *customActionCountModel) DecrLikeCount(ctx context.Context, targetID, targetType int64) error {
	query := fmt.Sprintf("update %s set `like_count` = `like_count` - 1 where `target_id` = ? and `target_type` = ? and `like_count` > 0", m.table)
	_, err := m.conn.ExecCtx(ctx, query, targetID, targetType)
	return err
}

// DecrFavoriteCount 原子减少收藏数，不会减到负数。
func (m *customActionCountModel) DecrFavoriteCount(ctx context.Context, targetID, targetType int64) error {
	query := fmt.Sprintf("update %s set `favorite_count` = `favorite_count` - 1 where `target_id` = ? and `target_type` = ? and `favorite_count` > 0", m.table)
	_, err := m.conn.ExecCtx(ctx, query, targetID, targetType)
	return err
}
