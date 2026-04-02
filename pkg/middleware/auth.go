package middleware

import (
	"context"
	"jwtx"
	"net/http"
	"strings"
)

// AuthMiddleware HTTP 认证中间件
func AuthMiddleware(config jwtx.JwtConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// 从 Header 获取 token
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				writeUnauthorized(w)
				return
			}

			// 解析 Bearer token
			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || parts[0] != "Bearer" {
				writeUnauthorized(w)
				return
			}

			tokenString := parts[1]

			// 解析 token
			claims, err := jwtx.ParseToken(tokenString, config)
			if err != nil {
				writeUnauthorized(w)
				return
			}

			// 将用户信息存入 context
			ctx := r.Context()
			ctx = contextWithUserId(ctx, claims.UserId)
			ctx = contextWithUsername(ctx, claims.Username)

			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// writeUnauthorized 写入未授权响应
func writeUnauthorized(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusUnauthorized)
	w.Write([]byte(`{"code":1006,"message":"请先登录","data":null}`))
}

// context key
type ctxKey string

const (
	userIdKey   ctxKey = "userId"
	usernameKey ctxKey = "username"
)

func contextWithUserId(ctx context.Context, userId int64) context.Context {
	return context.WithValue(ctx, userIdKey, userId)
}

func contextWithUsername(ctx context.Context, username string) context.Context {
	return context.WithValue(ctx, usernameKey, username)
}

// GetUserId 从 context 获取用户 ID
func GetUserId(ctx context.Context) int64 {
	userId, _ := ctx.Value(userIdKey).(int64)
	return userId
}

// GetUsername 从 context 获取用户名
func GetUsername(ctx context.Context) string {
	username, _ := ctx.Value(usernameKey).(string)
	return username
}
