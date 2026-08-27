package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"esx/pkg/errx"
	"esx/pkg/jwtx"
)

func TestRequiredAuthRejectsMissingToken(t *testing.T) {
	middleware := NewRequiredAuthMiddleware(jwtx.JwtConfig{AccessSecret: "test-secret"})
	called := false
	handler := middleware.Handle(func(http.ResponseWriter, *http.Request) { called = true })
	req := httptest.NewRequest(http.MethodPost, "/protected", nil)
	rec := httptest.NewRecorder()

	handler(rec, req)

	if called || rec.Code != http.StatusUnauthorized {
		t.Fatalf("called=%v status=%d", called, rec.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatal(err)
	}
	if int(body["code"].(float64)) != errx.LoginRequired {
		t.Fatalf("body=%v", body)
	}
}

func TestRequiredAuthInjectsClaimsAndRejectsRefreshToken(t *testing.T) {
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
	middleware := NewRequiredAuthMiddleware(config)
	handler := middleware.Handle(func(w http.ResponseWriter, r *http.Request) {
		userID, getErr := jwtx.GetUserIdFromContext(r.Context())
		if getErr != nil || userID != 42 {
			t.Fatalf("userID=%d err=%v", userID, getErr)
		}
		w.WriteHeader(http.StatusNoContent)
	})

	for name, tc := range map[string]struct {
		token  string
		status int
	}{
		"access":  {access, http.StatusNoContent},
		"refresh": {refresh, http.StatusUnauthorized},
	} {
		t.Run(name, func(t *testing.T) {
			req := httptest.NewRequest(http.MethodGet, "/protected", nil)
			req.Header.Set("Authorization", "Bearer "+tc.token)
			rec := httptest.NewRecorder()
			handler(rec, req)
			if rec.Code != tc.status {
				t.Fatalf("status=%d", rec.Code)
			}
		})
	}
}
