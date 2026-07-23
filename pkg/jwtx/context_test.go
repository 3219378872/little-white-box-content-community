package jwtx

import (
	"context"
	"encoding/json"
	"testing"
)

func TestWithClaimsContext_StoresUserIDAsJsonNumber(t *testing.T) {
	ctx := WithClaimsContext(context.Background(), &Claims{
		UserId:   42,
		Username: "alice",
	})

	got, ok := GetOptionalUserIdFromContext(ctx)
	if !ok {
		t.Fatal("expected user id in context")
	}
	if got != 42 {
		t.Fatalf("expected user id 42, got %d", got)
	}
}

func TestGetOptionalUserIdFromContext_GoZeroJWTCompatibility(t *testing.T) {
	tests := []struct {
		name  string
		value any
		want  int64
		ok    bool
	}{
		{name: "json number", value: json.Number("42"), want: 42, ok: true},
		{name: "map claims float", value: float64(42), want: 42, ok: true},
		{name: "fractional claim", value: float64(42.5), ok: false},
		{name: "string claim", value: "42", want: 42, ok: true},
		{name: "malformed claim", value: "not-a-number", ok: false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			//nolint:staticcheck // Reproduce go-zero's built-in JWT claim context.
			ctx := context.WithValue(context.Background(), "userId", tt.value)
			got, ok := GetOptionalUserIdFromContext(ctx)
			if ok != tt.ok || got != tt.want {
				t.Fatalf("got (%d, %v), want (%d, %v)", got, ok, tt.want, tt.ok)
			}
		})
	}
}

func TestGetUsernameFromContext_GoZeroJWTCompatibility(t *testing.T) {
	//nolint:staticcheck // Reproduce go-zero's built-in JWT claim context.
	ctx := context.WithValue(context.Background(), "username", "alice")
	got, ok := GetUsernameFromContext(ctx)
	if !ok || got != "alice" {
		t.Fatalf("got (%q, %v), want (%q, true)", got, ok, "alice")
	}
}

func TestGetOptionalUserIdFromContext_Missing_ReturnsFalse(t *testing.T) {
	got, ok := GetOptionalUserIdFromContext(context.Background())
	if ok {
		t.Fatalf("expected no user id, got %d", got)
	}
}

func TestGetUserIdFromContext_UsesOptionalHelper(t *testing.T) {
	ctx := WithClaimsContext(context.Background(), &Claims{
		UserId:   7,
		Username: "bob",
	})

	got, err := GetUserIdFromContext(ctx)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != 7 {
		t.Fatalf("expected 7, got %d", got)
	}
}
