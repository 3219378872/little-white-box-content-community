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
	messageRows = "`id`,`conversation_id`,`sender_id`,`receiver_id`,`content`,`msg_type`,`status`,`media_id`,`created_at`"

	cacheMessageIdPrefix = "cache:message:id:"
)

type (
	defaultMessageModel struct {
		sqlc.CachedConn
		table string
	}

	Message struct {
		Id             int64         `db:"id"`
		ConversationId int64         `db:"conversation_id"` // 会话ID
		SenderId       int64         `db:"sender_id"`       // 发送者ID
		ReceiverId     int64         `db:"receiver_id"`     // 接收者ID
		Content        string        `db:"content"`         // 消息内容
		MsgType        int64         `db:"msg_type"`        // 消息类型 1:文本 2:图片 3:视频 4:语音
		Status         int64         `db:"status"`          // 状态 0:未读 1:已读
		MediaId        sql.NullInt64 `db:"media_id"`        // 媒体消息引用
		CreatedAt      time.Time     `db:"created_at"`      // 创建时间
	}
)

// newMessageModel creates the cached base model.
func newMessageModel(conn sqlx.SqlConn, c cache.CacheConf, opts ...cache.Option) *defaultMessageModel {
	return &defaultMessageModel{
		CachedConn: sqlc.NewConn(conn, c, opts...),
		table:      "`message`",
	}
}

// Delete removes a message by id and evicts its cache key.
func (m *defaultMessageModel) Delete(ctx context.Context, id int64) error {
	messageIdKey := fmt.Sprintf("%s%v", cacheMessageIdPrefix, id)
	_, err := m.ExecCtx(ctx, func(ctx context.Context, conn sqlx.SqlConn) (result sql.Result, err error) {
		query := fmt.Sprintf("delete from %s where `id` = ?", m.table)
		return conn.ExecCtx(ctx, query, id)
	}, messageIdKey)
	return err
}

// FindOne loads a message by id through the cache.
func (m *defaultMessageModel) FindOne(ctx context.Context, id int64) (*Message, error) {
	messageIdKey := fmt.Sprintf("%s%v", cacheMessageIdPrefix, id)
	var resp Message
	err := m.QueryRowCtx(ctx, &resp, messageIdKey, func(ctx context.Context, conn sqlx.SqlConn, v any) error {
		query := fmt.Sprintf("select %s from %s where `id` = ? limit 1", messageRows, m.table)
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
