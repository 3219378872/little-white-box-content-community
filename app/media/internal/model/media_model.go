package model

import (
	"context"
	"fmt"
	"strings"

	"github.com/zeromicro/go-zero/core/stores/cache"
	"github.com/zeromicro/go-zero/core/stores/sqlx"
)

var _ MediaModel = (*customMediaModel)(nil)

type (
	// MediaModel is an interface to be customized, add more methods here,
	// and implement the added methods in customMediaModel.
	MediaModel interface {
		mediaModel
		FindByIds(ctx context.Context, ids []int64) ([]*Media, error)
	}

	customMediaModel struct {
		*defaultMediaModel
	}
)

// NewMediaModel returns a model for the database table.
func NewMediaModel(conn sqlx.SqlConn, c cache.CacheConf, opts ...cache.Option) MediaModel {
	return &customMediaModel{
		defaultMediaModel: newMediaModel(conn, c, opts...),
	}
}

// FindByIds 批量按主键查询，一次 WHERE IN 查询，不走缓存。
// 返回切片顺序不保证与入参 ids 一致；软删记录（status=0）由调用方过滤。
func (m *customMediaModel) FindByIds(ctx context.Context, ids []int64) ([]*Media, error) {
	if len(ids) == 0 {
		return []*Media{}, nil
	}
	args := make([]any, 0, len(ids))
	for _, id := range ids {
		args = append(args, id)
	}
	placeholders := strings.Repeat("?,", len(ids))
	placeholders = placeholders[:len(placeholders)-1]
	query := fmt.Sprintf("select %s from %s where `id` IN (%s)", mediaRows, m.table, placeholders)
	var result []*Media
	if err := m.CachedConn.QueryRowsNoCacheCtx(ctx, &result, query, args...); err != nil {
		return nil, err
	}
	return result, nil
}
