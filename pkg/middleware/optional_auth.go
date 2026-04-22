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

func OptionalAuthMiddleware(config jwtx.JwtConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				w.Header().Set(AuthStateHeader, AuthStateAnonymous)
				next.ServeHTTP(w, r)
				return
			}

			parts := strings.SplitN(authHeader, " ", 2)
			if len(parts) != 2 || parts[0] != "Bearer" {
				w.Header().Set(AuthStateHeader, AuthStateInvalid)
				next.ServeHTTP(w, r)
				return
			}

			claims, err := jwtx.ParseToken(parts[1], config)
			if err != nil {
				if errors.Is(err, jwtx.ErrTokenExpired) {
					w.Header().Set(AuthStateHeader, AuthStateExpired)
				} else {
					w.Header().Set(AuthStateHeader, AuthStateInvalid)
				}
				next.ServeHTTP(w, r)
				return
			}

			w.Header().Set(AuthStateHeader, AuthStateAuthenticated)
			next.ServeHTTP(w, r.WithContext(jwtx.WithClaimsContext(r.Context(), claims)))
		})
	}
}
