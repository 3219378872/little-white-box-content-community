package store

import (
	"context"
	"errors"
	"testing"
)

func TestLeaseGenerationFencesRecoveredWorkerWrites(t *testing.T) {
	ctx := context.Background()
	mem := NewMemoryStore()
	run, err := mem.InsertRun(ctx, Run{
		UserID: 1, SessionID: 1, RequestID: "r1", Source: SourceUser,
		Status: StatusQueued, Phase: PhaseQueued, ConsentVersion: 2, InputVersion: 1, CreatedAtMs: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	first, err := mem.Claim(ctx, "worker-one", NowMs(), 60_000)
	if err != nil || first == nil || first.ID != run.ID {
		t.Fatalf("first claim: %+v err=%v", first, err)
	}
	firstFence := first.Fence()
	mem.ExpireLease(first.ID, NowMs()-1)
	second, err := mem.Claim(ctx, "worker-two", NowMs(), 60_000)
	if err != nil || second == nil {
		t.Fatalf("second claim: %+v err=%v", second, err)
	}
	if second.LeaseGeneration != first.LeaseGeneration+1 {
		t.Fatalf("generation=%d want %d", second.LeaseGeneration, first.LeaseGeneration+1)
	}

	err = mem.RunStep(ctx, firstFence, func(ctx context.Context, tx Store) error {
		_, insertErr := tx.InsertEvent(ctx, run.ID, EventToken, []byte(`{"text":"stale"}`), NowMs())
		return insertErr
	})
	if !errors.Is(err, ErrLeaseLost) {
		t.Fatalf("stale worker write err=%v", err)
	}
	ok, err := mem.RenewLease(ctx, run.ID, firstFence.Owner, firstFence.Generation, NowMs()+60_000, NowMs())
	if err != nil || ok {
		t.Fatalf("stale renew ok=%v err=%v", ok, err)
	}
	if err := mem.RunStep(ctx, second.Fence(), func(ctx context.Context, tx Store) error {
		_, insertErr := tx.InsertEvent(ctx, run.ID, EventToken, []byte(`{"text":"fresh"}`), NowMs())
		return insertErr
	}); err != nil {
		t.Fatal(err)
	}
	events, err := mem.ListEventsAfter(ctx, run.ID, 0)
	if err != nil || len(events) != 1 || string(events[0].PayloadJSON) != `{"text":"fresh"}` {
		t.Fatalf("events=%+v err=%v", events, err)
	}
}

func TestTerminalEventIsUniquePerRun(t *testing.T) {
	ctx := context.Background()
	mem := NewMemoryStore()
	if _, err := mem.InsertEvent(ctx, 9, EventDone, []byte(`{}`), NowMs()); err != nil {
		t.Fatal(err)
	}
	if _, err := mem.InsertEvent(ctx, 9, EventError, []byte(`{}`), NowMs()); err == nil {
		t.Fatal("second terminal event must be rejected")
	}
}
