package logic

import (
	"context"
	"testing"

	"esx/app/assistant/internal/store"
	"esx/app/assistant/rpc/internal/svc"
	pb "esx/kitex_gen/assistant"
)

func TestRevokeConsentCancelsAllRuns(t *testing.T) {
	ctx := context.Background()
	mem := store.NewMemoryStore()
	userRun, _ := mem.InsertRun(ctx, store.Run{UserID: 5, Status: store.StatusQueued, Source: store.SourceUser})
	reviewRun, _ := mem.InsertRun(ctx, store.Run{UserID: 5, Status: store.StatusRunning, Source: store.SourceMemoryReview})
	logic := NewRevokeConsentLogic(ctx, &svc.ServiceContext{Store: mem})
	if _, err := logic.RevokeConsent(&pb.RevokeConsentReq{UserId: 5}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []int64{userRun.ID, reviewRun.ID} {
		run, err := mem.GetRun(ctx, id)
		if err != nil || !run.CancelRequested {
			t.Fatalf("run=%+v err=%v", run, err)
		}
	}
}
