package runtime

import (
	"context"
	"errors"
	"testing"
	"time"

	"esx/app/assistant/internal/llm"
	"esx/app/assistant/internal/memory"
	"esx/app/assistant/internal/store"
)

func TestExecutionKeepsClaimFenceAfterTakeover(t *testing.T) {
	ctx := context.Background()
	mem := store.NewMemoryStore()
	modelCalls := 0
	engine, first := newCancelTestEngine(t, mem, scriptLLM{complete: func(context.Context, llm.Request) (llm.Result, error) {
		modelCalls++
		return llm.Result{Text: "stale answer"}, nil
	}})
	execution, err := engine.prepareExecution(ctx, first)
	if err != nil {
		t.Fatal(err)
	}
	mem.ExpireLease(first.ID, store.NowMs()-1)
	second, err := mem.Claim(ctx, "replacement-worker", store.NowMs(), 60_000)
	if err != nil || second == nil {
		t.Fatalf("claim=%+v err=%v", second, err)
	}
	if _, err := execution.iterate(ctx, ctx); !errors.Is(err, store.ErrLeaseLost) {
		t.Fatalf("stale iteration error=%v", err)
	}
	fresh, _ := mem.GetRun(ctx, first.ID)
	if modelCalls != 0 || fresh.Status != store.StatusRunning || execution.run.Fence() != first.Fence() || fresh.Fence() != second.Fence() {
		t.Fatalf("calls=%d local=%+v fresh=%+v", modelCalls, execution.run, fresh)
	}
}

type takeoverMemory struct {
	memory.Store
	active func() ([]memory.Entry, error)
}

func (m takeoverMemory) Active(context.Context, int64) ([]memory.Entry, error) {
	return m.active()
}

func TestExecuteErrorFinalizersDoNotAdoptReplacementFence(t *testing.T) {
	for _, problem := range []error{errRunCancelled, errors.New("snapshot storage unavailable")} {
		t.Run(problem.Error(), func(t *testing.T) {
			ctx := context.Background()
			mem := store.NewMemoryStore()
			engine, first := newCancelTestEngine(t, mem, &scriptedLLM{})
			engine.Memory = takeoverMemory{active: func() ([]memory.Entry, error) {
				mem.ExpireLease(first.ID, store.NowMs()-1)
				if _, err := mem.Claim(ctx, "replacement-worker", store.NowMs(), 60_000); err != nil {
					t.Fatal(err)
				}
				return nil, problem
			}}
			engine.Execute(ctx, first, false)
			fresh, _ := mem.GetRun(ctx, first.ID)
			events, _ := mem.ListEventsAfter(ctx, first.ID, 0)
			if fresh.Status != store.StatusRunning || fresh.CancelRequested || fresh.LeaseGeneration != first.LeaseGeneration+1 || len(events) != 0 {
				t.Fatalf("old finalizer changed replacement: run=%+v events=%+v", fresh, events)
			}
		})
	}
}

func TestCancelWatcherStopsOnlyStaleWorkAfterTakeover(t *testing.T) {
	ctx := context.Background()
	mem := store.NewMemoryStore()
	_, run := newCancelTestEngine(t, mem, &scriptedLLM{})
	workCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	stop := watchCancel(ctx, mem, run, cancel)
	defer stop()
	mem.ExpireLease(run.ID, store.NowMs()-1)
	if _, err := mem.Claim(ctx, "replacement-worker", store.NowMs(), 60_000); err != nil {
		t.Fatal(err)
	}
	select {
	case <-workCtx.Done():
	case <-time.After(2 * time.Second):
		t.Fatal("stale work was not cancelled")
	}
	fresh, _ := mem.GetRun(ctx, run.ID)
	if fresh.CancelRequested || fresh.Status != store.StatusRunning {
		t.Fatalf("replacement was cancelled: %+v", fresh)
	}
}
