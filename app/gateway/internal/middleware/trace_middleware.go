package middleware

import (
	"context"
	"crypto/rand"
	"encoding/hex"
)

// TraceMiddleware 为每个请求生成/透传追踪标识并写入 ctx（REL-052），
// 同时注入客户端 IP 供下游频控使用。
type TraceMiddleware struct{}

// NewTraceMiddleware 创建追踪中间件。
func NewTraceMiddleware() *TraceMiddleware {
	return &TraceMiddleware{}
}

// clientIPKey 是客户端 IP 在 ctx 中的私有键。
type clientIPKey struct{}

// ClientIPFromContext 返回中间件提取的客户端 IP；未经过 TraceMiddleware
// 的上下文返回空串。来源优先级与行为链路一致：X-Forwarded-For 首跳 →
// X-Real-IP → RemoteAddr（生产由 nginx 覆写可信头）。
func ClientIPFromContext(ctx context.Context) string {
	ip, _ := ctx.Value(clientIPKey{}).(string)
	return ip
}

// newTraceID 生成随机追踪 ID；随机源失败时退回固定值，不阻断请求。
func newTraceID() string {
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return "gateway-trace"
	}
	return hex.EncodeToString(id[:])
}
