package model

import (
	"context"
	"database/sql"
	"errors"

	sqlx "esx/pkg/sqlstore"
)

var _ MessageCommandModel = (*customMessageCommandModel)(nil)

type (
	MessageCommandResult struct {
		MessageID int64
		Created   bool
	}

	IdempotencyConflictError struct{}

	MessageCommandModel interface {
		CreateMessageWithConversations(ctx context.Context, senderID int64, receiverID int64, content string, msgType int64, mediaID int64, idempotencyKey string) (MessageCommandResult, error)
		MarkConversationRead(ctx context.Context, userID int64, targetUserID int64) (int64, error)
	}

	customMessageCommandModel struct {
		conn sqlx.SqlConn
	}
)

// Error describes an idempotency key reused with a different message.
func (*IdempotencyConflictError) Error() string {
	return "idempotency key is already bound to another message command"
}

// IsIdempotencyConflict reports whether err is an idempotency conflict.
func IsIdempotencyConflict(err error) bool {
	var conflict *IdempotencyConflictError
	return errors.As(err, &conflict)
}

// NewMessageCommandModel creates the transactional message write model.
func NewMessageCommandModel(conn sqlx.SqlConn) MessageCommandModel {
	return &customMessageCommandModel{conn: conn}
}

// CreateMessageWithConversations inserts a message and updates both sides'
// conversations in one transaction. A repeated idempotency key returns the
// original message when the payload matches and a conflict otherwise.
func (m *customMessageCommandModel) CreateMessageWithConversations(ctx context.Context, senderID int64, receiverID int64, content string, msgType int64, mediaID int64, idempotencyKey string) (MessageCommandResult, error) {
	var commandResult MessageCommandResult
	err := m.conn.TransactCtx(ctx, func(ctx context.Context, session sqlx.Session) error {
		existing, err := findMessageCommand(ctx, session, senderID, idempotencyKey)
		if err == nil {
			if !existing.matches(receiverID, content, msgType, mediaID) {
				return &IdempotencyConflictError{}
			}
			commandResult.MessageID = existing.ID
			return nil
		}
		if !errors.Is(err, sqlx.ErrNotFound) {
			return err
		}

		// The sender's conversation gains no unread count; the receiver's gains one.
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
			"insert into `message` (`conversation_id`, `sender_id`, `receiver_id`, `content`, `msg_type`, `media_id`, `idempotency_key`, `status`) values (?, ?, ?, ?, ?, ?, ?, 0)",
			receiverConversationID, senderID, receiverID, content, msgType, mediaID, idempotencyKey)
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
		if !existing.matches(receiverID, content, msgType, mediaID) {
			return MessageCommandResult{}, &IdempotencyConflictError{}
		}
		return MessageCommandResult{MessageID: existing.ID}, nil
	}
	return MessageCommandResult{}, err
}

// messageCommandQuerier is satisfied by both a connection and a transaction.
type messageCommandQuerier interface {
	QueryRowCtx(ctx context.Context, v any, query string, args ...any) error
}

// storedMessageCommand is the stored payload compared on idempotent replay.
type storedMessageCommand struct {
	ID         int64  `db:"id"`
	ReceiverID int64  `db:"receiver_id"`
	Content    string `db:"content"`
	MsgType    int64  `db:"msg_type"`
	MediaID    int64  `db:"media_id"`
}

// findMessageCommand looks up the message a sender created with an idempotency key.
func findMessageCommand(ctx context.Context, querier messageCommandQuerier, senderID int64, idempotencyKey string) (*storedMessageCommand, error) {
	var command storedMessageCommand
	err := querier.QueryRowCtx(ctx, &command,
		"select `id`, `receiver_id`, `content`, `msg_type`, `media_id` from `message` where `sender_id` = ? and `idempotency_key` = ? limit 1",
		senderID, idempotencyKey)
	if err != nil {
		return nil, err
	}
	return &command, nil
}

// matches reports whether a replayed command carries the same payload.
func (c *storedMessageCommand) matches(receiverID int64, content string, msgType int64, mediaID int64) bool {
	return c.ReceiverID == receiverID && c.Content == content && c.MsgType == msgType && c.MediaID == mediaID
}

// upsertConversationForMessage creates or refreshes one side's conversation with
// the latest message and adds unreadIncrement to its unread count.
func upsertConversationForMessage(ctx context.Context, session sqlx.Session, userID int64, targetUserID int64, content string, unreadIncrement int64) error {
	_, err := session.ExecCtx(ctx, `insert into conversation (user_id, target_user_id, last_message, last_message_time, unread_count)
values (?, ?, ?, now(), ?)
on duplicate key update last_message = values(last_message), last_message_time = values(last_message_time), unread_count = unread_count + ?`,
		userID, targetUserID, content, unreadIncrement, unreadIncrement)
	return err
}

// MarkConversationRead marks the peer's messages read and lowers the
// conversation's unread count by the same amount (never below zero), atomically.
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
