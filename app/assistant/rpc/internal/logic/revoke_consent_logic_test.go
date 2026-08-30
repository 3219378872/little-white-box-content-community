package logic

import (
	"context"
	"testing"

	"esx/app/assistant/internal/store"
	"esx/app/assistant/rpc/internal/svc"
	"esx/app/assistant/rpc/xiaobaihe/assistant/pb"
)

func TestRevokeConsentCancelsAllRunsAndResetsScheduledBucket(t *testing.T) {
	ctx := context.Background()
	mem := store.NewMemoryStore()
	userRun, _ := mem.InsertRun(ctx, store.Run{UserID: 5, Status: store.StatusQueued, Source: store.SourceUser})
	watchRun, _ := mem.InsertRun(ctx, store.Run{UserID: 5, Status: store.StatusRunning, Source: store.SourceWatch})
	bucket, _ := mem.UpsertDeliveryBucket(ctx, 5, 10, 1, 1)
	if err := mem.MarkBucketScheduled(ctx, bucket.ID, watchRun.ID); err != nil {
		t.Fatal(err)
	}
	logic := NewRevokeConsentLogic(ctx, &svc.ServiceContext{Store: mem})
	if _, err := logic.RevokeConsent(&pb.RevokeConsentReq{UserId: 5}); err != nil {
		t.Fatal(err)
	}
	for _, id := range []int64{userRun.ID, watchRun.ID} {
		run, err := mem.GetRun(ctx, id)
		if err != nil || !run.CancelRequested {
			t.Fatalf("run=%+v err=%v", run, err)
		}
	}
	fresh, err := mem.GetBucket(ctx, bucket.ID)
	if err != nil || fresh.Status != "pending" || fresh.RunID != 0 {
		t.Fatalf("bucket=%+v err=%v", fresh, err)
	}
}
