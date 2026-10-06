package password

import (
	"math/rand"
	"os"

	"golang.org/x/crypto/bcrypt"
)

var hashedDefaultPassword []byte

// init 预先计算默认密码的哈希，供 IsDefault 识别仍使用系统默认密码的账号。
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

// generateRandomString 生成开发环境的随机默认密码；只用于未配置 DEFAULT_PASSWORD 的场景，不承担安全随机性。
func generateRandomString(n int) string {
	const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	b := make([]byte, n)
	for i := range b {
		b[i] = letters[rand.Intn(len(letters))]
	}
	return string(b)
}

// Hash 用 bcrypt 生成密码哈希。
func Hash(plain string) (string, error) {
	hashed, err := bcrypt.GenerateFromPassword([]byte(plain), bcrypt.DefaultCost)
	if err != nil {
		return "", err
	}
	return string(hashed), nil
}

// Compare 校验明文密码与哈希是否匹配，不匹配时返回错误。
func Compare(hash string, plain string) error {
	return bcrypt.CompareHashAndPassword([]byte(hash), []byte(plain))
}

// IsDefault 判断明文是否为系统默认密码；手机注册未设密码的账号持有该密码，不允许用它做密码登录。
func IsDefault(plain string) bool {
	return bcrypt.CompareHashAndPassword(hashedDefaultPassword, []byte(plain)) == nil
}
