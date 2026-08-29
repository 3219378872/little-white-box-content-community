package runtime

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"esx/app/assistant/internal/llm"
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
		Status: store.StatusRunning, Phase: store.PhaseQueued, CreatedAtMs: store.NowMs(),
	})
	if err != nil {
		t.Fatal(err)
	}
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
