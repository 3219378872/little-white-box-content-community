package runtime

import (
	"context"
	"strings"
	"testing"

	"esx/app/assistant/internal/prompt"
	"esx/app/assistant/internal/store"
)

func TestSelectKeepLastTwentyPercent(t *testing.T) {
	msgs := make([]store.Message, 0, 10)
	for i := 0; i < 10; i++ {
		msgs = append(msgs, store.Message{ID: int64(i + 1), Role: store.RoleUser, Content: strings.Repeat("字", 40), Visible: true})
	}
	total := EstimateMessageTokens(msgs)
	keep := total / 5
	selected := SelectKeep(msgs, keep, nil)
	if len(selected) == 0 {
		t.Fatal("expected some kept messages")
	}
	if selected[len(selected)-1].ID != 10 {
		t.Fatalf("should keep the newest, got %+v", selected)
	}
	if EstimateMessageTokens(selected) > keep+EstimateTokens(strings.Repeat("字", 40)) {
		t.Fatalf("kept too many tokens: %d vs %d", EstimateMessageTokens(selected), keep)
	}
}

func TestHistoryTurnsReplaysHiddenToolSidecar(t *testing.T) {
	assistant := prompt.Turn{Role: store.RoleAssistant, ToolCalls: []prompt.ToolCall{
		{ID: "c1", Name: "search_posts", Arguments: `{"keyword":"猫粮"}`},
	}}
	tool := prompt.Turn{Role: store.RoleTool, ToolCallID: "c1", Name: "search_posts", Content: "没有可展示的已发布帖子。"}
	got := HistoryTurns([]store.Message{
		{ID: 1, Role: store.RoleUser, Kind: store.KindMessage, Content: "查猫粮", Visible: true, APIContent: prompt.EncodeTurn(prompt.Turn{Role: store.RoleUser, Content: "查猫粮"})},
		{ID: 2, Role: store.RoleAssistant, Kind: store.KindTool, Visible: false, APIContent: prompt.EncodeTurn(assistant)},
		{ID: 3, Role: store.RoleTool, Kind: store.KindTool, Content: tool.Content, Visible: false, APIContent: prompt.EncodeTurn(tool)},
		{ID: 4, Role: store.RoleAssistant, Kind: store.KindMemoryChanged, Content: "ignored", Visible: false},
	})
	if len(got) != 3 {
		t.Fatalf("turns=%+v", got)
	}
	if got[0].Role != store.RoleUser || got[0].Content != "查猫粮" {
		t.Fatalf("user=%+v", got[0])
	}
	if len(got[1].ToolCalls) != 1 || got[1].ToolCalls[0].ID != "c1" {
		t.Fatalf("assistant=%+v", got[1])
	}
	if got[2].ToolCallID != "c1" || got[2].Content != tool.Content {
		t.Fatalf("tool=%+v", got[2])
	}
}

func TestCompactLeavesKeptMessagesLive(t *testing.T) {
	mem := store.NewMemoryStore()
	ctx := context.Background()
	session, err := mem.CreateSession(ctx, store.Session{
		UserID: 1, PromptEpoch: 1, Status: store.SessionOpen, CreatedAtMs: 1,
		PromptSnapshot: prompt.EncodeSnapshot(prompt.BuildSnapshot(nil, nil, "")),
	})
	if err != nil {
		t.Fatal(err)
	}
	msgs := make([]store.Message, 0, 10)
	for i := 0; i < 10; i++ {
		msg, err := mem.InsertMessage(ctx, store.Message{
			UserID: 1, SessionID: session.ID, Role: store.RoleUser, Kind: store.KindMessage,
			Content: strings.Repeat("字", 40), Visible: true, CreatedAtMs: int64(i + 1),
		})
		if err != nil {
			t.Fatal(err)
		}
		msgs = append(msgs, msg)
	}
	run, err := mem.InsertRun(ctx, store.Run{UserID: 1, SessionID: session.ID, Status: store.StatusRunning, CreatedAtMs: 1})
	if err != nil {
		t.Fatal(err)
	}
	engine := &Engine{Store: mem}
	if err := engine.compact(ctx, &run, &session, msgs); err != nil {
		t.Fatal(err)
	}
	listed, err := mem.ListSessionMessages(ctx, 1, session.ID, true)
	if err != nil {
		t.Fatal(err)
	}
	live, compacted := 0, 0
	for _, msg := range listed {
		if msg.Compacted {
			compacted++
			continue
		}
		live++
	}
	if live == 0 {
		t.Fatal("kept messages should remain uncompacted")
	}
	if compacted == 0 {
		t.Fatal("expected dropped messages to be compacted")
	}
	if live+compacted != 10 {
		t.Fatalf("live=%d compacted=%d", live, compacted)
	}
}
