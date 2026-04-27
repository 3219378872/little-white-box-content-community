package model

import (
	"github.com/zeromicro/go-zero/core/stores/cache"
	"github.com/zeromicro/go-zero/core/stores/sqlx"
)

var _ FavoriteFolderModel = (*customFavoriteFolderModel)(nil)

type (
	// FavoriteFolderModel is an interface to be customized, add more methods here,
	// and implement the added methods in customFavoriteFolderModel.
	FavoriteFolderModel interface {
		favoriteFolderModel
	}

	customFavoriteFolderModel struct {
		*defaultFavoriteFolderModel
	}
)

// NewFavoriteFolderModel returns a model for the database table.
func NewFavoriteFolderModel(conn sqlx.SqlConn, c cache.CacheConf, opts ...cache.Option) FavoriteFolderModel {
	return &customFavoriteFolderModel{
		defaultFavoriteFolderModel: newFavoriteFolderModel(conn, c, opts...),
	}
}
