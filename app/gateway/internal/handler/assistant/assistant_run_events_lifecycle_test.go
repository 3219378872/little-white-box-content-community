package assistant

import (
	"context"
	"esx/pkg/httpx"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/cloudwego/hertz/pkg/common/test/mock"
	"github.com/cloudwego/kitex/client/callopt"
	"github.com/cloudwego/kitex/pkg/streaming"

	"esx/app/assistant/rpc/assistantservice"
	"esx/app/gateway/internal/httpxconfig"
	"esx/app/gateway/internal/svc"
	assistantpb "esx/kitex_gen/assistant"
	"esx/pkg/errx"
	"esx/pkg/jwtx"

	"esx/pkg/httptestx"

	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

func assembledAssistantSSEHandler(t *testing.T, service assistantservice.AssistantService) http.HandlerFunc {
	t.Helper()
	return httptestx.Adapt(httptestx.Chain(httpx.RoutePolicy(0, 10<<20, true), AssistantRunEventsHandler(&svc.ServiceContext{AssistantService: service})))
}

func assistantEventRequest() *http.Request {
	req := httptest.NewRequest(http.MethodGet, "/api/v2/assistant/runs/9/events", nil)
	req = httptestx.WithVars(req, map[string]string{"id": "9"})
	return req.WithContext(jwtx.WithUserIdContext(req.Context(), 7))
}

func TestAssistantRunEventsWithSSEPreservesHTTPFailures(t *testing.T) {
	httpxconfig.ConfigureErrors()
	for _, tc := range []struct {
		name    string
		failure error
		code    int
	}{
		{"missing", status.Convert(errx.NewWithCode(errx.NotFound)).Err(), http.StatusNotFound},
		{"forbidden", status.Convert(errx.NewWithCode(errx.PermissionDenied)).Err(), http.StatusForbidden},
		{"unavailable", status.Error(codes.Unavailable, "private upstream detail"), http.StatusServiceUnavailable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			handler := assembledAssistantSSEHandler(t, &failedRunService{failure: tc.failure})
			rec := httptest.NewRecorder()
			handler(rec, assistantEventRequest())
			require.Equal(t, tc.code, rec.Code)
			require.Contains(t, rec.Header().Get("Content-Type"), "application/json")
			require.NotContains(t, rec.Body.String(), "private upstream")
			require.NotContains(t, rec.Body.String(), "event:")
		})
	}
}

func TestAssistantRunEventsWithSSEKeepsTransportErrorOutsideCursor(t *testing.T) {
	handler := assembledAssistantSSEHandler(t, &failedRunService{events: []*assistantpb.RunEvent{{RunId: 9, Seq: 8, Type: "token", Text: "partial"}}, failure: status.Error(codes.Unavailable, "private upstream detail")})
	rec := httptest.NewRecorder()
	handler(rec, assistantEventRequest())
	require.Equal(t, http.StatusOK, rec.Code)
	require.Equal(t, "text/event-stream", rec.Header().Get("Content-Type"))
	require.Equal(t, 1, strings.Count(rec.Body.String(), "id: "))
	require.Contains(t, rec.Body.String(), "id: 8\n")
	require.Contains(t, rec.Body.String(), "event: transport_error\n")
	require.Contains(t, rec.Body.String(), `"retryable":true`)
	require.NotContains(t, rec.Body.String(), `"type":"error"`)
}

type lifecycleRunService struct {
	assistantservice.AssistantService
	open func(context.Context) assistantservice.AssistantService_SubscribeRunEventsClient
}

func (s *lifecycleRunService) SubscribeRunEvents(ctx context.Context, _ *assistantservice.SubscribeRunEventsReq, _ ...callopt.Option) (assistantservice.AssistantService_SubscribeRunEventsClient, error) {
	return s.open(ctx), nil
}

type lifecycleRunStream struct {
	streaming.Stream
	recv func() (*assistantpb.RunEvent, error)
}

func (s *lifecycleRunStream) Recv() (*assistantpb.RunEvent, error) { return s.recv() }

type failedEventConn struct{ *mock.Conn }

func (c failedEventConn) Flush() error { return io.ErrClosedPipe }

func TestAssistantRunEventsWriteFailureCancelsUpstream(t *testing.T) {
	stopped := make(chan struct{})
	service := &lifecycleRunService{open: func(ctx context.Context) assistantservice.AssistantService_SubscribeRunEventsClient {
		first := true
		return &lifecycleRunStream{recv: func() (*assistantpb.RunEvent, error) {
			if first {
				first = false
				return &assistantpb.RunEvent{RunId: 9, Seq: 1, Type: "token", Text: "hello"}, nil
			}
			<-ctx.Done()
			close(stopped)
			return nil, ctx.Err()
		}}
	}}
	req := assistantEventRequest()
	c := httptestx.Context(req)
	c.SetConn(failedEventConn{mock.NewConn("")})
	AssistantRunEventsHandler(&svc.ServiceContext{AssistantService: service})(req.Context(), c)
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("upstream goroutine did not observe handler cancellation")
	}
}

func TestAssistantRunEventsPanicCompletesWithoutHanging(t *testing.T) {
	httpxconfig.ConfigureErrors()
	for _, streamed := range []bool{false, true} {
		t.Run(map[bool]string{false: "before stream", true: "after token"}[streamed], func(t *testing.T) {
			service := &lifecycleRunService{open: func(context.Context) assistantservice.AssistantService_SubscribeRunEventsClient {
				sent := false
				return &lifecycleRunStream{recv: func() (*assistantpb.RunEvent, error) {
					if streamed && !sent {
						sent = true
						return &assistantpb.RunEvent{RunId: 9, Seq: 1, Type: "token", Text: "partial"}, nil
					}
					panic("test upstream panic")
				}}
			}}
			handler := assembledAssistantSSEHandler(t, service)
			rec := httptest.NewRecorder()
			done := make(chan struct{})
			go func() { defer close(done); handler(rec, assistantEventRequest()) }()
			select {
			case <-done:
			case <-time.After(time.Second):
				t.Fatal("panic left completion channel hanging")
			}
			if streamed {
				require.Equal(t, http.StatusOK, rec.Code)
				require.Contains(t, rec.Body.String(), "event: transport_error\n")
				require.Equal(t, 1, strings.Count(rec.Body.String(), "id: "))
			} else {
				require.Equal(t, http.StatusInternalServerError, rec.Code)
				require.Contains(t, rec.Header().Get("Content-Type"), "application/json")
			}
			require.NotContains(t, rec.Body.String(), "test upstream panic")
		})
	}
}
