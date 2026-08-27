package middleware

import (
	"errors"
	"net/http"
	"strings"

	"esx/pkg/errx"
	"esx/pkg/jwtx"

	"github.com/zeromicro/go-zero/rest/httpx"
)

// RequiredAuthMiddleware authenticates protected REST routes without using
// go-zero's JWT handler, whose failure path dumps the complete HTTP request.
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
	httpx.WriteJson(w, http.StatusUnauthorized, map[string]any{
		"code":    errx.LoginRequired,
		"message": errx.GetMsg(errx.LoginRequired),
	})
}
