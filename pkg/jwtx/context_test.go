package jwtx

import (
	"context"
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
