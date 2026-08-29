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
	ok, err := mgr.Store.RenewLease(context.Background(), got.ID, "w1", now+120_000, now)
	if err != nil || !ok {
		t.Fatalf("renew: %v %v", ok, err)
	}
	ok, err = mgr.Store.RenewLease(context.Background(), got.ID, "other", now+120_000, now)
	if err != nil || ok {
		t.Fatalf("cas renew should fail for other owner: %v %v", ok, err)
	}
}
