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
	if id, ok := parseUserID(ctx.Value(ctxUserIDKey)); ok {
		return id, true
	}
	// go-zero's built-in JWT middleware stores custom claims with string keys.
	return parseUserID(ctx.Value("userId"))
}

func parseUserID(raw any) (int64, bool) {
	switch value := raw.(type) {
	case json.Number:
		id, err := value.Int64()
		if err != nil {
			return 0, false
		}
		return id, true
	case int64:
		return value, true
	case float64:
		if value != float64(int64(value)) {
			return 0, false
		}
		return int64(value), true
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
	if username, ok := ctx.Value(ctxUsernameKey).(string); ok {
		return username, true
	}
	username, ok := ctx.Value("username").(string)
	return username, ok
}
