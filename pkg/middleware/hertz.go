package middleware

import (
	"context"
	"errors"
	"esx/pkg/errx"
	"esx/pkg/httpx"
	"esx/pkg/jwtx"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/cloudwego/hertz/pkg/app"
)

func (m *RequiredAuthMiddleware) Hertz(ctx context.Context, c *app.RequestContext) {
	parts := strings.Fields(strings.TrimSpace(string(c.GetHeader("Authorization"))))
	if len(parts) != 2 || !strings.EqualFold(parts[0], "Bearer") {
		rejectAuth(ctx, c, AuthStateInvalid)
		return
	}
	claims, err := jwtx.ParseToken(parts[1], m.config)
	if err != nil {
		state := AuthStateInvalid
		if errors.Is(err, jwtx.ErrTokenExpired) {
			state = AuthStateExpired
		}
		rejectAuth(ctx, c, state)
		return
	}
	c.Header(AuthStateHeader, AuthStateAuthenticated)
	c.Next(jwtx.WithClaimsContext(ctx, claims))
}
func rejectAuth(ctx context.Context, c *app.RequestContext, state string) {
	c.Header(AuthStateHeader, state)
	httpx.ErrorCtx(ctx, c, errx.NewWithCode(errx.LoginRequired))
	c.Abort()
}
func (m *OptionalAuthMiddleware) Hertz(ctx context.Context, c *app.RequestContext) {
	header := string(c.GetHeader("Authorization"))
	if header == "" {
		c.Header(AuthStateHeader, AuthStateAnonymous)
		c.Next(ctx)
		return
	}
	parts := strings.SplitN(header, " ", 2)
	if len(parts) != 2 || parts[0] != "Bearer" {
		c.Header(AuthStateHeader, AuthStateInvalid)
		c.Next(ctx)
		return
	}
	claims, err := jwtx.ParseToken(parts[1], m.config)
	if err != nil {
		state := AuthStateInvalid
		if errors.Is(err, jwtx.ErrTokenExpired) {
			state = AuthStateExpired
		}
		c.Header(AuthStateHeader, state)
		c.Next(ctx)
		return
	}
	c.Header(AuthStateHeader, AuthStateAuthenticated)
	c.Next(jwtx.WithClaimsContext(ctx, claims))
}
func HertzCORS(config CORSConfig) app.HandlerFunc {
	return func(ctx context.Context, c *app.RequestContext) {
		origin := string(c.GetHeader("Origin"))
		if origin == "" {
			c.Next(ctx)
			return
		}
		c.Header("Vary", "Origin")
		if slices.Contains(config.AllowOrigins, origin) {
			c.Header("Access-Control-Allow-Origin", origin)
		} else if !config.AllowCredentials && len(config.AllowOrigins) == 1 && config.AllowOrigins[0] == "*" {
			c.Header("Access-Control-Allow-Origin", "*")
		}
		c.Header("Access-Control-Allow-Methods", strings.Join(config.AllowMethods, ", "))
		c.Header("Access-Control-Allow-Headers", strings.Join(config.AllowHeaders, ", "))
		if len(config.ExposeHeaders) > 0 {
			c.Header("Access-Control-Expose-Headers", strings.Join(config.ExposeHeaders, ", "))
		}
		if config.AllowCredentials {
			c.Header("Access-Control-Allow-Credentials", "true")
		}
		if config.MaxAge > 0 {
			c.Header("Access-Control-Max-Age", strconv.Itoa(config.MaxAge))
		}
		if string(c.Method()) == http.MethodOptions {
			c.Status(http.StatusNoContent)
			c.Abort()
			return
		}
		c.Next(ctx)
	}
}
