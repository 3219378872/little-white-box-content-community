package middleware

import (
	"errors"
	"net/http"
	"strings"

	"esx/pkg/errx"
	"esx/pkg/jwtx"

	"encoding/json"
)

// RequiredAuthMiddleware authenticates protected REST routes without using
// framework JWT handlers that may dump the complete HTTP request on failure.
type RequiredAuthMiddleware struct {
	config jwtx.JwtConfig
}

func NewRequiredAuthMiddleware(config jwtx.JwtConfig) *RequiredAuthMiddleware {
	return &RequiredAuthMiddleware{config: config}
}

func (m *RequiredAuthMiddleware) Handle(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		authHeader := strings.TrimSpace(r.Header.Get("Authorization"))
		parts := strings.Fields(authHeader)
		if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
			writeUnauthorized(w, AuthStateInvalid)
			return
		}

		claims, err := jwtx.ParseToken(parts[1], m.config)
		if err != nil {
			state := AuthStateInvalid
			if errors.Is(err, jwtx.ErrTokenExpired) {
				state = AuthStateExpired
			}
			writeUnauthorized(w, state)
			return
		}

		w.Header().Set(AuthStateHeader, AuthStateAuthenticated)
		next(w, r.WithContext(jwtx.WithClaimsContext(r.Context(), claims)))
	}
}

func writeUnauthorized(w http.ResponseWriter, state string) {
	w.Header().Set(AuthStateHeader, state)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(http.StatusUnauthorized)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"code":    errx.LoginRequired,
		"message": errx.GetMsg(errx.LoginRequired),
	})
}
