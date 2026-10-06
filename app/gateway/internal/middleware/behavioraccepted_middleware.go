package middleware

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net"
	"net/http"
	"strings"
)

const (
	requestIDHeader     = "X-Request-ID"
	traceIDHeader       = "X-Trace-ID"
	clientVersionHeader = "X-Client-Version"
)

// behaviorMetadataKey 是行为请求元数据在 ctx 中的私有键。
type behaviorMetadataKey struct{}

// BehaviorRequestMetadata 是网关为行为事件补充的请求侧信息，下游用于去重与风控。
type BehaviorRequestMetadata struct {
	ClientIP      string
	UserAgent     string
	ClientVersion string
	TraceID       string
}

// BehaviorAcceptedMiddleware 为行为上报补齐请求元数据，并把成功响应改写为 202 Accepted。
type BehaviorAcceptedMiddleware struct {
}

// NewBehaviorAcceptedMiddleware 创建行为上报中间件。
func NewBehaviorAcceptedMiddleware() *BehaviorAcceptedMiddleware {
	return &BehaviorAcceptedMiddleware{}
}

// Handle 是 net/http 版本，目前只被单测使用；线上路由走 hertz.go 中语义相同的 Hertz 方法。
func (m *BehaviorAcceptedMiddleware) Handle(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		requestID := strings.TrimSpace(r.Header.Get(requestIDHeader))
		if requestID == "" {
			requestID = newRequestID()
		}
		traceID := strings.TrimSpace(r.Header.Get(traceIDHeader))
		if traceID == "" {
			traceID = requestID
		}

		metadata := BehaviorRequestMetadata{
			ClientIP:      clientIP(r),
			UserAgent:     r.UserAgent(),
			ClientVersion: strings.TrimSpace(r.Header.Get(clientVersionHeader)),
			TraceID:       traceID,
		}
		ctx := context.WithValue(r.Context(), behaviorMetadataKey{}, metadata)
		w.Header().Set(requestIDHeader, requestID)
		next(&acceptedResponseWriter{ResponseWriter: w}, r.WithContext(ctx))
	}
}

// BehaviorRequestMetadataFromContext 读取中间件写入的元数据；未经过中间件时返回零值。
func BehaviorRequestMetadataFromContext(ctx context.Context) BehaviorRequestMetadata {
	metadata, _ := ctx.Value(behaviorMetadataKey{}).(BehaviorRequestMetadata)
	return metadata
}

// acceptedResponseWriter 把首个 200 状态改写为 202，表示事件已受理而非已处理。
type acceptedResponseWriter struct {
	http.ResponseWriter
	wroteHeader bool
}

// WriteHeader 只生效一次，与 net/http 语义一致。
func (w *acceptedResponseWriter) WriteHeader(statusCode int) {
	if w.wroteHeader {
		return
	}
	w.wroteHeader = true
	if statusCode == http.StatusOK {
		statusCode = http.StatusAccepted
	}
	w.ResponseWriter.WriteHeader(statusCode)
}

// Write 在未显式写状态时先走 WriteHeader，确保隐式 200 也被改写为 202。
func (w *acceptedResponseWriter) Write(body []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(body)
}

// clientIP 依次取 X-Forwarded-For 首跳、X-Real-IP 与连接对端地址。
func clientIP(r *http.Request) string {
	if forwarded := r.Header.Get("X-Forwarded-For"); forwarded != "" {
		if ip := strings.TrimSpace(strings.Split(forwarded, ",")[0]); ip != "" {
			return ip
		}
	}
	if realIP := strings.TrimSpace(r.Header.Get("X-Real-IP")); realIP != "" {
		return realIP
	}
	host, _, err := net.SplitHostPort(r.RemoteAddr)
	if err == nil {
		return host
	}
	return strings.TrimSpace(r.RemoteAddr)
}

// newRequestID 生成随机请求 ID；随机源失败时退回固定值，不阻断请求。
func newRequestID() string {
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return "gateway-request"
	}
	return hex.EncodeToString(id[:])
}
