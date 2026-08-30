package runtime

import (
	"context"
	"errors"
	"strings"
	"testing"

	"esx/app/assistant/internal/llm"
	"esx/app/assistant/internal/memory"
	"esx/app/assistant/internal/prompt"
	"esx/app/assistant/internal/store"
	"esx/app/assistant/internal/tool"
)

func TestNoProgressGuardCoversUserAndMemoryReviewRuns(t *testing.T) {
	for _, source := range []string{store.SourceUser, store.SourceMemoryReview} {
		t.Run(source+"-second", func(t *testing.T) {
			mem, run, registry := noProgressFixture(t, source, 2)
			engine := &Engine{Store: mem}
			var reviewLive []prompt.Turn
			err := engine.guardToolProgress(context.Background(), &run, registry, llm.ToolCall{
				ID: "call-2", Name: tool.GetMemory, Arguments: `{}`,
			}, &reviewLive)
			if err != nil {
				t.Fatal(err)
			}
			if source == store.SourceMemoryReview {
				if len(reviewLive) != 1 || !strings.Contains(reviewLive[0].Content, "工具无进展") {
					t.Fatalf("review convergence=%+v", reviewLive)
				}
				return
			}
			messages, err := mem.ListSessionMessages(context.Background(), run.UserID, run.SessionID, true)
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, message := range messages {
				turn, ok := prompt.DecodeTurn(message.APIContent)
				if ok && strings.Contains(turn.Content, "工具无进展") {
					found = true
				}
			}
			if !found {
				t.Fatalf("user convergence message missing: %+v", messages)
			}
		})

		t.Run(source+"-third", func(t *testing.T) {
			mem, run, registry := noProgressFixture(t, source, 3)
			engine := &Engine{Store: mem}
			err := engine.guardToolProgress(context.Background(), &run, registry, llm.ToolCall{
				ID: "call-3", Name: tool.GetMemory, Arguments: `{}`,
			}, nil)
			if !errors.Is(err, errRunTerminated) {
				t.Fatalf("guard err=%v", err)
			}
			fresh, err := mem.GetRun(context.Background(), run.ID)
			if err != nil || fresh.Status != store.StatusError || fresh.ErrorCode != "TOOL_NO_PROGRESS" {
				t.Fatalf("run=%+v err=%v", fresh, err)
			}
		})
	}
}

func noProgressFixture(t *testing.T, source string, count int) (*store.MemoryStore, store.Run, *tool.Registry) {
	t.Helper()
	mem := store.NewMemoryStore()
	_, run, _ := mustStartRun(t, mem, "repeat")
	if source != store.SourceUser {
		run.Source = source
		if err := mem.RunStep(context.Background(), run.Fence(), func(ctx context.Context, tx store.Store) error {
			return tx.UpdateRun(ctx, run)
		}); err != nil {
			t.Fatal(err)
		}
	}
	registry, err := tool.NewRegistry(tool.Clients{Memory: memory.NewMapStore()}, []string{tool.GetMemory})
	if err != nil {
		t.Fatal(err)
	}
	for i := 1; i <= count; i++ {
		_, err := mem.InsertToolCall(context.Background(), store.ToolCall{
			RunID: run.ID, CallID: "call-" + itoa(int64(i)), Tool: tool.GetMemory,
			CanonicalArgsDigest: "same-args", Status: "success", ResultJSON: `{"ok":true}`,
			CreatedAtMs: int64(i),
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	return mem, run, registry
}
