package middleware

import "esx/pkg/jwtx"

// RequiredAuthMiddleware authenticates protected REST routes without using
// framework JWT handlers that may dump the complete HTTP request on failure.
type RequiredAuthMiddleware struct {
	config jwtx.JwtConfig
}

// NewRequiredAuthMiddleware validates tokens with the gateway JWT configuration.
func NewRequiredAuthMiddleware(config jwtx.JwtConfig) *RequiredAuthMiddleware {
	return &RequiredAuthMiddleware{config: config}
}
