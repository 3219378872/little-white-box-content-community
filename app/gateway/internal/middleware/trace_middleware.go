package middleware

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strings"

	"interceptor"
)

// TraceMiddleware 为每个请求生成/透传追踪标识并写入 ctx（REL-052），
// 同时注入客户端 IP 供下游频控使用。
type TraceMiddleware struct{}

func NewTraceMiddleware() *TraceMiddleware {
	return &TraceMiddleware{}
}

type clientIPKey struct{}

func (m *TraceMiddleware) Handle(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		traceID := strings.TrimSpace(r.Header.Get("X-Trace-ID"))
		if traceID == "" {
			traceID = strings.TrimSpace(r.Header.Get("X-Request-ID"))
		}
		if traceID == "" {
			traceID = newTraceID()
		}
		ctx := interceptor.WithTraceID(r.Context(), traceID)
		ctx = context.WithValue(ctx, clientIPKey{}, clientIP(r))
		w.Header().Set("X-Trace-ID", traceID)
		next(w, r.WithContext(ctx))
	}
}

// ClientIPFromContext 返回中间件提取的客户端 IP；未经过 TraceMiddleware
// 的上下文返回空串。来源优先级与行为链路一致：X-Forwarded-For 首跳 →
// X-Real-IP → RemoteAddr（生产由 nginx 覆写可信头）。
func ClientIPFromContext(ctx context.Context) string {
	ip, _ := ctx.Value(clientIPKey{}).(string)
	return ip
}

func newTraceID() string {
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return "gateway-trace"
	}
	return hex.EncodeToString(id[:])
}
