package runtime

import (
	"context"
	"testing"

	"esx/app/assistant/internal/memory"
	"esx/app/assistant/internal/store"
	"esx/app/assistant/internal/tool"
)

func TestMemoryReviewWritesUndoableMemoryChangedMessage(t *testing.T) {
	ctx := context.Background()
	mem := store.NewMemoryStore()
	memoryStore := memory.NewMapStore()
	_, changeID, err := memoryStore.Add(ctx, 7, memory.TargetMemory, "偏好短答案", "review-change", 10)
	if err != nil {
		t.Fatal(err)
	}
	session, err := mem.CreateSession(ctx, store.Session{UserID: 7, PromptEpoch: 1, Status: store.SessionOpen, CreatedAtMs: 1})
	if err != nil {
		t.Fatal(err)
	}
	run, err := mem.InsertRun(ctx, store.Run{
		UserID: 7, SessionID: session.ID, RequestID: "review-1", Source: store.SourceMemoryReview,
		Status: store.StatusQueued, Phase: store.PhaseModelRequest, ConsentVersion: 2, InputVersion: 1, CreatedAtMs: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := mem.InsertToolCall(ctx, store.ToolCall{
		RunID: run.ID, CallID: "memory-1", Tool: tool.AddMemory, Status: "success",
		ResultJSON: encodeToolResultJSONWithChanges("已写入 memory#1", nil, []int64{changeID}), CreatedAtMs: 2,
	}); err != nil {
		t.Fatal(err)
	}
	claimed, err := mem.Claim(ctx, "review-worker", store.NowMs(), 60_000)
	if err != nil || claimed == nil || claimed.ID != run.ID {
		t.Fatalf("claim=%+v err=%v", claimed, err)
	}
	run = *claimed
	engine := &Engine{Store: mem, Memory: memoryStore}
	if err := engine.completeMemoryReview(ctx, run); err != nil {
		t.Fatal(err)
	}
	if err := engine.completeMemoryReview(ctx, run); err != nil {
		t.Fatal(err)
	}
	messages, _ := mem.ListMessages(ctx, 7, session.ID, 0, 0, 20)
	changed := 0
	for _, msg := range messages {
		if msg.Kind == store.KindMemoryChanged {
			changed++
			if msg.ChangeID != changeID || msg.Unread {
				t.Fatalf("memory_changed=%+v", msg)
			}
		}
	}
	if changed != 1 {
		t.Fatalf("memory_changed messages=%d all=%+v", changed, messages)
	}
	thread, _ := mem.GetThread(ctx, 7)
	if thread.UnreadCount != 0 {
		t.Fatalf("thread=%+v", thread)
	}
	if _, err := memoryStore.Undo(ctx, 7, changeID, 20); err != nil {
		t.Fatalf("undo change %d: %v", changeID, err)
	}
}
