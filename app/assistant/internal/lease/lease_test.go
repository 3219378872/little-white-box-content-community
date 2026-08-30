package lease

import (
	"context"
	"testing"
	"time"

	"esx/app/assistant/internal/store"
)

func TestClaimPrefersUserThenExpiredLease(t *testing.T) {
	mem := store.NewMemoryStore()
	now := store.NowMs()
	watch, err := mem.InsertRun(context.Background(), store.Run{
		UserID: 1, SessionID: 1, RequestID: "w", Source: store.SourceWatch, Status: store.StatusQueued,
		Phase: store.PhaseQueued, Priority: store.PriorityWatch, CreatedAtMs: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	user, err := mem.InsertRun(context.Background(), store.Run{
		UserID: 1, SessionID: 1, RequestID: "u", Source: store.SourceUser, Status: store.StatusQueued,
		Phase: store.PhaseQueued, Priority: store.PriorityUser, CreatedAtMs: now + 10,
	})
	if err != nil {
		t.Fatal(err)
	}
	mgr := Manager{Store: mem, Owner: "w1", Lease: time.Minute}
	got, recovered, err := mgr.Claim(context.Background())
	if err != nil || recovered || got == nil || got.ID != user.ID {
		t.Fatalf("want user run, got %+v recovered=%v err=%v watch=%d", got, recovered, err, watch.ID)
	}
	if got.LeaseOwner != "w1" || got.Status != store.StatusRunning {
		t.Fatalf("lease not assigned: %+v", got)
	}
	ok, err := mgr.Store.RenewLease(context.Background(), got.ID, "w1", got.LeaseGeneration, now+120_000, now)
	if err != nil || !ok {
		t.Fatalf("renew: %v %v", ok, err)
	}
	ok, err = mgr.Store.RenewLease(context.Background(), got.ID, "other", got.LeaseGeneration, now+120_000, now)
	if err != nil || ok {
		t.Fatalf("cas renew should fail for other owner: %v %v", ok, err)
	}
}

func TestRenewLoopCancelsWorkImmediatelyAfterLeaseTakeover(t *testing.T) {
	mem := store.NewMemoryStore()
	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	_, err := mem.InsertRun(ctx, store.Run{
		UserID: 1, SessionID: 1, RequestID: "takeover", Source: store.SourceUser,
		Status: store.StatusQueued, Phase: store.PhaseQueued, CreatedAtMs: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	firstManager := Manager{Store: mem, Owner: "worker-one", Lease: time.Millisecond, Renew: time.Millisecond}
	first, _, err := firstManager.Claim(ctx)
	if err != nil || first == nil {
		t.Fatalf("first claim: %+v err=%v", first, err)
	}
	mem.ExpireLease(first.ID, store.NowMs()-1)
	secondManager := Manager{Store: mem, Owner: "worker-two", Lease: time.Minute}
	second, _, err := secondManager.Claim(ctx)
	if err != nil || second == nil || second.LeaseGeneration <= first.LeaseGeneration {
		t.Fatalf("takeover: first=%+v second=%+v err=%v", first, second, err)
	}
	lost := make(chan struct{})
	go firstManager.RenewLoop(ctx, *first, func() { close(lost) })
	select {
	case <-lost:
	case <-time.After(250 * time.Millisecond):
		t.Fatal("lease takeover did not cancel in-flight work")
	}
}

func TestNewOwnerIsProcessUnique(t *testing.T) {
	first := NewOwner("assistant-agent")
	second := NewOwner("assistant-agent")
	if first == second || len(first) == 0 || len(first) > 64 || len(second) > 64 {
		t.Fatalf("owners must be unique and bounded: %q %q", first, second)
	}
}
