package model

import (
	"context"
	"fmt"

	"github.com/zeromicro/go-zero/core/stores/sqlx"
)

var _ UserTagModel = (*customUserTagModel)(nil)

type (
	// UserTagModel is an interface to be customized, add more methods here,
	// and implement the added methods in customUserTagModel.
	UserTagModel interface {
		userTagModel
		withSession(session sqlx.Session) UserTagModel
		FindByUserId(ctx context.Context, userID int64) ([]*UserTag, error)
	}

	customUserTagModel struct {
		*defaultUserTagModel
	}
)

// NewUserTagModel returns a model for the database table.
func NewUserTagModel(conn sqlx.SqlConn) UserTagModel {
	return &customUserTagModel{
		defaultUserTagModel: newUserTagModel(conn),
	}
}

func (m *customUserTagModel) withSession(session sqlx.Session) UserTagModel {
	return NewUserTagModel(sqlx.NewSqlConnFromSession(session))
}

// FindByUserId 返回用户的标签（按权重降序），供 GetUserTags RPC 使用。
func (m *customUserTagModel) FindByUserId(ctx context.Context, userID int64) ([]*UserTag, error) {
	if userID <= 0 {
		return nil, fmt.Errorf("user tag query requires a valid user id")
	}
	var tags []*UserTag
	query := fmt.Sprintf("select %s from %s where `user_id` = ? order by `weight` desc, `id` asc", userTagRows, m.table)
	if err := m.conn.QueryRowsCtx(ctx, &tags, query, userID); err != nil {
		return nil, err
	}
	return tags, nil
}
