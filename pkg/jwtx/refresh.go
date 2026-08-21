package jwtx

import (
	"crypto/rand"
	"encoding/hex"
)

// newJTI 生成 128-bit 随机令牌 ID，用于 refresh token 的服务端白名单轮换。
func newJTI() (string, error) {
	var buf [16]byte
	if _, err := rand.Read(buf[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf[:]), nil
}
