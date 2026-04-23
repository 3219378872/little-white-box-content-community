package jwtx

import (
	"testing"

	"github.com/golang-jwt/jwt/v4"
	"github.com/stretchr/testify/assert"
)

func TestParseToken_AlgNoneAttack(t *testing.T) {
	config := JwtConfig{AccessSecret: "test-secret", AccessExpire: 3600}

	// 构造 alg=none 的恶意 token (header: {"alg":"none","typ":"JWT"}, payload: {})
	maliciousToken := "eyJhbGciOiJub25lIiwidHlwIjoiSldUIn0.e30."

	_, err := ParseToken(maliciousToken, config)
	assert.ErrorIs(t, err, ErrTokenInvalid)
}

func TestParseToken_WrongAlgAttack(t *testing.T) {
	config := JwtConfig{AccessSecret: "test-secret", AccessExpire: 3600}

	// 构造 alg=RS256 的 token（非 HMAC，应被拒绝）
	// 使用 RS256 公钥/私钥生成 token，但用 HMAC secret 解析
	claims := Claims{UserId: 1, Username: "test"}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	// RS256 token 使用私钥签名；这里构造一个带 RS256 header 但无有效签名的 token
	wrongAlgToken, _ := token.SigningString()
	wrongAlgToken += ".dummy-sig"

	_, err := ParseToken(wrongAlgToken, config)
	assert.ErrorIs(t, err, ErrTokenInvalid)
}

func TestParseToken_ValidToken(t *testing.T) {
	config := JwtConfig{AccessSecret: "test-secret", AccessExpire: 3600}

	tokenStr, err := GenerateToken(42, "testuser", config)
	assert.NoError(t, err)

	claims, err := ParseToken(tokenStr, config)
	assert.NoError(t, err)
	assert.Equal(t, int64(42), claims.UserId)
	assert.Equal(t, "testuser", claims.Username)
}
