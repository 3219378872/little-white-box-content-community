package model

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"github.com/zeromicro/go-zero/core/stores/cache"
	"github.com/zeromicro/go-zero/core/stores/sqlx"
)

var _ LikeRecordModel = (*customLikeRecordModel)(nil)

type (
	// LikeRecordModel is an interface to be customized, add more methods here,
	// and implement the added methods in customLikeRecordModel.
	LikeRecordModel interface {
		likeRecordModel
		UpsertLikeStatus(ctx context.Context, userId, targetId, targetType, status int64) (sql.Result, error)
		UpsertLikeStatusTx(ctx context.Context, conn sqlx.SqlConn, userId, targetId, targetType, status int64) (sql.Result, int64, error)
		InvalidateLikeRecordCache(ctx context.Context, id, userId, targetId, targetType int64) error
		FindStatusByUserAndTargets(ctx context.Context, userId int64, targetIds []int64, targetType int64) (map[int64]bool, error)
		UpdateStatusById(ctx context.Context, id, expectedStatus, newStatus int64) (sql.Result, error)
	}

	customLikeRecordModel struct {
		*defaultLikeRecordModel
	}
)

// NewLikeRecordModel returns a model for the database table.
func NewLikeRecordModel(conn sqlx.SqlConn, c cache.CacheConf, opts ...cache.Option) LikeRecordModel {
	return &customLikeRecordModel{
		defaultLikeRecordModel: newLikeRecordModel(conn, c, opts...),
	}
}

func (m *customLikeRecordModel) UpsertLikeStatus(ctx context.Context, userId, targetId, targetType, status int64) (sql.Result, error) {
	query := fmt.Sprintf(
		"insert into %s (`user_id`,`target_id`,`target_type`,`status`) values (?,?,?,?) on duplicate key update `status`=values(`status`)",
		m.table,
	)
	return m.ExecCtx(ctx, func(ctx context.Context, conn sqlx.SqlConn) (sql.Result, error) {
		return conn.ExecCtx(ctx, query, userId, targetId, targetType, status)
	})
}

func (m *customLikeRecordModel) UpsertLikeStatusTx(ctx context.Context, conn sqlx.SqlConn, userId, targetId, targetType, status int64) (sql.Result, int64, error) {
	query := fmt.Sprintf(
		"insert into %s (`user_id`,`target_id`,`target_type`,`status`) values (?,?,?,?) on duplicate key update `id`=last_insert_id(`id`), `status`=values(`status`)",
		m.table,
	)
	result, err := conn.ExecCtx(ctx, query, userId, targetId, targetType, status)
	if err != nil {
		return nil, 0, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return nil, 0, err
	}
	return result, id, nil
}

func (m *customLikeRecordModel) InvalidateLikeRecordCache(ctx context.Context, id, userId, targetId, targetType int64) error {
	keys := []string{fmt.Sprintf("%s%v:%v:%v", cacheLikeRecordUserIdTargetIdTargetTypePrefix, userId, targetId, targetType)}
	if id > 0 {
		keys = append(keys, fmt.Sprintf("%s%v", cacheLikeRecordIdPrefix, id))
	}
	return m.DelCacheCtx(ctx, keys...)
}

func (m *customLikeRecordModel) FindStatusByUserAndTargets(ctx context.Context, userId int64, targetIds []int64, targetType int64) (map[int64]bool, error) {
	if len(targetIds) == 0 {
		return map[int64]bool{}, nil
	}
	placeholders := strings.Repeat("?,", len(targetIds))
	placeholders = placeholders[:len(placeholders)-1]
	args := make([]interface{}, 0, len(targetIds)+2)
	args = append(args, userId, targetType)
	for _, id := range targetIds {
		args = append(args, id)
	}

	var rows []struct {
		TargetId int64 `db:"target_id"`
		Status   int64 `db:"status"`
	}
	query := fmt.Sprintf("select `target_id`,`status` from %s where `user_id`=? and `target_type`=? and `target_id` in (%s)", m.table, placeholders)
	if err := m.QueryRowsNoCacheCtx(ctx, &rows, query, args...); err != nil {
		return nil, err
	}

	results := make(map[int64]bool, len(rows))
	for _, r := range rows {
		results[r.TargetId] = r.Status == StatusActive
	}
	return results, nil
}

func (m *customLikeRecordModel) UpdateStatusById(ctx context.Context, id, expectedStatus, newStatus int64) (sql.Result, error) {
	query := fmt.Sprintf("update %s set `status`=? where `id`=? and `status`=?", m.table)
	return m.ExecCtx(ctx, func(ctx context.Context, conn sqlx.SqlConn) (sql.Result, error) {
		return conn.ExecCtx(ctx, query, newStatus, id, expectedStatus)
	})
}
