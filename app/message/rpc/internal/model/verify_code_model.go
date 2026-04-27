package model

import (
	"github.com/zeromicro/go-zero/core/stores/cache"
	"github.com/zeromicro/go-zero/core/stores/sqlx"
)

var _ VerifyCodeModel = (*customVerifyCodeModel)(nil)

type (
	// VerifyCodeModel is an interface to be customized, add more methods here,
	// and implement the added methods in customVerifyCodeModel.
	VerifyCodeModel interface {
		verifyCodeModel
	}

	customVerifyCodeModel struct {
		*defaultVerifyCodeModel
	}
)

// NewVerifyCodeModel returns a model for the database table.
func NewVerifyCodeModel(conn sqlx.SqlConn, c cache.CacheConf, opts ...cache.Option) VerifyCodeModel {
	return &customVerifyCodeModel{
		defaultVerifyCodeModel: newVerifyCodeModel(conn, c, opts...),
	}
}
