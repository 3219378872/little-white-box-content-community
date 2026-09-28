package logcontext

import (
	"context"
	"esx/pkg/logging"
)

var Field = logging.Field

func Errorw(ctx context.Context, msg string, fields ...logging.LogField) {
	logging.WithContext(ctx).Errorw(msg, fields...)
}
