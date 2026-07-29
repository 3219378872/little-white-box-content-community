package model

import (
	"context"
	"database/sql"
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
	CreatePost(ctx context.Context, post *Post, tags []string, tagIDs []int64, event outboxx.Event) error
	UpdatePost(ctx context.Context, postID int64, fields map[string]any, tags []string, tagIDs []int64, event outboxx.Event) error
	DeletePost(ctx context.Context, postID int64, event outboxx.Event) error
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
) error {
	if post == nil || m.conn == nil || m.outbox == nil {
		return fmt.Errorf("content command model is not configured")
	}
	if len(tags) != len(tagIDs) {
		return fmt.Errorf("tags and tag ids length mismatch")
	}
	return m.conn.TransactCtx(ctx, func(ctx context.Context, session sqlx.Session) error {
		if err := insertPostSession(ctx, session, post); err != nil {
			return err
		}
		if err := insertPostTagsSession(ctx, session, post.Id, tags, tagIDs); err != nil {
			return err
		}
		return m.outbox.Enqueue(ctx, session, event)
	})
}

func (m *postCommandModel) UpdatePost(
	ctx context.Context,
	postID int64,
	fields map[string]any,
	tags []string,
	tagIDs []int64,
	event outboxx.Event,
) error {
	if postID <= 0 || m.conn == nil || m.outbox == nil {
		return fmt.Errorf("content command model is not configured")
	}
	if len(tags) != len(tagIDs) {
		return fmt.Errorf("tags and tag ids length mismatch")
	}
	return m.conn.TransactCtx(ctx, func(ctx context.Context, session sqlx.Session) error {
		if err := updatePostFieldsSession(ctx, session, postID, fields); err != nil {
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

func (m *postCommandModel) DeletePost(ctx context.Context, postID int64, event outboxx.Event) error {
	if postID <= 0 || m.conn == nil || m.outbox == nil {
		return fmt.Errorf("content command model is not configured")
	}
	return m.conn.TransactCtx(ctx, func(ctx context.Context, session sqlx.Session) error {
		result, err := session.ExecCtx(ctx, "UPDATE `post` SET `status` = 2 WHERE `id` = ? AND `status` <> 2", postID)
		if err != nil {
			return err
		}
		changed, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if changed != 1 {
			return fmt.Errorf("post %d was not updated", postID)
		}
		return m.outbox.Enqueue(ctx, session, event)
	})
}

func insertPostSession(ctx context.Context, session sqlx.Session, post *Post) error {
	if !post.Images.Valid {
		_, err := session.ExecCtx(ctx,
			"INSERT INTO `post` (`id`, `author_id`, `title`, `content`, `status`) VALUES (?, ?, ?, ?, ?)",
			post.Id, post.AuthorId, post.Title, post.Content, post.Status,
		)
		return err
	}
	_, err := session.ExecCtx(ctx,
		"INSERT INTO `post` (`id`, `author_id`, `title`, `content`, `images`, `status`) VALUES (?, ?, ?, ?, ?, ?)",
		post.Id, post.AuthorId, post.Title, post.Content, post.Images.String, post.Status,
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
) error {
	if len(fields) == 0 {
		return nil
	}
	columns := make([]string, 0, len(fields))
	for column := range fields {
		if _, ok := allowedUpdateCols[column]; !ok {
			return fmt.Errorf("update post: disallowed column %q", column)
		}
		columns = append(columns, column)
	}
	sort.Strings(columns)
	clauses := make([]string, 0, len(columns))
	args := make([]any, 0, len(columns)+1)
	for _, column := range columns {
		clauses = append(clauses, fmt.Sprintf("`%s` = ?", column))
		args = append(args, fields[column])
	}
	args = append(args, postID)
	result, err := session.ExecCtx(ctx,
		fmt.Sprintf("UPDATE `post` SET %s WHERE `id` = ?", strings.Join(clauses, ", ")),
		args...,
	)
	if err != nil {
		return err
	}
	changed, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if changed == 0 {
		return sql.ErrNoRows
	}
	return nil
}
