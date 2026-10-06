package middleware

import (
	"context"
	"fmt"
	"net/http"
	"testing"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/ut"

	"esx/pkg/jwtx"
)

// runOptionalAuthRequest 经 OptionalAuth 的 Hertz 中间件发一次请求，返回状态码、认证状态头与处理器看到的身份。
func runOptionalAuthRequest(t *testing.T, authHeader string, expire int64) (int, string, string) {
	t.Helper()

	mw := NewOptionalAuthMiddleware(jwtx.JwtConfig{
		AccessSecret: "secret",
		AccessExpire: expire,
	})

	var seenUser string
	next := func(ctx context.Context, c *app.RequestContext) {
		if userID, ok := jwtx.GetOptionalUserIdFromContext(ctx); ok {
			seenUser = fmt.Sprintf("%d", userID)
		} else {
			seenUser = "anonymous"
		}
		c.String(http.StatusOK, "ok")
	}

	var headers []ut.Header
	if authHeader != "" {
		headers = append(headers, ut.Header{Key: "Authorization", Value: authHeader})
	}
	resp := performHertz(mw.Hertz, next, http.MethodGet, "/api/v1/posts", headers...).Result()
	return resp.StatusCode(), string(resp.Header.Peek(AuthStateHeader)), seenUser + ":" + string(resp.Body())
}

func TestOptionalAuthMiddleware_NoToken_SetsAnonymous(t *testing.T) {
	status, authState, seen := runOptionalAuthRequest(t, "", 3600)
	if status != http.StatusOK {
		t.Fatalf("expected 200, got %d", status)
	}
	if authState != AuthStateAnonymous {
		t.Fatalf("expected %q, got %q", AuthStateAnonymous, authState)
	}
	if seen != "anonymous:ok" {
		t.Fatalf("expected anonymous context, got %s", seen)
	}
}

func TestOptionalAuthMiddleware_ValidToken_SetsAuthenticated(t *testing.T) {
	token, err := jwtx.GenerateToken(42, "alice", jwtx.JwtConfig{
		AccessSecret: "secret",
		AccessExpire: 3600,
	})
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}

	status, authState, seen := runOptionalAuthRequest(t, "Bearer "+token, 3600)
	if status != http.StatusOK {
		t.Fatalf("expected 200, got %d", status)
	}
	if authState != AuthStateAuthenticated {
		t.Fatalf("expected %q, got %q", AuthStateAuthenticated, authState)
	}
	if seen != "42:ok" {
		t.Fatalf("expected authenticated context, got %s", seen)
	}
}

func TestOptionalAuthMiddleware_ExpiredToken_SetsExpired(t *testing.T) {
	token, err := jwtx.GenerateToken(42, "alice", jwtx.JwtConfig{
		AccessSecret: "secret",
		AccessExpire: -1,
	})
	if err != nil {
		t.Fatalf("GenerateToken: %v", err)
	}

	status, authState, seen := runOptionalAuthRequest(t, "Bearer "+token, 3600)
	if status != http.StatusOK {
		t.Fatalf("expected 200, got %d", status)
	}
	if authState != AuthStateExpired {
		t.Fatalf("expected %q, got %q", AuthStateExpired, authState)
	}
	if seen != "anonymous:ok" {
		t.Fatalf("expected anonymous fallback, got %s", seen)
	}
}

func TestOptionalAuthMiddleware_InvalidToken_SetsInvalid(t *testing.T) {
	status, authState, seen := runOptionalAuthRequest(t, "Bearer not-a-token", 3600)
	if status != http.StatusOK {
		t.Fatalf("expected 200, got %d", status)
	}
	if authState != AuthStateInvalid {
		t.Fatalf("expected %q, got %q", AuthStateInvalid, authState)
	}
	if seen != "anonymous:ok" {
		t.Fatalf("expected anonymous fallback, got %s", seen)
	}
}

func TestOptionalAuthMiddleware_BadBearerFormat_SetsInvalid(t *testing.T) {
	status, authState, seen := runOptionalAuthRequest(t, "Token abc", 3600)
	if status != http.StatusOK {
		t.Fatalf("expected 200, got %d", status)
	}
	if authState != AuthStateInvalid {
		t.Fatalf("expected %q, got %q", AuthStateInvalid, authState)
	}
	if seen != "anonymous:ok" {
		t.Fatalf("expected anonymous fallback, got %s", seen)
	}
}
