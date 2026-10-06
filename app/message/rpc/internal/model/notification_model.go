package model

import (
	"context"
	"database/sql"
	"fmt"

	cache "esx/pkg/modelcache"
	sqlx "esx/pkg/sqlstore"
)

var _ NotificationModel = (*customNotificationModel)(nil)

type (
	NotificationModel interface {
		notificationModel
		FindByUser(ctx context.Context, userID int64, typ int64, page int64, pageSize int64) ([]*Notification, int64, error)
		CountUnread(ctx context.Context, userID int64) (int64, error)
		MarkAllRead(ctx context.Context, userID int64) (int64, error)
	}

	customNotificationModel struct {
		*defaultNotificationModel
	}
)

// NewNotificationModel 创建带缓存的通知模型。
func NewNotificationModel(conn sqlx.SqlConn, c cache.CacheConf, opts ...cache.Option) NotificationModel {
	return &customNotificationModel{defaultNotificationModel: newNotificationModel(conn, c, opts...)}
}

// FindByUser 分页列出用户的通知，可按类型过滤，并返回总数；直查数据库，不经缓存。
func (m *customNotificationModel) FindByUser(ctx context.Context, userID int64, typ int64, page int64, pageSize int64) ([]*Notification, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	offset := (page - 1) * pageSize
	where := "where `user_id` = ?"
	args := []any{userID}
	if typ > 0 {
		where += " and `type` = ?"
		args = append(args, typ)
	}

	var total int64
	countQuery := fmt.Sprintf("select count(*) from %s %s", m.table, where)
	if err := m.QueryRowNoCacheCtx(ctx, &total, countQuery, args...); err != nil {
		return nil, 0, err
	}

	query := fmt.Sprintf("select %s from %s %s order by `id` desc limit ? offset ?", notificationRows, m.table, where)
	args = append(args, pageSize, offset)
	var rows []*Notification
	if err := m.QueryRowsNoCacheCtx(ctx, &rows, query, args...); err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

// CountUnread 统计用户的未读通知数。
func (m *customNotificationModel) CountUnread(ctx context.Context, userID int64) (int64, error) {
	var count int64
	query := fmt.Sprintf("select count(*) from %s where `user_id` = ? and `status` = 0", m.table)
	if err := m.QueryRowNoCacheCtx(ctx, &count, query, userID); err != nil {
		return 0, err
	}
	return count, nil
}

// MarkAllRead 把用户的全部未读通知标为已读，返回更新条数。
func (m *customNotificationModel) MarkAllRead(ctx context.Context, userID int64) (int64, error) {
	query := fmt.Sprintf("update %s set `status` = 1 where `user_id` = ? and `status` = 0", m.table)
	result, err := m.ExecNoCacheCtx(ctx, query, userID)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

var _ sql.Result
