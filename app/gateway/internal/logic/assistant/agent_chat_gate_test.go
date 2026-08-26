package assistant

import (
	"context"
	"testing"

	"esx/app/assistant/rpc/assistantservice"
	assistantpb "esx/app/assistant/rpc/xiaobaihe/assistant/pb"
	"esx/app/gateway/internal/svc"
	"esx/app/gateway/internal/types"
	"esx/app/user/rpc/userservice"
	"esx/pkg/jwtx"

	"google.golang.org/grpc"
)

type fakeConsentUserService struct {
	userservice.UserService
	granted bool
	err     error
	called  int
}

func (f *fakeConsentUserService) GetAgentCapabilityConsent(_ context.Context, _ *userservice.GetAgentCapabilityConsentReq, _ ...grpc.CallOption) (*userservice.GetAgentCapabilityConsentResp, error) {
	f.called++
	return &userservice.GetAgentCapabilityConsentResp{Granted: f.granted}, f.err
}

func agentTestContext(t *testing.T) context.Context {
	t.Helper()
	return jwtx.WithUserIdContext(context.Background(), 7)
}

func TestAgentChatWithoutConsentIsRejectedBeforeStream(t *testing.T) {
	var chatCalls int
	service := fakeAssistantService{chat: func(_ context.Context, _ *assistantservice.ChatReq) (assistantpb.AssistantService_ChatClient, error) {
		chatCalls++
		return &fakeAssistantChatClient{}, nil
	}}
	user := &fakeConsentUserService{granted: false}
	svcCtx := &svc.ServiceContext{AssistantService: service, UserService: user}
	events, err := collectEvents(t, NewAssistantChatLogic(agentTestContext(t), svcCtx),
		&types.AssistantChatReq{Message: "hello", Mode: "agent", RequestId: "r-1"})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Type != "error" || events[0].ErrorCode != "AGENT_NOT_AUTHORIZED" {
		t.Fatalf("expected single AGENT_NOT_AUTHORIZED error event, got %+v", events)
	}
	if chatCalls != 0 {
		t.Fatalf("chat must not start without consent, got %d calls", chatCalls)
	}
	if user.called != 1 {
		t.Fatalf("consent must be checked exactly once, got %d", user.called)
	}
}

func TestAgentChatWithConsentForwardsModeAndAttachments(t *testing.T) {
	var captured *assistantservice.ChatReq
	service := fakeAssistantService{chat: func(_ context.Context, request *assistantservice.ChatReq) (assistantpb.AssistantService_ChatClient, error) {
		captured = request
		return &fakeAssistantChatClient{ctx: context.Background(), events: []*assistantpb.ChatEvent{
			{Type: assistantpb.ChatEventType_CHAT_EVENT_TYPE_DONE},
		}}, nil
	}}
	user := &fakeConsentUserService{granted: true}
	svcCtx := &svc.ServiceContext{AssistantService: service, UserService: user}
	events, err := collectEvents(t, NewAssistantChatLogic(agentTestContext(t), svcCtx),
		&types.AssistantChatReq{
			Message: "hello", Mode: "agent", RequestId: "r-1",
			Attachments: []types.AssistantAttachment{{MediaId: 11, Url: "http://x/11.png"}},
		})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) == 0 || events[len(events)-1].Type != "done" {
		t.Fatalf("expected done terminal event, got %+v", events)
	}
	if captured == nil || captured.Mode != assistantpb.AssistantMode_ASSISTANT_MODE_AGENT {
		t.Fatalf("agent mode not forwarded: %+v", captured)
	}
	if len(captured.Attachments) != 1 || captured.Attachments[0].MediaId != 11 {
		t.Fatalf("attachments not forwarded: %+v", captured.Attachments)
	}
}

func TestEnhancedSearchIsDefaultMode(t *testing.T) {
	var captured *assistantservice.ChatReq
	service := fakeAssistantService{chat: func(_ context.Context, request *assistantservice.ChatReq) (assistantpb.AssistantService_ChatClient, error) {
		captured = request
		return &fakeAssistantChatClient{ctx: context.Background(), events: []*assistantpb.ChatEvent{
			{Type: assistantpb.ChatEventType_CHAT_EVENT_TYPE_DONE},
		}}, nil
	}}
	user := &fakeConsentUserService{}
	svcCtx := &svc.ServiceContext{AssistantService: service, UserService: user}
	if _, err := collectEvents(t, NewAssistantChatLogic(agentTestContext(t), svcCtx),
		&types.AssistantChatReq{Message: "hello"}); err != nil {
		t.Fatal(err)
	}
	if captured.Mode != assistantpb.AssistantMode_ASSISTANT_MODE_ENHANCED_SEARCH {
		t.Fatalf("default mode must be enhanced_search, got %v", captured.Mode)
	}
	if user.called != 0 {
		t.Fatalf("enhanced_search must not consult consent, got %d checks", user.called)
	}
}

func TestAgentChatRejectsInvalidAttachments(t *testing.T) {
	var chatCalls int
	service := fakeAssistantService{chat: func(context.Context, *assistantservice.ChatReq) (assistantpb.AssistantService_ChatClient, error) {
		chatCalls++
		return &fakeAssistantChatClient{}, nil
	}}
	user := &fakeConsentUserService{granted: true}
	svcCtx := &svc.ServiceContext{AssistantService: service, UserService: user}
	invalid := []types.AssistantAttachment{{MediaId: 0, Url: "http://x"}}
	events, err := collectEvents(t, NewAssistantChatLogic(agentTestContext(t), svcCtx),
		&types.AssistantChatReq{Message: "hello", Mode: "agent", Attachments: invalid})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].ErrorCode != "INVALID_REQUEST" || chatCalls != 0 {
		t.Fatalf("invalid attachment must fail before stream: %+v calls=%d", events, chatCalls)
	}

	tooMany := make([]types.AssistantAttachment, 10)
	for i := range tooMany {
		tooMany[i] = types.AssistantAttachment{MediaId: int64(i + 1), Url: "http://x"}
	}
	events, err = collectEvents(t, NewAssistantChatLogic(agentTestContext(t), svcCtx),
		&types.AssistantChatReq{Message: "hello", Mode: "agent", Attachments: tooMany})
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].ErrorCode != "INVALID_REQUEST" {
		t.Fatalf("attachment overflow must be rejected: %+v", events)
	}
}
