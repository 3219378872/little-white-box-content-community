// Code scaffolded by goctl. Safe to edit.
// goctl 1.10.1

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

type behaviorMetadataKey struct{}

type BehaviorRequestMetadata struct {
	ClientIP      string
	UserAgent     string
	ClientVersion string
	TraceID       string
}

type BehaviorAcceptedMiddleware struct {
}

func NewBehaviorAcceptedMiddleware() *BehaviorAcceptedMiddleware {
	return &BehaviorAcceptedMiddleware{}
}

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

func BehaviorRequestMetadataFromContext(ctx context.Context) BehaviorRequestMetadata {
	metadata, _ := ctx.Value(behaviorMetadataKey{}).(BehaviorRequestMetadata)
	return metadata
}

type acceptedResponseWriter struct {
	http.ResponseWriter
	wroteHeader bool
}

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

func (w *acceptedResponseWriter) Write(body []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	return w.ResponseWriter.Write(body)
}

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

func newRequestID() string {
	var id [16]byte
	if _, err := rand.Read(id[:]); err != nil {
		return "gateway-request"
	}
	return hex.EncodeToString(id[:])
}
