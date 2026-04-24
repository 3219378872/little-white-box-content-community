package model

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/zeromicro/go-zero/core/stores/cache"
	"github.com/zeromicro/go-zero/core/stores/sqlx"
)

var _ ConversationModel = (*customConversationModel)(nil)

type (
	ConversationModel interface {
		conversationModel
		UpsertPairForMessage(ctx context.Context, senderID int64, receiverID int64, content string) (int64, int64, error)
		FindByUser(ctx context.Context, userID int64, page int64, pageSize int64) ([]*Conversation, int64, error)
		FindOneForUser(ctx context.Context, userID int64, conversationID int64) (*Conversation, error)
	}

	customConversationModel struct {
		*defaultConversationModel
	}
)

func NewConversationModel(conn sqlx.SqlConn, c cache.CacheConf, opts ...cache.Option) ConversationModel {
	return &customConversationModel{defaultConversationModel: newConversationModel(conn, c, opts...)}
}

func (m *customConversationModel) UpsertPairForMessage(ctx context.Context, senderID int64, receiverID int64, content string) (int64, int64, error) {
	if _, err := m.upsertOne(ctx, senderID, receiverID, content, 0); err != nil {
		return 0, 0, err
	}
	receiverConversationID, err := m.upsertOne(ctx, receiverID, senderID, content, 1)
	if err != nil {
		return 0, 0, err
	}
	senderConversation, err := m.FindOneByUserIdTargetUserId(ctx, senderID, receiverID)
	if err != nil {
		return 0, 0, err
	}
	return senderConversation.Id, receiverConversationID, nil
}

func (m *customConversationModel) upsertOne(ctx context.Context, userID int64, targetUserID int64, content string, unreadIncrement int64) (int64, error) {
	query := fmt.Sprintf(`insert into %s (user_id, target_user_id, last_message, last_message_time, unread_count)
values (?, ?, ?, now(), ?)
on duplicate key update last_message = values(last_message), last_message_time = values(last_message_time), unread_count = unread_count + ?`, m.table)
	if _, err := m.ExecNoCacheCtx(ctx, query, userID, targetUserID, content, unreadIncrement, unreadIncrement); err != nil {
		return 0, err
	}
	conversation, err := m.FindOneByUserIdTargetUserId(ctx, userID, targetUserID)
	if err != nil {
		return 0, err
	}
	return conversation.Id, nil
}

func (m *customConversationModel) FindByUser(ctx context.Context, userID int64, page int64, pageSize int64) ([]*Conversation, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize <= 0 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	offset := (page - 1) * pageSize
	var total int64
	countQuery := fmt.Sprintf("select count(*) from %s where `user_id` = ?", m.table)
	if err := m.QueryRowNoCacheCtx(ctx, &total, countQuery, userID); err != nil {
		return nil, 0, err
	}
	query := fmt.Sprintf("select %s from %s where `user_id` = ? order by `last_message_time` desc, `id` desc limit ? offset ?", conversationRows, m.table)
	var rows []*Conversation
	if err := m.QueryRowsNoCacheCtx(ctx, &rows, query, userID, pageSize, offset); err != nil {
		return nil, 0, err
	}
	return rows, total, nil
}

func (m *customConversationModel) FindOneForUser(ctx context.Context, userID int64, conversationID int64) (*Conversation, error) {
	query := fmt.Sprintf("select %s from %s where `id` = ? and `user_id` = ? limit 1", conversationRows, m.table)
	var row Conversation
	if err := m.QueryRowNoCacheCtx(ctx, &row, query, conversationID, userID); err != nil {
		return nil, err
	}
	return &row, nil
}

var _ sql.Result
