package model

import (
	"context"
	"encoding/json"
	"errors"

	behavior "esx/pkg/event"
	"fmt"

	"esx/pkg/outboxx"

	sqlx "esx/pkg/sqlstore"
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
		// Insert a placeholder and activate only an inactive row. A duplicate
		// active row matches no UPDATE predicate, even with clientFoundRows.
		if _, err := session.ExecCtx(ctx, `INSERT IGNORE INTO like_record
            (user_id, target_id, target_type, status) VALUES (?, ?, ?, ?)`,
			userID, targetID, targetType, StatusInactive); err != nil {
			return err
		}
		result, err := session.ExecCtx(ctx, `UPDATE like_record SET id = LAST_INSERT_ID(id), status = ?
            WHERE user_id = ? AND target_id = ? AND target_type = ? AND status = ?`,
			StatusActive, userID, targetID, targetType, StatusInactive)
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
            (target_id, target_type, like_count, favorite_count, comment_count, share_count, revision)
            VALUES (?, ?, 1, 0, 0, 0, 1)
            ON DUPLICATE KEY UPDATE like_count = like_count + 1, revision = revision + 1`, targetID, targetType); err != nil {
			return err
		}
		return m.enqueueCountSnapshot(ctx, session, targetID, targetType, event)
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
            SET like_count = GREATEST(like_count - 1, 0), revision = revision + 1
            WHERE target_id = ? AND target_type = ?`, targetID, targetType); err != nil {
			return err
		}
		return m.enqueueCountSnapshot(ctx, session, targetID, targetType, event)
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
		if _, err := session.ExecCtx(ctx, `INSERT IGNORE INTO favorite (user_id, post_id, status)
            VALUES (?, ?, ?)`, userID, postID, StatusInactive); err != nil {
			return err
		}
		result, err := session.ExecCtx(ctx, `UPDATE favorite SET id = LAST_INSERT_ID(id), status = ?
            WHERE user_id = ? AND post_id = ? AND status = ?`,
			StatusActive, userID, postID, StatusInactive)
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
            (target_id, target_type, like_count, favorite_count, comment_count, share_count, revision)
            VALUES (?, 1, 0, 1, 0, 0, 1)
            ON DUPLICATE KEY UPDATE favorite_count = favorite_count + 1, revision = revision + 1`, postID); err != nil {
			return err
		}
		return m.enqueueCountSnapshot(ctx, session, postID, 1, event)
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
            SET favorite_count = GREATEST(favorite_count - 1, 0), revision = revision + 1
            WHERE target_id = ? AND target_type = 1`, postID); err != nil {
			return err
		}
		return m.enqueueCountSnapshot(ctx, session, postID, 1, event)
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

// The mutation already holds the action_count row lock. Read and serialize its
// snapshot before enqueuing, so a later transaction cannot supply this version.
func (m *interactionCommandModel) enqueueCountSnapshot(ctx context.Context, session sqlx.Session, targetID, targetType int64, out outboxx.Event) error {
	var snapshot behavior.InteractionCountSnapshot
	if err := session.QueryRowCtx(ctx, &snapshot, "SELECT revision, like_count, favorite_count FROM action_count WHERE target_id = ? AND target_type = ?", targetID, targetType); err != nil {
		return err
	}
	// Preserve any additive event fields owned by other consumers.
	var payload map[string]json.RawMessage
	if err := json.Unmarshal(out.Payload, &payload); err != nil {
		return fmt.Errorf("decode interaction event: %w", err)
	}
	if payload == nil {
		return fmt.Errorf("interaction event must be an object")
	}
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		return err
	}
	payload["count_snapshot"] = encoded
	out.Payload, err = json.Marshal(payload)
	if err != nil {
		return err
	}
	return m.outbox.Enqueue(ctx, session, out)
}
