package model

import (
	"context"
	"database/sql"

	"github.com/zeromicro/go-zero/core/stores/sqlx"
)

type Notification struct {
	Id         int64          `db:"id"`
	UserId     int64          `db:"user_id"`
	Type       int64          `db:"type"`
	Title      sql.NullString `db:"title"`
	Content    sql.NullString `db:"content"`
	TargetId   sql.NullInt64  `db:"target_id"`
	TargetType sql.NullInt64  `db:"target_type"`
	SenderId   sql.NullInt64  `db:"sender_id"`
	Status     int64          `db:"status"`
}

type NotificationModel interface {
	Insert(ctx context.Context, data *Notification) (sql.Result, error)
}

type notificationModel struct {
	conn  sqlx.SqlConn
	table string
}

func NewNotificationModel(conn sqlx.SqlConn) NotificationModel {
	return &notificationModel{conn: conn, table: "notification"}
}

func (m *notificationModel) Insert(ctx context.Context, data *Notification) (sql.Result, error) {
	query := `INSERT INTO notification (user_id, type, title, content, target_id, target_type, sender_id, status) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`
	return m.conn.ExecCtx(ctx, query, data.UserId, data.Type, data.Title, data.Content, data.TargetId, data.TargetType, data.SenderId, data.Status)
}
