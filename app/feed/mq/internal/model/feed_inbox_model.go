package model

import (
	"context"
	"strings"

	"github.com/zeromicro/go-zero/core/stores/sqlx"
)

type FeedInbox struct {
	Id        int64 `db:"id"`
	UserId    int64 `db:"user_id"`
	AuthorId  int64 `db:"author_id"`
	PostId    int64 `db:"post_id"`
	CreatedAt int64 `db:"created_at"`
}

type FeedInboxModel interface {
	BatchInsertIgnore(ctx context.Context, rows []*FeedInbox) (int64, error)
}

type feedInboxModel struct {
	conn  sqlx.SqlConn
	table string
}

func NewFeedInboxModel(conn sqlx.SqlConn) FeedInboxModel {
	return &feedInboxModel{conn: conn, table: "feed_inbox"}
}

func (m *feedInboxModel) BatchInsertIgnore(ctx context.Context, rows []*FeedInbox) (int64, error) {
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
