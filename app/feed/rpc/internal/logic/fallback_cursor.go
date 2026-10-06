package logic

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"time"

	"esx/app/feed/rpc/internal/model"
)

// 兜底游标前缀带版本号；TTL 限制状态在 Redis 中的存活时间。
const (
	fallbackCursorPrefix = "feedv3."
	fallbackCursorTTL    = 10 * time.Minute
)

// fallbackCursor 是签名游标的载荷：状态 ID、请求绑定与过期时间。
type fallbackCursor struct {
	Version   int                   `json:"v"`
	StateID   string                `json:"state"`
	Binding   model.FallbackBinding `json:"binding"`
	ExpiresAt int64                 `json:"e"`
}

// encodeFallbackCursor 生成 HMAC 签名的兜底游标；过期时间必须在 TTL 范围内。
func encodeFallbackCursor(secret, stateID string, binding model.FallbackBinding, expiresAt int64, now time.Time) (string, error) {
	if secret == "" || stateID == "" || binding.IdentityHash == "" || binding.RequestID == "" || binding.PageSize <= 0 || expiresAt <= now.Unix() || expiresAt > now.Add(fallbackCursorTTL).Unix() {
		return "", fmt.Errorf("invalid fallback cursor input")
	}
	payload, err := json.Marshal(fallbackCursor{
		Version: 3, StateID: stateID, Binding: binding, ExpiresAt: expiresAt,
	})
	if err != nil {
		return "", err
	}
	encoded := base64.RawURLEncoding.EncodeToString(payload)
	signature := signFallbackCursor(secret, encoded)
	return fallbackCursorPrefix + encoded + "." + signature, nil
}

// decodeFallbackCursor 校验并解析兜底游标。matched=false 表示这不是兜底游标（交给推荐服务处理）；
// 旧版本、签名错误、绑定不一致或已过期的兜底游标都返回错误。
func decodeFallbackCursor(secret, token string, binding model.FallbackBinding, now time.Time) (stateID string, expiresAt int64, matched bool, err error) {
	if !strings.HasPrefix(token, fallbackCursorPrefix) {
		if strings.HasPrefix(token, "feedv") {
			return "", 0, true, fmt.Errorf("unsupported fallback cursor")
		}
		return "", 0, false, nil
	}
	if secret == "" || len(token) > 4096 {
		return "", 0, true, fmt.Errorf("fallback cursor cannot be verified")
	}
	parts := strings.Split(strings.TrimPrefix(token, fallbackCursorPrefix), ".")
	if len(parts) != 2 || !hmac.Equal([]byte(parts[1]), []byte(signFallbackCursor(secret, parts[0]))) {
		return "", 0, true, fmt.Errorf("invalid fallback cursor signature")
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return "", 0, true, fmt.Errorf("decode fallback cursor: %w", err)
	}
	// 严格解码：拒绝未知字段与尾随数据。
	var cursor fallbackCursor
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&cursor); err != nil {
		return "", 0, true, fmt.Errorf("parse fallback cursor: %w", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return "", 0, true, fmt.Errorf("trailing fallback cursor data")
	}
	if cursor.Version != 3 || cursor.StateID == "" || cursor.Binding != binding {
		return "", 0, true, fmt.Errorf("invalid fallback cursor payload")
	}
	if cursor.ExpiresAt <= now.Unix() {
		return "", 0, true, fmt.Errorf("fallback cursor expired")
	}
	return cursor.StateID, cursor.ExpiresAt, true, nil
}

// signFallbackCursor 对编码后的载荷做 HMAC-SHA256 签名。
func signFallbackCursor(secret, payload string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = mac.Write([]byte(payload))
	return base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
}
