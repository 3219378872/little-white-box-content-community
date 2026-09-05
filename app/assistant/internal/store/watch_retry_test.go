package store

import (
	"context"
	"encoding/json"
	"strconv"
	"testing"
	"time"
)

func TestMemoryWatchFailureDefersAndReleasesQuota(t *testing.T) {
	ctx := context.Background()
	mem := NewMemoryStore()
	now := time.Date(2026, 9, 5, 9, 0, 0, 0, time.UTC).UnixMilli()
	bucket, err := mem.UpsertDeliveryBucket(ctx, 41, 101, now-2*time.Minute.Milliseconds(), now)
	if err != nil {
		t.Fatal(err)
	}
	dayStart := now / (24 * time.Hour).Milliseconds() * (24 * time.Hour).Milliseconds()
	hourStart := now / time.Hour.Milliseconds() * time.Hour.Milliseconds()
	allowed, _, err := mem.ReserveWatchQuota(ctx, bucket.ID, bucket.UserID, []int64{7}, dayStart, hourStart, 1, 1)
	if err != nil || !allowed {
		t.Fatalf("reserve allowed=%v err=%v", allowed, err)
	}
	payload, err := json.Marshal(map[string]any{"bucket_id": bucket.ID})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := mem.InsertRun(ctx, Run{
		UserID: bucket.UserID, SessionID: 1, RequestID: "watch-stale-before-bucket", Source: SourceWatch,
		Status: StatusError, Phase: PhaseDone, QueuedPayload: payload, CreatedAtMs: bucket.CreatedAtMs - 1,
	}); err != nil {
		t.Fatal(err)
	}
	run := insertMemoryWatchAttempt(t, mem, bucket, StatusError, 1, now)
	if _, err := mem.InsertRun(ctx, Run{
		UserID: bucket.UserID, SessionID: 1, RequestID: "watch-future-run", Source: SourceWatch,
		Status: StatusError, Phase: PhaseDone, QueuedPayload: payload, CreatedAtMs: now + 1,
	}); err != nil {
		t.Fatal(err)
	}
	if err := mem.FinishWatchDelivery(ctx, bucket.ID, bucket.UserID, run.ID, StatusError, now); err != nil {
		t.Fatal(err)
	}

	fresh, err := mem.GetBucket(ctx, bucket.ID)
	wantNotBefore := now + time.Minute.Milliseconds()
	if err != nil || fresh.Status != "deferred" || fresh.RunID != 0 || fresh.NotBeforeMs != wantNotBefore {
		t.Fatalf("bucket=%+v err=%v", fresh, err)
	}
	for _, dueAt := range []int64{now, wantNotBefore - 1} {
		if due, err := mem.ListDueBuckets(ctx, dueAt, 2*time.Minute.Milliseconds()); err != nil || len(due) != 0 {
			t.Fatalf("due at %d: buckets=%+v err=%v", dueAt, due, err)
		}
	}
	if due, err := mem.ListDueBuckets(ctx, wantNotBefore, 2*time.Minute.Milliseconds()); err != nil || len(due) != 1 || due[0].ID != bucket.ID {
		t.Fatalf("due at retry: buckets=%+v err=%v", due, err)
	}

	next, err := mem.UpsertDeliveryBucket(ctx, bucket.UserID, 102, now, now)
	if err != nil {
		t.Fatal(err)
	}
	allowed, _, err = mem.ReserveWatchQuota(ctx, next.ID, next.UserID, []int64{7}, dayStart, hourStart, 1, 1)
	if err != nil || !allowed {
		t.Fatalf("released quota reusable=%v err=%v", allowed, err)
	}
}

func TestMemoryWatchCancellationReturnsPendingImmediately(t *testing.T) {
	ctx := context.Background()
	mem := NewMemoryStore()
	now := time.Date(2026, 9, 5, 9, 0, 0, 0, time.UTC).UnixMilli()
	bucket, err := mem.UpsertDeliveryBucket(ctx, 42, 201, now-2*time.Minute.Milliseconds(), now)
	if err != nil {
		t.Fatal(err)
	}
	run := insertMemoryWatchAttempt(t, mem, bucket, StatusCancelled, 1, now)
	if err := mem.FinishWatchDelivery(ctx, bucket.ID, bucket.UserID, run.ID, StatusCancelled, now); err != nil {
		t.Fatal(err)
	}

	fresh, err := mem.GetBucket(ctx, bucket.ID)
	if err != nil || fresh.Status != "pending" || fresh.RunID != 0 || fresh.NotBeforeMs != 0 {
		t.Fatalf("bucket=%+v err=%v", fresh, err)
	}
	if due, err := mem.ListDueBuckets(ctx, now, 2*time.Minute.Milliseconds()); err != nil || len(due) != 1 || due[0].ID != bucket.ID {
		t.Fatalf("cancelled bucket not immediately due: buckets=%+v err=%v", due, err)
	}
}

func TestMemoryWatchFailureBackoffStopsAfterEightAttempts(t *testing.T) {
	ctx := context.Background()
	mem := NewMemoryStore()
	now := time.Date(2026, 9, 5, 9, 0, 0, 0, time.UTC).UnixMilli()
	bucket, err := mem.UpsertDeliveryBucket(ctx, 43, 301, now-2*time.Minute.Milliseconds(), now)
	if err != nil {
		t.Fatal(err)
	}
	delays := []time.Duration{time.Minute, 2 * time.Minute, 4 * time.Minute, 8 * time.Minute, 16 * time.Minute, 30 * time.Minute, 30 * time.Minute}
	attemptAt := now
	for attempt := 1; attempt <= watchDeliveryMaxAttempts; attempt++ {
		run := insertMemoryWatchAttempt(t, mem, bucket, StatusError, attempt, attemptAt)
		if err := mem.FinishWatchDelivery(ctx, bucket.ID, bucket.UserID, run.ID, StatusError, attemptAt); err != nil {
			t.Fatal(err)
		}
		fresh, err := mem.GetBucket(ctx, bucket.ID)
		if err != nil {
			t.Fatal(err)
		}
		if attempt == watchDeliveryMaxAttempts {
			if fresh.Status != "discarded" || fresh.RunID != 0 || fresh.NotBeforeMs != 0 {
				t.Fatalf("attempt %d bucket=%+v", attempt, fresh)
			}
			break
		}
		wantNotBefore := attemptAt + delays[attempt-1].Milliseconds()
		if fresh.Status != "deferred" || fresh.RunID != 0 || fresh.NotBeforeMs != wantNotBefore {
			t.Fatalf("attempt %d bucket=%+v wantNotBefore=%d", attempt, fresh, wantNotBefore)
		}
		bucket = *fresh
		attemptAt = wantNotBefore
	}
	if due, err := mem.ListDueBuckets(ctx, attemptAt+watchDeliveryRetention.Milliseconds(), 2*time.Minute.Milliseconds()); err != nil || len(due) != 0 {
		t.Fatalf("discarded bucket became due: buckets=%+v err=%v", due, err)
	}
}

func TestMemoryRequeueFailedBucketsUsesTerminalPolicy(t *testing.T) {
	ctx := context.Background()
	mem := NewMemoryStore()
	now := time.Date(2026, 9, 5, 9, 0, 0, 0, time.UTC).UnixMilli()
	errorBucket, err := mem.UpsertDeliveryBucket(ctx, 44, 401, now-2*time.Minute.Milliseconds(), now)
	if err != nil {
		t.Fatal(err)
	}
	cancelledBucket, err := mem.UpsertDeliveryBucket(ctx, 45, 402, now-2*time.Minute.Milliseconds(), now)
	if err != nil {
		t.Fatal(err)
	}
	insertMemoryWatchAttempt(t, mem, errorBucket, StatusError, 1, now)
	insertMemoryWatchAttempt(t, mem, cancelledBucket, StatusCancelled, 1, now)

	if err := mem.RequeueFailedBuckets(ctx, now); err != nil {
		t.Fatal(err)
	}
	freshError, _ := mem.GetBucket(ctx, errorBucket.ID)
	if freshError.Status != "deferred" || freshError.NotBeforeMs != now+time.Minute.Milliseconds() {
		t.Fatalf("error bucket=%+v", freshError)
	}
	freshCancelled, _ := mem.GetBucket(ctx, cancelledBucket.ID)
	if freshCancelled.Status != "pending" || freshCancelled.NotBeforeMs != 0 {
		t.Fatalf("cancelled bucket=%+v", freshCancelled)
	}
}

func TestMemoryRequeueValidatesRunAssociation(t *testing.T) {
	ctx := context.Background()
	mem := NewMemoryStore()
	now := time.Date(2026, 9, 5, 9, 0, 0, 0, time.UTC).UnixMilli()
	bucket, err := mem.UpsertDeliveryBucket(ctx, 49, 801, now-2*time.Minute.Milliseconds(), now)
	if err != nil {
		t.Fatal(err)
	}
	run, err := mem.InsertRun(ctx, Run{
		UserID: bucket.UserID, SessionID: 1, RequestID: "not-a-watch-run",
		Source: SourceUser, Status: StatusCancelled, Phase: PhaseDone, CreatedAtMs: now, EndedAtMs: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := mem.MarkBucketScheduled(ctx, bucket.ID, run.ID); err != nil {
		t.Fatal(err)
	}
	if err := mem.RequeueFailedBuckets(ctx, now); err != nil {
		t.Fatal(err)
	}
	fresh, err := mem.GetBucket(ctx, bucket.ID)
	if err != nil || fresh.Status != "scheduled" || fresh.RunID != run.ID {
		t.Fatalf("invalid association was requeued: bucket=%+v err=%v", fresh, err)
	}

	crossUserBucket, err := mem.UpsertDeliveryBucket(ctx, 50, 802, now-2*time.Minute.Milliseconds(), now)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(map[string]any{"bucket_id": crossUserBucket.ID})
	if err != nil {
		t.Fatal(err)
	}
	crossUserRun, err := mem.InsertRun(ctx, Run{
		UserID: 51, SessionID: 1, RequestID: "cross-user-watch-run", QueuedPayload: payload,
		Source: SourceWatch, Status: StatusCancelled, Phase: PhaseDone, CreatedAtMs: now, EndedAtMs: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := mem.MarkBucketScheduled(ctx, crossUserBucket.ID, crossUserRun.ID); err != nil {
		t.Fatal(err)
	}
	if err := mem.RequeueFailedBuckets(ctx, now); err != nil {
		t.Fatal(err)
	}
	fresh, err = mem.GetBucket(ctx, crossUserBucket.ID)
	if err != nil || fresh.Status != "scheduled" || fresh.RunID != crossUserRun.ID {
		t.Fatalf("cross-user association was requeued: bucket=%+v err=%v", fresh, err)
	}

	wrongPayloadBucket, err := mem.UpsertDeliveryBucket(ctx, 53, 803, now-2*time.Minute.Milliseconds(), now)
	if err != nil {
		t.Fatal(err)
	}
	wrongPayload, err := json.Marshal(map[string]any{"bucket_id": wrongPayloadBucket.ID + 1})
	if err != nil {
		t.Fatal(err)
	}
	wrongPayloadRun, err := mem.InsertRun(ctx, Run{
		UserID: wrongPayloadBucket.UserID, SessionID: 1, RequestID: "watch-wrong-bucket", QueuedPayload: wrongPayload,
		Source: SourceWatch, Status: StatusCancelled, Phase: PhaseDone, CreatedAtMs: now, EndedAtMs: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := mem.MarkBucketScheduled(ctx, wrongPayloadBucket.ID, wrongPayloadRun.ID); err != nil {
		t.Fatal(err)
	}
	if err := mem.RequeueFailedBuckets(ctx, now); err != nil {
		t.Fatal(err)
	}
	fresh, err = mem.GetBucket(ctx, wrongPayloadBucket.ID)
	if err != nil || fresh.Status != "discarded" || fresh.RunID != 0 {
		t.Fatalf("wrong-payload association was requeued: bucket=%+v err=%v", fresh, err)
	}
}

func TestMemoryWatchFailureDoesNotRetryPastRetention(t *testing.T) {
	ctx := context.Background()
	mem := NewMemoryStore()
	now := time.Date(2026, 9, 5, 9, 0, 0, 0, time.UTC).UnixMilli()
	createdAt := now - watchDeliveryRetention.Milliseconds() + 30*time.Second.Milliseconds()
	bucket, err := mem.UpsertDeliveryBucket(ctx, 46, 501, createdAt, createdAt)
	if err != nil {
		t.Fatal(err)
	}
	run := insertMemoryWatchAttempt(t, mem, bucket, StatusError, 1, now)
	if err := mem.FinishWatchDelivery(ctx, bucket.ID, bucket.UserID, run.ID, StatusError, now); err != nil {
		t.Fatal(err)
	}
	fresh, err := mem.GetBucket(ctx, bucket.ID)
	if err != nil || fresh.Status != "discarded" || fresh.NotBeforeMs != 0 {
		t.Fatalf("bucket=%+v err=%v", fresh, err)
	}
}

func TestMemoryWatchFailureWithInvalidBucketPayloadIsDiscarded(t *testing.T) {
	ctx := context.Background()
	mem := NewMemoryStore()
	now := time.Date(2026, 9, 5, 9, 0, 0, 0, time.UTC).UnixMilli()
	bucket, err := mem.UpsertDeliveryBucket(ctx, 47, 601, now-2*time.Minute.Milliseconds(), now)
	if err != nil {
		t.Fatal(err)
	}
	run, err := mem.InsertRun(ctx, Run{
		UserID: bucket.UserID, SessionID: 1, RequestID: "watch-invalid-payload",
		Source: SourceWatch, Status: StatusError, Phase: PhaseDone, QueuedPayload: []byte(`{}`),
		CreatedAtMs: now, EndedAtMs: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := mem.MarkBucketScheduled(ctx, bucket.ID, run.ID); err != nil {
		t.Fatal(err)
	}
	if err := mem.FinishWatchDelivery(ctx, bucket.ID, bucket.UserID, run.ID, StatusError, now); err != nil {
		t.Fatal(err)
	}
	fresh, err := mem.GetBucket(ctx, bucket.ID)
	if err != nil || fresh.Status != "discarded" || fresh.RunID != 0 {
		t.Fatalf("bucket=%+v err=%v", fresh, err)
	}
}

func TestMemoryFinishWatchDeliveryCleansResidualReservation(t *testing.T) {
	ctx := context.Background()
	mem := NewMemoryStore()
	now := time.Date(2026, 9, 5, 9, 0, 0, 0, time.UTC).UnixMilli()
	dayStart := now / (24 * time.Hour).Milliseconds() * (24 * time.Hour).Milliseconds()
	hourStart := now / time.Hour.Milliseconds() * time.Hour.Milliseconds()
	bucket, err := mem.UpsertDeliveryBucket(ctx, 48, 701, now-2*time.Minute.Milliseconds(), now)
	if err != nil {
		t.Fatal(err)
	}
	allowed, _, err := mem.ReserveWatchQuota(ctx, bucket.ID, bucket.UserID, []int64{78}, dayStart, hourStart, 1, 1)
	if err != nil || !allowed {
		t.Fatalf("reserve allowed=%v err=%v", allowed, err)
	}
	if err := mem.DeferBucket(ctx, bucket.ID, now+time.Minute.Milliseconds()); err != nil {
		t.Fatal(err)
	}
	if err := mem.FinishWatchDelivery(ctx, bucket.ID, bucket.UserID, 999, StatusCancelled, now); err != nil {
		t.Fatal(err)
	}

	next, err := mem.UpsertDeliveryBucket(ctx, bucket.UserID, 702, now, now)
	if err != nil {
		t.Fatal(err)
	}
	allowed, _, err = mem.ReserveWatchQuota(ctx, next.ID, next.UserID, []int64{78}, dayStart, hourStart, 1, 1)
	if err != nil || !allowed {
		t.Fatalf("residual reservation was not released: allowed=%v err=%v", allowed, err)
	}
}

func TestMemoryFinishWatchDeliveryIgnoresStaleRescheduledRun(t *testing.T) {
	ctx := context.Background()
	mem := NewMemoryStore()
	now := time.Date(2026, 9, 5, 9, 0, 0, 0, time.UTC).UnixMilli()
	dayStart := now / (24 * time.Hour).Milliseconds() * (24 * time.Hour).Milliseconds()
	hourStart := now / time.Hour.Milliseconds() * time.Hour.Milliseconds()
	bucket, err := mem.UpsertDeliveryBucket(ctx, 52, 901, now-2*time.Minute.Milliseconds(), now)
	if err != nil {
		t.Fatal(err)
	}
	allowed, _, err := mem.ReserveWatchQuota(ctx, bucket.ID, bucket.UserID, []int64{79}, dayStart, hourStart, 1, 1)
	if err != nil || !allowed {
		t.Fatalf("reserve old run allowed=%v err=%v", allowed, err)
	}
	oldRun := insertMemoryWatchAttempt(t, mem, bucket, StatusCancelled, 1, now)
	if err := mem.ResetBucket(ctx, bucket.ID, oldRun.ID); err != nil {
		t.Fatal(err)
	}

	allowed, _, err = mem.ReserveWatchQuota(ctx, bucket.ID, bucket.UserID, []int64{79}, dayStart, hourStart, 1, 1)
	if err != nil || !allowed {
		t.Fatalf("reserve replacement run allowed=%v err=%v", allowed, err)
	}
	newRun := insertMemoryWatchAttempt(t, mem, bucket, StatusCancelled, 2, now+1)
	if err := mem.FinishWatchDelivery(ctx, bucket.ID, bucket.UserID, oldRun.ID, StatusCancelled, now+2); err != nil {
		t.Fatal(err)
	}
	fresh, err := mem.GetBucket(ctx, bucket.ID)
	if err != nil || fresh.Status != "scheduled" || fresh.RunID != newRun.ID {
		t.Fatalf("stale finalizer mutated replacement bucket: bucket=%+v err=%v", fresh, err)
	}

	blocked, err := mem.UpsertDeliveryBucket(ctx, bucket.UserID, 902, now, now)
	if err != nil {
		t.Fatal(err)
	}
	allowed, _, err = mem.ReserveWatchQuota(ctx, blocked.ID, blocked.UserID, []int64{79}, dayStart, hourStart, 1, 1)
	if err != nil || allowed {
		t.Fatalf("stale finalizer released replacement reservation: allowed=%v err=%v", allowed, err)
	}
}

func insertMemoryWatchAttempt(t *testing.T, mem *MemoryStore, bucket DeliveryBucket, status string, attempt int, nowMs int64) Run {
	t.Helper()
	payload, err := json.Marshal(map[string]any{"bucket_id": bucket.ID, "hit_ids": bucket.HitIDs})
	if err != nil {
		t.Fatal(err)
	}
	run, err := mem.InsertRun(context.Background(), Run{
		UserID: bucket.UserID, SessionID: 1, RequestID: "watch-attempt-" + strconv.Itoa(attempt),
		Source: SourceWatch, Status: status, Phase: PhaseDone, QueuedPayload: payload,
		ConsentVersion: 2, InputVersion: 1, CreatedAtMs: nowMs, EndedAtMs: nowMs,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := mem.MarkBucketScheduled(context.Background(), bucket.ID, run.ID); err != nil {
		t.Fatal(err)
	}
	return run
}
