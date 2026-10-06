package middleware

import (
	"bufio"
	"errors"
	"net"
	"net/http"
	"time"

	"esx/pkg/logging"
)

// SafeAccessLogMiddleware records request metadata only. Headers, query values,
// request bodies and response bodies are deliberately excluded.
type SafeAccessLogMiddleware struct{}

// statusResponseWriter records the final status while preserving Flusher and Hijacker.
type statusResponseWriter struct {
	http.ResponseWriter
	status int
}

// Unwrap lets http.ResponseController reach the underlying writer.
func (w *statusResponseWriter) Unwrap() http.ResponseWriter {
	return w.ResponseWriter
}

// WriteHeader records only the first status, matching net/http semantics.
func (w *statusResponseWriter) WriteHeader(status int) {
	if w.status != 0 {
		return
	}
	w.status = status
	w.ResponseWriter.WriteHeader(status)
}

// Write records the implicit 200 when the handler never called WriteHeader.
func (w *statusResponseWriter) Write(body []byte) (int, error) {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	return w.ResponseWriter.Write(body)
}

// Flush also counts as an implicit 200, since streaming handlers may never write a status.
func (w *statusResponseWriter) Flush() {
	if w.status == 0 {
		w.status = http.StatusOK
	}
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

// Hijack passes through for handlers that take over the connection.
func (w *statusResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := w.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, errors.New("response writer does not support hijacking")
	}
	return hijacker.Hijack()
}

// NewSafeAccessLogMiddleware creates the access log middleware.
func NewSafeAccessLogMiddleware() *SafeAccessLogMiddleware {
	return &SafeAccessLogMiddleware{}
}

// Handle is the net/http variant used only by unit tests; live routes use the Hertz method in hertz.go.
func (m *SafeAccessLogMiddleware) Handle(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		started := time.Now()
		writer := &statusResponseWriter{ResponseWriter: w}
		next(writer, r)
		status := writer.status
		if status == 0 {
			status = http.StatusOK
		}
		logging.WithContext(r.Context()).Infow("gateway request completed",
			logging.Field("method", r.Method),
			logging.Field("path", r.URL.Path),
			logging.Field("status", status),
			logging.Field("duration_ms", time.Since(started).Milliseconds()))
	}
}
