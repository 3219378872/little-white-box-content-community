package jwtx

import (
	"context"
	"encoding/json"
	"strconv"
)

const (
	ContextUserIDKey   = "userId"
	ContextUsernameKey = "username"
)

func WithClaimsContext(ctx context.Context, claims *Claims) context.Context {
	if claims == nil {
		return ctx
	}

	ctx = context.WithValue(
		ctx,
		ContextUserIDKey,
		json.Number(strconv.FormatInt(claims.UserId, 10)),
	)
	ctx = context.WithValue(ctx, ContextUsernameKey, claims.Username)

	return ctx
}

func GetOptionalUserIdFromContext(ctx context.Context) (int64, bool) {
	switch value := ctx.Value(ContextUserIDKey).(type) {
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
