// Explicit SQL model. Maintained with its domain query extensions.

package model

import (
	"context"
	"database/sql"
	sqlx "esx/pkg/sqlstore"
	"fmt"
	"time"
)

var (
	userTagRows                = "`id`,`user_id`,`tag_name`,`weight`,`created_at`,`updated_at`"
	userTagRowsExpectAutoSet   = "`user_id`,`tag_name`,`weight`"
	userTagRowsWithPlaceHolder = "`user_id`=?,`tag_name`=?,`weight`=?"
)

type (
	userTagModel interface {
		Insert(ctx context.Context, data *UserTag) (sql.Result, error)
		FindOne(ctx context.Context, id int64) (*UserTag, error)
		FindOneByUserIdTagName(ctx context.Context, userId int64, tagName string) (*UserTag, error)
		Update(ctx context.Context, data *UserTag) error
		Delete(ctx context.Context, id int64) error
	}

	defaultUserTagModel struct {
		conn  sqlx.SqlConn
		table string
	}

	UserTag struct {
		Id        int64     `db:"id"`
		UserId    int64     `db:"user_id"`    // 用户ID
		TagName   string    `db:"tag_name"`   // 标签名
		Weight    int64     `db:"weight"`     // 权重
		CreatedAt time.Time `db:"created_at"` // 创建时间
		UpdatedAt time.Time `db:"updated_at"` // 更新时间
	}
)

func newUserTagModel(conn sqlx.SqlConn) *defaultUserTagModel {
	return &defaultUserTagModel{
		conn:  conn,
		table: "`user_tag`",
	}
}

func (m *defaultUserTagModel) Delete(ctx context.Context, id int64) error {
	query := fmt.Sprintf("delete from %s where `id` = ?", m.table)
	_, err := m.conn.ExecCtx(ctx, query, id)
	return err
}

func (m *defaultUserTagModel) FindOne(ctx context.Context, id int64) (*UserTag, error) {
	query := fmt.Sprintf("select %s from %s where `id` = ? limit 1", userTagRows, m.table)
	var resp UserTag
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

func (m *defaultUserTagModel) FindOneByUserIdTagName(ctx context.Context, userId int64, tagName string) (*UserTag, error) {
	var resp UserTag
	query := fmt.Sprintf("select %s from %s where `user_id` = ? and `tag_name` = ? limit 1", userTagRows, m.table)
	err := m.conn.QueryRowCtx(ctx, &resp, query, userId, tagName)
	switch err {
	case nil:
		return &resp, nil
	case sqlx.ErrNotFound:
		return nil, ErrNotFound
	default:
		return nil, err
	}
}

func (m *defaultUserTagModel) Insert(ctx context.Context, data *UserTag) (sql.Result, error) {
	query := fmt.Sprintf("insert into %s (%s) values (?, ?, ?)", m.table, userTagRowsExpectAutoSet)
	ret, err := m.conn.ExecCtx(ctx, query, data.UserId, data.TagName, data.Weight)
	return ret, err
}

func (m *defaultUserTagModel) Update(ctx context.Context, newData *UserTag) error {
	query := fmt.Sprintf("update %s set %s where `id` = ?", m.table, userTagRowsWithPlaceHolder)
	_, err := m.conn.ExecCtx(ctx, query, newData.UserId, newData.TagName, newData.Weight, newData.Id)
	return err
}
