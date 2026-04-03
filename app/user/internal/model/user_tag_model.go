package model

import "github.com/zeromicro/go-zero/core/stores/sqlx"

var _ UserTagModel = (*customUserTagModel)(nil)

type (
	// UserTagModel is an interface to be customized, add more methods here,
	// and implement the added methods in customUserTagModel.
	UserTagModel interface {
		userTagModel
		withSession(session sqlx.Session) UserTagModel
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
