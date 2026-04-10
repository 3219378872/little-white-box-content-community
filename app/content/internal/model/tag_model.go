package model

import (
	"context"
	"fmt"

	"github.com/zeromicro/go-zero/core/stores/cache"
	"github.com/zeromicro/go-zero/core/stores/sqlx"
)

var _ TagModel = (*customTagModel)(nil)

type (
	// TagModel is an interface to be customized, add more methods here,
	// and implement the added methods in customTagModel.
	TagModel interface {
		tagModel
		FindList(ctx context.Context, limit int) ([]*Tag, error)
	}

	customTagModel struct {
		*defaultTagModel
	}
)

// NewTagModel returns a model for the database table.
func NewTagModel(conn sqlx.SqlConn, c cache.CacheConf, opts ...cache.Option) TagModel {
	return &customTagModel{
		defaultTagModel: newTagModel(conn, c, opts...),
	}
}

func (m *customTagModel) FindList(ctx context.Context, limit int) ([]*Tag, error) {
	if limit <= 0 {
		limit = 20
	}

	var tags []*Tag
	query := fmt.Sprintf("select %s from %s where `status` = 1 order by `post_count` desc limit ?", tagRows, m.table)
	err := m.CachedConn.QueryRowsNoCacheCtx(ctx, &tags, query, limit)
	if err != nil {
		return nil, err
	}
	return tags, nil
}
