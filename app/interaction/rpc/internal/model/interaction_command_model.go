package model

import (
	"context"
	"errors"
	"fmt"

	"esx/pkg/outboxx"

	sqlx "esx/pkg/sqlstore"
)

var ErrNoStateChange = errors.New("interaction state did not change")

// targetTypePost 是收藏唯一支持的目标类型（帖子）。
const targetTypePost int64 = 1

// OutboxEnqueuer 在业务事务内写入 outbox 事件。
type OutboxEnqueuer interface {
	Enqueue(ctx context.Context, session sqlx.Session, event outboxx.Event) error
}

// CountSnapshot 是本事务更新计数后目标在 action_count 中的绝对计数与序号。
// Seq 在计数行的行锁内递增，同一目标的快照因此可以按 Seq 全序比较。
type CountSnapshot struct {
	LikeCount     int64 `db:"like_count"`
	FavoriteCount int64 `db:"favorite_count"`
	Seq           int64 `db:"count_seq"`
}

// EventBuilder 在事务内根据最新计数快照构造 outbox 事件，使事件携带与状态同时提交的权威计数。
type EventBuilder func(CountSnapshot) (outboxx.Event, error)

// InteractionCommandModel 是点赞/收藏的事务写入：状态、计数与行为事件一起提交。
type InteractionCommandModel interface {
	Like(ctx context.Context, userID, targetID, targetType int64, build EventBuilder) (int64, error)
	Unlike(ctx context.Context, recordID, targetID, targetType int64, build EventBuilder) error
	Favorite(ctx context.Context, userID, postID int64, build EventBuilder) (int64, error)
	Unfavorite(ctx context.Context, recordID, postID int64, build EventBuilder) error
}

// interactionCommandModel 是基于 MySQL 事务的实现。
type interactionCommandModel struct {
	conn   sqlx.SqlConn
	outbox OutboxEnqueuer
}

// NewInteractionCommandModel 创建互动写模型；outbox 必须与业务写入共用同一连接。
func NewInteractionCommandModel(conn sqlx.SqlConn, outbox OutboxEnqueuer) InteractionCommandModel {
	return &interactionCommandModel{conn: conn, outbox: outbox}
}

// 计数行的 upsert：行不存在时以本次变化后的值创建（序号从 1 开始），存在时调整计数并递增序号。
// 递减也走 upsert，保证事务内总能读回快照；GREATEST 防止历史数据异常时减成负数。
const (
	incrLikeCountSQL = `INSERT INTO action_count
            (target_id, target_type, like_count, favorite_count, comment_count, share_count, count_seq)
            VALUES (?, ?, 1, 0, 0, 0, 1)
            ON DUPLICATE KEY UPDATE like_count = like_count + 1, count_seq = count_seq + 1`
	decrLikeCountSQL = `INSERT INTO action_count
            (target_id, target_type, like_count, favorite_count, comment_count, share_count, count_seq)
            VALUES (?, ?, 0, 0, 0, 0, 1)
            ON DUPLICATE KEY UPDATE like_count = GREATEST(like_count - 1, 0), count_seq = count_seq + 1`
	incrFavoriteCountSQL = `INSERT INTO action_count
            (target_id, target_type, like_count, favorite_count, comment_count, share_count, count_seq)
            VALUES (?, ?, 0, 1, 0, 0, 1)
            ON DUPLICATE KEY UPDATE favorite_count = favorite_count + 1, count_seq = count_seq + 1`
	decrFavoriteCountSQL = `INSERT INTO action_count
            (target_id, target_type, like_count, favorite_count, comment_count, share_count, count_seq)
            VALUES (?, ?, 0, 0, 0, 0, 1)
            ON DUPLICATE KEY UPDATE favorite_count = GREATEST(favorite_count - 1, 0), count_seq = count_seq + 1`
	// 本事务刚写过该行并持有行锁，读到的就是本次变化后的计数与序号。
	countSnapshotSQL = `SELECT like_count, favorite_count, count_seq FROM action_count
            WHERE target_id = ? AND target_type = ? LIMIT 1`
)

// Like 激活点赞记录并增加点赞数，同事务写入带计数快照的行为事件，返回记录 ID。
// 已处于点赞状态时 upsert 不改变任何行，返回 ErrNoStateChange，计数不会重复增加。
func (m *interactionCommandModel) Like(
	ctx context.Context,
	userID, targetID, targetType int64,
	build EventBuilder,
) (int64, error) {
	if err := m.validate(build); err != nil {
		return 0, err
	}
	var recordID int64
	err := m.conn.TransactCtx(ctx, func(ctx context.Context, session sqlx.Session) error {
		// 激活或复活点赞记录；LAST_INSERT_ID(id) 让既有行也能返回记录 ID。
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
		// 影响 0 行说明已是点赞状态，不调整计数也不发事件。
		if changed == 0 {
			return ErrNoStateChange
		}
		recordID, err = result.LastInsertId()
		if err != nil {
			return err
		}
		if _, err = session.ExecCtx(ctx, incrLikeCountSQL, targetID, targetType); err != nil {
			return err
		}
		return m.enqueueWithSnapshot(ctx, session, targetID, targetType, build)
	})
	return recordID, err
}

// Unlike 把点赞记录从激活改为取消并减少点赞数，同事务写入带计数快照的行为事件；
// 已取消时返回 ErrNoStateChange。
func (m *interactionCommandModel) Unlike(
	ctx context.Context,
	recordID, targetID, targetType int64,
	build EventBuilder,
) error {
	if err := m.validate(build); err != nil {
		return err
	}
	return m.conn.TransactCtx(ctx, func(ctx context.Context, session sqlx.Session) error {
		// 条件更新保证并发取消只有一个成功，计数只减一次。
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
		if _, err = session.ExecCtx(ctx, decrLikeCountSQL, targetID, targetType); err != nil {
			return err
		}
		return m.enqueueWithSnapshot(ctx, session, targetID, targetType, build)
	})
}

// Favorite 激活收藏记录并增加收藏数，同事务写入带计数快照的行为事件，返回记录 ID；
// 已收藏时返回 ErrNoStateChange。
func (m *interactionCommandModel) Favorite(
	ctx context.Context,
	userID, postID int64,
	build EventBuilder,
) (int64, error) {
	if err := m.validate(build); err != nil {
		return 0, err
	}
	var recordID int64
	err := m.conn.TransactCtx(ctx, func(ctx context.Context, session sqlx.Session) error {
		// 激活或复活收藏记录；LAST_INSERT_ID(id) 让既有行也能返回记录 ID。
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
		// 影响 0 行说明已是收藏状态，不调整计数也不发事件。
		if changed == 0 {
			return ErrNoStateChange
		}
		recordID, err = result.LastInsertId()
		if err != nil {
			return err
		}
		if _, err = session.ExecCtx(ctx, incrFavoriteCountSQL, postID, targetTypePost); err != nil {
			return err
		}
		return m.enqueueWithSnapshot(ctx, session, postID, targetTypePost, build)
	})
	return recordID, err
}

// Unfavorite 把收藏记录从激活改为取消并减少收藏数，同事务写入带计数快照的行为事件；
// 已取消时返回 ErrNoStateChange。
func (m *interactionCommandModel) Unfavorite(
	ctx context.Context,
	recordID, postID int64,
	build EventBuilder,
) error {
	if err := m.validate(build); err != nil {
		return err
	}
	return m.conn.TransactCtx(ctx, func(ctx context.Context, session sqlx.Session) error {
		// 条件更新保证并发取消只有一个成功，计数只减一次。
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
		if _, err = session.ExecCtx(ctx, decrFavoriteCountSQL, postID, targetTypePost); err != nil {
			return err
		}
		return m.enqueueWithSnapshot(ctx, session, postID, targetTypePost, build)
	})
}

// enqueueWithSnapshot 读回本事务刚更新的计数行，据此构造事件并在同一事务入队，
// 下游拿到的计数与关系状态同时提交，不会出现事件与计数不一致。
func (m *interactionCommandModel) enqueueWithSnapshot(
	ctx context.Context,
	session sqlx.Session,
	targetID, targetType int64,
	build EventBuilder,
) error {
	var snapshot CountSnapshot
	if err := session.QueryRowCtx(ctx, &snapshot, countSnapshotSQL, targetID, targetType); err != nil {
		return fmt.Errorf("read action count snapshot: %w", err)
	}
	event, err := build(snapshot)
	if err != nil {
		return fmt.Errorf("build interaction event: %w", err)
	}
	return m.outbox.Enqueue(ctx, session, event)
}

// validate 确认连接、outbox 与事件构造器已配置，避免写入状态却丢失事件。
func (m *interactionCommandModel) validate(build EventBuilder) error {
	if m == nil || m.conn == nil || m.outbox == nil || build == nil {
		return fmt.Errorf("interaction command model is not configured")
	}
	return nil
}

// requireStateChange 要求条件更新恰好改变一行，否则说明状态已是目标值。
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
