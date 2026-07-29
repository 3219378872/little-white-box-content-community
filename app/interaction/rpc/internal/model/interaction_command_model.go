package model

import (
	"context"
	"errors"
	"fmt"

	"esx/pkg/outboxx"

	"github.com/zeromicro/go-zero/core/stores/sqlx"
)

var ErrNoStateChange = errors.New("interaction state did not change")

type OutboxEnqueuer interface {
	Enqueue(ctx context.Context, session sqlx.Session, event outboxx.Event) error
}

type InteractionCommandModel interface {
	Like(ctx context.Context, userID, targetID, targetType int64, event outboxx.Event) (int64, error)
	Unlike(ctx context.Context, recordID, targetID, targetType int64, event outboxx.Event) error
	Favorite(ctx context.Context, userID, postID int64, event outboxx.Event) (int64, error)
	Unfavorite(ctx context.Context, recordID, postID int64, event outboxx.Event) error
}

type interactionCommandModel struct {
	conn   sqlx.SqlConn
	outbox OutboxEnqueuer
}

func NewInteractionCommandModel(conn sqlx.SqlConn, outbox OutboxEnqueuer) InteractionCommandModel {
	return &interactionCommandModel{conn: conn, outbox: outbox}
}

func (m *interactionCommandModel) Like(
	ctx context.Context,
	userID, targetID, targetType int64,
	event outboxx.Event,
) (int64, error) {
	if err := m.validate(); err != nil {
		return 0, err
	}
	var recordID int64
	err := m.conn.TransactCtx(ctx, func(ctx context.Context, session sqlx.Session) error {
		result, err := session.ExecCtx(ctx, `INSERT INTO like_record
            (user_id, target_id, target_type, status) VALUES (?, ?, ?, ?)
            ON DUPLICATE KEY UPDATE id = LAST_INSERT_ID(id), status = VALUES(status)`,
			userID, targetID, targetType, StatusActive,
		)
		if err != nil {
			return err
		}
		changed, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if changed == 0 {
			return ErrNoStateChange
		}
		recordID, err = result.LastInsertId()
		if err != nil {
			return err
		}
		if _, err = session.ExecCtx(ctx, `INSERT INTO action_count
            (target_id, target_type, like_count, favorite_count, comment_count, share_count)
            VALUES (?, ?, 1, 0, 0, 0)
            ON DUPLICATE KEY UPDATE like_count = like_count + 1`, targetID, targetType); err != nil {
			return err
		}
		return m.outbox.Enqueue(ctx, session, event)
	})
	return recordID, err
}

func (m *interactionCommandModel) Unlike(
	ctx context.Context,
	recordID, targetID, targetType int64,
	event outboxx.Event,
) error {
	if err := m.validate(); err != nil {
		return err
	}
	return m.conn.TransactCtx(ctx, func(ctx context.Context, session sqlx.Session) error {
		result, err := session.ExecCtx(ctx,
			"UPDATE like_record SET status = ? WHERE id = ? AND status = ?",
			StatusInactive, recordID, StatusActive,
		)
		if err != nil {
			return err
		}
		if err := requireStateChange(result); err != nil {
			return err
		}
		if _, err = session.ExecCtx(ctx, `UPDATE action_count
            SET like_count = GREATEST(like_count - 1, 0)
            WHERE target_id = ? AND target_type = ?`, targetID, targetType); err != nil {
			return err
		}
		return m.outbox.Enqueue(ctx, session, event)
	})
}

func (m *interactionCommandModel) Favorite(
	ctx context.Context,
	userID, postID int64,
	event outboxx.Event,
) (int64, error) {
	if err := m.validate(); err != nil {
		return 0, err
	}
	var recordID int64
	err := m.conn.TransactCtx(ctx, func(ctx context.Context, session sqlx.Session) error {
		result, err := session.ExecCtx(ctx, `INSERT INTO favorite (user_id, post_id, status)
            VALUES (?, ?, ?) ON DUPLICATE KEY UPDATE id = LAST_INSERT_ID(id), status = VALUES(status)`,
			userID, postID, StatusActive,
		)
		if err != nil {
			return err
		}
		changed, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if changed == 0 {
			return ErrNoStateChange
		}
		recordID, err = result.LastInsertId()
		if err != nil {
			return err
		}
		if _, err = session.ExecCtx(ctx, `INSERT INTO action_count
            (target_id, target_type, like_count, favorite_count, comment_count, share_count)
            VALUES (?, 1, 0, 1, 0, 0)
            ON DUPLICATE KEY UPDATE favorite_count = favorite_count + 1`, postID); err != nil {
			return err
		}
		return m.outbox.Enqueue(ctx, session, event)
	})
	return recordID, err
}

func (m *interactionCommandModel) Unfavorite(
	ctx context.Context,
	recordID, postID int64,
	event outboxx.Event,
) error {
	if err := m.validate(); err != nil {
		return err
	}
	return m.conn.TransactCtx(ctx, func(ctx context.Context, session sqlx.Session) error {
		result, err := session.ExecCtx(ctx,
			"UPDATE favorite SET status = ? WHERE id = ? AND status = ?",
			StatusInactive, recordID, StatusActive,
		)
		if err != nil {
			return err
		}
		if err := requireStateChange(result); err != nil {
			return err
		}
		if _, err = session.ExecCtx(ctx, `UPDATE action_count
            SET favorite_count = GREATEST(favorite_count - 1, 0)
            WHERE target_id = ? AND target_type = 1`, postID); err != nil {
			return err
		}
		return m.outbox.Enqueue(ctx, session, event)
	})
}

func (m *interactionCommandModel) validate() error {
	if m == nil || m.conn == nil || m.outbox == nil {
		return fmt.Errorf("interaction command model is not configured")
	}
	return nil
}

func requireStateChange(result interface{ RowsAffected() (int64, error) }) error {
	changed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if changed != 1 {
		return ErrNoStateChange
	}
	return nil
}
