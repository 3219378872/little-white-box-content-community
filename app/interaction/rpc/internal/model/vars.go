package model

import "github.com/zeromicro/go-zero/core/stores/sqlx"

var ErrNotFound = sqlx.ErrNotFound

// 状态常量
const (
	StatusInactive = 0
	StatusActive   = 1
)

// 缓存 TTL 常量 (秒)
const (
	CacheShortTTL = 30  // 空数据防穿透缓存
	CacheLongTTL  = 300 // 正常数据缓存
)
