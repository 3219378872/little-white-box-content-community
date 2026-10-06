package model

import (
	"context"

	sqlx "esx/pkg/sqlstore"
)

// FeedOutbox 是作者发件箱中的一条帖子引用，拉模式读关注流时使用。
type FeedOutbox struct {
	Id        int64 `db:"id"`
	AuthorId  int64 `db:"author_id"`
	PostId    int64 `db:"post_id"`
	CreatedAt int64 `db:"created_at"`
}

// FeedOutboxModel 是消费者需要的 outbox 写操作。
type FeedOutboxModel interface {
	InsertIgnore(ctx context.Context, row *FeedOutbox) error
}

// feedOutboxModel 是基于 MySQL 的 outbox 写入实现。
type feedOutboxModel struct {
	conn  sqlx.SqlConn
	table string
}

// NewFeedOutboxModel 创建写 feed_outbox 表的模型。
func NewFeedOutboxModel(conn sqlx.SqlConn) FeedOutboxModel {
	return &feedOutboxModel{conn: conn, table: "feed_outbox"}
}

// InsertIgnore 写入一条 outbox 记录；(author_id, post_id) 唯一键使重放幂等。
func (m *feedOutboxModel) InsertIgnore(ctx context.Context, row *FeedOutbox) error {
	query := "INSERT IGNORE INTO feed_outbox (author_id, post_id, created_at) VALUES (?, ?, ?)"
	_, err := m.conn.ExecCtx(ctx, query, row.AuthorId, row.PostId, row.CreatedAt)
	return err
}
