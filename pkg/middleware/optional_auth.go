package middleware

import "esx/pkg/jwtx"

const (
	AuthStateHeader        = "X-Auth-State"
	AuthStateAnonymous     = "anonymous"
	AuthStateAuthenticated = "authenticated"
	AuthStateExpired       = "expired"
	AuthStateInvalid       = "invalid"
)

// OptionalAuthMiddleware 允许匿名访问，令牌有效时把登录身份放入上下文。
type OptionalAuthMiddleware struct {
	config jwtx.JwtConfig
}

// NewOptionalAuthMiddleware 使用网关的 JWT 配置校验令牌。
func NewOptionalAuthMiddleware(config jwtx.JwtConfig) *OptionalAuthMiddleware {
	return &OptionalAuthMiddleware{config: config}
}
