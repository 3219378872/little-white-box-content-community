package middleware

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"jwtx"
)

func runOptionalAuthRequest(t *testing.T, authHeader string, expire int64) (int, string, string) {
	t.Helper()

	mw := NewOptionalAuthMiddleware(jwtx.JwtConfig{
		AccessSecret: "secret",
		AccessExpire: expire,
	})

	var seenUser string
	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if userID, ok := jwtx.GetOptionalUserIdFromContext(r.Context()); ok {
			seenUser = fmt.Sprintf("%d", userID)
		} else {
			seenUser = "anonymous"
		}
		_, _ = w.Write([]byte("ok"))
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/posts", nil)
	if authHeader != "" {
		req.Header.Set("Authorization", authHeader)
	}
	rec := httptest.NewRecorder()
	mw.Handle(next)(rec, req)

	body, _ := io.ReadAll(rec.Result().Body)
	return rec.Result().StatusCode, rec.Header().Get(AuthStateHeader), seenUser + ":" + string(body)
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
	status, authState, seen := runOptionalAuthRequest(t, strings.TrimSpace("Token abc"), 3600)
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
