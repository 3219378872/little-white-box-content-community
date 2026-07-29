package model

import (
	"context"
	"fmt"
	"strings"

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
		FindByIDs(ctx context.Context, ids []int64) ([]*UserProfile, error)
		SearchPublic(ctx context.Context, keyword string, offset, limit int64) ([]*UserProfile, int64, error)
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

func (m *customUserProfileModel) FindByIDs(ctx context.Context, ids []int64) ([]*UserProfile, error) {
	if len(ids) == 0 {
		return []*UserProfile{}, nil
	}
	placeholders := make([]string, 0, len(ids))
	args := make([]any, 0, len(ids))
	for _, id := range ids {
		placeholders = append(placeholders, "?")
		args = append(args, id)
	}
	query := fmt.Sprintf("select %s from %s where `id` in (%s)", userProfileRows, m.table, strings.Join(placeholders, ","))
	var profiles []*UserProfile
	if err := m.conn.QueryRowsCtx(ctx, &profiles, query, args...); err != nil {
		return nil, err
	}
	return profiles, nil
}

func (m *customUserProfileModel) SearchPublic(
	ctx context.Context,
	keyword string,
	offset, limit int64,
) ([]*UserProfile, int64, error) {
	const predicate = "`status` = 1 AND (LOCATE(?, `username`) > 0 OR LOCATE(?, COALESCE(`nickname`, '')) > 0 OR LOCATE(?, COALESCE(`bio`, '')) > 0)"
	var total int64
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE %s", m.table, predicate)
	if err := m.conn.QueryRowCtx(ctx, &total, countQuery, keyword, keyword, keyword); err != nil {
		return nil, 0, err
	}
	query := fmt.Sprintf(
		"SELECT %s FROM %s WHERE %s ORDER BY `follower_count` DESC, `id` ASC LIMIT ? OFFSET ?",
		userProfileRows, m.table, predicate,
	)
	profiles := make([]*UserProfile, 0, limit)
	if err := m.conn.QueryRowsCtx(ctx, &profiles, query, keyword, keyword, keyword, limit, offset); err != nil {
		return nil, 0, err
	}
	return profiles, total, nil
}
