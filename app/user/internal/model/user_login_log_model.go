package model

import "github.com/zeromicro/go-zero/core/stores/sqlx"

var _ UserLoginLogModel = (*customUserLoginLogModel)(nil)

type (
	// UserLoginLogModel is an interface to be customized, add more methods here,
	// and implement the added methods in customUserLoginLogModel.
	UserLoginLogModel interface {
		userLoginLogModel
		withSession(session sqlx.Session) UserLoginLogModel
	}

	customUserLoginLogModel struct {
		*defaultUserLoginLogModel
	}
)

// NewUserLoginLogModel returns a model for the database table.
func NewUserLoginLogModel(conn sqlx.SqlConn) UserLoginLogModel {
	return &customUserLoginLogModel{
		defaultUserLoginLogModel: newUserLoginLogModel(conn),
	}
}

func (m *customUserLoginLogModel) withSession(session sqlx.Session) UserLoginLogModel {
	return NewUserLoginLogModel(sqlx.NewSqlConnFromSession(session))
}
