package model

import (
	"context"
	"errors"
	"esx/pkg/idempotencyx"
	"esx/pkg/util"
	"fmt"
	"sort"
	"strings"

	"esx/pkg/outboxx"

	sqlx "esx/pkg/sqlstore"
)

// OutboxEnqueuer 在业务事务内写入 outbox 事件。
type OutboxEnqueuer interface {
	Enqueue(ctx context.Context, session sqlx.Session, event outboxx.Event) error
}

// PostCommandModel 是帖子的事务写入：帖子、标签、幂等记录与 outbox 事件一起提交。
type PostCommandModel interface {
	CreatePost(ctx context.Context, post *Post, tags []string, tagIDs []int64, event outboxx.Event, idem idempotencyx.IdempotencyRecord) (postID int64, created bool, err error)
	// replaceTags=false 时保留现有 post_tag 关联（局部更新未显式提供 tags），
	// 仅更新字段与 outbox 事件。
	UpdatePost(ctx context.Context, postID int64, fields map[string]any, tags []string, tagIDs []int64, event outboxx.Event, expectedRevision int64, replaceTags bool) error
	DeletePost(ctx context.Context, postID int64, event outboxx.Event, expectedRevision int64) error
}

// IdempotentPostCommandModel 是带幂等键的更新与删除；重放时返回首次执行的结果而不再次修改。
type IdempotentPostCommandModel interface {
	ReplayPostCommand(ctx context.Context, idem idempotencyx.IdempotencyRecord) (result int64, found bool, err error)
	UpdatePostIdempotent(ctx context.Context, postID int64, fields map[string]any, tags []string, tagIDs []int64,
		event outboxx.Event, expectedRevision int64, replaceTags bool, result int64, idem idempotencyx.IdempotencyRecord) (applied bool, err error)
	DeletePostIdempotent(ctx context.Context, postID int64, event outboxx.Event, expectedRevision, result int64,
		idem idempotencyx.IdempotencyRecord) (applied bool, err error)
}

// postCommandModel 是基于 MySQL 事务的实现。
type postCommandModel struct {
	conn   sqlx.SqlConn
	outbox OutboxEnqueuer
}

// NewPostCommandModel 创建帖子写模型；outbox 必须与业务写入共用同一连接。
func NewPostCommandModel(conn sqlx.SqlConn, outbox OutboxEnqueuer) PostCommandModel {
	return &postCommandModel{conn: conn, outbox: outbox}
}

// CreatePost 在一个事务内解析幂等键、插入帖子与标签并写入事件；
// 幂等重放返回首次创建的帖子 ID，created=false。
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

// UpdatePost 按期望 revision 条件更新帖子字段，可选整体替换标签，并在同一事务内写入事件；revision 不符视为冲突。
func (m *postCommandModel) UpdatePost(
	ctx context.Context,
	postID int64,
	fields map[string]any,
	tags []string,
	tagIDs []int64,
	event outboxx.Event,
	expectedRevision int64,
	replaceTags bool,
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
		if replaceTags {
			if _, err := session.ExecCtx(ctx, "DELETE FROM `post_tag` WHERE `post_id` = ?", postID); err != nil {
				return err
			}
			if err := insertPostTagsSession(ctx, session, postID, tags, tagIDs); err != nil {
				return err
			}
		}
		return m.outbox.Enqueue(ctx, session, event)
	})
}

// ReplayPostCommand 查询幂等键是否已执行；找到时返回首次结果，命令内容不同则返回冲突。
func (m *postCommandModel) ReplayPostCommand(ctx context.Context, idem idempotencyx.IdempotencyRecord) (int64, bool, error) {
	if idem.Key == "" {
		return 0, false, nil
	}
	if !idem.Valid() || m.conn == nil {
		return 0, false, fmt.Errorf("content command model is not configured")
	}
	var row struct {
		ResourceID  int64  `db:"resource_id"`
		CommandHash string `db:"command_hash"`
	}
	err := m.conn.QueryRowCtx(ctx, &row, "SELECT resource_id, command_hash FROM idempotency "+
		"WHERE scope=? AND user_id=? AND `key`=? LIMIT 1", idem.Scope, idem.UserID, idem.Key)
	if errors.Is(err, sqlx.ErrNotFound) {
		return 0, false, nil
	}
	if err != nil {
		return 0, false, err
	}
	if row.CommandHash != idem.CommandHash {
		return 0, false, idempotencyx.ErrIdempotencyConflict
	}
	return row.ResourceID, true, nil
}

// UpdatePostIdempotent 与 UpdatePost 相同，但先在事务内登记幂等键；键已存在时 applied=false，不再修改。
func (m *postCommandModel) UpdatePostIdempotent(
	ctx context.Context,
	postID int64,
	fields map[string]any,
	tags []string,
	tagIDs []int64,
	event outboxx.Event,
	expectedRevision int64,
	replaceTags bool,
	result int64,
	idem idempotencyx.IdempotencyRecord,
) (applied bool, err error) {
	if postID <= 0 || m.conn == nil || m.outbox == nil || !idem.Valid() || idem.Key == "" {
		return false, fmt.Errorf("content command model is not configured")
	}
	if len(tags) != len(tagIDs) {
		return false, fmt.Errorf("tags and tag ids length mismatch")
	}
	recordID, err := util.NextID()
	if err != nil {
		return false, err
	}
	err = m.conn.TransactCtx(ctx, func(ctx context.Context, session sqlx.Session) error {
		_, shouldApply, resolveErr := idempotencyx.ResolveIdempotencySession(ctx, session, idem, recordID, result)
		if resolveErr != nil {
			return resolveErr
		}
		if !shouldApply {
			return nil
		}
		if err := updatePostFieldsSession(ctx, session, postID, fields, expectedRevision); err != nil {
			return err
		}
		if replaceTags {
			if _, err := session.ExecCtx(ctx, "DELETE FROM `post_tag` WHERE `post_id` = ?", postID); err != nil {
				return err
			}
			if err := insertPostTagsSession(ctx, session, postID, tags, tagIDs); err != nil {
				return err
			}
		}
		applied = true
		return m.outbox.Enqueue(ctx, session, event)
	})
	return applied, err
}

// DeletePost 按期望 revision 软删帖子（status=2）并推进 revision，同事务写入删除事件。
func (m *postCommandModel) DeletePost(ctx context.Context, postID int64, event outboxx.Event, expectedRevision int64) error {
	if postID <= 0 || m.conn == nil || m.outbox == nil {
		return fmt.Errorf("content command model is not configured")
	}
	return m.conn.TransactCtx(ctx, func(ctx context.Context, session sqlx.Session) error {
		if expectedRevision <= 0 {
			return fmt.Errorf("expected revision is required")
		}
		result, err := session.ExecCtx(ctx,
			"UPDATE `post` SET `status` = 2, `revision` = `revision` + 1 WHERE `id` = ? AND `status` <> 2 AND `revision` = ?",
			postID, expectedRevision)
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

// DeletePostIdempotent 与 DeletePost 相同，但先在事务内登记幂等键；键已存在时 applied=false。
func (m *postCommandModel) DeletePostIdempotent(
	ctx context.Context,
	postID int64,
	event outboxx.Event,
	expectedRevision, result int64,
	idem idempotencyx.IdempotencyRecord,
) (applied bool, err error) {
	if postID <= 0 || m.conn == nil || m.outbox == nil || !idem.Valid() || idem.Key == "" {
		return false, fmt.Errorf("content command model is not configured")
	}
	recordID, err := util.NextID()
	if err != nil {
		return false, err
	}
	err = m.conn.TransactCtx(ctx, func(ctx context.Context, session sqlx.Session) error {
		_, shouldApply, resolveErr := idempotencyx.ResolveIdempotencySession(ctx, session, idem, recordID, result)
		if resolveErr != nil {
			return resolveErr
		}
		if !shouldApply {
			return nil
		}
		if expectedRevision <= 0 {
			return fmt.Errorf("expected revision is required")
		}
		changedResult, updateErr := session.ExecCtx(ctx,
			"UPDATE `post` SET `status` = 2, `revision` = `revision` + 1 WHERE `id` = ? AND `status` <> 2 AND `revision` = ?",
			postID, expectedRevision)
		if updateErr != nil {
			return updateErr
		}
		changed, rowsErr := changedResult.RowsAffected()
		if rowsErr != nil {
			return rowsErr
		}
		if changed != 1 {
			return ErrVersionConflict
		}
		applied = true
		return m.outbox.Enqueue(ctx, session, event)
	})
	return applied, err
}

// insertPostSession 在事务内插入帖子行。
func insertPostSession(ctx context.Context, session sqlx.Session, post *Post) error {
	_, err := session.ExecCtx(ctx,
		"INSERT INTO `post` (`id`, `author_id`, `title`, `content`, `images`, `media_ids`, `status`, `revision`) VALUES (?, ?, ?, ?, ?, ?, ?, ?)",
		post.Id, post.AuthorId, post.Title, post.Content, post.Images, post.MediaIds, post.Status, post.Revision,
	)
	return err
}

// insertPostTagsSession 在事务内插入帖子的标签关联，tagIDs 与 tags 一一对应。
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

// updatePostFieldsSession 只允许白名单列，按期望 revision 条件更新并推进 revision。
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
	if expectedRevision <= 0 {
		return fmt.Errorf("expected revision is required")
	}
	query := fmt.Sprintf("UPDATE `post` SET %s WHERE `id` = ? AND `revision` = ? AND `status` <> 2", strings.Join(clauses, ", "))
	args = append(args, expectedRevision)
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
