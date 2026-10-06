// Explicit SQL model. Maintained with its domain query extensions.

package model

import (
	"context"
	"database/sql"
	sqlc "esx/pkg/cachedstore"
	cache "esx/pkg/modelcache"
	sqlx "esx/pkg/sqlstore"
	"fmt"
	"time"
)

var (
	postTagRows                = "`id`,`post_id`,`tag_name`,`created_at`"
	postTagRowsExpectAutoSet   = "`id`,`post_id`,`tag_name`"
	postTagRowsWithPlaceHolder = "`post_id`=?,`tag_name`=?"

	cachePostTagIdPrefix            = "cache:postTag:id:"
	cachePostTagPostIdTagNamePrefix = "cache:postTag:postId:tagName:"
)

type (
	postTagModel interface {
		Insert(ctx context.Context, data *PostTag) (sql.Result, error)
		FindOne(ctx context.Context, id int64) (*PostTag, error)
		FindOneByPostIdTagName(ctx context.Context, postId int64, tagName string) (*PostTag, error)
		Update(ctx context.Context, data *PostTag) error
		Delete(ctx context.Context, id int64) error
	}

	defaultPostTagModel struct {
		sqlc.CachedConn
		table string
	}

	PostTag struct {
		Id        int64     `db:"id"`
		PostId    int64     `db:"post_id"`    // 帖子ID
		TagName   string    `db:"tag_name"`   // 标签名
		CreatedAt time.Time `db:"created_at"` // 创建时间
	}
)

// newPostTagModel creates the cached base model.
func newPostTagModel(conn sqlx.SqlConn, c cache.CacheConf, opts ...cache.Option) *defaultPostTagModel {
	return &defaultPostTagModel{
		CachedConn: sqlc.NewConn(conn, c, opts...),
		table:      "`post_tag`",
	}
}

// Delete removes a post-tag relation and evicts its cache keys.
func (m *defaultPostTagModel) Delete(ctx context.Context, id int64) error {
	data, err := m.FindOne(ctx, id)
	if err != nil {
		return err
	}

	postTagIdKey := fmt.Sprintf("%s%v", cachePostTagIdPrefix, id)
	postTagPostIdTagNameKey := fmt.Sprintf("%s%v:%v", cachePostTagPostIdTagNamePrefix, data.PostId, data.TagName)
	_, err = m.ExecCtx(ctx, func(ctx context.Context, conn sqlx.SqlConn) (result sql.Result, err error) {
		query := fmt.Sprintf("delete from %s where `id` = ?", m.table)
		return conn.ExecCtx(ctx, query, id)
	}, postTagIdKey, postTagPostIdTagNameKey)
	return err
}

// FindOne loads a post-tag relation by id through the cache.
func (m *defaultPostTagModel) FindOne(ctx context.Context, id int64) (*PostTag, error) {
	postTagIdKey := fmt.Sprintf("%s%v", cachePostTagIdPrefix, id)
	var resp PostTag
	err := m.QueryRowCtx(ctx, &resp, postTagIdKey, func(ctx context.Context, conn sqlx.SqlConn, v any) error {
		query := fmt.Sprintf("select %s from %s where `id` = ? limit 1", postTagRows, m.table)
		return conn.QueryRowCtx(ctx, v, query, id)
	})
	switch err {
	case nil:
		return &resp, nil
	case sqlc.ErrNotFound:
		return nil, ErrNotFound
	default:
		return nil, err
	}
}

// FindOneByPostIdTagName loads a relation by its unique (post, tag) key; the
// cache maps that key to the id, then to the row.
func (m *defaultPostTagModel) FindOneByPostIdTagName(ctx context.Context, postId int64, tagName string) (*PostTag, error) {
	postTagPostIdTagNameKey := fmt.Sprintf("%s%v:%v", cachePostTagPostIdTagNamePrefix, postId, tagName)
	var resp PostTag
	err := m.QueryRowIndexCtx(ctx, &resp, postTagPostIdTagNameKey, m.formatPrimary, func(ctx context.Context, conn sqlx.SqlConn, v any) (i any, e error) {
		query := fmt.Sprintf("select %s from %s where `post_id` = ? and `tag_name` = ? limit 1", postTagRows, m.table)
		if err := conn.QueryRowCtx(ctx, &resp, query, postId, tagName); err != nil {
			return nil, err
		}
		return resp.Id, nil
	}, m.queryPrimary)
	switch err {
	case nil:
		return &resp, nil
	case sqlc.ErrNotFound:
		return nil, ErrNotFound
	default:
		return nil, err
	}
}

// Insert creates a post-tag relation and evicts its cache keys.
func (m *defaultPostTagModel) Insert(ctx context.Context, data *PostTag) (sql.Result, error) {
	postTagIdKey := fmt.Sprintf("%s%v", cachePostTagIdPrefix, data.Id)
	postTagPostIdTagNameKey := fmt.Sprintf("%s%v:%v", cachePostTagPostIdTagNamePrefix, data.PostId, data.TagName)
	ret, err := m.ExecCtx(ctx, func(ctx context.Context, conn sqlx.SqlConn) (result sql.Result, err error) {
		query := fmt.Sprintf("insert into %s (%s) values (?, ?, ?)", m.table, postTagRowsExpectAutoSet)
		return conn.ExecCtx(ctx, query, data.Id, data.PostId, data.TagName)
	}, postTagIdKey, postTagPostIdTagNameKey)
	return ret, err
}

// Update rewrites a post-tag relation and evicts its cache keys.
func (m *defaultPostTagModel) Update(ctx context.Context, newData *PostTag) error {
	data, err := m.FindOne(ctx, newData.Id)
	if err != nil {
		return err
	}

	postTagIdKey := fmt.Sprintf("%s%v", cachePostTagIdPrefix, data.Id)
	postTagPostIdTagNameKey := fmt.Sprintf("%s%v:%v", cachePostTagPostIdTagNamePrefix, data.PostId, data.TagName)
	_, err = m.ExecCtx(ctx, func(ctx context.Context, conn sqlx.SqlConn) (result sql.Result, err error) {
		query := fmt.Sprintf("update %s set %s where `id` = ?", m.table, postTagRowsWithPlaceHolder)
		return conn.ExecCtx(ctx, query, newData.PostId, newData.TagName, newData.Id)
	}, postTagIdKey, postTagPostIdTagNameKey)
	return err
}

// formatPrimary builds the id cache key used by the unique-key index cache.
func (m *defaultPostTagModel) formatPrimary(primary any) string {
	return fmt.Sprintf("%s%v", cachePostTagIdPrefix, primary)
}

// queryPrimary loads the row for an id resolved from the unique-key cache.
func (m *defaultPostTagModel) queryPrimary(ctx context.Context, conn sqlx.SqlConn, v, primary any) error {
	query := fmt.Sprintf("select %s from %s where `id` = ? limit 1", postTagRows, m.table)
	return conn.QueryRowCtx(ctx, v, query, primary)
}
