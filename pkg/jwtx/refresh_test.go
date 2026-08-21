package jwtx

import (
	"errors"
	"testing"
)

func dualTokenConfig() JwtConfig {
	return JwtConfig{
		AccessSecret:  "access-secret-for-tests",
		AccessExpire:  1800,
		RefreshSecret: "refresh-secret-for-tests",
		RefreshExpire: 7 * 24 * 3600,
	}
}

func TestGenerateRefreshTokenAndParse(t *testing.T) {
	cfg := dualTokenConfig()
	token, err := GenerateRefreshToken(42, "alice", cfg)
	if err != nil {
		t.Fatal(err)
	}
	claims, err := ParseRefreshToken(token, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if claims.UserId != 42 || claims.Username != "alice" {
		t.Fatalf("unexpected claims: %+v", claims)
	}
	if claims.TokenType != TokenTypeRefresh {
		t.Fatalf("token type = %q, want refresh", claims.TokenType)
	}
	if claims.ID == "" {
		t.Fatal("refresh token must carry a jti for server-side rotation")
	}
}

func TestAccessTokenCannotBeUsedAsRefresh(t *testing.T) {
	cfg := dualTokenConfig()
	access, err := GenerateToken(1, "bob", cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseRefreshToken(access, cfg); !errors.Is(err, ErrTokenInvalid) {
		t.Fatalf("access token accepted as refresh: %v", err)
	}
}

func TestRefreshTokenCannotBeUsedAsAccess(t *testing.T) {
	cfg := dualTokenConfig()
	refresh, err := GenerateRefreshToken(1, "bob", cfg)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseToken(refresh, cfg); !errors.Is(err, ErrTokenInvalid) {
		t.Fatalf("refresh token accepted as access: %v", err)
	}
}

func TestRefreshTokenRejectedUnderAccessSecret(t *testing.T) {
	cfg := dualTokenConfig()
	// 同密钥配置下类型校验仍必须拒绝，防止误配退化成单 token。
	same := cfg
	same.RefreshSecret = cfg.AccessSecret
	refresh, err := GenerateRefreshToken(1, "bob", same)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ParseToken(refresh, same); !errors.Is(err, ErrTokenInvalid) {
		t.Fatalf("refresh token accepted as access under shared secret: %v", err)
	}
}

func TestGenerateRefreshTokenRequiresSecret(t *testing.T) {
	if _, err := GenerateRefreshToken(1, "bob", JwtConfig{AccessSecret: "x"}); err == nil {
		t.Fatal("empty refresh secret must fail")
	}
}
