package model

import (
	"context"
	"database/sql"

	"github.com/zeromicro/go-zero/core/stores/sqlx"
)

var _ MessageCommandModel = (*customMessageCommandModel)(nil)

type (
	MessageCommandModel interface {
		CreateMessageWithConversations(ctx context.Context, senderID int64, receiverID int64, content string, msgType int64) (int64, error)
		MarkConversationRead(ctx context.Context, userID int64, targetUserID int64) (int64, error)
	}

	customMessageCommandModel struct {
		conn sqlx.SqlConn
	}
)

func NewMessageCommandModel(conn sqlx.SqlConn) MessageCommandModel {
	return &customMessageCommandModel{conn: conn}
}

func (m *customMessageCommandModel) CreateMessageWithConversations(ctx context.Context, senderID int64, receiverID int64, content string, msgType int64) (int64, error) {
	var messageID int64
	err := m.conn.TransactCtx(ctx, func(ctx context.Context, session sqlx.Session) error {
		if err := upsertConversationForMessage(ctx, session, senderID, receiverID, content, 0); err != nil {
			return err
		}
		if err := upsertConversationForMessage(ctx, session, receiverID, senderID, content, 1); err != nil {
			return err
		}

		var receiverConversationID int64
		if err := session.QueryRowCtx(ctx, &receiverConversationID,
			"select `id` from `conversation` where `user_id` = ? and `target_user_id` = ? limit 1",
			receiverID, senderID); err != nil {
			return err
		}

		result, err := session.ExecCtx(ctx,
			"insert into `message` (`conversation_id`, `sender_id`, `receiver_id`, `content`, `msg_type`, `status`) values (?, ?, ?, ?, ?, 0)",
			receiverConversationID, senderID, receiverID, content, msgType)
		if err != nil {
			return err
		}
		messageID, err = result.LastInsertId()
		return err
	})
	if err != nil {
		return 0, err
	}
	return messageID, nil
}

func upsertConversationForMessage(ctx context.Context, session sqlx.Session, userID int64, targetUserID int64, content string, unreadIncrement int64) error {
	_, err := session.ExecCtx(ctx, `insert into conversation (user_id, target_user_id, last_message, last_message_time, unread_count)
values (?, ?, ?, now(), ?)
on duplicate key update last_message = values(last_message), last_message_time = values(last_message_time), unread_count = unread_count + ?`,
		userID, targetUserID, content, unreadIncrement, unreadIncrement)
	return err
}

func (m *customMessageCommandModel) MarkConversationRead(ctx context.Context, userID int64, targetUserID int64) (int64, error) {
	var affected int64
	err := m.conn.TransactCtx(ctx, func(ctx context.Context, session sqlx.Session) error {
		result, err := session.ExecCtx(ctx,
			"update `message` set `status` = 1 where `receiver_id` = ? and `sender_id` = ? and `status` = 0",
			userID, targetUserID)
		if err != nil {
			return err
		}
		affected, err = result.RowsAffected()
		if err != nil {
			return err
		}
		if affected == 0 {
			return nil
		}
		_, err = session.ExecCtx(ctx,
			"update `conversation` set `unread_count` = greatest(`unread_count` - ?, 0) where `user_id` = ? and `target_user_id` = ?",
			affected, userID, targetUserID)
		return err
	})
	if err != nil {
		return 0, err
	}
	return affected, nil
}

var _ sql.Result
