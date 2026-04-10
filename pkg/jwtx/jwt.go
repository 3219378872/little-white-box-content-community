package jwtx

import (
	"context"
	"encoding/json"
	"errors"
	"errx"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v4"
)

// JwtConfig JWT 配置
type JwtConfig struct {
	AccessSecret string
	AccessExpire int64
}

// Claims JWT 声明
type Claims struct {
	UserId   int64  `json:"userId"`
	Username string `json:"username"`
	jwt.RegisteredClaims
}

var (
	ErrTokenExpired     = errors.New("token已过期")
	ErrTokenInvalid     = errors.New("token无效")
	ErrTokenMalformed   = errors.New("token格式错误")
	ErrTokenNotValidYet = errors.New("token尚未生效")
)

// GenerateToken 生成 token
func GenerateToken(userId int64, username string, config JwtConfig) (string, error) {
	now := time.Now()
	claims := Claims{
		UserId:   userId,
		Username: username,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(now.Add(time.Duration(config.AccessExpire) * time.Second)),
			IssuedAt:  jwt.NewNumericDate(now),
			NotBefore: jwt.NewNumericDate(now),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(config.AccessSecret))
}

// ParseToken 解析 token
func ParseToken(tokenString string, config JwtConfig) (*Claims, error) {
	token, err := jwt.ParseWithClaims(tokenString, &Claims{}, func(token *jwt.Token) (interface{}, error) {
		return []byte(config.AccessSecret), nil
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
	value := ctx.Value("userId")
	userId, ok := value.(json.Number)
	if !ok {
		return 0, fmt.Errorf("转换userId失败%w", errx.NewWithCode(errx.SystemError))
	}
	return userId.Int64()
}
