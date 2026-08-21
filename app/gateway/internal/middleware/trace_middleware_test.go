package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"esx/pkg/interceptor"
)

func TestTraceMiddlewareSetsContextAndHeader(t *testing.T) {
	m := NewTraceMiddleware()
	var traceFromContext string
	handler := m.Handle(func(w http.ResponseWriter, r *http.Request) {
		traceFromContext = interceptor.GetTraceID(r.Context())
		w.WriteHeader(http.StatusOK)
	})
	req := httptest.NewRequest(http.MethodGet, "http://example/health", nil)
	rec := httptest.NewRecorder()
	handler(rec, req)
	if traceFromContext == "" {
		t.Fatal("trace id was not set in context")
	}
	if got := rec.Header().Get("X-Trace-ID"); got != traceFromContext {
		t.Fatalf("response header X-Trace-ID = %q, want %q", got, traceFromContext)
	}
}

func TestTraceMiddlewarePreservesIncomingTrace(t *testing.T) {
	m := NewTraceMiddleware()
	var traceFromContext string
	handler := m.Handle(func(w http.ResponseWriter, r *http.Request) {
		traceFromContext = interceptor.GetTraceID(r.Context())
		w.WriteHeader(http.StatusOK)
	})
	req := httptest.NewRequest(http.MethodGet, "http://example/health", nil)
	req.Header.Set("X-Trace-ID", "incoming-trace")
	rec := httptest.NewRecorder()
	handler(rec, req)
	if traceFromContext != "incoming-trace" {
		t.Fatalf("trace id = %q, want incoming-trace", traceFromContext)
	}
}
