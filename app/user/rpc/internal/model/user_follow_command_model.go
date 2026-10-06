package model

import (
	"context"
	"fmt"

	"esx/pkg/outboxx"

	sqlx "esx/pkg/sqlstore"
)

// OutboxEnqueuer 在业务事务内写入 outbox 事件。
type OutboxEnqueuer interface {
	Enqueue(ctx context.Context, session sqlx.Session, event outboxx.Event) error
}

// UserFollowCommandModel 是关注关系的事务写入。
type UserFollowCommandModel interface {
	Follow(ctx context.Context, userID, targetUserID int64, event outboxx.Event) error
	Unfollow(ctx context.Context, userID, targetUserID int64, event outboxx.Event) error
}

// userFollowCommandModel 是基于 MySQL 事务的实现。
type userFollowCommandModel struct {
	conn   sqlx.SqlConn
	outbox OutboxEnqueuer
}

// NewUserFollowCommandModel 创建关注写模型；outbox 必须与业务写入共用同一连接。
func NewUserFollowCommandModel(conn sqlx.SqlConn, outbox OutboxEnqueuer) UserFollowCommandModel {
	return &userFollowCommandModel{conn: conn, outbox: outbox}
}

// Follow 写入关注关系并更新双方计数，同事务写入行为事件；已关注时什么都不做，计数与事件都不重复。
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

// Unfollow 删除关注关系并减少双方计数，同事务写入行为事件；未关注时什么都不做。
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

// validate 确认连接与 outbox 已配置，避免写入关系却丢失事件。
func (m *userFollowCommandModel) validate() error {
	if m == nil || m.conn == nil || m.outbox == nil {
		return fmt.Errorf("user follow command model is not configured")
	}
	return nil
}
