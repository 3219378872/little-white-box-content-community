package middleware

import (
	"context"
	"esx/pkg/jwtx"
)

// GetUserId 从 context 获取用户 ID（统一走 jwtx）
func GetUserId(ctx context.Context) int64 {
	userId, _ := jwtx.GetOptionalUserIdFromContext(ctx)
	return userId
}

// GetUsername 从 context 获取用户名（统一走 jwtx）
func GetUsername(ctx context.Context) string {
	username, _ := jwtx.GetUsernameFromContext(ctx)
	return username
}
