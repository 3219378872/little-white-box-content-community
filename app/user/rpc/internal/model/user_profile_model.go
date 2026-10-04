package model

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	sqlc "esx/pkg/cachedstore"
	cache "esx/pkg/modelcache"
	sqlx "esx/pkg/sqlstore"
)

// Author card cache: display fields only, so follow/post/like counters never
// invalidate it. One hour bounds staleness when a post-commit invalidation
// fails (CORE-053); not-found entries live 60 seconds.
const (
	cacheUserCardPrefix        = "user:card:"
	userCardTTLSeconds         = 3600
	userCardNotFoundTTLSeconds = 60
	userCardRows               = "`id`,`username`,`nickname`,`avatar_url`"
)

// UserCard is the cached author display projection.
type UserCard struct {
	Id        int64  `db:"id"`
	Username  string `db:"username"`
	Nickname  string `db:"nickname"`
	AvatarUrl string `db:"avatar_url"`
}

func userCardKey(id int64) string { return fmt.Sprintf("%s%d", cacheUserCardPrefix, id) }

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
		FindCardsByIDs(ctx context.Context, ids []int64) ([]*UserCard, error)
		SearchPublic(ctx context.Context, keyword string, offset, limit int64) ([]*UserProfile, int64, error)
		SearchPublicPage(ctx context.Context, keyword string, offset, limit int64) ([]*UserProfile, error)
	}

	customUserProfileModel struct {
		*defaultUserProfileModel
		cards sqlc.CachedConn
	}
)

// NewUserProfileModel returns a model whose author cards bypass the cache.
func NewUserProfileModel(conn sqlx.SqlConn) UserProfileModel {
	return NewCachedUserProfileModel(conn, nil)
}

// NewCachedUserProfileModel caches author cards in c. Every write to card
// fields must go through this model so it can invalidate after commit.
func NewCachedUserProfileModel(conn sqlx.SqlConn, c cache.CacheConf) UserProfileModel {
	return &customUserProfileModel{
		defaultUserProfileModel: newUserProfileModel(conn),
		cards: sqlc.NewConn(conn, c, func(o *cache.Options) {
			o.TTLSeconds = userCardTTLSeconds
			o.NotFoundTTLSeconds = userCardNotFoundTTLSeconds
		}),
	}
}

func (m *customUserProfileModel) withSession(session sqlx.Session) UserProfileModel {
	return NewUserProfileModel(sqlx.NewSqlConnFromSession(session))
}

// UpdateUserDes 更新用户描述信息
func (m *customUserProfileModel) UpdateUserDes(ctx context.Context, userId int64, nickname, avatarUrl, bio string) error {
	query := fmt.Sprintf("update %s set `nickname` = ?, `avatar_url` = ?, `bio` = ? where `id` = ?", m.table)
	_, err := m.cards.ExecCtx(ctx, func(ctx context.Context, conn sqlx.SqlConn) (sql.Result, error) {
		return conn.ExecCtx(ctx, query, nickname, avatarUrl, bio, userId)
	}, userCardKey(userId))
	return err
}

// FindCardsByIDs returns author cards in request order, omitting unknown IDs.
// Cache misses share one primary-key IN query.
func (m *customUserProfileModel) FindCardsByIDs(ctx context.Context, ids []int64) ([]*UserCard, error) {
	cards, err := sqlc.QueryRowsByIDs(ctx, m.cards, ids, userCardKey,
		func(ctx context.Context, conn sqlx.SqlConn, missing []int64) (map[int64]UserCard, error) {
			placeholders := make([]string, len(missing))
			args := make([]any, len(missing))
			for i, id := range missing {
				placeholders[i] = "?"
				args[i] = id
			}
			query := fmt.Sprintf("select %s from %s where `id` in (%s)", userCardRows, m.table, strings.Join(placeholders, ","))
			var rows []struct {
				Id        int64          `db:"id"`
				Username  string         `db:"username"`
				Nickname  sql.NullString `db:"nickname"`
				AvatarUrl sql.NullString `db:"avatar_url"`
			}
			if err := conn.QueryRowsCtx(ctx, &rows, query, args...); err != nil {
				return nil, err
			}
			out := make(map[int64]UserCard, len(rows))
			for _, row := range rows {
				out[row.Id] = UserCard{Id: row.Id, Username: row.Username, Nickname: row.Nickname.String, AvatarUrl: row.AvatarUrl.String}
			}
			return out, nil
		})
	if err != nil {
		return nil, err
	}
	result := make([]*UserCard, 0, len(cards))
	for _, id := range ids {
		if card, ok := cards[id]; ok {
			result = append(result, &card)
		}
	}
	return result, nil
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
	var total int64
	countQuery := fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE %s", m.table, searchPublicPredicate)
	if err := m.conn.QueryRowCtx(ctx, &total, countQuery, keyword, keyword, keyword); err != nil {
		return nil, 0, err
	}
	profiles, err := m.SearchPublicPage(ctx, keyword, offset, limit)
	if err != nil {
		return nil, 0, err
	}
	return profiles, total, nil
}

const searchPublicPredicate = "`status` = 1 AND (LOCATE(?, `username`) > 0 OR LOCATE(?, COALESCE(`nickname`, '')) > 0 OR LOCATE(?, COALESCE(`bio`, '')) > 0)"

// SearchPublicPage returns one page of SearchPublic without counting matches.
func (m *customUserProfileModel) SearchPublicPage(
	ctx context.Context,
	keyword string,
	offset, limit int64,
) ([]*UserProfile, error) {
	query := fmt.Sprintf(
		"SELECT %s FROM %s WHERE %s ORDER BY `follower_count` DESC, `id` ASC LIMIT ? OFFSET ?",
		userProfileRows, m.table, searchPublicPredicate,
	)
	profiles := make([]*UserProfile, 0, limit)
	if err := m.conn.QueryRowsCtx(ctx, &profiles, query, keyword, keyword, keyword, limit, offset); err != nil {
		return nil, err
	}
	return profiles, nil
}
