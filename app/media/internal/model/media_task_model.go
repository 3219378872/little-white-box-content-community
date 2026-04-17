package model

import (
	"github.com/zeromicro/go-zero/core/stores/cache"
	"github.com/zeromicro/go-zero/core/stores/sqlx"
)

var _ MediaTaskModel = (*customMediaTaskModel)(nil)

type (
	// MediaTaskModel is an interface to be customized, add more methods here,
	// and implement the added methods in customMediaTaskModel.
	MediaTaskModel interface {
		mediaTaskModel
	}

	customMediaTaskModel struct {
		*defaultMediaTaskModel
	}
)

// NewMediaTaskModel returns a model for the database table.
func NewMediaTaskModel(conn sqlx.SqlConn, c cache.CacheConf, opts ...cache.Option) MediaTaskModel {
	return &customMediaTaskModel{
		defaultMediaTaskModel: newMediaTaskModel(conn, c, opts...),
	}
}
