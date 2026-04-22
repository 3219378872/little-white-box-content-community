package middleware

import (
	"errors"
	"net/http"
	"strings"

	"jwtx"
)

const (
	AuthStateHeader        = "X-Auth-State"
	AuthStateAnonymous     = "anonymous"
	AuthStateAuthenticated = "authenticated"
	AuthStateExpired       = "expired"
	AuthStateInvalid       = "invalid"
)

type OptionalAuthMiddleware struct {
	config jwtx.JwtConfig
}

func NewOptionalAuthMiddleware(config jwtx.JwtConfig) *OptionalAuthMiddleware {
	return &OptionalAuthMiddleware{config: config}
}

func (m *OptionalAuthMiddleware) Handle(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			w.Header().Set(AuthStateHeader, AuthStateAnonymous)
			next(w, r)
			return
		}

		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			w.Header().Set(AuthStateHeader, AuthStateInvalid)
			next(w, r)
			return
		}

		claims, err := jwtx.ParseToken(parts[1], m.config)
		if err != nil {
			if errors.Is(err, jwtx.ErrTokenExpired) {
				w.Header().Set(AuthStateHeader, AuthStateExpired)
			} else {
				w.Header().Set(AuthStateHeader, AuthStateInvalid)
			}
			next(w, r)
			return
		}

		w.Header().Set(AuthStateHeader, AuthStateAuthenticated)
		next(w, r.WithContext(jwtx.WithClaimsContext(r.Context(), claims)))
	}
}
