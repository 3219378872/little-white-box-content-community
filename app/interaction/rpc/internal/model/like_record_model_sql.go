// Explicit SQL model. Maintained with its domain query extensions.

package model

import (
	"context"
	"database/sql"
	sqlc "esx/pkg/cachedstore"
	cache "esx/pkg/modelcache"
	sqlx "esx/pkg/sqlstore"
	"fmt"
	"time"
)

var (
	likeRecordRows                = "`id`,`user_id`,`target_id`,`target_type`,`status`,`created_at`,`updated_at`"
	likeRecordRowsExpectAutoSet   = "`user_id`,`target_id`,`target_type`,`status`"
	likeRecordRowsWithPlaceHolder = "`user_id`=?,`target_id`=?,`target_type`=?,`status`=?"

	cacheLikeRecordIdPrefix                       = "cache:likeRecord:id:"
	cacheLikeRecordUserIdTargetIdTargetTypePrefix = "cache:likeRecord:userId:targetId:targetType:"
)

type (
	likeRecordModel interface {
		Insert(ctx context.Context, data *LikeRecord) (sql.Result, error)
		FindOne(ctx context.Context, id int64) (*LikeRecord, error)
		FindOneByUserIdTargetIdTargetType(ctx context.Context, userId int64, targetId int64, targetType int64) (*LikeRecord, error)
		Update(ctx context.Context, data *LikeRecord) error
		Delete(ctx context.Context, id int64) error
	}

	defaultLikeRecordModel struct {
		sqlc.CachedConn
		table string
	}

	LikeRecord struct {
		Id         int64     `db:"id"`
		UserId     int64     `db:"user_id"`     // 用户ID
		TargetId   int64     `db:"target_id"`   // 目标ID
		TargetType int64     `db:"target_type"` // 目标类型 1:帖子 2:评论
		Status     int64     `db:"status"`      // 状态 0:取消 1:点赞
		CreatedAt  time.Time `db:"created_at"`  // 创建时间
		UpdatedAt  time.Time `db:"updated_at"`  // 更新时间
	}
)

// newLikeRecordModel creates the cached base model.
func newLikeRecordModel(conn sqlx.SqlConn, c cache.CacheConf, opts ...cache.Option) *defaultLikeRecordModel {
	return &defaultLikeRecordModel{
		CachedConn: sqlc.NewConn(conn, c, opts...),
		table:      "`like_record`",
	}
}

// Delete removes a like record and evicts its id and unique-key cache keys.
func (m *defaultLikeRecordModel) Delete(ctx context.Context, id int64) error {
	data, err := m.FindOne(ctx, id)
	if err != nil {
		return err
	}

	likeRecordIdKey := fmt.Sprintf("%s%v", cacheLikeRecordIdPrefix, id)
	likeRecordUserIdTargetIdTargetTypeKey := fmt.Sprintf("%s%v:%v:%v", cacheLikeRecordUserIdTargetIdTargetTypePrefix, data.UserId, data.TargetId, data.TargetType)
	_, err = m.ExecCtx(ctx, func(ctx context.Context, conn sqlx.SqlConn) (result sql.Result, err error) {
		query := fmt.Sprintf("delete from %s where `id` = ?", m.table)
		return conn.ExecCtx(ctx, query, id)
	}, likeRecordIdKey, likeRecordUserIdTargetIdTargetTypeKey)
	return err
}

// FindOne loads a like record by id through the cache.
func (m *defaultLikeRecordModel) FindOne(ctx context.Context, id int64) (*LikeRecord, error) {
	likeRecordIdKey := fmt.Sprintf("%s%v", cacheLikeRecordIdPrefix, id)
	var resp LikeRecord
	err := m.QueryRowCtx(ctx, &resp, likeRecordIdKey, func(ctx context.Context, conn sqlx.SqlConn, v any) error {
		query := fmt.Sprintf("select %s from %s where `id` = ? limit 1", likeRecordRows, m.table)
		return conn.QueryRowCtx(ctx, v, query, id)
	})
	switch err {
	case nil:
		return &resp, nil
	case sqlc.ErrNotFound:
		return nil, ErrNotFound
	default:
		return nil, err
	}
}

// FindOneByUserIdTargetIdTargetType loads a like record by its unique key; the
// cache maps that key to the id, then to the row.
func (m *defaultLikeRecordModel) FindOneByUserIdTargetIdTargetType(ctx context.Context, userId int64, targetId int64, targetType int64) (*LikeRecord, error) {
	likeRecordUserIdTargetIdTargetTypeKey := fmt.Sprintf("%s%v:%v:%v", cacheLikeRecordUserIdTargetIdTargetTypePrefix, userId, targetId, targetType)
	var resp LikeRecord
	err := m.QueryRowIndexCtx(ctx, &resp, likeRecordUserIdTargetIdTargetTypeKey, m.formatPrimary, func(ctx context.Context, conn sqlx.SqlConn, v any) (i any, e error) {
		query := fmt.Sprintf("select %s from %s where `user_id` = ? and `target_id` = ? and `target_type` = ? limit 1", likeRecordRows, m.table)
		if err := conn.QueryRowCtx(ctx, &resp, query, userId, targetId, targetType); err != nil {
			return nil, err
		}
		return resp.Id, nil
	}, m.queryPrimary)
	switch err {
	case nil:
		return &resp, nil
	case sqlc.ErrNotFound:
		return nil, ErrNotFound
	default:
		return nil, err
	}
}

// Insert creates a like record and evicts its cache keys.
func (m *defaultLikeRecordModel) Insert(ctx context.Context, data *LikeRecord) (sql.Result, error) {
	likeRecordIdKey := fmt.Sprintf("%s%v", cacheLikeRecordIdPrefix, data.Id)
	likeRecordUserIdTargetIdTargetTypeKey := fmt.Sprintf("%s%v:%v:%v", cacheLikeRecordUserIdTargetIdTargetTypePrefix, data.UserId, data.TargetId, data.TargetType)
	ret, err := m.ExecCtx(ctx, func(ctx context.Context, conn sqlx.SqlConn) (result sql.Result, err error) {
		query := fmt.Sprintf("insert into %s (%s) values (?, ?, ?, ?)", m.table, likeRecordRowsExpectAutoSet)
		return conn.ExecCtx(ctx, query, data.UserId, data.TargetId, data.TargetType, data.Status)
	}, likeRecordIdKey, likeRecordUserIdTargetIdTargetTypeKey)
	return ret, err
}

// Update rewrites a like record and evicts its cache keys.
func (m *defaultLikeRecordModel) Update(ctx context.Context, newData *LikeRecord) error {
	data, err := m.FindOne(ctx, newData.Id)
	if err != nil {
		return err
	}

	likeRecordIdKey := fmt.Sprintf("%s%v", cacheLikeRecordIdPrefix, data.Id)
	likeRecordUserIdTargetIdTargetTypeKey := fmt.Sprintf("%s%v:%v:%v", cacheLikeRecordUserIdTargetIdTargetTypePrefix, data.UserId, data.TargetId, data.TargetType)
	_, err = m.ExecCtx(ctx, func(ctx context.Context, conn sqlx.SqlConn) (result sql.Result, err error) {
		query := fmt.Sprintf("update %s set %s where `id` = ?", m.table, likeRecordRowsWithPlaceHolder)
		return conn.ExecCtx(ctx, query, newData.UserId, newData.TargetId, newData.TargetType, newData.Status, newData.Id)
	}, likeRecordIdKey, likeRecordUserIdTargetIdTargetTypeKey)
	return err
}

// formatPrimary builds the id cache key used by the unique-key index cache.
func (m *defaultLikeRecordModel) formatPrimary(primary any) string {
	return fmt.Sprintf("%s%v", cacheLikeRecordIdPrefix, primary)
}

// queryPrimary loads the row for an id resolved from the unique-key cache.
func (m *defaultLikeRecordModel) queryPrimary(ctx context.Context, conn sqlx.SqlConn, v, primary any) error {
	query := fmt.Sprintf("select %s from %s where `id` = ? limit 1", likeRecordRows, m.table)
	return conn.QueryRowCtx(ctx, v, query, primary)
}
