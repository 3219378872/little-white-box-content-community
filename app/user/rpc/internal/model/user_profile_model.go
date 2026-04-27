package model

import (
	"context"
	"fmt"

	"github.com/zeromicro/go-zero/core/stores/sqlx"
)

var _ UserProfileModel = (*customUserProfileModel)(nil)

type (
	// UserProfileModel is an interface to be customized, add more methods here,
	// and implement the added methods in customUserProfileModel.
	UserProfileModel interface {
		userProfileModel
		withSession(session sqlx.Session) UserProfileModel
		UpdateUserDes(ctx context.Context, userId int64, nickname, avatarUrl, bio string) error
		FindOneByIdForUpdate(ctx context.Context, session sqlx.Session, id int64) (*UserProfile, error)
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

// UpdateUserDes 更新用户描述信息
func (m *customUserProfileModel) UpdateUserDes(ctx context.Context, userId int64, nickname, avatarUrl, bio string) error {
	query := fmt.Sprintf("update %s set `nickname` = ?, `avatar_url` = ?, `bio` = ? where `id` = ?", m.table)
	_, err := m.conn.ExecCtx(ctx, query, nickname, avatarUrl, bio, userId)
	return err
}

func (m *customUserProfileModel) FindOneByIdForUpdate(ctx context.Context, session sqlx.Session, id int64) (*UserProfile, error) {
	query := fmt.Sprintf("select %s from %s where id = ? for update", userProfileRows, m.table)
	var userProfile UserProfile
	err := session.QueryRowCtx(ctx, &userProfile, query, id)
	if err != nil {
		return nil, err
	}
	return &userProfile, nil
}
