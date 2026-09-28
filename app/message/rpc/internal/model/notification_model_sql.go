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
	notificationRows                = "`id`,`user_id`,`type`,`title`,`content`,`target_id`,`target_type`,`sender_id`,`status`,`created_at`"
	notificationRowsExpectAutoSet   = "`user_id`,`type`,`title`,`content`,`target_id`,`target_type`,`sender_id`,`status`"
	notificationRowsWithPlaceHolder = "`user_id`=?,`type`=?,`title`=?,`content`=?,`target_id`=?,`target_type`=?,`sender_id`=?,`status`=?"

	cacheNotificationIdPrefix = "cache:notification:id:"
)

type (
	notificationModel interface {
		Insert(ctx context.Context, data *Notification) (sql.Result, error)
		FindOne(ctx context.Context, id int64) (*Notification, error)
		Update(ctx context.Context, data *Notification) error
		Delete(ctx context.Context, id int64) error
	}

	defaultNotificationModel struct {
		sqlc.CachedConn
		table string
	}

	Notification struct {
		Id         int64          `db:"id"`
		UserId     int64          `db:"user_id"`     // 用户ID
		Type       int64          `db:"type"`        // 通知类型 1:点赞 2:评论 3:关注 4:系统 5:at
		Title      sql.NullString `db:"title"`       // 标题
		Content    sql.NullString `db:"content"`     // 内容
		TargetId   sql.NullInt64  `db:"target_id"`   // 目标ID
		TargetType sql.NullInt64  `db:"target_type"` // 目标类型
		SenderId   sql.NullInt64  `db:"sender_id"`   // 发送者ID
		Status     int64          `db:"status"`      // 状态 0:未读 1:已读
		CreatedAt  time.Time      `db:"created_at"`  // 创建时间
	}
)

func newNotificationModel(conn sqlx.SqlConn, c cache.CacheConf, opts ...cache.Option) *defaultNotificationModel {
	return &defaultNotificationModel{
		CachedConn: sqlc.NewConn(conn, c, opts...),
		table:      "`notification`",
	}
}

func (m *defaultNotificationModel) Delete(ctx context.Context, id int64) error {
	notificationIdKey := fmt.Sprintf("%s%v", cacheNotificationIdPrefix, id)
	_, err := m.ExecCtx(ctx, func(ctx context.Context, conn sqlx.SqlConn) (result sql.Result, err error) {
		query := fmt.Sprintf("delete from %s where `id` = ?", m.table)
		return conn.ExecCtx(ctx, query, id)
	}, notificationIdKey)
	return err
}

func (m *defaultNotificationModel) FindOne(ctx context.Context, id int64) (*Notification, error) {
	notificationIdKey := fmt.Sprintf("%s%v", cacheNotificationIdPrefix, id)
	var resp Notification
	err := m.QueryRowCtx(ctx, &resp, notificationIdKey, func(ctx context.Context, conn sqlx.SqlConn, v any) error {
		query := fmt.Sprintf("select %s from %s where `id` = ? limit 1", notificationRows, m.table)
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

func (m *defaultNotificationModel) Insert(ctx context.Context, data *Notification) (sql.Result, error) {
	notificationIdKey := fmt.Sprintf("%s%v", cacheNotificationIdPrefix, data.Id)
	ret, err := m.ExecCtx(ctx, func(ctx context.Context, conn sqlx.SqlConn) (result sql.Result, err error) {
		query := fmt.Sprintf("insert into %s (%s) values (?, ?, ?, ?, ?, ?, ?, ?)", m.table, notificationRowsExpectAutoSet)
		return conn.ExecCtx(ctx, query, data.UserId, data.Type, data.Title, data.Content, data.TargetId, data.TargetType, data.SenderId, data.Status)
	}, notificationIdKey)
	return ret, err
}

func (m *defaultNotificationModel) Update(ctx context.Context, data *Notification) error {
	notificationIdKey := fmt.Sprintf("%s%v", cacheNotificationIdPrefix, data.Id)
	_, err := m.ExecCtx(ctx, func(ctx context.Context, conn sqlx.SqlConn) (result sql.Result, err error) {
		query := fmt.Sprintf("update %s set %s where `id` = ?", m.table, notificationRowsWithPlaceHolder)
		return conn.ExecCtx(ctx, query, data.UserId, data.Type, data.Title, data.Content, data.TargetId, data.TargetType, data.SenderId, data.Status, data.Id)
	}, notificationIdKey)
	return err
}
