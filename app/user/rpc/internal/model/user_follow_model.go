package model

import (
	"context"
	"fmt"
	"strings"

	"github.com/zeromicro/go-zero/core/stores/sqlx"
)

var _ UserFollowModel = (*customUserFollowModel)(nil)

type (
	// UserFollowModel is an interface to be customized, add more methods here,
	// and implement the added methods in customUserFollowModel.
	UserFollowModel interface {
		userFollowModel
		withSession(session sqlx.Session) UserFollowModel
		FindFollowers(ctx context.Context, userID int64, offset, limit int64) ([]*UserProfile, error)
		FindFollowing(ctx context.Context, userID int64, offset, limit int64) ([]*UserProfile, error)
		CountFollowers(ctx context.Context, userID int64) (int64, error)
		CountFollowing(ctx context.Context, userID int64) (int64, error)
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

func (m *customUserFollowModel) FindFollowers(ctx context.Context, userID int64, offset, limit int64) ([]*UserProfile, error) {
	query := fmt.Sprintf(`SELECT %s
	FROM user_follow f
	JOIN user_profile p ON p.id = f.user_id
	WHERE f.target_user_id = ?
	ORDER BY f.id DESC
	LIMIT ?, ?`, prefixedProfileColumns("p."))
	var rows []*UserProfile
	err := m.conn.QueryRowsCtx(ctx, &rows, query, userID, offset, limit)
	return rows, err
}

func (m *customUserFollowModel) FindFollowing(ctx context.Context, userID int64, offset, limit int64) ([]*UserProfile, error) {
	query := fmt.Sprintf(`SELECT %s
	FROM user_follow f
	JOIN user_profile p ON p.id = f.target_user_id
	WHERE f.user_id = ?
	ORDER BY f.id DESC
	LIMIT ?, ?`, prefixedProfileColumns("p."))
	var rows []*UserProfile
	err := m.conn.QueryRowsCtx(ctx, &rows, query, userID, offset, limit)
	return rows, err
}

// prefixedProfileColumns returns userProfileRows with each column prefixed.
func prefixedProfileColumns(prefix string) string {
	cols := strings.Split(userProfileRows, ",")
	for i, c := range cols {
		cols[i] = prefix + c
	}
	return strings.Join(cols, ",")
}


func (m *customUserFollowModel) CountFollowers(ctx context.Context, userID int64) (int64, error) {
	var total int64
	err := m.conn.QueryRowCtx(ctx, &total, "SELECT COUNT(*) FROM user_follow WHERE target_user_id = ?", userID)
	return total, err
}

func (m *customUserFollowModel) CountFollowing(ctx context.Context, userID int64) (int64, error) {
	var total int64
	err := m.conn.QueryRowCtx(ctx, &total, "SELECT COUNT(*) FROM user_follow WHERE user_id = ?", userID)
	return total, err
}
