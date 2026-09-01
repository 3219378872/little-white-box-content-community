package runtime

import (
	"bytes"
	"context"
	"testing"

	"esx/app/assistant/internal/memory"
	"esx/app/assistant/internal/prompt"
	"esx/app/assistant/internal/store"
)

func TestAcceptColdConversationSplicesPersistentLayersOnSameSession(t *testing.T) {
	ctx := context.Background()
	mem := store.NewMemoryStore()
	memories := memory.NewMapStore()
	a := &Acceptor{Store: mem, Memory: memories, Notify: store.NewMemoryNotifier()}
	first, err := a.Accept(ctx, AcceptInput{UserID: 1, Message: "hello", RequestID: "r1", ConsentOK: true, ConsentVersion: 2})
	if err != nil {
		t.Fatal(err)
	}
	finishUserRun(t, mem, first.RunID)
	session, err := mem.GetSession(ctx, first.SessionID)
	if err != nil || session.PromptEpoch != 1 {
		t.Fatalf("session=%+v err=%v", session, err)
	}
	frozen := append([]byte(nil), session.PromptSnapshot...)
	if _, _, err := memories.Add(ctx, 1, memory.TargetMemory, "喜欢短答案", "mem-1", store.NowMs()); err != nil {
		t.Fatal(err)
	}
	thread, _ := mem.GetThread(ctx, 1)
	thread.ActiveRunID = 0
	thread.LastMessageAtMs = store.NowMs() - ColdConversationIdle.Milliseconds()
	if err := mem.SaveThread(ctx, *thread); err != nil {
		t.Fatal(err)
	}

	second, err := a.Accept(ctx, AcceptInput{UserID: 1, Message: "again", RequestID: "r2", ConsentOK: true, ConsentVersion: 2})
	if err != nil {
		t.Fatal(err)
	}
	if second.SessionID != first.SessionID {
		t.Fatalf("session changed %d -> %d", first.SessionID, second.SessionID)
	}
	updated, err := mem.GetSession(ctx, second.SessionID)
	if err != nil || updated.PromptEpoch != 2 {
		t.Fatalf("updated=%+v err=%v", updated, err)
	}
	if bytes.Equal(updated.PromptSnapshot, frozen) {
		t.Fatal("cold splice should rebuild prompt snapshot")
	}
	snap, ok := prompt.DecodeSnapshot(updated.PromptSnapshot)
	if !ok || len(snap.Memory) != 1 || snap.Memory[0].Content != "喜欢短答案" {
		t.Fatalf("memory not spliced: %+v", snap.Memory)
	}
	msgs, err := mem.ListSessionMessages(ctx, 1, first.SessionID, true)
	if err != nil || len(msgs) != 2 {
		t.Fatalf("history should remain in the same session: %+v err=%v", msgs, err)
	}
}

func TestAcceptWarmConversationDoesNotSplice(t *testing.T) {
	ctx := context.Background()
	mem := store.NewMemoryStore()
	memories := memory.NewMapStore()
	a := &Acceptor{Store: mem, Memory: memories, Notify: store.NewMemoryNotifier()}
	first, err := a.Accept(ctx, AcceptInput{UserID: 1, Message: "hello", RequestID: "r1", ConsentOK: true, ConsentVersion: 2})
	if err != nil {
		t.Fatal(err)
	}
	finishUserRun(t, mem, first.RunID)
	if _, _, err := memories.Add(ctx, 1, memory.TargetMemory, "喜欢短答案", "mem-1", store.NowMs()); err != nil {
		t.Fatal(err)
	}
	thread, _ := mem.GetThread(ctx, 1)
	thread.ActiveRunID = 0
	thread.LastMessageAtMs = store.NowMs() - ColdConversationIdle.Milliseconds() + 60_000
	if err := mem.SaveThread(ctx, *thread); err != nil {
		t.Fatal(err)
	}
	second, err := a.Accept(ctx, AcceptInput{UserID: 1, Message: "again", RequestID: "r2", ConsentOK: true, ConsentVersion: 2})
	if err != nil {
		t.Fatal(err)
	}
	if second.SessionID != first.SessionID {
		t.Fatalf("session changed %d -> %d", first.SessionID, second.SessionID)
	}
	updated, _ := mem.GetSession(ctx, second.SessionID)
	if updated.PromptEpoch != 1 {
		t.Fatalf("warm splice epoch=%d", updated.PromptEpoch)
	}
	snap, _ := prompt.DecodeSnapshot(updated.PromptSnapshot)
	if len(snap.Memory) != 0 {
		t.Fatalf("warm snapshot picked up memory: %+v", snap.Memory)
	}
}

func TestAcceptEmptyThreadIsNotCold(t *testing.T) {
	ctx := context.Background()
	mem := store.NewMemoryStore()
	a := &Acceptor{Store: mem, Notify: store.NewMemoryNotifier()}
	got, err := a.Accept(ctx, AcceptInput{UserID: 1, Message: "hello", RequestID: "r1", ConsentOK: true, ConsentVersion: 2})
	if err != nil {
		t.Fatal(err)
	}
	session, _ := mem.GetSession(ctx, got.SessionID)
	if session.PromptEpoch != 1 {
		t.Fatalf("empty thread epoch=%d", session.PromptEpoch)
	}
}

func TestAcceptRedirectDoesNotSpliceColdSnapshot(t *testing.T) {
	ctx := context.Background()
	mem := store.NewMemoryStore()
	memories := memory.NewMapStore()
	a := &Acceptor{Store: mem, Memory: memories, Notify: store.NewMemoryNotifier()}
	first, err := a.Accept(ctx, AcceptInput{UserID: 1, Message: "hello", RequestID: "r1", ConsentOK: true, ConsentVersion: 2})
	if err != nil {
		t.Fatal(err)
	}
	run, _ := mem.GetRun(ctx, first.RunID)
	run.Status = store.StatusRunning
	run.Phase = store.PhaseModelRequest
	_ = mem.UpdateRun(ctx, *run)
	thread, _ := mem.GetThread(ctx, 1)
	thread.ActiveRunID = run.ID
	thread.LastMessageAtMs = store.NowMs() - ColdConversationIdle.Milliseconds()
	_ = mem.SaveThread(ctx, *thread)
	if _, _, err := memories.Add(ctx, 1, memory.TargetMemory, "喜欢短答案", "mem-1", store.NowMs()); err != nil {
		t.Fatal(err)
	}

	second, err := a.Accept(ctx, AcceptInput{UserID: 1, Message: "redirect", RequestID: "r2", ConsentOK: true, ConsentVersion: 2})
	if err != nil || second.Disposition != store.DispositionRedirected || second.SessionID != first.SessionID {
		t.Fatalf("redirect=%+v err=%v", second, err)
	}
	session, _ := mem.GetSession(ctx, first.SessionID)
	if session.PromptEpoch != 1 {
		t.Fatalf("redirect spliced epoch=%d", session.PromptEpoch)
	}
}

func TestEnsureForegroundSessionReopensClosedLegacyRow(t *testing.T) {
	ctx := context.Background()
	mem := store.NewMemoryStore()
	now := store.NowMs()
	session, err := mem.CreateSession(ctx, store.Session{
		UserID: 4, PromptEpoch: 3, Status: store.SessionClosed, ClosedAtMs: now - 1, CreatedAtMs: now - 2,
		PromptSnapshot: prompt.EncodeSnapshot(prompt.BuildSnapshot(nil, nil, "旧摘要")),
		CompactSummary: "旧摘要",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := mem.SaveThread(ctx, store.Thread{UserID: 4, SessionID: session.ID, UpdatedAtMs: now}); err != nil {
		t.Fatal(err)
	}
	a := &Acceptor{Store: mem, Notify: store.NewMemoryNotifier()}
	got, err := a.Accept(ctx, AcceptInput{UserID: 4, Message: "resume", RequestID: "r1", ConsentOK: true, ConsentVersion: 2})
	if err != nil {
		t.Fatal(err)
	}
	if got.SessionID != session.ID {
		t.Fatalf("created new session %d want %d", got.SessionID, session.ID)
	}
	updated, _ := mem.GetSession(ctx, session.ID)
	if updated.Status != store.SessionOpen || updated.ClosedAtMs != 0 {
		t.Fatalf("reopen=%+v", updated)
	}
}

func TestSpliceColdSessionKeepsCompactSummary(t *testing.T) {
	ctx := context.Background()
	mem := store.NewMemoryStore()
	session, err := mem.CreateSession(ctx, store.Session{
		UserID: 5, PromptEpoch: 1, Status: store.SessionOpen, CreatedAtMs: 1,
		CompactSummary: "压缩过的旧对话",
		PromptSnapshot: prompt.EncodeSnapshot(prompt.BuildSnapshot(nil, nil, "压缩过的旧对话")),
	})
	if err != nil {
		t.Fatal(err)
	}
	updated, err := spliceColdSession(ctx, mem, nil, &session)
	if err != nil || updated.PromptEpoch != 2 || updated.CompactSummary != "压缩过的旧对话" {
		t.Fatalf("updated=%+v err=%v", updated, err)
	}
	snap, ok := prompt.DecodeSnapshot(updated.PromptSnapshot)
	if !ok || snap.CompactSummary != "压缩过的旧对话" {
		t.Fatalf("summary dropped: %+v", snap)
	}
}

func TestWatchColdScheduleSplicesExistingSession(t *testing.T) {
	ctx := context.Background()
	mem := store.NewMemoryStore()
	memories := memory.NewMapStore()
	if _, _, err := memories.Add(ctx, 7, memory.TargetUser, "设计师", "mem-1", store.NowMs()); err != nil {
		t.Fatal(err)
	}
	now := store.NowMs()
	session, err := mem.CreateSession(ctx, store.Session{
		UserID: 7, PromptEpoch: 1, Status: store.SessionOpen, CreatedAtMs: now - ColdConversationIdle.Milliseconds(),
		PromptSnapshot: prompt.EncodeSnapshot(prompt.BuildSnapshot(nil, nil, "")),
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := mem.SaveThread(ctx, store.Thread{
		UserID: 7, SessionID: session.ID, LastMessageAtMs: now - ColdConversationIdle.Milliseconds(), UpdatedAtMs: now,
	}); err != nil {
		t.Fatal(err)
	}
	watchStore, bucket, _ := watchFixture(t, mem)
	if err := scheduleBucket(ctx, mem, memories, watchStore, func(context.Context, int64) (bool, error) { return true, nil }, allowAllWatchPosts, bucket, now); err != nil {
		t.Fatal(err)
	}
	updated, err := mem.GetSession(ctx, session.ID)
	if err != nil || updated.PromptEpoch != 2 {
		t.Fatalf("watch splice=%+v err=%v", updated, err)
	}
	snap, _ := prompt.DecodeSnapshot(updated.PromptSnapshot)
	if len(snap.Memory) != 1 || snap.Memory[0].Content != "设计师" {
		t.Fatalf("watch memory=%+v", snap.Memory)
	}
	thread, _ := mem.GetThread(ctx, 7)
	if thread.SessionID != session.ID {
		t.Fatalf("watch created another session: %+v", thread)
	}
}

func finishUserRun(t *testing.T, mem *store.MemoryStore, runID int64) {
	t.Helper()
	run, err := mem.GetRun(context.Background(), runID)
	if err != nil {
		t.Fatal(err)
	}
	run.Status = store.StatusDone
	run.Phase = store.PhaseDone
	if err := mem.UpdateRun(context.Background(), *run); err != nil {
		t.Fatal(err)
	}
}
