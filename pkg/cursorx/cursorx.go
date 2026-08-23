// Package cursorx 提供列表游标（不透明 token）的编解码。
//
// 游标本质是 base64url(JSON) 的整数字段集合：客户端只需原样回传，
// 服务端负责构造与校验；解析失败一律按参数错误处理，不暴露内部结构。
// 与 feed 的 fallback cursor（绑定 requestId 与时效，需 HMAC）不同，
// 排序游标的字段不含越权面，无需签名。
package cursorx

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
)

const maxEncodedLen = 512 // 防御超长输入

// ErrInvalidCursor 游标格式非法。
var ErrInvalidCursor = fmt.Errorf("invalid cursor")

// Data 游标负载：仅支持整数字段（排序键值、id 等）。
type Data map[string]int64

// Encode 编码为 URL 安全的不透明游标。
func Encode(d Data) (string, error) {
	if len(d) == 0 {
		return "", fmt.Errorf("%w: empty payload", ErrInvalidCursor)
	}
	payload, err := json.Marshal(d)
	if err != nil {
		return "", fmt.Errorf("cursor marshal: %w", err)
	}
	return base64.RawURLEncoding.EncodeToString(payload), nil
}

// Decode 解析并校验游标。任何失败返回包裹 ErrInvalidCursor 的错误。
func Decode(token string) (Data, error) {
	if token == "" {
		return nil, fmt.Errorf("%w: empty", ErrInvalidCursor)
	}
	if len(token) > maxEncodedLen {
		return nil, fmt.Errorf("%w: too long", ErrInvalidCursor)
	}
	payload, err := base64.RawURLEncoding.DecodeString(token)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidCursor, err)
	}
	var fields Data
	if err := json.Unmarshal(payload, &fields); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidCursor, err)
	}
	if len(fields) == 0 {
		return nil, fmt.Errorf("%w: empty payload", ErrInvalidCursor)
	}
	return fields, nil
}
