package model

import (
	"context"

	"github.com/zeromicro/go-zero/core/stores/sqlx"
)

type FeedOutbox struct {
	Id        int64 `db:"id"`
	AuthorId  int64 `db:"author_id"`
	PostId    int64 `db:"post_id"`
	CreatedAt int64 `db:"created_at"`
}

type FeedOutboxModel interface {
	InsertIgnore(ctx context.Context, row *FeedOutbox) error
}

type feedOutboxModel struct {
	conn  sqlx.SqlConn
	table string
}

func NewFeedOutboxModel(conn sqlx.SqlConn) FeedOutboxModel {
	return &feedOutboxModel{conn: conn, table: "feed_outbox"}
}

func (m *feedOutboxModel) InsertIgnore(ctx context.Context, row *FeedOutbox) error {
	query := "INSERT IGNORE INTO feed_outbox (author_id, post_id, created_at) VALUES (?, ?, ?)"
	_, err := m.conn.ExecCtx(ctx, query, row.AuthorId, row.PostId, row.CreatedAt)
	return err
}
