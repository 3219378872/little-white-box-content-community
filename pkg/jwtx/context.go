package jwtx

import (
	"context"
	"encoding/json"
	"strconv"
)

// contextKey 强类型 context key，防止与外部包字符串冲突
type contextKey string

const (
	ctxUserIDKey   contextKey = "userId"
	ctxUsernameKey contextKey = "username"
)

// WithClaimsContext 将 JWT Claims 注入 context，使用强类型 key
func WithClaimsContext(ctx context.Context, claims *Claims) context.Context {
	if claims == nil {
		return ctx
	}

	ctx = context.WithValue(
		ctx,
		ctxUserIDKey,
		json.Number(strconv.FormatInt(claims.UserId, 10)),
	)
	ctx = context.WithValue(ctx, ctxUsernameKey, claims.Username)

	return ctx
}

// WithUserIdContext 将用户 ID 注入 context（非 JWT 场景，如中间件直接写入）
func WithUserIdContext(ctx context.Context, userId int64) context.Context {
	return context.WithValue(ctx, ctxUserIDKey, json.Number(strconv.FormatInt(userId, 10)))
}

// WithUsernameContext 将用户名注入 context（非 JWT 场景）
func WithUsernameContext(ctx context.Context, username string) context.Context {
	return context.WithValue(ctx, ctxUsernameKey, username)
}

func GetOptionalUserIdFromContext(ctx context.Context) (int64, bool) {
	switch value := ctx.Value(ctxUserIDKey).(type) {
	case json.Number:
		id, err := value.Int64()
		if err != nil {
			return 0, false
		}
		return id, true
	case int64:
		return value, true
	case string:
		id, err := strconv.ParseInt(value, 10, 64)
		if err != nil {
			return 0, false
		}
		return id, true
	default:
		return 0, false
	}
}

// GetUsernameFromContext 从上下文中获取用户名
func GetUsernameFromContext(ctx context.Context) (string, bool) {
	username, ok := ctx.Value(ctxUsernameKey).(string)
	return username, ok
}
