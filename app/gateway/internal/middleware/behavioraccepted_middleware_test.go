package middleware

import (
	"context"
	"net/http"
	"testing"

	"github.com/cloudwego/hertz/pkg/app"
	"github.com/cloudwego/hertz/pkg/common/ut"
)

func TestBehaviorAcceptedMiddleware_StatusMapping(t *testing.T) {
	tests := []struct {
		name       string
		nextStatus int
		wantStatus int
	}{
		{name: "successful behavior publish is accepted", nextStatus: http.StatusOK, wantStatus: http.StatusAccepted},
		{name: "bad request is unchanged", nextStatus: http.StatusBadRequest, wantStatus: http.StatusBadRequest},
		{name: "downstream failure is unchanged", nextStatus: http.StatusInternalServerError, wantStatus: http.StatusInternalServerError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := performHertz(NewBehaviorAcceptedMiddleware().Hertz, func(_ context.Context, c *app.RequestContext) {
				c.Status(tt.nextStatus)
			}, http.MethodPost, "/api/v2/behavior/events")

			if got := rec.Result().StatusCode(); got != tt.wantStatus {
				t.Fatalf("status=%d, want %d", got, tt.wantStatus)
			}
		})
	}
}

func TestBehaviorAcceptedMiddleware_InjectsRequestMetadata(t *testing.T) {
	var metadata BehaviorRequestMetadata
	rec := performHertz(NewBehaviorAcceptedMiddleware().Hertz, func(ctx context.Context, c *app.RequestContext) {
		metadata = BehaviorRequestMetadataFromContext(ctx)
		c.String(http.StatusOK, "ok")
	}, http.MethodPost, "/api/v2/behavior/events",
		ut.Header{Key: "X-Forwarded-For", Value: "203.0.113.8, 10.0.0.1"},
		ut.Header{Key: "User-Agent", Value: "little-white-box/2"},
		ut.Header{Key: "X-Client-Version", Value: "2.1.0"},
		ut.Header{Key: "X-Request-ID", Value: "request-1"},
		ut.Header{Key: "X-Trace-ID", Value: "trace-1"})

	if metadata.ClientIP != "203.0.113.8" || metadata.UserAgent != "little-white-box/2" || metadata.ClientVersion != "2.1.0" || metadata.TraceID != "trace-1" {
		t.Fatalf("unexpected request metadata: %+v", metadata)
	}
	resp := rec.Result()
	if resp.StatusCode() != http.StatusAccepted {
		t.Fatalf("status=%d, want %d", resp.StatusCode(), http.StatusAccepted)
	}
	if got := string(resp.Header.Peek("X-Request-ID")); got != "request-1" {
		t.Fatalf("unexpected response request id: %q", got)
	}
}

func TestBehaviorAcceptedMiddleware_TraceIDDefaultsToRequestID(t *testing.T) {
	var metadata BehaviorRequestMetadata
	performHertz(NewBehaviorAcceptedMiddleware().Hertz, func(ctx context.Context, _ *app.RequestContext) {
		metadata = BehaviorRequestMetadataFromContext(ctx)
	}, http.MethodPost, "/api/v2/behavior/events", ut.Header{Key: "X-Request-ID", Value: "request-2"})

	if metadata.TraceID != "request-2" {
		t.Fatalf("trace id = %q, want request-2", metadata.TraceID)
	}
}
