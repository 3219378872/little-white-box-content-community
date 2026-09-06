package password

import (
	"math/rand"
	"os"

	"golang.org/x/crypto/bcrypt"
)

var hashedDefaultPassword []byte

func init() {
	defaultPass := os.Getenv("DEFAULT_PASSWORD")
	if defaultPass == "" {
		// 生产环境必须在启动前设置 DEFAULT_PASSWORD。
		// 开发环境若未设置则使用随机值，避免固定默认值。
		defaultPass = "DEV_ONLY_" + generateRandomString(16)
	}
	hashed, err := bcrypt.GenerateFromPassword([]byte(defaultPass), bcrypt.DefaultCost)
	if err != nil {
		panic("默认密码初始化错误: " + err.Error())
	}
	hashedDefaultPassword = hashed
}

func generateRandomString(n int) string {
	const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, n)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}

func Hash(plain string) (string, error) {
	hashed, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hashed), nil
}

func Compare(hash string, plain string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain))
}

func IsDefault(plain string) bool {
	return bcrypt.CompareHashAndPassword(hashedDefaultPassword, []byte(plain)) == nil
}
