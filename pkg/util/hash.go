package util

import (
	"crypto/sha256"
	"encoding/hex"
	"math/rand"
	"os"

	"golang.org/x/crypto/bcrypt"
)

var hashedDefaultPassword []byte

func init() {
	defaultPass := os.Getenv("DEFAULT_PASSWORD")
	if defaultPass == "" {
		// 生产环境必须在启动前设置 DEFAULT_PASSWORD
		// 开发环境若未设置则使用随机值，避免固定默认值
		defaultPass = "DEV_ONLY_" + generateRandomString(16)
	}
	password, err := bcrypt.GenerateFromPassword([]byte(defaultPass), bcrypt.DefaultCost)
	if err != nil {
		panic("默认密码初始化错误: " + err.Error())
	}
	hashedDefaultPassword = password
}

// generateRandomString 生成指定长度的随机字符串（仅用于开发兜底）
func generateRandomString(n int) string {
	// 简单实现，生产环境不依赖此路径
	const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, n)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}

// HashPassword 使用bcrypt算法加密
func HashPassword(password string) (string, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hash), nil
}

// ComparePassword 使用bcrypt算法解密，nil返回则成功
func ComparePassword(hash string, password string) error {
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	if err != nil {
		return err
	}
	return nil
}

func IsDefaultPassword(password string) bool {
	err := bcrypt.CompareHashAndPassword(hashedDefaultPassword, []byte(password))
	if err != nil {
		return false
	}
	return true
}

// SHA256 SHA256 哈希
func SHA256(s string) string {
	h := sha256.New()
	h.Write([]byte(s))
	return hex.EncodeToString(h.Sum(nil))
}
