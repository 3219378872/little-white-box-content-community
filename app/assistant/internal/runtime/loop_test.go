package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"esx/app/assistant/internal/llm"
	"esx/app/assistant/internal/prompt"
	"esx/app/assistant/internal/store"
	"esx/app/assistant/internal/tool"
	"esx/app/content/rpc/contentservice"

	"google.golang.org/grpc"
)

type runtimeContent struct {
	contentservice.ContentService
}

func (*runtimeContent) GetPostsByIds(_ context.Context, _ *contentservice.GetPostsByIdsReq, _ ...grpc.CallOption) (*contentservice.GetPostsByIdsResp, error) {
	return &contentservice.GetPostsByIdsResp{Posts: []*contentservice.PostInfo{{Id: 9, Status: 1, Revision: 1, Title: "猫粮", Content: "来源正文"}}}, nil
}

type scriptedLLM struct {
	reqs    []llm.Request
	replies []llm.Result
}

func (s *scriptedLLM) Complete(_ context.Context, req llm.Request) (llm.Result, error) {
	copied := req
	copied.Messages = append([]prompt.Turn(nil), req.Messages...)
	s.reqs = append(s.reqs, copied)
	if len(s.reqs) > len(s.replies) {
		return llm.Result{}, fmt.Errorf("unexpected complete #%d", len(s.reqs))
	}
	return s.replies[len(s.reqs)-1], nil
}
func (s *scriptedLLM) SupportsTools() bool      { return true }
func (s *scriptedLLM) WireAPI() string          { return llm.WireAPIResponses }
func (s *scriptedLLM) MaxOutputTokens() int     { return 128 }
func (s *scriptedLLM) ContextWindowTokens() int { return 128000 }

func TestToolRoundIsReplayedOnNextComplete(t *testing.T) {
	mem := store.NewMemoryStore()
	ctx := context.Background()
	reg, err := tool.NewRegistry(tool.Clients{Store: mem, Content: &runtimeContent{}}, []string{tool.PresentSources})
	if err != nil {
		t.Fatal(err)
	}
	session, run, userMsg := mustStartRun(t, mem, "查猫粮")
	if _, err := mem.InsertSource(ctx, store.Source{RunID: run.ID, Handle: "h1", Kind: "post", AuthorityID: "9", Revision: 1, CreatedAtMs: 1}); err != nil {
		t.Fatal(err)
	}
	script := &scriptedLLM{replies: []llm.Result{
		{ToolCalls: []llm.ToolCall{{ID: "c1", Name: tool.PresentSources, Arguments: `"{\"handles\":[\"h1\"]}"`}}},
		{Text: "这是整理后的结果"},
	}}
	engine := &Engine{Store: mem, Tools: reg, LLM: script, Window: 128000}
	engine.Execute(ctx, run, false)

	if len(script.reqs) != 2 {
		t.Fatalf("complete calls=%d", len(script.reqs))
	}
	if countUserText(script.reqs[0].Messages, "查猫粮") != 1 {
		t.Fatalf("first request duplicated user text: %+v", roles(script.reqs[0].Messages))
	}
	second := script.reqs[1].Messages
	if countUserText(second, "查猫粮") != 1 {
		t.Fatalf("second request duplicated user text: %+v", roles(second))
	}
	if !hasToolCall(second, "c1", tool.PresentSources) {
		t.Fatalf("missing function_call in second request: %+v", second)
	}
	if !hasToolResult(second, "c1", "h1") && !hasToolResult(second, "c1", "来源") {
		t.Fatalf("missing tool result in second request: %+v", second)
	}
	listed, _ := mem.ListSessionMessages(ctx, 1, session.ID, true)
	hidden := 0
	visibleAssistant := 0
	for _, msg := range listed {
		if msg.Kind == store.KindTool && !msg.Visible {
			hidden++
		}
		if msg.Role == store.RoleAssistant && msg.Visible && strings.Contains(msg.Content, "这是整理后的结果") {
			visibleAssistant++
		}
	}
	if hidden < 2 {
		t.Fatalf("expected hidden tool-round messages, listed=%+v", listed)
	}
	if visibleAssistant != 1 {
		t.Fatalf("final assistant visible=%d userMsg=%d", visibleAssistant, userMsg.ID)
	}
	outbox, err := mem.ListUnpublishedOutbox(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(outbox) != 1 || outbox[0].MessageID == 0 {
		t.Fatalf("assistant history outbox=%+v", outbox)
	}
	call, err := mem.GetToolCall(ctx, run.ID, "c1")
	if err != nil {
		t.Fatal(err)
	}
	if call.Status != "success" || !json.Valid([]byte(call.ResultJSON)) {
		t.Fatalf("tool row=%+v", call)
	}
}

func TestResumeExecutesUnmatchedToolCalls(t *testing.T) {
	mem := store.NewMemoryStore()
	ctx := context.Background()
	reg, err := tool.NewRegistry(tool.Clients{Store: mem, Content: &runtimeContent{}}, []string{tool.PresentSources})
	if err != nil {
		t.Fatal(err)
	}
	session, run, _ := mustStartRun(t, mem, "查猫粮")
	if _, err := mem.InsertSource(ctx, store.Source{RunID: run.ID, Handle: "h1", Kind: "post", AuthorityID: "9", Revision: 1, CreatedAtMs: 1}); err != nil {
		t.Fatal(err)
	}
	assistant := prompt.Turn{Role: store.RoleAssistant, ToolCalls: []prompt.ToolCall{
		{ID: "c1", Name: tool.PresentSources, Arguments: `{"handles":["h1"]}`},
	}}
	if _, err := mem.InsertMessage(ctx, store.Message{
		UserID: 1, SessionID: session.ID, RunID: run.ID, Role: store.RoleAssistant, Kind: store.KindTool,
		APIContent: prompt.EncodeTurn(assistant), Visible: false, CreatedAtMs: store.NowMs(),
	}); err != nil {
		t.Fatal(err)
	}
	script := &scriptedLLM{replies: []llm.Result{{Text: "done"}}}
	engine := &Engine{Store: mem, Tools: reg, LLM: script, Window: 128000}
	engine.Execute(ctx, run, true)
	if len(script.reqs) != 1 {
		t.Fatalf("complete calls=%d", len(script.reqs))
	}
	if !hasToolResult(script.reqs[0].Messages, "c1", "h1") && !hasToolResult(script.reqs[0].Messages, "c1", "来源") {
		t.Fatalf("resume did not feed tool result: %+v", script.reqs[0].Messages)
	}
}

func TestIncompleteResponsePersistsPartialAndEndsError(t *testing.T) {
	mem := store.NewMemoryStore()
	_, run, _ := mustStartRun(t, mem, "long answer")
	reg, err := tool.NewRegistry(tool.Clients{Store: mem}, []string{tool.PresentSources})
	if err != nil {
		t.Fatal(err)
	}
	script := &scriptedLLM{replies: []llm.Result{{Text: "partial body", Raw: []byte(`{"status":"incomplete"}`), IncompleteReason: "content_filter"}}}
	engine := &Engine{Store: mem, Tools: reg, LLM: script, Window: 128000}
	engine.Execute(context.Background(), run, false)
	fresh, err := mem.GetRun(context.Background(), run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if fresh.Status != store.StatusError || fresh.ErrorCode != "LLM_INCOMPLETE_CONTENT_FILTER" {
		t.Fatalf("run=%+v", fresh)
	}
	events, _ := mem.ListEventsAfter(context.Background(), run.ID, 0)
	if len(events) < 3 || events[len(events)-1].Type != store.EventError {
		t.Fatalf("events=%+v", events)
	}
	msgs, _ := mem.ListSessionMessages(context.Background(), 1, run.SessionID, true)
	partial := 0
	for _, msg := range msgs {
		if msg.Role == store.RoleAssistant && msg.Content == "partial body" && msg.Visible {
			partial++
		}
	}
	if partial != 1 {
		t.Fatalf("partial messages=%d messages=%+v", partial, msgs)
	}
}

func mustStartRun(t *testing.T, mem *store.MemoryStore, text string) (store.Session, store.Run, store.Message) {
	t.Helper()
	ctx := context.Background()
	session, err := mem.CreateSession(ctx, store.Session{
		UserID: 1, PromptEpoch: 1, Status: store.SessionOpen, CreatedAtMs: 1,
		PromptSnapshot: prompt.EncodeSnapshot(prompt.BuildSnapshot(nil, nil, "")),
	})
	if err != nil {
		t.Fatal(err)
	}
	userMsg, err := mem.InsertMessage(ctx, store.Message{
		UserID: 1, SessionID: session.ID, Role: store.RoleUser, Kind: store.KindMessage,
		Content: text, Visible: true, APIContent: prompt.EncodeTurn(prompt.Turn{Role: store.RoleUser, Content: text}),
		CreatedAtMs: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	payload, _ := json.Marshal(map[string]any{"text": text, "message_id": userMsg.ID})
	run, err := mem.InsertRun(ctx, store.Run{
		UserID: 1, SessionID: session.ID, RequestID: "r1", Source: store.SourceUser,
		Status: store.StatusQueued, Phase: store.PhaseModelRequest, QueuedPayload: payload,
		ConsentVersion: 2, InputVersion: 1, CreatedAtMs: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := mem.Claim(ctx, "test-worker", store.NowMs(), 60_000)
	if err != nil || claimed == nil || claimed.ID != run.ID {
		t.Fatalf("claim run: %+v %v", claimed, err)
	}
	return session, *claimed, userMsg
}

func countUserText(turns []prompt.Turn, text string) int {
	n := 0
	for _, turn := range turns {
		if turn.Role == store.RoleUser && turn.Content == text && turn.ToolCallID == "" && len(turn.ToolCalls) == 0 {
			n++
		}
	}
	return n
}

func hasToolCall(turns []prompt.Turn, id, name string) bool {
	for _, turn := range turns {
		for _, call := range turn.ToolCalls {
			if call.ID == id && call.Name == name {
				return true
			}
		}
	}
	return false
}

func hasToolResult(turns []prompt.Turn, id, substr string) bool {
	for _, turn := range turns {
		if turn.ToolCallID == id && strings.Contains(turn.Content, substr) {
			return true
		}
	}
	return false
}

func roles(turns []prompt.Turn) []string {
	out := make([]string, 0, len(turns))
	for _, turn := range turns {
		label := turn.Role
		if turn.ToolCallID != "" {
			label += "/result:" + turn.ToolCallID
		}
		if len(turn.ToolCalls) > 0 {
			label += "/calls"
		}
		out = append(out, label)
	}
	return out
}
