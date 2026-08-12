package model

import (
	"errors"

	"github.com/zeromicro/go-zero/core/stores/sqlx"
)

var ErrNotFound = sqlx.ErrNotFound

// ErrVersionConflict 表示内容 revision 不匹配，调用方应返回 409 Conflict。
var ErrVersionConflict = errors.New("content version conflict")
