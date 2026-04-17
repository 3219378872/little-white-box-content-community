package model

import (
	"github.com/zeromicro/go-zero/core/stores/cache"
	"github.com/zeromicro/go-zero/core/stores/sqlx"
)

var _ MediaModel = (*customMediaModel)(nil)

type (
	// MediaModel is an interface to be customized, add more methods here,
	// and implement the added methods in customMediaModel.
	MediaModel interface {
		mediaModel
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
