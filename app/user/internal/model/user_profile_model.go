package model

import "github.com/zeromicro/go-zero/core/stores/sqlx"

var _ UserProfileModel = (*customUserProfileModel)(nil)

type (
	// UserProfileModel is an interface to be customized, add more methods here,
	// and implement the added methods in customUserProfileModel.
	UserProfileModel interface {
		userProfileModel
		withSession(session sqlx.Session) UserProfileModel
	}

	customUserProfileModel struct {
		*defaultUserProfileModel
	}
)

// NewUserProfileModel returns a model for the database table.
func NewUserProfileModel(conn sqlx.SqlConn) UserProfileModel {
	return &customUserProfileModel{
		defaultUserProfileModel: newUserProfileModel(conn),
	}
}

func (m *customUserProfileModel) withSession(session sqlx.Session) UserProfileModel {
	return NewUserProfileModel(sqlx.NewSqlConnFromSession(session))
}
