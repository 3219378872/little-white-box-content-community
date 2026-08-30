package assistant

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"esx/app/gateway/internal/svc"

	"github.com/zeromicro/go-zero/rest/pathvar"
)

func TestAssistantHandlersRejectMalformedInputBeforeCallingServices(t *testing.T) {
	tests := []struct {
		name        string
		handler     func(*svc.ServiceContext) http.HandlerFunc
		method      string
		target      string
		body        string
		pathVars    map[string]string
		contentType string
	}{
		{name: "add memory json", handler: AddAssistantMemoryHandler, method: http.MethodPost, target: "/api/v2/assistant/memory", body: "{", contentType: "application/json"},
		{name: "batch memory json", handler: BatchAssistantMemoryHandler, method: http.MethodPost, target: "/api/v2/assistant/memory/batch", body: "{", contentType: "application/json"},
		{name: "cancel run id", handler: CancelAssistantRunHandler, method: http.MethodPost, target: "/api/v2/assistant/runs/bad/cancel", pathVars: map[string]string{"id": "bad"}},
		{name: "undo change id", handler: UndoAssistantMemoryChangeHandler, method: http.MethodPost, target: "/api/v2/assistant/memory/changes/bad/undo", pathVars: map[string]string{"id": "bad"}},
		{name: "message cursor", handler: ListAssistantMessagesHandler, method: http.MethodGet, target: "/api/v2/assistant/messages?beforeId=bad"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			req := httptest.NewRequest(test.method, test.target, strings.NewReader(test.body))
			if test.contentType != "" {
				req.Header.Set("Content-Type", test.contentType)
			}
			if test.pathVars != nil {
				req = pathvar.WithVars(req, test.pathVars)
			}
			recorder := httptest.NewRecorder()
			test.handler(nil)(recorder, req)
			if recorder.Code != http.StatusBadRequest {
				t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
			}
		})
	}
}

func TestAssistantListHandlersRejectAnonymousRequests(t *testing.T) {
	tests := []struct {
		name    string
		handler func(*svc.ServiceContext) http.HandlerFunc
		method  string
		target  string
	}{
		{name: "memory", handler: ListAssistantMemoryHandler, method: http.MethodGet, target: "/api/v2/assistant/memory"},
		{name: "watch", handler: ListAssistantWatchHandler, method: http.MethodGet, target: "/api/v2/assistant/watch"},
		{name: "consent", handler: GetAgentConsentHandler, method: http.MethodGet, target: "/api/v2/assistant/consent"},
		{name: "thread", handler: GetAssistantThreadHandler, method: http.MethodGet, target: "/api/v2/assistant/thread"},

	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			test.handler(nil)(recorder, httptest.NewRequest(test.method, test.target, nil))
			if recorder.Code != http.StatusUnauthorized {
				t.Fatalf("status=%d body=%s", recorder.Code, recorder.Body.String())
			}
		})
	}
}
