package assistant

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"esx/app/assistant/rpc/assistantservice"
	assistantpb "esx/app/assistant/rpc/xiaobaihe/assistant/pb"
	"esx/app/gateway/internal/svc"
	"esx/pkg/jwtx"

	"github.com/zeromicro/go-zero/rest/pathvar"
	"google.golang.org/grpc"
)

type handlerRunStream struct {
	grpc.ClientStream
	events []*assistantpb.RunEvent
}

func (s *handlerRunStream) Recv() (*assistantpb.RunEvent, error) {
	if len(s.events) == 0 {
		return nil, io.EOF
	}
	event := s.events[0]
	s.events = s.events[1:]
	return event, nil
}

type handlerAssistantService struct {
	assistantservice.AssistantService
	received *assistantservice.SubscribeRunEventsReq
}

func (s *handlerAssistantService) SubscribeRunEvents(
	_ context.Context,
	in *assistantservice.SubscribeRunEventsReq,
	_ ...grpc.CallOption,
) (assistantpb.AssistantService_SubscribeRunEventsClient, error) {
	s.received = in
	return &handlerRunStream{events: []*assistantpb.RunEvent{
		{RunId: in.RunId, Seq: 6, Type: "token", Text: "hi", SessionId: 3},
		{RunId: in.RunId, Seq: 7, Type: "done", SessionId: 3},
	}}, nil
}

func TestResumeAfterSeqUsesNewestCursor(t *testing.T) {
	tests := []struct {
		query  int64
		header string
		want   int64
	}{
		{query: 8, header: "12", want: 12},
		{query: 12, header: "8", want: 12},
		{query: 7, header: "bad", want: 7},
		{query: 0, header: " 9 ", want: 9},
	}
	for _, test := range tests {
		if got := resumeAfterSeq(test.query, test.header); got != test.want {
			t.Fatalf("query=%d header=%q got=%d want=%d", test.query, test.header, got, test.want)
		}
	}
}

func TestAssistantSSEHeartbeatIsACommentWithinThirtySeconds(t *testing.T) {
	if assistantSSEHeartbeatInterval > 30*time.Second {
		t.Fatalf("heartbeat interval=%s", assistantSSEHeartbeatInterval)
	}
	recorder := httptest.NewRecorder()
	if err := writeAssistantSSEHeartbeat(recorder); err != nil {
		t.Fatal(err)
	}
	if got := recorder.Body.String(); got != ": heartbeat\n\n" {
		t.Fatalf("heartbeat=%q", got)
	}
}

func TestAssistantRunEventsHandlerStreamsIDsAndUsesNewestCursor(t *testing.T) {
	service := &handlerAssistantService{}
	req := httptest.NewRequest(http.MethodGet, "/api/v2/assistant/runs/9/events?afterSeq=3", nil)
	req.Header.Set("Last-Event-ID", "5")
	req = pathvar.WithVars(req, map[string]string{"id": "9"})
	req = req.WithContext(jwtx.WithUserIdContext(req.Context(), 7))
	recorder := httptest.NewRecorder()

	AssistantRunEventsHandler(&svc.ServiceContext{AssistantService: service})(recorder, req)

	if service.received == nil || service.received.UserId != 7 || service.received.RunId != 9 || service.received.AfterSeq != 5 {
		t.Fatalf("received=%+v", service.received)
	}
	if got := recorder.Header().Get("Content-Type"); got != "text/event-stream" {
		t.Fatalf("content-type=%q", got)
	}
	body := recorder.Body.String()
	if !strings.Contains(body, "id: 6\ndata:") || !strings.Contains(body, "id: 7\ndata:") || !strings.Contains(body, `"type":"done"`) {
		t.Fatalf("body=%q", body)
	}
}
