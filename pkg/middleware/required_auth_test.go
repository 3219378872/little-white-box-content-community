package middleware

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/ut"

	"esx/pkg/errx"
	"esx/pkg/jwtx"
)

func TestRequiredAuthRejectsMissingToken(t *testing.T) {
	middleware := NewRequiredAuthMiddleware(jwtx.JwtConfig{AccessSecret: "test-secret"})
	called := false

	resp := performHertz(middleware.Hertz, func(context.Context, *app.RequestContext) { called = true },
		http.MethodPost, "/protected").Result()

	if called || resp.StatusCode() != http.StatusUnauthorized {
		t.Fatalf("called=%v status=%d", called, resp.StatusCode())
	}
	if got := string(resp.Header.Peek(AuthStateHeader)); got != AuthStateInvalid {
		t.Fatalf("auth state=%q, want %q", got, AuthStateInvalid)
	}
	var body map[string]any
	if err := json.Unmarshal(resp.Body(), &body); err != nil {
		t.Fatal(err)
	}
	if int(body["code"].(float64)) != errx.LoginRequired {
		t.Fatalf("body=%v", body)
	}
}

func TestRequiredAuthInjectsClaimsAndRejectsRefreshAndExpiredTokens(t *testing.T) {
	config := jwtx.JwtConfig{
		AccessSecret:  "access-secret",
		AccessExpire:  60,
		RefreshSecret: "refresh-secret",
		RefreshExpire: 60,
	}
	access, err := jwtx.GenerateToken(42, "alice", config)
	if err != nil {
		t.Fatal(err)
	}
	refresh, err := jwtx.GenerateRefreshToken(42, "alice", config)
	if err != nil {
		t.Fatal(err)
	}
	expiredConfig := config
	expiredConfig.AccessExpire = -1
	expired, err := jwtx.GenerateToken(42, "alice", expiredConfig)
	if err != nil {
		t.Fatal(err)
	}
	middleware := NewRequiredAuthMiddleware(config)
	handler := func(ctx context.Context, c *app.RequestContext) {
		userID, getErr := jwtx.GetUserIdFromContext(ctx)
		if getErr != nil || userID != 42 {
			t.Fatalf("userID=%d err=%v", userID, getErr)
		}
		c.Status(http.StatusNoContent)
	}

	for name, tc := range map[string]struct {
		token     string
		status    int
		authState string
	}{
		"access":  {access, http.StatusNoContent, AuthStateAuthenticated},
		"refresh": {refresh, http.StatusUnauthorized, AuthStateInvalid},
		"expired": {expired, http.StatusUnauthorized, AuthStateExpired},
	} {
		t.Run(name, func(t *testing.T) {
			resp := performHertz(middleware.Hertz, handler, http.MethodGet, "/protected",
				ut.Header{Key: "Authorization", Value: "Bearer " + tc.token}).Result()
			if resp.StatusCode() != tc.status {
				t.Fatalf("status=%d", resp.StatusCode())
			}
			if got := string(resp.Header.Peek(AuthStateHeader)); got != tc.authState {
				t.Fatalf("auth state=%q, want %q", got, tc.authState)
			}
		})
	}
}
