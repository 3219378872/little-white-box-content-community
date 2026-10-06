// Explicit SQL model. Maintained with its domain query extensions.

package model

import (
	"context"
	"database/sql"
	sqlx "esx/pkg/sqlstore"
	"fmt"
)

var (
	feedInboxRows                = "`id`,`user_id`,`author_id`,`post_id`,`created_at`"
	feedInboxRowsExpectAutoSet   = "`user_id`,`author_id`,`post_id`"
	feedInboxRowsWithPlaceHolder = "`user_id`=?,`author_id`=?,`post_id`=?"
)

type (
	feedInboxModel interface {
		Insert(ctx context.Context, data *FeedInbox) (sql.Result, error)
		FindOne(ctx context.Context, id int64) (*FeedInbox, error)
		FindOneByUserIdPostId(ctx context.Context, userId int64, postId int64) (*FeedInbox, error)
		Update(ctx context.Context, data *FeedInbox) error
		Delete(ctx context.Context, id int64) error
	}

	defaultFeedInboxModel struct {
		conn  sqlx.SqlConn
		table string
	}

	FeedInbox struct {
		Id        int64 `db:"id"`
		UserId    int64 `db:"user_id"`
		AuthorId  int64 `db:"author_id"`
		PostId    int64 `db:"post_id"`
		CreatedAt int64 `db:"created_at"`
	}
)

// newFeedInboxModel 创建基础 CRUD 模型。
func newFeedInboxModel(conn sqlx.SqlConn) *defaultFeedInboxModel {
	return &defaultFeedInboxModel{
		conn:  conn,
		table: "`feed_inbox`",
	}
}

// Delete 按主键删除。
func (m *defaultFeedInboxModel) Delete(ctx context.Context, id int64) error {
	query := fmt.Sprintf("delete from %s where `id` = ?", m.table)
	_, err := m.conn.ExecCtx(ctx, query, id)
	return err
}

// FindOne 按主键查询，未找到返回 ErrNotFound。
func (m *defaultFeedInboxModel) FindOne(ctx context.Context, id int64) (*FeedInbox, error) {
	query := fmt.Sprintf("select %s from %s where `id` = ? limit 1", feedInboxRows, m.table)
	var resp FeedInbox
	err := m.conn.QueryRowCtx(ctx, &resp, query, id)
	switch err {
	case nil:
		return &resp, nil
	case sqlx.ErrNotFound:
		return nil, ErrNotFound
	default:
		return nil, err
	}
}

// FindOneByUserIdPostId 按唯一键查询，未找到返回 ErrNotFound。
func (m *defaultFeedInboxModel) FindOneByUserIdPostId(ctx context.Context, userId int64, postId int64) (*FeedInbox, error) {
	var resp FeedInbox
	query := fmt.Sprintf("select %s from %s where `user_id` = ? and `post_id` = ? limit 1", feedInboxRows, m.table)
	err := m.conn.QueryRowCtx(ctx, &resp, query, userId, postId)
	switch err {
	case nil:
		return &resp, nil
	case sqlx.ErrNotFound:
		return nil, ErrNotFound
	default:
		return nil, err
	}
}

// Insert 是基础 CRUD 写入，不含 created_at 列；关注流写入走 InsertIgnore/BatchInsertIgnore。
func (m *defaultFeedInboxModel) Insert(ctx context.Context, data *FeedInbox) (sql.Result, error) {
	query := fmt.Sprintf("insert into %s (%s) values (?, ?, ?)", m.table, feedInboxRowsExpectAutoSet)
	ret, err := m.conn.ExecCtx(ctx, query, data.UserId, data.AuthorId, data.PostId)
	return ret, err
}

// Update 按主键更新。
func (m *defaultFeedInboxModel) Update(ctx context.Context, newData *FeedInbox) error {
	query := fmt.Sprintf("update %s set %s where `id` = ?", m.table, feedInboxRowsWithPlaceHolder)
	_, err := m.conn.ExecCtx(ctx, query, newData.UserId, newData.AuthorId, newData.PostId, newData.Id)
	return err
}
