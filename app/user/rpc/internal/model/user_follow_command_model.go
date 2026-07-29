package model

import (
	"context"
	"fmt"

	"esx/pkg/outboxx"

	"github.com/zeromicro/go-zero/core/stores/sqlx"
)

type OutboxEnqueuer interface {
	Enqueue(ctx context.Context, session sqlx.Session, event outboxx.Event) error
}

type UserFollowCommandModel interface {
	Follow(ctx context.Context, userID, targetUserID int64, event outboxx.Event) error
	Unfollow(ctx context.Context, userID, targetUserID int64, event outboxx.Event) error
}

type userFollowCommandModel struct {
	conn   sqlx.SqlConn
	outbox OutboxEnqueuer
}

func NewUserFollowCommandModel(conn sqlx.SqlConn, outbox OutboxEnqueuer) UserFollowCommandModel {
	return &userFollowCommandModel{conn: conn, outbox: outbox}
}

func (m *userFollowCommandModel) Follow(
	ctx context.Context,
	userID, targetUserID int64,
	event outboxx.Event,
) error {
	if err := m.validate(); err != nil {
		return err
	}
	return m.conn.TransactCtx(ctx, func(ctx context.Context, session sqlx.Session) error {
		result, err := session.ExecCtx(ctx,
			"INSERT IGNORE INTO user_follow (user_id, target_user_id) VALUES (?, ?)",
			userID, targetUserID,
		)
		if err != nil {
			return err
		}
		changed, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if changed == 0 {
			return nil
		}
		if _, err = session.ExecCtx(ctx,
			"UPDATE user_profile SET following_count = following_count + 1 WHERE id = ?", userID,
		); err != nil {
			return err
		}
		if _, err = session.ExecCtx(ctx,
			"UPDATE user_profile SET follower_count = follower_count + 1 WHERE id = ?", targetUserID,
		); err != nil {
			return err
		}
		return m.outbox.Enqueue(ctx, session, event)
	})
}

func (m *userFollowCommandModel) Unfollow(
	ctx context.Context,
	userID, targetUserID int64,
	event outboxx.Event,
) error {
	if err := m.validate(); err != nil {
		return err
	}
	return m.conn.TransactCtx(ctx, func(ctx context.Context, session sqlx.Session) error {
		result, err := session.ExecCtx(ctx,
			"DELETE FROM user_follow WHERE user_id = ? AND target_user_id = ?", userID, targetUserID,
		)
		if err != nil {
			return err
		}
		changed, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if changed == 0 {
			return nil
		}
		if _, err = session.ExecCtx(ctx,
			"UPDATE user_profile SET following_count = GREATEST(following_count - 1, 0) WHERE id = ?", userID,
		); err != nil {
			return err
		}
		if _, err = session.ExecCtx(ctx,
			"UPDATE user_profile SET follower_count = GREATEST(follower_count - 1, 0) WHERE id = ?", targetUserID,
		); err != nil {
			return err
		}
		return m.outbox.Enqueue(ctx, session, event)
	})
}

func (m *userFollowCommandModel) validate() error {
	if m == nil || m.conn == nil || m.outbox == nil {
		return fmt.Errorf("user follow command model is not configured")
	}
	return nil
}
