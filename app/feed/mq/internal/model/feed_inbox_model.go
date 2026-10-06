package model

import (
	"context"
	"strings"

	sqlx "esx/pkg/sqlstore"
)

// FeedInbox 是推送到某个粉丝收件箱的一条帖子引用。
type FeedInbox struct {
	Id        int64 `db:"id"`
	UserId    int64 `db:"user_id"`
	AuthorId  int64 `db:"author_id"`
	PostId    int64 `db:"post_id"`
	CreatedAt int64 `db:"created_at"`
}

// FeedInboxModel 是消费者需要的 inbox 写操作。
type FeedInboxModel interface {
	BatchInsertIgnore(ctx context.Context, rows []*FeedInbox) (int64, error)
}

// feedInboxModel 是基于 MySQL 的 inbox 写入实现。
type feedInboxModel struct {
	conn  sqlx.SqlConn
	table string
}

// NewFeedInboxModel 创建写 feed_inbox 表的模型。
func NewFeedInboxModel(conn sqlx.SqlConn) FeedInboxModel {
	return &feedInboxModel{conn: conn, table: "feed_inbox"}
}

// BatchInsertIgnore 用一条多值 INSERT IGNORE 批量写入，(user_id, post_id) 唯一键使重放幂等；
// 返回实际新增的行数。
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
