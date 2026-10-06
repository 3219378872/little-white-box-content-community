package model

import (
	"context"
	"strings"

	sqlx "esx/pkg/sqlstore"
)

var _ FeedOutboxModel = (*customFeedOutboxModel)(nil)

type (
	FeedOutboxModel interface {
		feedOutboxModel
		withSession(session sqlx.Session) FeedOutboxModel
		InsertIgnore(ctx context.Context, row *FeedOutbox) error
		FindByAuthorsBefore(ctx context.Context, authorIDs []int64, cursorCreatedAt, cursorPostID int64, limit int64) ([]*FeedOutbox, error)
	}

	customFeedOutboxModel struct {
		*defaultFeedOutboxModel
	}
)

// NewFeedOutboxModel 创建带关注流查询扩展的 outbox 模型。
func NewFeedOutboxModel(conn sqlx.SqlConn) FeedOutboxModel {
	return &customFeedOutboxModel{defaultFeedOutboxModel: newFeedOutboxModel(conn)}
}

// withSession 返回绑定到事务会话的模型。
func (m *customFeedOutboxModel) withSession(session sqlx.Session) FeedOutboxModel {
	return NewFeedOutboxModel(sqlx.NewSqlConnFromSession(session))
}

// InsertIgnore 写入一条 outbox 记录；(author_id, post_id) 唯一键使重复写入幂等。
func (m *customFeedOutboxModel) InsertIgnore(ctx context.Context, row *FeedOutbox) error {
	query := `INSERT IGNORE INTO feed_outbox (author_id, post_id, created_at) VALUES (?, ?, ?)`
	_, err := m.conn.ExecCtx(ctx, query, row.AuthorId, row.PostId, row.CreatedAt)
	return err
}

// FindByAuthorsBefore 读取一批作者在游标之前的帖子，按 (created_at, post_id) 降序。
func (m *customFeedOutboxModel) FindByAuthorsBefore(ctx context.Context, authorIDs []int64, cursorCreatedAt, cursorPostID int64, limit int64) ([]*FeedOutbox, error) {
	if len(authorIDs) == 0 {
		return []*FeedOutbox{}, nil
	}
	placeholders := make([]string, 0, len(authorIDs))
	args := make([]any, 0, len(authorIDs)+4)
	for _, authorID := range authorIDs {
		placeholders = append(placeholders, "?")
		args = append(args, authorID)
	}
	args = append(args, cursorCreatedAt, cursorCreatedAt, cursorPostID, limit)
	query := `SELECT id, author_id, post_id, created_at
FROM feed_outbox
WHERE author_id IN (` + strings.Join(placeholders, ",") + `)
  AND (created_at < ? OR (created_at = ? AND post_id < ?))
ORDER BY created_at DESC, post_id DESC
LIMIT ?`
	var rows []*FeedOutbox
	if err := m.conn.QueryRowsCtx(ctx, &rows, query, args...); err != nil {
		return nil, err
	}
	return rows, nil
}
