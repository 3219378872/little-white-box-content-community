package model

import (
	"context"
	"esx/pkg/idempotencyx"
	"fmt"
	"sort"
	"strings"

	"esx/pkg/outboxx"

	"github.com/zeromicro/go-zero/core/stores/sqlx"
)

type OutboxEnqueuer interface {
	Enqueue(ctx context.Context, session sqlx.Session, event outboxx.Event) error
}

type PostCommandModel interface {
	CreatePost(ctx context.Context, post *Post, tags []string, tagIDs []int64, event outboxx.Event, idem idempotencyx.IdempotencyRecord) (postID int64, created bool, err error)
	UpdatePost(ctx context.Context, postID int64, fields map[string]any, tags []string, tagIDs []int64, event outboxx.Event, expectedRevision int64) error
	DeletePost(ctx context.Context, postID int64, event outboxx.Event, expectedRevision int64) error
}

type postCommandModel struct {
	conn   sqlx.SqlConn
	outbox OutboxEnqueuer
}

func NewPostCommandModel(conn sqlx.SqlConn, outbox OutboxEnqueuer) PostCommandModel {
	return &postCommandModel{conn: conn, outbox: outbox}
}

func (m *postCommandModel) CreatePost(
	ctx context.Context,
	post *Post,
	tags []string,
	tagIDs []int64,
	event outboxx.Event,
	idem idempotencyx.IdempotencyRecord,
) (postID int64, created bool, err error) {
	if post == nil || m.conn == nil || m.outbox == nil {
		return 0, false, fmt.Errorf("content command model is not configured")
	}
	if len(tags) != len(tagIDs) {
		return 0, false, fmt.Errorf("tags and tag ids length mismatch")
	}
	err = m.conn.TransactCtx(ctx, func(ctx context.Context, session sqlx.Session) error {
		resourceID, shouldCreate, err := idempotencyx.ResolveIdempotencySession(ctx, session, idem, post.Id, post.Id)
		if err != nil {
			return err
		}
		if !shouldCreate {
			postID = resourceID
			return nil
		}
		if err := insertPostSession(ctx, session, post); err != nil {
			return err
		}
		if err := insertPostTagsSession(ctx, session, post.Id, tags, tagIDs); err != nil {
			return err
		}
		postID = post.Id
		created = true
		return m.outbox.Enqueue(ctx, session, event)
	})
	return postID, created, err
}

func (m *postCommandModel) UpdatePost(
	ctx context.Context,
	postID int64,
	fields map[string]any,
	tags []string,
	tagIDs []int64,
	event outboxx.Event,
	expectedRevision int64,
) error {
	if postID <= 0 || m.conn == nil || m.outbox == nil {
		return fmt.Errorf("content command model is not configured")
	}
	if len(tags) != len(tagIDs) {
		return fmt.Errorf("tags and tag ids length mismatch")
	}
	return m.conn.TransactCtx(ctx, func(ctx context.Context, session sqlx.Session) error {
		if err := updatePostFieldsSession(ctx, session, postID, fields, expectedRevision); err != nil {
			return err
		}
		if _, err := session.ExecCtx(ctx, "DELETE FROM `post_tag` WHERE `post_id` = ?", postID); err != nil {
			return err
		}
		if err := insertPostTagsSession(ctx, session, postID, tags, tagIDs); err != nil {
			return err
		}
		return m.outbox.Enqueue(ctx, session, event)
	})
}

func (m *postCommandModel) DeletePost(ctx context.Context, postID int64, event outboxx.Event, expectedRevision int64) error {
	if postID <= 0 || m.conn == nil || m.outbox == nil {
		return fmt.Errorf("content command model is not configured")
	}
	return m.conn.TransactCtx(ctx, func(ctx context.Context, session sqlx.Session) error {
		// expectedRevision=0 表示迁移期旧客户端（CORE-062），不做版本检查。
		var result interface{ RowsAffected() (int64, error) }
		var err error
		if expectedRevision > 0 {
			result, err = session.ExecCtx(ctx,
				"UPDATE `post` SET `status` = 2, `revision` = `revision` + 1 WHERE `id` = ? AND `status` <> 2 AND `revision` = ?",
				postID, expectedRevision)
		} else {
			result, err = session.ExecCtx(ctx,
				"UPDATE `post` SET `status` = 2, `revision` = `revision` + 1 WHERE `id` = ? AND `status` <> 2",
				postID)
		}
		if err != nil {
			return err
		}
		changed, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if changed != 1 {
			return ErrVersionConflict
		}
		return m.outbox.Enqueue(ctx, session, event)
	})
}

func insertPostSession(ctx context.Context, session sqlx.Session, post *Post) error {
	if !post.Images.Valid {
		_, err := session.ExecCtx(ctx,
			"INSERT INTO `post` (`id`, `author_id`, `title`, `content`, `status`, `revision`) VALUES (?, ?, ?, ?, ?, ?)",
			post.Id, post.AuthorId, post.Title, post.Content, post.Status, post.Revision,
		)
		return err
	}
	_, err := session.ExecCtx(ctx,
		"INSERT INTO `post` (`id`, `author_id`, `title`, `content`, `images`, `status`, `revision`) VALUES (?, ?, ?, ?, ?, ?, ?)",
		post.Id, post.AuthorId, post.Title, post.Content, post.Images.String, post.Status, post.Revision,
	)
	return err
}

func insertPostTagsSession(
	ctx context.Context,
	session sqlx.Session,
	postID int64,
	tags []string,
	tagIDs []int64,
) error {
	for i, tag := range tags {
		if _, err := session.ExecCtx(ctx,
			"INSERT INTO `post_tag` (`id`, `post_id`, `tag_name`) VALUES (?, ?, ?)",
			tagIDs[i], postID, tag,
		); err != nil {
			return fmt.Errorf("insert post tag %q: %w", tag, err)
		}
	}
	return nil
}

func updatePostFieldsSession(
	ctx context.Context,
	session sqlx.Session,
	postID int64,
	fields map[string]any,
	expectedRevision int64,
) error {
	columns := make([]string, 0, len(fields)+1)
	for column := range fields {
		if _, ok := allowedUpdateCols[column]; !ok {
			return fmt.Errorf("update post: disallowed column %q", column)
		}
		columns = append(columns, column)
	}
	sort.Strings(columns)
	clauses := make([]string, 0, len(columns)+1)
	args := make([]any, 0, len(columns)+1)
	for _, column := range columns {
		clauses = append(clauses, fmt.Sprintf("`%s` = ?", column))
		args = append(args, fields[column])
	}
	clauses = append(clauses, "`revision` = `revision` + 1")
	args = append(args, postID)
	query := fmt.Sprintf("UPDATE `post` SET %s WHERE `id` = ?", strings.Join(clauses, ", "))
	if expectedRevision > 0 {
		query += " AND `revision` = ?"
		args = append(args, expectedRevision)
	}
	result, err := session.ExecCtx(ctx, query, args...)
	if err != nil {
		return err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if changed == 0 {
		return ErrVersionConflict
	}
	return nil
}
