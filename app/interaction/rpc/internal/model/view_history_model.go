package model

import (
	"github.com/zeromicro/go-zero/core/stores/cache"
	"github.com/zeromicro/go-zero/core/stores/sqlx"
)

var _ ViewHistoryModel = (*customViewHistoryModel)(nil)

type (
	// ViewHistoryModel is an interface to be customized, add more methods here,
	// and implement the added methods in customViewHistoryModel.
	ViewHistoryModel interface {
		viewHistoryModel
	}

	customViewHistoryModel struct {
		*defaultViewHistoryModel
	}
)

// NewViewHistoryModel returns a model for the database table.
func NewViewHistoryModel(conn sqlx.SqlConn, c cache.CacheConf, opts ...cache.Option) ViewHistoryModel {
	return &customViewHistoryModel{
		defaultViewHistoryModel: newViewHistoryModel(conn, c, opts...),
	}
}
