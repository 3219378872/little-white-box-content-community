package model

import (
	"context"
	"strings"

	"github.com/zeromicro/go-zero/core/stores/sqlx"
)

var _ FeedInboxModel = (*customFeedInboxModel)(nil)

type (
	FeedInboxModel interface {
		feedInboxModel
		withSession(session sqlx.Session) FeedInboxModel
		BatchInsertIgnore(ctx context.Context, rows []*FeedInbox) (int64, error)
		FindByUserBefore(ctx context.Context, userID, cursorCreatedAt, cursorPostID int64, limit int64) ([]*FeedInbox, error)
	}

	customFeedInboxModel struct {
		*defaultFeedInboxModel
	}
)

func NewFeedInboxModel(conn sqlx.SqlConn) FeedInboxModel {
	return &customFeedInboxModel{defaultFeedInboxModel: newFeedInboxModel(conn)}
}

func (m *customFeedInboxModel) withSession(session sqlx.Session) FeedInboxModel {
	return NewFeedInboxModel(sqlx.NewSqlConnFromSession(session))
}

func (m *customFeedInboxModel) BatchInsertIgnore(ctx context.Context, rows []*FeedInbox) (int64, error) {
	if len(rows) == 0 {
		return 0, nil
	}
	values := make([]string, 0, len(rows))
	args := make([]any, 0, len(rows)*4)
	for _, row := range rows {
		values = append(values, "(?, ?, ?, ?)")
		args = append(args, row.UserId, row.AuthorId, row.PostId, row.CreatedAt)
	}
	query := "INSERT IGNORE INTO feed_inbox (user_id, author_id, post_id, created_at) VALUES " + strings.Join(values, ",")
	ret, err := m.conn.ExecCtx(ctx, query, args...)
	if err != nil {
		return 0, err
	}
	return ret.RowsAffected()
}

func (m *customFeedInboxModel) FindByUserBefore(ctx context.Context, userID, cursorCreatedAt, cursorPostID int64, limit int64) ([]*FeedInbox, error) {
	query := `SELECT id, user_id, author_id, post_id, created_at
FROM feed_inbox
WHERE user_id = ? AND (created_at < ? OR (created_at = ? AND post_id < ?))
ORDER BY created_at DESC, post_id DESC
LIMIT ?`
	var rows []*FeedInbox
	if err := m.conn.QueryRowsCtx(ctx, &rows, query, userID, cursorCreatedAt, cursorCreatedAt, cursorPostID, limit); err != nil {
		return nil, err
	}
	return rows, nil
}
