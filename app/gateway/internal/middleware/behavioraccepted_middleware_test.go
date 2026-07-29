package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
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
			recorder := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodPost, "/api/v2/behavior/events", nil)
			handler := NewBehaviorAcceptedMiddleware().Handle(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tt.nextStatus)
			})

			handler(recorder, req)
			if recorder.Code != tt.wantStatus {
				t.Fatalf("status=%d, want %d", recorder.Code, tt.wantStatus)
			}
		})
	}
}

func TestBehaviorAcceptedMiddleware_InjectsRequestMetadata(t *testing.T) {
	recorder := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/v2/behavior/events", nil)
	req.RemoteAddr = "10.0.0.9:12345"
	req.Header.Set("X-Forwarded-For", "203.0.113.8, 10.0.0.1")
	req.Header.Set("User-Agent", "little-white-box/2")
	req.Header.Set("X-Client-Version", "2.1.0")
	req.Header.Set("X-Request-ID", "request-1")
	req.Header.Set("X-Trace-ID", "trace-1")

	handler := NewBehaviorAcceptedMiddleware().Handle(func(w http.ResponseWriter, r *http.Request) {
		metadata := BehaviorRequestMetadataFromContext(r.Context())
		if metadata.ClientIP != "203.0.113.8" || metadata.UserAgent != "little-white-box/2" || metadata.ClientVersion != "2.1.0" || metadata.TraceID != "trace-1" {
			t.Fatalf("unexpected request metadata: %+v", metadata)
		}
		_, _ = w.Write([]byte("ok"))
	})

	handler(recorder, req)
	if recorder.Code != http.StatusAccepted {
		t.Fatalf("status=%d, want %d", recorder.Code, http.StatusAccepted)
	}
	if recorder.Header().Get("X-Request-ID") != "request-1" {
		t.Fatalf("unexpected response request id: %q", recorder.Header().Get("X-Request-ID"))
	}
}
