package runtime

import (
	"context"
	"errors"
	"testing"

	"esx/app/assistant/internal/llm"
	"esx/app/assistant/internal/memory"
	"esx/app/assistant/internal/store"
	"esx/app/assistant/internal/tool"
)

func TestExecuteMemoryReviewPublishesCommittedChangesOnEveryTerminal(t *testing.T) {
	for _, outcome := range []string{"done", "cancelled", "error", "budget"} {
		t.Run(outcome, func(t *testing.T) {
			ctx := context.Background()
			st := store.NewMemoryStore()
			memories := memory.NewMapStore()
			var run store.Run
			calls := 0
			model := scriptLLM{complete: func(context.Context, llm.Request) (llm.Result, error) {
				calls++
				if calls == 1 {
					return llm.Result{ToolCalls: []llm.ToolCall{{ID: "remember", Name: tool.AddMemory, Arguments: `{"target":"memory","content":"Prefers concise answers"}`}}}, nil
				}
				switch outcome {
				case "cancelled":
					if err := st.RequestCancel(ctx, run.UserID, run.ID); err != nil {
						t.Fatal(err)
					}
				case "error":
					return llm.Result{}, errors.New("model unavailable")
				case "budget":
					return llm.Result{Usage: llm.Usage{CompletionTokens: HardOutputTokens}}, nil
				}
				return llm.Result{Text: "review completed"}, nil
			}}
			engine, initial := newCancelTestEngine(t, st, model)
			run = initial
			run.Source = store.SourceMemoryReview
			if err := st.UpdateRun(ctx, run); err != nil {
				t.Fatal(err)
			}
			engine.Memory = memories
			registry, err := tool.NewRegistry(tool.Clients{Store: st, Memory: memories}, []string{tool.GetMemory, tool.AddMemory})
			if err != nil {
				t.Fatal(err)
			}
			engine.Tools = registry
			engine.Execute(ctx, run, false)
			fresh, err := st.GetRun(ctx, run.ID)
			if err != nil {
				t.Fatal(err)
			}
			want := outcome
			if outcome == "budget" {
				want = store.StatusError
			}
			if fresh.Status != want {
				t.Fatalf("run=%+v", fresh)
			}
			if calls != 2 {
				t.Fatalf("model calls=%d", calls)
			}
			messages, err := st.ListSessionMessages(ctx, run.UserID, run.SessionID, true)
			if err != nil {
				t.Fatal(err)
			}
			changes := 0
			for _, message := range messages {
				if message.RunID != run.ID {
					continue
				}
				if message.Kind != store.KindMemoryChanged || !message.Visible || message.Unread || message.ChangeID <= 0 {
					t.Fatalf("unexpected review message=%+v", message)
				}
				changes++
				if _, err := memories.Undo(ctx, run.UserID, message.ChangeID, store.NowMs()); err != nil {
					t.Fatalf("undo: %v", err)
				}
			}
			thread, _ := st.GetThread(ctx, run.UserID)
			if changes != 1 || thread.UnreadCount != 0 {
				t.Fatalf("changes=%d thread=%+v", changes, thread)
			}
			engine.Execute(ctx, run, false)
			after, _ := st.ListSessionMessages(ctx, run.UserID, run.SessionID, true)
			if len(after) != len(messages) {
				t.Fatal("terminal replay duplicated notification")
			}
		})
	}
}
