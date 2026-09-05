package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"esx/app/assistant/internal/llm"
	"esx/app/assistant/internal/memory"
	"esx/app/assistant/internal/prompt"
	"esx/app/assistant/internal/store"
	"esx/app/assistant/internal/tool"
)

type scriptLLM struct {
	complete func(ctx context.Context, req llm.Request) (llm.Result, error)
}

func (s scriptLLM) Complete(ctx context.Context, req llm.Request) (llm.Result, error) {
	return s.complete(ctx, req)
}
func (s scriptLLM) SupportsTools() bool      { return true }
func (s scriptLLM) WireAPI() string          { return llm.WireAPIChatCompletions }
func (s scriptLLM) MaxOutputTokens() int     { return 128 }
func (s scriptLLM) ContextWindowTokens() int { return 128000 }

func TestUpdateRunDoesNotClearCancelRequested(t *testing.T) {
	mem := store.NewMemoryStore()
	ctx := context.Background()
	run, err := mem.InsertRun(ctx, store.Run{UserID: 1, Status: store.StatusRunning, Phase: store.PhaseModelRequest})
	if err != nil {
		t.Fatal(err)
	}
	if err := mem.RequestCancel(ctx, 1, run.ID); err != nil {
		t.Fatal(err)
	}
	run.CancelRequested = false
	run.Phase = store.PhaseToolExecuting
	if err := mem.UpdateRun(ctx, run); err != nil {
		t.Fatal(err)
	}
	got, err := mem.GetRun(ctx, run.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !got.CancelRequested {
		t.Fatal("UpdateRun cleared cancel_requested")
	}
	if got.Phase != store.PhaseToolExecuting {
		t.Fatalf("phase=%s", got.Phase)
	}
}

func TestStopDuringModelCallAbortsAndDoesNotComplete(t *testing.T) {
	orig := cancelWatchInterval
	cancelWatchInterval = time.Millisecond
	t.Cleanup(func() { cancelWatchInterval = orig })

	mem := store.NewMemoryStore()
	ctx := context.Background()
	var engine *Engine
	var run store.Run
	engine, run = newCancelTestEngine(t, mem, scriptLLM{
		complete: func(ctx context.Context, _ llm.Request) (llm.Result, error) {
			if err := mem.RequestCancel(ctx, run.UserID, run.ID); err != nil {
				return llm.Result{}, err
			}
			return llm.Result{Text: "should not publish"}, nil
		},
	})

	engine.Execute(ctx, run, false)
	assertCancelled(t, mem, run.ID)
}

func TestStopCancelsInFlightModelContext(t *testing.T) {
	orig := cancelWatchInterval
	cancelWatchInterval = time.Millisecond
	t.Cleanup(func() { cancelWatchInterval = orig })

	mem := store.NewMemoryStore()
	entered := make(chan struct{})
	engine, run := newCancelTestEngine(t, mem, scriptLLM{
		complete: func(ctx context.Context, _ llm.Request) (llm.Result, error) {
			close(entered)
			<-ctx.Done()
			return llm.Result{}, ctx.Err()
		},
	})

	done := make(chan struct{})
	go func() {
		defer close(done)
		engine.Execute(context.Background(), run, false)
	}()

	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("model call did not start")
	}
	if err := mem.RequestCancel(context.Background(), run.UserID, run.ID); err != nil {
		t.Fatal(err)
	}
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("execute did not observe stop")
	}
	assertCancelled(t, mem, run.ID)
}

func TestStopAfterToolCallDoesNotMarkRunDone(t *testing.T) {
	orig := cancelWatchInterval
	cancelWatchInterval = time.Millisecond
	t.Cleanup(func() { cancelWatchInterval = orig })

	mem := store.NewMemoryStore()
	ctx := context.Background()
	var engine *Engine
	var run store.Run
	engine, run = newCancelTestEngine(t, mem, scriptLLM{
		complete: func(ctx context.Context, _ llm.Request) (llm.Result, error) {
			if err := mem.RequestCancel(ctx, run.UserID, run.ID); err != nil {
				return llm.Result{}, err
			}
			return llm.Result{ToolCalls: []llm.ToolCall{{
				ID: "c1", Name: tool.SearchPosts, Arguments: `{"keyword":"go"}`,
			}}}, nil
		},
	})

	engine.Execute(ctx, run, false)
	assertCancelled(t, mem, run.ID)
}

func TestWatchStickyCancellationWinsFailureAndDoesNotConsumeRetry(t *testing.T) {
	ctx := context.Background()
	mem := store.NewMemoryStore()
	watchStore, bucket, _ := watchFixture(t, mem)
	now := store.NowMs()
	if err := scheduleBucket(ctx, mem, nil, watchStore, nil, allowAllWatchPosts, bucket, now); err != nil {
		t.Fatal(err)
	}
	scheduled, err := mem.GetBucket(ctx, bucket.ID)
	if err != nil || scheduled == nil {
		t.Fatalf("scheduled bucket=%+v err=%v", scheduled, err)
	}
	oldRun, err := mem.Claim(ctx, "watch-worker", now, 60_000)
	if err != nil || oldRun == nil || oldRun.ID != scheduled.RunID {
		t.Fatalf("claim old run=%+v err=%v", oldRun, err)
	}
	if err := mem.RequestCancel(ctx, oldRun.UserID, oldRun.ID); err != nil {
		t.Fatal(err)
	}
	if err := mem.ResetBucket(ctx, bucket.ID, oldRun.ID); err != nil {
		t.Fatal(err)
	}
	if err := scheduleBucket(ctx, mem, nil, watchStore, nil, allowAllWatchPosts, bucket, now+1); err != nil {
		t.Fatal(err)
	}
	replacementBucket, err := mem.GetBucket(ctx, bucket.ID)
	if err != nil || replacementBucket == nil || replacementBucket.RunID == 0 || replacementBucket.RunID == oldRun.ID {
		t.Fatalf("replacement bucket=%+v err=%v", replacementBucket, err)
	}

	engine := &Engine{Store: mem}
	if err := engine.fail(ctx, *oldRun, "RUN_FAILED", "simulated failure"); err != nil {
		t.Fatal(err)
	}
	freshOld, err := mem.GetRun(ctx, oldRun.ID)
	if err != nil || freshOld.Status != store.StatusCancelled || freshOld.ErrorCode != "CANCELLED" {
		t.Fatalf("old run=%+v err=%v", freshOld, err)
	}
	freshBucket, err := mem.GetBucket(ctx, bucket.ID)
	if err != nil || freshBucket.Status != "scheduled" || freshBucket.RunID != replacementBucket.RunID {
		t.Fatalf("old failure mutated replacement bucket=%+v err=%v", freshBucket, err)
	}

	replacement, err := mem.Claim(ctx, "watch-worker", now+2, 60_000)
	if err != nil || replacement == nil || replacement.ID != replacementBucket.RunID {
		t.Fatalf("claim replacement=%+v err=%v", replacement, err)
	}
	failureStartedAt := store.NowMs()
	if err := engine.fail(ctx, *replacement, "RUN_FAILED", "simulated failure"); err != nil {
		t.Fatal(err)
	}
	failedBucket, err := mem.GetBucket(ctx, bucket.ID)
	if err != nil || failedBucket.Status != "deferred" {
		t.Fatalf("failed replacement bucket=%+v err=%v", failedBucket, err)
	}
	minRetryAt := failureStartedAt + time.Minute.Milliseconds()
	maxRetryAt := store.NowMs() + time.Minute.Milliseconds()
	if failedBucket.NotBeforeMs < minRetryAt || failedBucket.NotBeforeMs > maxRetryAt {
		t.Fatalf("first error retry=%d want [%d,%d]", failedBucket.NotBeforeMs, minRetryAt, maxRetryAt)
	}
}

func TestWatchStickyCancellationBlocksLegacyCompletion(t *testing.T) {
	ctx := context.Background()
	mem := store.NewMemoryStore()
	watchStore, bucket, _ := watchFixture(t, mem)
	now := store.NowMs()
	if err := scheduleBucket(ctx, mem, nil, watchStore, nil, allowAllWatchPosts, bucket, now); err != nil {
		t.Fatal(err)
	}
	scheduled, err := mem.GetBucket(ctx, bucket.ID)
	if err != nil || scheduled == nil {
		t.Fatalf("scheduled bucket=%+v err=%v", scheduled, err)
	}
	run, err := mem.Claim(ctx, "watch-worker", now, 60_000)
	if err != nil || run == nil || run.ID != scheduled.RunID {
		t.Fatalf("claim run=%+v err=%v", run, err)
	}
	if err := mem.RequestCancel(ctx, run.UserID, run.ID); err != nil {
		t.Fatal(err)
	}

	engine := &Engine{Store: mem, Watch: watchStore, WatchPosts: allowAllWatchPosts}
	err = engine.completeWatchWithStream(ctx, *run, "must not publish", nil, true, "stream-stale")
	if !errors.Is(err, errRunCancelled) {
		t.Fatalf("complete error=%v want cancellation", err)
	}
	if err := engine.cancel(ctx, *run); err != nil {
		t.Fatal(err)
	}
	assertCancelled(t, mem, run.ID)

	freshBucket, err := mem.GetBucket(ctx, bucket.ID)
	if err != nil || freshBucket.Status != "pending" || freshBucket.RunID != 0 {
		t.Fatalf("cancelled bucket=%+v err=%v", freshBucket, err)
	}
	messages, err := mem.ListSessionMessages(ctx, run.UserID, run.SessionID, true)
	if err != nil {
		t.Fatal(err)
	}
	for _, message := range messages {
		if message.RunID == run.ID && message.Role == store.RoleAssistant {
			t.Fatalf("cancelled watch published message=%+v", message)
		}
	}
	outbox, err := mem.ListUnpublishedOutbox(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(outbox) != 0 {
		t.Fatalf("cancelled watch published outbox=%+v", outbox)
	}
	thread, err := mem.GetThread(ctx, run.UserID)
	if err != nil || thread.UnreadCount != 0 {
		t.Fatalf("cancelled watch unread=%d err=%v", thread.UnreadCount, err)
	}
}

func TestWatchStickyCancellationBlocksDismissal(t *testing.T) {
	ctx := context.Background()
	mem := store.NewMemoryStore()
	watchStore, bucket, _ := watchFixture(t, mem)
	now := store.NowMs()
	if err := scheduleBucket(ctx, mem, nil, watchStore, nil, allowAllWatchPosts, bucket, now); err != nil {
		t.Fatal(err)
	}
	scheduled, err := mem.GetBucket(ctx, bucket.ID)
	if err != nil || scheduled == nil {
		t.Fatalf("scheduled bucket=%+v err=%v", scheduled, err)
	}
	run, err := mem.Claim(ctx, "watch-worker", now, 60_000)
	if err != nil || run == nil || run.ID != scheduled.RunID {
		t.Fatalf("claim run=%+v err=%v", run, err)
	}
	if err := mem.RequestCancel(ctx, run.UserID, run.ID); err != nil {
		t.Fatal(err)
	}

	engine := &Engine{Store: mem}
	err = engine.dismissWatchRun(ctx, *run)
	if !errors.Is(err, errRunCancelled) {
		t.Fatalf("dismiss error=%v want cancellation", err)
	}
	if err := engine.cancel(ctx, *run); err != nil {
		t.Fatal(err)
	}
	assertCancelled(t, mem, run.ID)
	freshBucket, err := mem.GetBucket(ctx, bucket.ID)
	if err != nil || freshBucket.Status != "pending" || freshBucket.RunID != 0 {
		t.Fatalf("cancelled bucket=%+v err=%v", freshBucket, err)
	}
}

func TestMemoryReviewStickyCancellationPreservesUndoNotification(t *testing.T) {
	ctx := context.Background()
	mem := store.NewMemoryStore()
	memoryStore := memory.NewMapStore()
	_, changeID, err := memoryStore.Add(ctx, 71, memory.TargetMemory, "偏好短答案", "review-cancelled", store.NowMs())
	if err != nil {
		t.Fatal(err)
	}
	session, err := mem.CreateSession(ctx, store.Session{UserID: 71})
	if err != nil {
		t.Fatal(err)
	}
	if err := mem.SaveThread(ctx, store.Thread{UserID: 71, SessionID: session.ID}); err != nil {
		t.Fatal(err)
	}
	run, err := mem.InsertRun(ctx, store.Run{
		UserID: 71, SessionID: session.ID, RequestID: "review-cancelled", Source: store.SourceMemoryReview,
		Status: store.StatusQueued, Phase: store.PhaseQueued, ConsentVersion: 2, InputVersion: 1,
		CreatedAtMs: store.NowMs(),
	})
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := mem.Claim(ctx, "review-worker", store.NowMs(), 60_000)
	if err != nil || claimed == nil || claimed.ID != run.ID {
		t.Fatalf("claim run=%+v err=%v", claimed, err)
	}
	if _, err := mem.InsertToolCall(ctx, store.ToolCall{
		RunID: claimed.ID, CallID: "memory-change", Tool: tool.AddMemory,
		Status: "success", ResultJSON: encodeToolResultJSONWithChanges("memory updated", nil, []int64{changeID}),
	}); err != nil {
		t.Fatal(err)
	}
	if err := mem.RequestCancel(ctx, claimed.UserID, claimed.ID); err != nil {
		t.Fatal(err)
	}

	engine := &Engine{Store: mem, Memory: memoryStore}
	if err := engine.completeMemoryReview(ctx, *claimed); err != nil {
		t.Fatal(err)
	}
	assertCancelled(t, mem, claimed.ID)
	messages, err := mem.ListSessionMessages(ctx, claimed.UserID, claimed.SessionID, true)
	if err != nil {
		t.Fatal(err)
	}
	changed := 0
	for _, message := range messages {
		if message.RunID == claimed.ID && message.Kind == store.KindMemoryChanged {
			changed++
			if message.ChangeID != changeID || message.Unread {
				t.Fatalf("memory changed message=%+v", message)
			}
		}
	}
	if changed != 1 {
		t.Fatalf("memory changed messages=%d all=%+v", changed, messages)
	}
	if _, err := memoryStore.Undo(ctx, claimed.UserID, changeID, store.NowMs()); err != nil {
		t.Fatalf("undo change %d: %v", changeID, err)
	}
}

func TestMemoryReviewCancellationClosesPendingToolCallsAndJournals(t *testing.T) {
	ctx := context.Background()
	mem := store.NewMemoryStore()
	session, err := mem.CreateSession(ctx, store.Session{UserID: 72})
	if err != nil {
		t.Fatal(err)
	}
	run, err := mem.InsertRun(ctx, store.Run{
		UserID: 72, SessionID: session.ID, RequestID: "review-pending", Source: store.SourceMemoryReview,
		Status: store.StatusQueued, Phase: store.PhaseQueued, ConsentVersion: 2, InputVersion: 1,
		CreatedAtMs: store.NowMs(),
	})
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := mem.Claim(ctx, "review-worker", store.NowMs(), 60_000)
	if err != nil || claimed == nil || claimed.ID != run.ID {
		t.Fatalf("claim run=%+v err=%v", claimed, err)
	}

	for _, call := range []store.ToolCall{
		{RunID: claimed.ID, CallID: "read-pending", Tool: tool.GetMemory, Status: "running"},
		{RunID: claimed.ID, CallID: "write-pending", Tool: tool.AddMemory, CanonicalArgsDigest: "pending-digest", Status: "running"},
		{RunID: claimed.ID, CallID: "write-done", Tool: tool.AddMemory, CanonicalArgsDigest: "done-digest", Status: "success", ResultJSON: `{"ok":true}`},
	} {
		if _, err := mem.InsertToolCall(ctx, call); err != nil {
			t.Fatal(err)
		}
	}
	pendingJournal, reserved, err := mem.ReserveJournal(ctx, store.Journal{
		UserID: claimed.UserID, RequestID: claimed.RequestID, Tool: tool.AddMemory,
		CanonicalArgsDigest: "pending-digest", RunID: claimed.ID, LeaseGeneration: claimed.LeaseGeneration,
		Status: store.JournalPending, CreatedAtMs: store.NowMs(), UpdatedAtMs: store.NowMs(),
	})
	if err != nil || !reserved || pendingJournal == nil {
		t.Fatalf("reserve pending journal=%+v reserved=%v err=%v", pendingJournal, reserved, err)
	}
	doneJournal, reserved, err := mem.ReserveJournal(ctx, store.Journal{
		UserID: claimed.UserID, RequestID: claimed.RequestID, Tool: tool.AddMemory,
		CanonicalArgsDigest: "done-digest", RunID: claimed.ID, LeaseGeneration: claimed.LeaseGeneration,
		Status: store.JournalPending, CreatedAtMs: store.NowMs(), UpdatedAtMs: store.NowMs(),
	})
	if err != nil || !reserved || doneJournal == nil {
		t.Fatalf("reserve done journal=%+v reserved=%v err=%v", doneJournal, reserved, err)
	}
	if err := mem.CompleteJournal(ctx, doneJournal.ID, store.JournalSuccess, `{"ok":true}`); err != nil {
		t.Fatal(err)
	}
	if err := mem.RequestCancel(ctx, claimed.UserID, claimed.ID); err != nil {
		t.Fatal(err)
	}
	if err := (&Engine{Store: mem}).cancel(ctx, *claimed); err != nil {
		t.Fatal(err)
	}
	assertCancelled(t, mem, claimed.ID)

	for _, id := range []string{"read-pending", "write-pending"} {
		call, err := mem.GetToolCall(ctx, claimed.ID, id)
		if err != nil || call == nil || call.Status != store.StatusCancelled || !strings.Contains(call.ResultJSON, `"ok":false`) {
			t.Fatalf("pending call %q=%+v err=%v", id, call, err)
		}
	}
	completedCall, err := mem.GetToolCall(ctx, claimed.ID, "write-done")
	if err != nil || completedCall == nil || completedCall.Status != "success" || completedCall.ResultJSON != `{"ok":true}` {
		t.Fatalf("completed call=%+v err=%v", completedCall, err)
	}
	pendingJournal, err = mem.GetJournal(ctx, claimed.UserID, claimed.RequestID, tool.AddMemory, "pending-digest")
	if err != nil || pendingJournal == nil || pendingJournal.Status != store.JournalError || !strings.Contains(pendingJournal.ResultJSON, `"ok":false`) {
		t.Fatalf("pending journal=%+v err=%v", pendingJournal, err)
	}
	doneJournal, err = mem.GetJournal(ctx, claimed.UserID, claimed.RequestID, tool.AddMemory, "done-digest")
	if err != nil || doneJournal == nil || doneJournal.Status != store.JournalSuccess || doneJournal.ResultJSON != `{"ok":true}` {
		t.Fatalf("done journal=%+v err=%v", doneJournal, err)
	}
	messages, err := mem.ListSessionMessages(ctx, claimed.UserID, claimed.SessionID, true)
	if err != nil {
		t.Fatal(err)
	}
	for _, message := range messages {
		if message.RunID == claimed.ID && message.Kind == store.KindTool {
			t.Fatalf("memory review leaked tool message=%+v", message)
		}
	}
}

func TestPublishAnswerCancellationSkipsSuccessCommitAndClosesToolCall(t *testing.T) {
	ctx := context.Background()
	mem := store.NewMemoryStore()
	engine, run := newCancelTestEngine(t, mem, scriptLLM{complete: func(context.Context, llm.Request) (llm.Result, error) {
		return llm.Result{}, nil
	}})
	call := llm.ToolCall{ID: "publish-cancelled", Name: tool.PublishAnswer, Arguments: `{}`}
	turn := prompt.Turn{Role: store.RoleAssistant, ToolCalls: []prompt.ToolCall{{ID: call.ID, Name: call.Name, Arguments: call.Arguments}}}
	if err := engine.recordModelToolStep(ctx, run, turn, nil); err != nil {
		t.Fatal(err)
	}
	if _, _, err := engine.startToolStep(ctx, run, call, "publish-digest", false); err != nil {
		t.Fatal(err)
	}
	if err := mem.RequestCancel(ctx, run.UserID, run.ID); err != nil {
		t.Fatal(err)
	}
	answer := store.AnswerPresentation{
		Version: 1,
		RunID:   run.ID,
		Blocks: []store.AnswerBlock{{
			ID: "limitation", Kind: "limitation", Text: "must not publish",
		}},
	}
	if err := engine.publishAnswer(ctx, &run, call, answer); !errors.Is(err, errRunTerminated) {
		t.Fatalf("publish error=%v", err)
	}
	assertCancelled(t, mem, run.ID)

	recorded, err := mem.GetToolCall(ctx, run.ID, call.ID)
	if err != nil || recorded == nil || recorded.Status != store.StatusCancelled || !strings.Contains(recorded.ResultJSON, `"ok":false`) {
		t.Fatalf("tool call=%+v err=%v", recorded, err)
	}
	messages, err := mem.ListSessionMessages(ctx, run.UserID, run.SessionID, true)
	if err != nil {
		t.Fatal(err)
	}
	hiddenResult := 0
	for _, message := range messages {
		if message.RunID != run.ID {
			continue
		}
		if message.Visible && message.Role == store.RoleAssistant {
			t.Fatalf("cancelled publish exposed answer=%+v", message)
		}
		if message.Role == store.RoleTool {
			hiddenResult++
			if message.Visible {
				t.Fatalf("terminal tool result is visible=%+v", message)
			}
		}
		presentation, err := mem.GetPresentation(ctx, message.ID)
		if err != nil || presentation != nil {
			t.Fatalf("cancelled publish presentation=%+v err=%v", presentation, err)
		}
	}
	if hiddenResult != 1 {
		t.Fatalf("hidden terminal results=%d messages=%+v", hiddenResult, messages)
	}
	events, err := mem.ListEventsAfter(ctx, run.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	for _, event := range events {
		if event.Type == store.EventAnswerCommitted || event.Type == store.EventDone || event.Type == store.EventToolResult {
			t.Fatalf("cancelled publish emitted %s", event.Type)
		}
	}
}

func newCancelTestEngine(t *testing.T, mem *store.MemoryStore, model llm.Client) (*Engine, store.Run) {
	t.Helper()
	ctx := context.Background()
	session, err := mem.CreateSession(ctx, store.Session{UserID: 1})
	if err != nil {
		t.Fatal(err)
	}
	if err := mem.SaveThread(ctx, store.Thread{UserID: 1, SessionID: session.ID, ActiveRunID: 0}); err != nil {
		t.Fatal(err)
	}
	run, err := mem.InsertRun(ctx, store.Run{
		UserID: 1, SessionID: session.ID, RequestID: "r1", Source: store.SourceUser,
		Status: store.StatusQueued, Phase: store.PhaseQueued, ConsentVersion: 2, InputVersion: 1,
		CreatedAtMs: store.NowMs(),
	})
	if err != nil {
		t.Fatal(err)
	}
	claimed, err := mem.Claim(ctx, "test-worker", store.NowMs(), 60_000)
	if err != nil || claimed == nil || claimed.ID != run.ID {
		t.Fatalf("claim run: %+v %v", claimed, err)
	}
	run = *claimed
	thread, _ := mem.GetThread(ctx, 1)
	thread.ActiveRunID = run.ID
	_ = mem.SaveThread(ctx, *thread)
	reg, err := tool.NewRegistry(tool.Clients{Store: mem}, []string{tool.SearchPosts, tool.PresentSources})
	if err != nil {
		t.Fatal(err)
	}
	return &Engine{
		Store: mem, Tools: reg, LLM: model, Notify: store.NewMemoryNotifier(), Window: 128000,
	}, run
}

func assertCancelled(t *testing.T, mem *store.MemoryStore, runID int64) {
	t.Helper()
	got, err := mem.GetRun(context.Background(), runID)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != store.StatusCancelled {
		t.Fatalf("status=%s want cancelled", got.Status)
	}
	if got.ErrorCode != "CANCELLED" {
		t.Fatalf("error=%s", got.ErrorCode)
	}
	events, err := mem.ListEventsAfter(context.Background(), runID, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) == 0 {
		t.Fatal("expected cancel event")
	}
	last := events[len(events)-1]
	if last.Type != store.EventError {
		t.Fatalf("last event=%s", last.Type)
	}
	var payload store.EventPayload
	if err := json.Unmarshal(last.PayloadJSON, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.ErrorCode != "CANCELLED" {
		t.Fatalf("payload=%+v", payload)
	}
	for _, ev := range events {
		if ev.Type == store.EventDone || ev.Type == store.EventToken {
			t.Fatalf("cancelled run still emitted %s", ev.Type)
		}
	}
	thread, err := mem.GetThread(context.Background(), got.UserID)
	if err != nil {
		t.Fatal(err)
	}
	if thread.ActiveRunID != 0 {
		t.Fatalf("active run still %d", thread.ActiveRunID)
	}
}
