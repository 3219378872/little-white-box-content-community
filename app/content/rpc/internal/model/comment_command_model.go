package model

import (
	"context"
	"esx/pkg/idempotencyx"
	"fmt"

	"esx/pkg/outboxx"

	"github.com/zeromicro/go-zero/core/stores/sqlx"
)

type CommentCommandModel interface {
	CreateComment(ctx context.Context, comment *Comment, event outboxx.Event, idem idempotencyx.IdempotencyRecord) (commentID int64, created bool, err error)
	DeleteComment(ctx context.Context, comment *Comment) error
}

type commentCommandModel struct {
	conn   sqlx.SqlConn
	outbox OutboxEnqueuer
}

func NewCommentCommandModel(conn sqlx.SqlConn, outbox OutboxEnqueuer) CommentCommandModel {
	return &commentCommandModel{conn: conn, outbox: outbox}
}

func (m *commentCommandModel) CreateComment(ctx context.Context, comment *Comment, event outboxx.Event, idem idempotencyx.IdempotencyRecord) (commentID int64, created bool, err error) {
	if comment == nil || m.conn == nil || m.outbox == nil {
		return 0, false, fmt.Errorf("comment command model is not configured")
	}
	err = m.conn.TransactCtx(ctx, func(ctx context.Context, session sqlx.Session) error {
		resourceID, shouldCreate, err := idempotencyx.ResolveIdempotencySession(ctx, session, idem, comment.Id, comment.Id)
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
		if comment.ParentId.Valid {
			// 楼中楼回复：同事务维护父评论回复数；父评论在提交前被删除则整体回滚。
			result, err := session.ExecCtx(ctx,
				"UPDATE comment SET reply_count = reply_count + 1 WHERE id = ? AND status <> 0",
				comment.ParentId.Int64,
			)
			if err != nil {
				return err
			}
			changed, err := result.RowsAffected()
			if err != nil {
				return err
			}
			if changed != 1 {
				return fmt.Errorf("parent comment %d is unavailable", comment.ParentId.Int64)
			}
		}
		result, err := session.ExecCtx(ctx,
			"UPDATE post SET comment_count = comment_count + 1 WHERE id = ? AND status = 1",
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
			return ErrTargetNotInteractable
		}
		commentID = comment.Id
		created = true
		return m.outbox.Enqueue(ctx, session, event)
	})
	return commentID, created, err
}

// DeleteComment 软删评论并保持计数一致：
//   - 删除子回复：父评论 reply_count 回减；
//   - 删除顶级评论：级联软删其全部可见子回复，post.comment_count 按实际行数回减。
func (m *commentCommandModel) DeleteComment(ctx context.Context, comment *Comment) error {
	if comment == nil || comment.Id <= 0 || m.conn == nil {
		return fmt.Errorf("comment command model is not configured")
	}
	return m.conn.TransactCtx(ctx, func(ctx context.Context, session sqlx.Session) error {
		result, err := session.ExecCtx(ctx,
			"UPDATE comment SET status = 0 WHERE id = ? AND status <> 0", comment.Id,
		)
		if err != nil {
			return err
		}
		changed, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if changed != 1 {
			return fmt.Errorf("comment %d was not updated", comment.Id)
		}

		postDelta := int64(1)
		if comment.ParentId.Valid {
			if _, err := session.ExecCtx(ctx,
				"UPDATE comment SET reply_count = GREATEST(reply_count - 1, 0) WHERE id = ?",
				comment.ParentId.Int64,
			); err != nil {
				return err
			}
		} else {
			cascade, err := session.ExecCtx(ctx,
				"UPDATE comment SET status = 0 WHERE parent_id = ? AND status <> 0", comment.Id,
			)
			if err != nil {
				return err
			}
			n, err := cascade.RowsAffected()
			if err != nil {
				return err
			}
			postDelta += n
		}

		_, err = session.ExecCtx(ctx,
			"UPDATE post SET comment_count = GREATEST(comment_count - ?, 0) WHERE id = ?",
			postDelta, comment.PostId,
		)
		return err
	})
}
