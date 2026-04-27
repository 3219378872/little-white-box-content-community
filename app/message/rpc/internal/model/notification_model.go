package model

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/zeromicro/go-zero/core/stores/cache"
	"github.com/zeromicro/go-zero/core/stores/sqlx"
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

func NewNotificationModel(conn sqlx.SqlConn, c cache.CacheConf, opts ...cache.Option) NotificationModel {
	return &customNotificationModel{defaultNotificationModel: newNotificationModel(conn, c, opts...)}
}

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

func (m *customNotificationModel) CountUnread(ctx context.Context, userID int64) (int64, error) {
	var count int64
	query := fmt.Sprintf("select count(*) from %s where `user_id` = ? and `status` = 0", m.table)
	if err := m.QueryRowNoCacheCtx(ctx, &count, query, userID); err != nil {
		return 0, err
	}
	return count, nil
}

func (m *customNotificationModel) MarkAllRead(ctx context.Context, userID int64) (int64, error) {
	query := fmt.Sprintf("update %s set `status` = 1 where `user_id` = ? and `status` = 0", m.table)
	result, err := m.ExecNoCacheCtx(ctx, query, userID)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

var _ sql.Result
