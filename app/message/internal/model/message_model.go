package model

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/zeromicro/go-zero/core/stores/cache"
	"github.com/zeromicro/go-zero/core/stores/sqlx"
)

var _ MessageModel = (*customMessageModel)(nil)

type (
	MessageModel interface {
		messageModel
		FindByConversation(ctx context.Context, conversationID int64, lastID int64, limit int64) ([]*Message, bool, error)
		CountUnreadByUser(ctx context.Context, userID int64) (int64, error)
		MarkConversationRead(ctx context.Context, userID int64, conversationID int64) (int64, error)
	}

	customMessageModel struct {
		*defaultMessageModel
	}
)

func NewMessageModel(conn sqlx.SqlConn, c cache.CacheConf, opts ...cache.Option) MessageModel {
	return &customMessageModel{defaultMessageModel: newMessageModel(conn, c, opts...)}
}

func (m *customMessageModel) FindByConversation(ctx context.Context, conversationID int64, lastID int64, limit int64) ([]*Message, bool, error) {
	if limit <= 0 {
		limit = 20
	}
	if limit > 100 {
		limit = 100
	}
	queryLimit := limit + 1
	where := "where `conversation_id` = ?"
	args := []any{conversationID}
	if lastID > 0 {
		where += " and `id` < ?"
		args = append(args, lastID)
	}
	query := fmt.Sprintf("select %s from %s %s order by `id` desc limit ?", messageRows, m.table, where)
	args = append(args, queryLimit)
	var rows []*Message
	if err := m.QueryRowsNoCacheCtx(ctx, &rows, query, args...); err != nil {
		return nil, false, err
	}
	hasMore := int64(len(rows)) > limit
	if hasMore {
		rows = rows[:limit]
	}
	return rows, hasMore, nil
}

func (m *customMessageModel) CountUnreadByUser(ctx context.Context, userID int64) (int64, error) {
	var count int64
	query := fmt.Sprintf("select count(*) from %s where `receiver_id` = ? and `status` = 0", m.table)
	if err := m.QueryRowNoCacheCtx(ctx, &count, query, userID); err != nil {
		return 0, err
	}
	return count, nil
}

func (m *customMessageModel) MarkConversationRead(ctx context.Context, userID int64, conversationID int64) (int64, error) {
	query := fmt.Sprintf("update %s set `status` = 1 where `receiver_id` = ? and `conversation_id` = ? and `status` = 0", m.table)
	result, err := m.ExecNoCacheCtx(ctx, query, userID, conversationID)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

var _ sql.Result
