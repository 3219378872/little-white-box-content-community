package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestSafeAccessLogPreservesStatusAndBody(t *testing.T) {
	handler := NewSafeAccessLogMiddleware().Handle(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusTeapot)
		_, _ = w.Write([]byte("response"))
	})
	recorder := httptest.NewRecorder()
	handler(recorder, httptest.NewRequest(http.MethodPost, "/private?token=hidden", nil))
	if recorder.Code != http.StatusTeapot || recorder.Body.String() != "response" {
		t.Fatalf("status=%d body=%q", recorder.Code, recorder.Body.String())
	}
}
