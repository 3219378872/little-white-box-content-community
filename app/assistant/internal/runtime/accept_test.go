package runtime

import (
	"context"
	"testing"

	"esx/app/assistant/internal/store"
	"esx/pkg/errx"
)

func TestAcceptDispositionAndFIFO(t *testing.T) {
	mem := store.NewMemoryStore()
	a := &Acceptor{Store: mem, Notify: store.NewMemoryNotifier(), MaxRunes: 2000}
	ctx := context.Background()
	first, err := a.Accept(ctx, AcceptInput{UserID: 1, Message: "hello", RequestID: "r1", ConsentOK: true})
	if err != nil || first.Disposition != store.DispositionStarted {
		t.Fatalf("started: %+v err=%v", first, err)
	}
	run, _ := mem.GetRun(ctx, first.RunID)
	run.Status = store.StatusRunning
	run.Phase = store.PhaseModelRequest
	_ = mem.UpdateRun(ctx, *run)
	thread, _ := mem.GetThread(ctx, 1)
	thread.ActiveRunID = run.ID
	_ = mem.SaveThread(ctx, *thread)

	second, err := a.Accept(ctx, AcceptInput{UserID: 1, Message: "redirect", RequestID: "r2", ConsentOK: true})
	if err != nil || second.Disposition != store.DispositionRedirected {
		t.Fatalf("redirect: %+v err=%v", second, err)
	}
	run.Phase = store.PhaseToolExecuting
	_ = mem.UpdateRun(ctx, *run)
	third, err := a.Accept(ctx, AcceptInput{UserID: 1, Message: "steer", RequestID: "r3", ConsentOK: true})
	if err != nil || third.Disposition != store.DispositionSteered {
		t.Fatalf("steer: %+v err=%v", third, err)
	}
	run.Phase = store.PhaseCompact
	_ = mem.UpdateRun(ctx, *run)
	for i := 0; i < store.MaxInputQueue; i++ {
		got, err := a.Accept(ctx, AcceptInput{UserID: 1, Message: "queued", RequestID: "q" + itoa(int64(i)), ConsentOK: true})
		if err != nil || got.Disposition != store.DispositionQueued {
			t.Fatalf("queue %d: %+v err=%v", i, got, err)
		}
	}
	if _, err := a.Accept(ctx, AcceptInput{UserID: 1, Message: "overflow", RequestID: "full", ConsentOK: true}); !errx.Is(err, errx.AgentQueueFull) {
		t.Fatalf("want queue full, got %v", err)
	}
	if _, err := a.Accept(ctx, AcceptInput{UserID: 1, Message: "nope", RequestID: "x", ConsentOK: false}); !errx.Is(err, errx.AgentNotAuthorized) {
		t.Fatalf("consent: %v", err)
	}
}
