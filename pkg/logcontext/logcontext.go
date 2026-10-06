package logcontext

import (
	"context"
	"esx/pkg/logging"
)

var Field = logging.Field

// Errorw 以请求上下文记录错误日志，供没有 Logger 字段的辅助函数使用。
func Errorw(ctx context.Context, msg string, fields ...logging.LogField) {
	logging.WithContext(ctx).Errorw(msg, fields...)
}
