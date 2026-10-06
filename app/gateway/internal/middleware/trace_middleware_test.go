package middleware

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/config"
	"github.com/cloudwego/hertz/pkg/common/ut"
	"github.com/cloudwego/hertz/pkg/route"

	"esx/pkg/rpcx"
)

// performHertz 在不经网络的 Hertz 引擎上执行一次请求：middleware 挂在 url 的路径上，handler 作为最终处理器。
func performHertz(middleware, handler app.HandlerFunc, method, url string, headers ...ut.Header) *ut.ResponseRecorder {
	engine := route.NewEngine(config.NewOptions(nil))
	path, _, _ := strings.Cut(url, "?")
	engine.Handle(method, path, middleware, handler)
	return ut.PerformRequest(engine, method, url, nil, headers...)
}

func TestTraceMiddlewareSetsContextAndHeader(t *testing.T) {
	var traceFromContext string
	rec := performHertz(NewTraceMiddleware().Hertz, func(ctx context.Context, c *app.RequestContext) {
		traceFromContext = rpcx.TraceID(ctx)
		c.Status(http.StatusOK)
	}, http.MethodGet, "/health")

	if traceFromContext == "" {
		t.Fatal("trace id was not set in context")
	}
	if got := string(rec.Result().Header.Peek("X-Trace-ID")); got != traceFromContext {
		t.Fatalf("response header X-Trace-ID = %q, want %q", got, traceFromContext)
	}
}

func TestTraceMiddlewarePreservesIncomingTrace(t *testing.T) {
	var traceFromContext string
	rec := performHertz(NewTraceMiddleware().Hertz, func(ctx context.Context, c *app.RequestContext) {
		traceFromContext = rpcx.TraceID(ctx)
	}, http.MethodGet, "/health", ut.Header{Key: "X-Trace-ID", Value: "incoming-trace"})

	if traceFromContext != "incoming-trace" {
		t.Fatalf("trace id = %q, want incoming-trace", traceFromContext)
	}
	if got := string(rec.Result().Header.Peek("X-Trace-ID")); got != "incoming-trace" {
		t.Fatalf("response header X-Trace-ID = %q, want incoming-trace", got)
	}
}

func TestTraceMiddlewareFallsBackToRequestIDAndRecordsClientIP(t *testing.T) {
	var traceFromContext, ipFromContext string
	performHertz(NewTraceMiddleware().Hertz, func(ctx context.Context, c *app.RequestContext) {
		traceFromContext = rpcx.TraceID(ctx)
		ipFromContext = ClientIPFromContext(ctx)
	}, http.MethodGet, "/health",
		ut.Header{Key: "X-Request-ID", Value: "request-1"},
		ut.Header{Key: "X-Forwarded-For", Value: "203.0.113.8, 10.0.0.1"},
		ut.Header{Key: "X-Real-IP", Value: "198.51.100.2"})

	if traceFromContext != "request-1" {
		t.Fatalf("trace id = %q, want request-1", traceFromContext)
	}
	if ipFromContext != "203.0.113.8" {
		t.Fatalf("client ip = %q, want first X-Forwarded-For hop", ipFromContext)
	}
}
