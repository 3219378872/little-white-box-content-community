package model

import "github.com/zeromicro/go-zero/core/stores/sqlx"

var _ UserFollowModel = (*customUserFollowModel)(nil)

type (
	// UserFollowModel is an interface to be customized, add more methods here,
	// and implement the added methods in customUserFollowModel.
	UserFollowModel interface {
		userFollowModel
		withSession(session sqlx.Session) UserFollowModel
	}

	customUserFollowModel struct {
		*defaultUserFollowModel
	}
)

// NewUserFollowModel returns a model for the database table.
func NewUserFollowModel(conn sqlx.SqlConn) UserFollowModel {
	return &customUserFollowModel{
		defaultUserFollowModel: newUserFollowModel(conn),
	}
}

func (m *customUserFollowModel) withSession(session sqlx.Session) UserFollowModel {
	return NewUserFollowModel(sqlx.NewSqlConnFromSession(session))
}
