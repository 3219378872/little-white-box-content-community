package model

import (
	"context"
	"fmt"

	"esx/pkg/outboxx"

	"github.com/zeromicro/go-zero/core/stores/sqlx"
)

type CommentCommandModel interface {
	CreateComment(ctx context.Context, comment *Comment, event outboxx.Event, idem IdempotencyRecord) (commentID int64, created bool, err error)
	DeleteComment(ctx context.Context, commentID, postID int64) error
}

type commentCommandModel struct {
	conn   sqlx.SqlConn
	outbox OutboxEnqueuer
}

func NewCommentCommandModel(conn sqlx.SqlConn, outbox OutboxEnqueuer) CommentCommandModel {
	return &commentCommandModel{conn: conn, outbox: outbox}
}

func (m *commentCommandModel) CreateComment(ctx context.Context, comment *Comment, event outboxx.Event, idem IdempotencyRecord) (commentID int64, created bool, err error) {
	if comment == nil || m.conn == nil || m.outbox == nil {
		return 0, false, fmt.Errorf("comment command model is not configured")
	}
	err = m.conn.TransactCtx(ctx, func(ctx context.Context, session sqlx.Session) error {
		resourceID, shouldCreate, err := resolveIdempotencySession(ctx, session, idem, comment.Id, comment.Id)
		if err != nil {
			return err
		}
		if !shouldCreate {
			commentID = resourceID
			return nil
		}
		if _, err := session.ExecCtx(ctx, `INSERT INTO comment
            (id, post_id, user_id, parent_id, reply_user_id, content, status)
            VALUES (?, ?, ?, ?, ?, ?, ?)`,
			comment.Id, comment.PostId, comment.UserId, comment.ParentId,
			comment.ReplyUserId, comment.Content, comment.Status,
		); err != nil {
			return err
		}
		result, err := session.ExecCtx(ctx,
			"UPDATE post SET comment_count = comment_count + 1 WHERE id = ? AND status <> 2",
			comment.PostId,
		)
		if err != nil {
			return err
		}
		changed, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if changed != 1 {
			return fmt.Errorf("post %d is unavailable", comment.PostId)
		}
		commentID = comment.Id
		created = true
		return m.outbox.Enqueue(ctx, session, event)
	})
	return commentID, created, err
}

func (m *commentCommandModel) DeleteComment(ctx context.Context, commentID, postID int64) error {
	if commentID <= 0 || postID <= 0 || m.conn == nil {
		return fmt.Errorf("comment command model is not configured")
	}
	return m.conn.TransactCtx(ctx, func(ctx context.Context, session sqlx.Session) error {
		result, err := session.ExecCtx(ctx,
			"UPDATE comment SET status = 0 WHERE id = ? AND status <> 0", commentID,
		)
		if err != nil {
			return err
		}
		changed, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if changed != 1 {
			return fmt.Errorf("comment %d was not updated", commentID)
		}
		_, err = session.ExecCtx(ctx,
			"UPDATE post SET comment_count = GREATEST(comment_count - 1, 0) WHERE id = ?", postID,
		)
		return err
	})
}
