package middleware

import (
	"bufio"
	"errors"
	"net"
	"net/http"
	"time"

	"github.com/zeromicro/go-zero/core/logx"
)

// SafeAccessLogMiddleware records request metadata only. Headers, query values,
// request bodies and response bodies are deliberately excluded.
type SafeAccessLogMiddleware struct{}

type statusResponseWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusResponseWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

func (w *statusResponseWriter) WriteHeader(status int) {
	if w.status != 0 {
		return
	}
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

func (w *statusResponseWriter) Write(body []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.ResponseWriter.Write(body)
}

func (w *statusResponseWriter) Flush() {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (w *statusResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := w.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, errors.New("response writer does not support hijacking")
	}
	return hijacker.Hijack()
}

func NewSafeAccessLogMiddleware() *SafeAccessLogMiddleware {
	return &SafeAccessLogMiddleware{}
}

func (m *SafeAccessLogMiddleware) Handle(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		writer := &statusResponseWriter{ResponseWriter: w}
		next(writer, r)
		status := writer.status
		if status == 0 {
			status = http.StatusOK
		}
		logx.WithContext(r.Context()).Infow("gateway request completed",
			logx.Field("method", r.Method),
			logx.Field("path", r.URL.Path),
			logx.Field("status", status),
			logx.Field("duration_ms", time.Since(started).Milliseconds()))
	}
}
