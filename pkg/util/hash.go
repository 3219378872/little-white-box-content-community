package util

import (
	"crypto/sha256"
	"encoding/hex"

	"golang.org/x/crypto/bcrypt"
)

var hashedDefaultPassword []byte
var DefaultPassword = "NoPassWord"

func init() {
	password, err := bcrypt.GenerateFromPassword([]byte(DefaultPassword), bcrypt.DefaultCost)
	if err != nil {
		panic("默认密码初始化错误")
	}
	hashedDefaultPassword = password
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
