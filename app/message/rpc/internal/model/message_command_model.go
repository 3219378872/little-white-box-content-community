package model

import (
	"context"
	"database/sql"
	"errors"

	"github.com/zeromicro/go-zero/core/stores/sqlx"
)

var _ MessageCommandModel = (*customMessageCommandModel)(nil)

type (
	MessageCommandResult struct {
		MessageID int64
		Created   bool
	}

	IdempotencyConflictError struct{}

	MessageCommandModel interface {
		CreateMessageWithConversations(ctx context.Context, senderID int64, receiverID int64, content string, msgType int64, idempotencyKey string) (MessageCommandResult, error)
		MarkConversationRead(ctx context.Context, userID int64, targetUserID int64) (int64, error)
	}

	customMessageCommandModel struct {
		conn sqlx.SqlConn
	}
)

func (*IdempotencyConflictError) Error() string {
	return "idempotency key is already bound to another message command"
}

func IsIdempotencyConflict(err error) bool {
	var conflict *IdempotencyConflictError
	return errors.As(err, &conflict)
}

func NewMessageCommandModel(conn sqlx.SqlConn) MessageCommandModel {
	return &customMessageCommandModel{conn: conn}
}

func (m *customMessageCommandModel) CreateMessageWithConversations(ctx context.Context, senderID int64, receiverID int64, content string, msgType int64, idempotencyKey string) (MessageCommandResult, error) {
	var commandResult MessageCommandResult
	err := m.conn.TransactCtx(ctx, func(ctx context.Context, session sqlx.Session) error {
		existing, err := findMessageCommand(ctx, session, senderID, idempotencyKey)
		if err == nil {
			if !existing.matches(receiverID, content, msgType) {
				return &IdempotencyConflictError{}
			}
			commandResult.MessageID = existing.ID
			return nil
		}
		if !errors.Is(err, sqlx.ErrNotFound) {
			return err
		}

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
			"insert into `message` (`conversation_id`, `sender_id`, `receiver_id`, `content`, `msg_type`, `idempotency_key`, `status`) values (?, ?, ?, ?, ?, ?, 0)",
			receiverConversationID, senderID, receiverID, content, msgType, idempotencyKey)
		if err != nil {
			return err
		}
		commandResult.MessageID, err = result.LastInsertId()
		commandResult.Created = err == nil
		return err
	})
	if err == nil {
		return commandResult, nil
	}

	// A concurrent request can win the unique-key race after the initial lookup.
	// The failed transaction has rolled back its conversation updates, so it is
	// safe to resolve the winner and return the original message id.
	existing, lookupErr := findMessageCommand(ctx, m.conn, senderID, idempotencyKey)
	if lookupErr == nil {
		if !existing.matches(receiverID, content, msgType) {
			return MessageCommandResult{}, &IdempotencyConflictError{}
		}
		return MessageCommandResult{MessageID: existing.ID}, nil
	}
	return MessageCommandResult{}, err
}

type messageCommandQuerier interface {
	QueryRowCtx(ctx context.Context, v any, query string, args ...any) error
}

type storedMessageCommand struct {
	ID         int64  `db:"id"`
	ReceiverID int64  `db:"receiver_id"`
	Content    string `db:"content"`
	MsgType    int64  `db:"msg_type"`
}

func findMessageCommand(ctx context.Context, querier messageCommandQuerier, senderID int64, idempotencyKey string) (*storedMessageCommand, error) {
	var command storedMessageCommand
	err := querier.QueryRowCtx(ctx, &command,
		"select `id`, `receiver_id`, `content`, `msg_type` from `message` where `sender_id` = ? and `idempotency_key` = ? limit 1",
		senderID, idempotencyKey)
	if err != nil {
		return nil, err
	}
	return &command, nil
}

func (c *storedMessageCommand) matches(receiverID int64, content string, msgType int64) bool {
	return c.ReceiverID == receiverID && c.Content == content && c.MsgType == msgType
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
