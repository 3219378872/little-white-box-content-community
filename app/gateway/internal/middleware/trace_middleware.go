package middleware

import (
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strings"

	"interceptor"
)

// TraceMiddleware 为每个请求生成/透传追踪标识并写入 ctx（REL-052）。
type TraceMiddleware struct{}

func NewTraceMiddleware() *TraceMiddleware {
	return &TraceMiddleware{}
}

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
		w.Header().Set("X-Trace-ID", traceID)
		next(w, r.WithContext(ctx))
	}
}

func newTraceID() string {
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return "gateway-trace"
	}
	return hex.EncodeToString(id[:])
}
