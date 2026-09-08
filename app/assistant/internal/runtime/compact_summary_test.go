package runtime

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"esx/app/assistant/internal/llm"
	"esx/app/assistant/internal/prompt"
	"esx/app/assistant/internal/store"
)

func TestRepeatedCompactCarriesPreviousSummaryAsUntrustedInput(t *testing.T) {
	ctx := context.Background()
	st := store.NewMemoryStore()
	decision := "Do not publish the draft before review."
	var inputs []struct {
		PreviousSummary string `json:"previous_summary"`
		Conversation    string `json:"conversation"`
	}
	model := scriptLLM{complete: func(_ context.Context, req llm.Request) (llm.Result, error) {
		if len(req.Messages) != 2 || req.Messages[1].Role != store.RoleUser || strings.Contains(req.Messages[0].Content, decision) {
			t.Fatalf("summary promoted to instructions: %+v", req.Messages)
		}
		var input struct {
			PreviousSummary string `json:"previous_summary"`
			Conversation    string `json:"conversation"`
		}
		if err := json.Unmarshal([]byte(req.Messages[1].Content), &input); err != nil {
			t.Fatal(err)
		}
		inputs = append(inputs, input)
		if !strings.Contains(input.PreviousSummary+input.Conversation, decision) {
			t.Fatal("decision absent from compaction input")
		}
		return llm.Result{Text: decision, Usage: llm.Usage{PromptTokens: 100, CompletionTokens: 10}}, nil
	}}
	engine, run := newCancelTestEngine(t, st, model)
	session, err := st.GetSession(ctx, run.SessionID)
	if err != nil {
		t.Fatal(err)
	}
	session.PromptSnapshot = prompt.EncodeSnapshot(prompt.BuildSnapshot(nil, nil, ""))
	if err := st.UpdateSession(ctx, *session); err != nil {
		t.Fatal(err)
	}
	for round := 0; round < 2; round++ {
		for i := 0; i < 10; i++ {
			text := strings.Repeat("recent conversation ", 200)
			if round == 0 && i == 0 {
				text = decision + text
			}
			if _, err := st.InsertMessage(ctx, store.Message{UserID: run.UserID, SessionID: run.SessionID, Role: store.RoleUser, Kind: store.KindMessage, Content: text, Visible: true}); err != nil {
				t.Fatal(err)
			}
		}
		messages, err := st.ListSessionMessages(ctx, run.UserID, run.SessionID, true)
		if err != nil {
			t.Fatal(err)
		}
		if err := engine.compact(ctx, ctx, &run, session, messages, model); err != nil {
			t.Fatal(err)
		}
	}
	if len(inputs) != 2 || inputs[0].PreviousSummary != "" || inputs[1].PreviousSummary != decision {
		t.Fatalf("inputs=%+v", inputs)
	}
	updated, _ := st.GetSession(ctx, run.SessionID)
	snapshot, ok := prompt.DecodeSnapshot(updated.PromptSnapshot)
	if !ok || snapshot.CompactSummary != decision || !strings.Contains(snapshot.SummarySidecar, decision) {
		t.Fatalf("snapshot=%+v", snapshot)
	}
	fresh, _ := st.GetRun(ctx, run.ID)
	if fresh.Rounds != 2 || fresh.InputTokens != 200 || fresh.OutputTokens != 20 {
		t.Fatalf("compact usage=%+v", fresh)
	}
}
