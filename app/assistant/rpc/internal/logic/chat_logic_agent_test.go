package logic

import (
	"context"
	"errors"
	"strings"
	"testing"

	"esx/app/assistant/rpc/internal/agent"
	assistantstore "esx/app/assistant/rpc/internal/store"
	"esx/app/assistant/rpc/internal/svc"
	"esx/app/assistant/rpc/internal/tool"
	"esx/app/assistant/rpc/xiaobaihe/assistant/pb"
	"esx/app/user/rpc/userservice"

	"google.golang.org/grpc"
)

type fakeConsentUser struct {
	userservice.UserService
	granted bool
	version int32
	err     error
}

func (f *fakeConsentUser) GetAgentCapabilityConsent(_ context.Context, _ *userservice.GetAgentCapabilityConsentReq, _ ...grpc.CallOption) (*userservice.GetAgentCapabilityConsentResp, error) {
	if f.err != nil {
		return nil, f.err
	}
	return &userservice.GetAgentCapabilityConsentResp{Granted: f.granted, ConsentVersion: f.version}, nil
}

type recordingAgentRunner struct {
	session *agent.Session
	result  *agent.Result
	err     error
	called  bool
}

func (r *recordingAgentRunner) Run(_ context.Context, session *agent.Session) (*agent.Result, error) {
	r.called = true
	r.session = session
	return r.result, r.err
}

func mustAgentTools(t *testing.T) *agent.ToolRegistry {
	t.Helper()
	registry, err := agent.NewToolRegistry(agent.Clients{}, agent.Version1Tools())
	if err != nil {
		t.Fatal(err)
	}
	return registry
}

func TestAgentChatRejectsUnauthorizedBeforeSideEffects(t *testing.T) {
	t.Parallel()
	state := &fakeAssistantState{allowed: true}
	runner := &recordingAgentRunner{result: &agent.Result{Text: "should not run"}}
	serviceContext := &svc.ServiceContext{
		UserService:   &fakeConsentUser{granted: false, version: 1},
		AgentRunner:   runner,
		AgentTools:    mustAgentTools(t),
		AgentQuota:    state,
		Conversations: state,
	}
	stream := &collectingChatStream{ctx: context.Background()}
	if err := NewChatLogic(context.Background(), serviceContext).Chat(&pb.ChatReq{
		UserId: 42, Message: "帮我发帖", RequestId: "req-1", Mode: pb.AssistantMode_ASSISTANT_MODE_AGENT,
	}, stream); err != nil {
		t.Fatal(err)
	}
	if runner.called {
		t.Fatal("unauthorized agent request must not execute tools")
	}
	if len(state.messages) != 0 {
		t.Fatalf("unauthorized request must not persist: %+v", state.messages)
	}
	if len(stream.events) != 1 || stream.events[0].Type != pb.ChatEventType_CHAT_EVENT_TYPE_ERROR ||
		stream.events[0].ErrorCode != "AGENT_NOT_AUTHORIZED" {
		t.Fatalf("expected AGENT_NOT_AUTHORIZED, got %+v", stream.events)
	}
}

func TestAgentChatConsentLookupFailureDoesNotRun(t *testing.T) {
	t.Parallel()
	runner := &recordingAgentRunner{result: &agent.Result{Text: "should not run"}}
	serviceContext := &svc.ServiceContext{
		UserService: &fakeConsentUser{err: errors.New("user rpc down")},
		AgentRunner: runner,
		AgentTools:  mustAgentTools(t),
		AgentQuota:  &fakeAssistantState{allowed: true},
	}
	stream := &collectingChatStream{ctx: context.Background()}
	if err := NewChatLogic(context.Background(), serviceContext).Chat(&pb.ChatReq{
		UserId: 42, Message: "hello", Mode: pb.AssistantMode_ASSISTANT_MODE_AGENT,
	}, stream); err != nil {
		t.Fatal(err)
	}
	if runner.called {
		t.Fatal("consent lookup failure must not run the agent")
	}
	if len(stream.events) != 1 || stream.events[0].ErrorCode != "ASSISTANT_UNAVAILABLE" {
		t.Fatalf("expected ASSISTANT_UNAVAILABLE, got %+v", stream.events)
	}
}

func TestAgentChatNeutralizesForgedCitationsAndAppendsVerifiedSources(t *testing.T) {
	t.Parallel()
	state := &fakeAssistantState{allowed: true}
	runner := &recordingAgentRunner{result: &agent.Result{
		Text: "结论见 [post:999] 和 [comment:8]。",
		Sources: []tool.Source{
			{Type: "post", ID: "11", Title: "实帖", Snippet: "正文摘录", Revision: 3},
			{Type: "comment", ID: "21", Title: "热评", Snippet: "评论摘录", Revision: 3},
			{Type: "web", ID: "https://example.test", Title: "外网", Snippet: "不能当帖子证据"},
		},
	}}
	serviceContext := &svc.ServiceContext{
		UserService:   &fakeConsentUser{granted: true, version: 2},
		AgentRunner:   runner,
		AgentTools:    mustAgentTools(t),
		AgentQuota:    state,
		Conversations: state,
	}
	stream := &collectingChatStream{ctx: context.Background()}
	if err := NewChatLogic(context.Background(), serviceContext).Chat(&pb.ChatReq{
		UserId: 42, Message: "社区里怎么看", RequestId: "req-1", Mode: pb.AssistantMode_ASSISTANT_MODE_AGENT,
	}, stream); err != nil {
		t.Fatal(err)
	}
	var streamed strings.Builder
	var sourceTypes []string
	for _, event := range stream.events {
		switch event.Type {
		case pb.ChatEventType_CHAT_EVENT_TYPE_TOKEN:
			streamed.WriteString(event.Text)
		case pb.ChatEventType_CHAT_EVENT_TYPE_SOURCE:
			sourceTypes = append(sourceTypes, event.Source.SourceType+":"+event.Source.SourceId)
		}
	}
	text := streamed.String()
	if strings.Contains(text, "[post:999]") || strings.Contains(text, "[comment:8]") {
		t.Fatalf("forged community markers survived: %q", text)
	}
	if !strings.Contains(text, "［post:999］") || !strings.Contains(text, "［comment:8］") {
		t.Fatalf("forged markers were not neutralized: %q", text)
	}
	if !strings.Contains(text, "SOURCE [post:11]") || !strings.Contains(text, "SOURCE [comment:21]") {
		t.Fatalf("verified sources were not appended: %q", text)
	}
	if strings.Contains(text, "SOURCE [web:") {
		t.Fatalf("web sources must not become community evidence: %q", text)
	}
	if strings.Join(sourceTypes, ",") != "post:11,comment:21,web:https://example.test" {
		t.Fatalf("unexpected streamed sources: %v", sourceTypes)
	}
}

func TestAgentChatInjectsPriorTurnsAndSkipsCurrentRequest(t *testing.T) {
	t.Parallel()
	state := &fakeAssistantState{
		allowed: true,
		messages: []assistantstore.Message{
			{Role: "user", Content: "推荐几个周末攻略", RequestID: "old"},
			{Role: "assistant", Content: "先看这三篇", RequestID: "old"},
		},
	}
	runner := &recordingAgentRunner{result: &agent.Result{Text: "还有这几篇"}}
	serviceContext := &svc.ServiceContext{
		UserService:   &fakeConsentUser{granted: true, version: 2},
		AgentRunner:   runner,
		AgentTools:    mustAgentTools(t),
		AgentQuota:    state,
		Conversations: state,
	}
	stream := &collectingChatStream{ctx: context.Background()}
	if err := NewChatLogic(context.Background(), serviceContext).Chat(&pb.ChatReq{
		UserId: 42, ConversationId: "conv-1", Message: "还有吗", RequestId: "new",
		Mode: pb.AssistantMode_ASSISTANT_MODE_AGENT,
	}, stream); err != nil {
		t.Fatal(err)
	}
	if !runner.called || runner.session == nil {
		t.Fatal("expected agent runner to execute")
	}
	if runner.session.UserMessage != "还有吗" {
		t.Fatalf("user message=%q", runner.session.UserMessage)
	}
	if len(runner.session.History) != 2 ||
		runner.session.History[0].Content != "推荐几个周末攻略" ||
		runner.session.History[1].Content != "先看这三篇" {
		t.Fatalf("history=%+v", runner.session.History)
	}
}
