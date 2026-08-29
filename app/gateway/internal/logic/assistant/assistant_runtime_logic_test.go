package assistant

import (
	"context"
	"io"
	"testing"

	"esx/app/assistant/rpc/assistantservice"
	assistantpb "esx/app/assistant/rpc/xiaobaihe/assistant/pb"
	"esx/app/gateway/internal/svc"
	"esx/app/gateway/internal/types"
	"esx/pkg/errx"
	"esx/pkg/jwtx"

	"google.golang.org/grpc"
)

type fakeRunStream struct {
	grpc.ClientStream
	ctx    context.Context
	events []*assistantpb.RunEvent
	index  int
}

func (s *fakeRunStream) Recv() (*assistantpb.RunEvent, error) {
	if s.index >= len(s.events) {
		return nil, io.EOF
	}
	ev := s.events[s.index]
	s.index++
	return ev, nil
}

type fakeAssistant struct {
	assistantservice.AssistantService
	posted *assistantservice.PostMessageReq
}

func (f *fakeAssistant) PostMessage(_ context.Context, in *assistantservice.PostMessageReq, _ ...grpc.CallOption) (*assistantpb.PostMessageResp, error) {
	f.posted = in
	return &assistantpb.PostMessageResp{MessageId: 3, SessionId: 2, RunId: 9, Disposition: "started"}, nil
}

func (f *fakeAssistant) SubscribeRunEvents(ctx context.Context, _ *assistantservice.SubscribeRunEventsReq, _ ...grpc.CallOption) (assistantpb.AssistantService_SubscribeRunEventsClient, error) {
	return &fakeRunStream{ctx: ctx, events: []*assistantpb.RunEvent{
		{RunId: 9, Seq: 1, Type: "token", Text: "hi", SessionId: 2},
		{RunId: 9, Seq: 2, Type: "done", SessionId: 2},
	}}, nil
}

func TestPostAssistantMessageRequiresAuthAndConsentPath(t *testing.T) {
	logic := NewPostAssistantMessageLogic(context.Background(), &svc.ServiceContext{AssistantService: &fakeAssistant{}})
	if _, err := logic.PostAssistantMessage(&types.PostAssistantMessageReq{Message: "hello"}); !errx.Is(err, errx.LoginRequired) {
		t.Fatalf("want login, got %v", err)
	}
	ctx := jwtx.WithUserIdContext(context.Background(), 7)
	fake := &fakeAssistant{}
	logic = NewPostAssistantMessageLogic(ctx, &svc.ServiceContext{AssistantService: fake})
	resp, err := logic.PostAssistantMessage(&types.PostAssistantMessageReq{Message: "hello", RequestId: "r1"})
	if err != nil {
		t.Fatal(err)
	}
	if resp.RunId != 9 || fake.posted == nil || fake.posted.UserId != 7 {
		t.Fatalf("resp=%+v posted=%+v", resp, fake.posted)
	}
}

func TestAssistantRunEventsStreams(t *testing.T) {
	ctx := jwtx.WithUserIdContext(context.Background(), 7)
	logic := NewAssistantRunEventsLogic(ctx, &svc.ServiceContext{AssistantService: &fakeAssistant{}})
	client := make(chan *types.AssistantRunEvent, 8)
	if err := logic.AssistantRunEvents(&types.AssistantRunEventsReq{Id: 9}, client); err != nil {
		t.Fatal(err)
	}
	if len(client) != 2 {
		t.Fatalf("events=%d", len(client))
	}
}
