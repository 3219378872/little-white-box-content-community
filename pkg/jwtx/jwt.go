package jwtx

import (
	"context"
	"errors"
	"esx/pkg/errx"
	"fmt"
	"strings"
	"time"

	"github.com/golang-jwt/jwt/v4"
)

// JwtConfig JWT 配置
type JwtConfig struct {
	AccessSecret string
	AccessExpire int64
	// RefreshSecret/RefreshExpire 刷新令牌的独立签名密钥与有效期；
	// 与访问令牌分离密钥可防止 refresh token 被当作 access token 使用。
	RefreshSecret string
	RefreshExpire int64
}

// 令牌类型声明。历史 access token 无该字段，解析时按空串视为 access。
const (
	TokenTypeAccess  = "access"
	TokenTypeRefresh = "refresh"
)

// Claims JWT 声明
type Claims struct {
	UserId    int64  `json:"userId"`
	Username  string `json:"username"`
	TokenType string `json:"tokenType,omitempty"`
	jwt.RegisteredClaims
}

var (
	ErrTokenExpired     = errors.New("token已过期")
	ErrTokenInvalid     = errors.New("token无效")
	ErrTokenMalformed   = errors.New("token格式错误")
	ErrTokenNotValidYet = errors.New("token尚未生效")
)

// GenerateToken 生成访问令牌
func GenerateToken(userId int64, username string, config JwtConfig) (string, error) {
	now := time.Now()
	claims := Claims{
		UserId:    userId,
		Username:  username,
		TokenType: TokenTypeAccess,
		ExpiresAt: jwt.NewNumericDate(now.Add(time.Duration(config.AccessExpire) * time.Second)),
		IssuedAt:  jwt.NewNumericDate(now),
		NotBefore: jwt.NewNumericDate(now),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(config.AccessSecret))
}

// GenerateRefreshToken 生成刷新令牌：独立密钥签名、独立有效期，并携带
// jti（RegisteredClaims.ID）供服务端白名单轮换。
func GenerateRefreshToken(userId int64, username string, config JwtConfig) (string, error) {
	if strings.TrimSpace(config.RefreshSecret) == "" {
		return "", errx.New(errx.SystemError, "refresh secret is empty")
	}
	jti, err := newJTI()
	if err != nil {
		return "", err
	}
	now := time.Now()
	expire := config.RefreshExpire
	if expire <= 0 {
		expire = 7 * 24 * 3600
	}
	claims := Claims{
		UserId:    userId,
		Username:  username,
		TokenType: TokenTypeRefresh,
		ID:        jti,
		ExpiresAt: jwt.NewNumericDate(now.Add(time.Duration(expire) * time.Second)),
		IssuedAt:  jwt.NewNumericDate(now),
		NotBefore: jwt.NewNumericDate(now),
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(config.RefreshSecret))
}

// ParseToken 解析访问令牌；显式拒绝 refresh token，
// 防止长效令牌绕过短有效期被当作访问凭证使用。
func ParseToken(tokenString string, config JwtConfig) (*Claims, error) {
	claims, err := parseSigned(tokenString, config.AccessSecret)
	if err != nil {
		return nil, err
	}
	if claims.TokenType == TokenTypeRefresh {
		return nil, ErrTokenInvalid
	}
	return claims, nil
}

// ParseRefreshToken 解析刷新令牌；仅接受 refresh 类型，
// 访问令牌不能用于换取新令牌对。
func ParseRefreshToken(tokenString string, config JwtConfig) (*Claims, error) {
	claims, err := parseSigned(tokenString, config.RefreshSecret)
	if err != nil {
		return nil, err
	}
	if claims.TokenType != TokenTypeRefresh {
		return nil, ErrTokenInvalid
	}
	return claims, nil
}

func parseSigned(tokenString, secret string) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (any, error) {
		// 强制校验签名算法，防止算法混淆攻击（alg:none 或 RS256→HS256 等）
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		return []byte(secret), nil
	})

	if err != nil {
		if errors.Is(err, jwt.ErrTokenExpired) {
			return nil, ErrTokenExpired
		}
		if errors.Is(err, jwt.ErrTokenMalformed) {
			return nil, ErrTokenMalformed
		}
		if errors.Is(err, jwt.ErrTokenNotValidYet) {
			return nil, ErrTokenNotValidYet
		}
		return nil, ErrTokenInvalid
	}

	if claims, ok := token.Claims.(*Claims); ok && token.Valid {
		return claims, nil
	}

	return nil, ErrTokenInvalid
}

// GetUserIdFromContext 从上下文中获取userId
func GetUserIdFromContext(ctx context.Context) (int64, error) {
	userId, ok := GetOptionalUserIdFromContext(ctx)
	if !ok {
		return 0, errx.NewWithCode(errx.LoginRequired)
	}
	return userId, nil
}
