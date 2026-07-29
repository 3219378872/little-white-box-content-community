package assistant

import (
	"context"
	"errors"
	"io"
	"testing"

	"esx/app/assistant/rpc/assistantservice"
	assistantpb "esx/app/assistant/rpc/xiaobaihe/assistant/pb"
	"gateway/internal/svc"
	"gateway/internal/types"
	"jwtx"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type fakeAssistantService struct {
	assistantservice.AssistantService
	chat func(context.Context, *assistantservice.ChatReq) (assistantpb.AssistantService_ChatClient, error)
}

func (f fakeAssistantService) Chat(ctx context.Context, request *assistantservice.ChatReq, _ ...grpc.CallOption) (assistantpb.AssistantService_ChatClient, error) {
	return f.chat(ctx, request)
}

type fakeAssistantChatClient struct {
	grpc.ClientStream
	ctx    context.Context
	events []*assistantpb.ChatEvent
	err    error
	index  int
	block  bool
}

func (f *fakeAssistantChatClient) Context() context.Context {
	return f.ctx
}

func (f *fakeAssistantChatClient) Recv() (*assistantpb.ChatEvent, error) {
	if f.block {
		<-f.ctx.Done()
		return nil, f.ctx.Err()
	}
	if f.index < len(f.events) {
		event := f.events[f.index]
		f.index++
		return event, nil
	}
	if f.err != nil {
		return nil, f.err
	}
	return nil, io.EOF
}

func collectEvents(t *testing.T, logic *AssistantChatLogic, request *types.AssistantChatReq) ([]*types.AssistantChatEvent, error) {
	t.Helper()
	client := make(chan *types.AssistantChatEvent, 16)
	err := logic.AssistantChat(request, client)
	close(client)
	var events []*types.AssistantChatEvent
	for event := range client {
		events = append(events, event)
	}
	return events, err
}

func TestAssistantChatBridgesIdentityContextAndEvents(t *testing.T) {
	t.Parallel()
	type contextKey string
	const marker contextKey = "marker"
	ctx := jwtx.WithUserIdContext(context.WithValue(context.Background(), marker, "preserved"), 42)
	service := fakeAssistantService{chat: func(callCtx context.Context, request *assistantservice.ChatReq) (assistantpb.AssistantService_ChatClient, error) {
		if callCtx.Value(marker) != "preserved" {
			t.Fatal("context was not propagated")
		}
		if request.UserId != 42 || request.Message != "hello" || request.ConversationId != "client-conversation" || request.RequestId != "request-1" {
			t.Fatalf("unexpected request: %+v", request)
		}
		return &fakeAssistantChatClient{ctx: callCtx, events: []*assistantpb.ChatEvent{
			{Type: assistantpb.ChatEventType_CHAT_EVENT_TYPE_TOKEN, Text: "answer", ConversationId: "server-conversation"},
			{Type: assistantpb.ChatEventType_CHAT_EVENT_TYPE_SOURCE, Source: &assistantpb.SourceReference{SourceType: "post", SourceId: "11", Title: "title"}, ConversationId: "server-conversation"},
			{Type: assistantpb.ChatEventType_CHAT_EVENT_TYPE_DONE, ConversationId: "server-conversation"},
		}}, nil
	}}
	events, err := collectEvents(t, NewAssistantChatLogic(ctx, &svc.ServiceContext{AssistantService: service}), &types.AssistantChatReq{
		ConversationId: "client-conversation",
		Message:        " hello ",
		RequestId:      "request-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 3 || events[0].Type != "token" || events[1].Type != "source" || events[2].Type != "done" {
		t.Fatalf("unexpected events: %+v", events)
	}
	if events[1].Source == nil || events[1].Source.SourceId != "11" {
		t.Fatalf("unexpected source: %+v", events[1])
	}
	for _, event := range events {
		if event.ConversationId != "server-conversation" {
			t.Fatalf("conversation id=%q", event.ConversationId)
		}
	}
}

func TestAssistantChatEmptyStreamIsStructuredError(t *testing.T) {
	t.Parallel()
	ctx := jwtx.WithUserIdContext(context.Background(), 1)
	service := fakeAssistantService{chat: func(callCtx context.Context, _ *assistantservice.ChatReq) (assistantpb.AssistantService_ChatClient, error) {
		return &fakeAssistantChatClient{ctx: callCtx}, nil
	}}
	events, err := collectEvents(t, NewAssistantChatLogic(ctx, &svc.ServiceContext{AssistantService: service}), &types.AssistantChatReq{Message: "hello"})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Type != "error" || !events[0].Degraded || events[0].ErrorCode != "EMPTY_STREAM" {
		t.Fatalf("unexpected events: %+v", events)
	}
}

func TestAssistantChatRPCFailureIsStructuredError(t *testing.T) {
	t.Parallel()
	ctx := jwtx.WithUserIdContext(context.Background(), 1)
	service := fakeAssistantService{chat: func(context.Context, *assistantservice.ChatReq) (assistantpb.AssistantService_ChatClient, error) {
		return nil, status.Error(codes.DeadlineExceeded, "internal timeout")
	}}
	events, err := collectEvents(t, NewAssistantChatLogic(ctx, &svc.ServiceContext{AssistantService: service}), &types.AssistantChatReq{Message: "hello"})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].ErrorCode != "ASSISTANT_TIMEOUT" || events[0].Text == "internal timeout" {
		t.Fatalf("unexpected events: %+v", events)
	}
}

func TestAssistantChatInvalidInputDoesNotCallRPC(t *testing.T) {
	t.Parallel()
	called := false
	service := fakeAssistantService{chat: func(context.Context, *assistantservice.ChatReq) (assistantpb.AssistantService_ChatClient, error) {
		called = true
		return nil, nil
	}}
	events, err := collectEvents(t, NewAssistantChatLogic(jwtx.WithUserIdContext(context.Background(), 1), &svc.ServiceContext{AssistantService: service}), &types.AssistantChatReq{})
	if err != nil {
		t.Fatal(err)
	}
	if called || len(events) != 1 || events[0].ErrorCode != "INVALID_REQUEST" {
		t.Fatalf("called=%v events=%+v", called, events)
	}
}

func TestAssistantChatCancellationPropagatesToRPCStream(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(jwtx.WithUserIdContext(context.Background(), 1))
	started := make(chan struct{})
	service := fakeAssistantService{chat: func(callCtx context.Context, _ *assistantservice.ChatReq) (assistantpb.AssistantService_ChatClient, error) {
		close(started)
		return &fakeAssistantChatClient{ctx: callCtx, block: true}, nil
	}}
	result := make(chan error, 1)
	go func() {
		_, err := collectEvents(t, NewAssistantChatLogic(ctx, &svc.ServiceContext{AssistantService: service}), &types.AssistantChatReq{Message: "hello"})
		result <- err
	}()
	<-started
	cancel()
	err := <-result
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error=%v want context canceled", err)
	}
}
