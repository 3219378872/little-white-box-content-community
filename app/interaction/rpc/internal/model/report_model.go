package model

import (
	"github.com/zeromicro/go-zero/core/stores/cache"
	"github.com/zeromicro/go-zero/core/stores/sqlx"
)

var _ ReportModel = (*customReportModel)(nil)

type (
	// ReportModel is an interface to be customized, add more methods here,
	// and implement the added methods in customReportModel.
	ReportModel interface {
		reportModel
	}

	customReportModel struct {
		*defaultReportModel
	}
)

// NewReportModel returns a model for the database table.
func NewReportModel(conn sqlx.SqlConn, c cache.CacheConf, opts ...cache.Option) ReportModel {
	return &customReportModel{
		defaultReportModel: newReportModel(conn, c, opts...),
	}
}
