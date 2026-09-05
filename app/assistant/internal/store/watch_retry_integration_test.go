//go:build integration

package store

import (
	"context"
	"encoding/json"
	"fmt"
	"testing"
	"time"
)

func TestSQLWatchFailureDefersAndCancellationReturnsPending(t *testing.T) {
	assistantTestEnv.TruncateAll(t, "watch_send_reservation", "watch_send_stat", "watch_delivery_bucket", "agent_run_event", "agent_run")
	ctx := context.Background()
	st := newAssistantTestStore()
	now := time.Date(2026, 9, 5, 10, 0, 0, 0, time.UTC).UnixMilli()
	dayStart := now / (24 * time.Hour).Milliseconds() * (24 * time.Hour).Milliseconds()
	hourStart := now / time.Hour.Milliseconds() * time.Hour.Milliseconds()

	failed, err := st.UpsertDeliveryBucket(ctx, 91, 1001, now-2*time.Minute.Milliseconds(), now)
	if err != nil {
		t.Fatal(err)
	}
	allowed, _, err := st.ReserveWatchQuota(ctx, failed.ID, failed.UserID, []int64{71}, dayStart, hourStart, 1, 1)
	if err != nil || !allowed {
		t.Fatalf("reserve failed bucket allowed=%v err=%v", allowed, err)
	}
	failedRun := scheduleSQLWatchAttempt(t, st, failed, StatusError, 1, now)
	if err := st.FinishWatchDelivery(ctx, failed.ID, failed.UserID, failedRun.ID, StatusError, now); err != nil {
		t.Fatal(err)
	}
	fresh, err := st.GetBucket(ctx, failed.ID)
	wantNotBefore := now + time.Minute.Milliseconds()
	if err != nil || fresh.Status != "deferred" || fresh.RunID != 0 || fresh.NotBeforeMs != wantNotBefore {
		t.Fatalf("failed bucket=%+v err=%v", fresh, err)
	}
	if due, err := st.ListDueBuckets(ctx, now, 2*time.Minute.Milliseconds()); err != nil || len(due) != 0 {
		t.Fatalf("failed bucket was immediately due: buckets=%+v err=%v", due, err)
	}

	cancelled, err := st.UpsertDeliveryBucket(ctx, 91, 1002, now-4*time.Minute.Milliseconds(), now)
	if err != nil {
		t.Fatal(err)
	}
	allowed, _, err = st.ReserveWatchQuota(ctx, cancelled.ID, cancelled.UserID, []int64{71}, dayStart, hourStart, 1, 1)
	if err != nil || !allowed {
		t.Fatalf("error reservation was not released: allowed=%v err=%v", allowed, err)
	}
	cancelledRun := scheduleSQLWatchAttempt(t, st, cancelled, StatusCancelled, 1, now)
	if err := st.FinishWatchDelivery(ctx, cancelled.ID, cancelled.UserID, cancelledRun.ID, StatusCancelled, now); err != nil {
		t.Fatal(err)
	}
	fresh, err = st.GetBucket(ctx, cancelled.ID)
	if err != nil || fresh.Status != "pending" || fresh.RunID != 0 || fresh.NotBeforeMs != 0 {
		t.Fatalf("cancelled bucket=%+v err=%v", fresh, err)
	}
	if due, err := st.ListDueBuckets(ctx, now, 2*time.Minute.Milliseconds()); err != nil || len(due) != 1 || due[0].ID != cancelled.ID {
		t.Fatalf("cancelled bucket not immediately due: buckets=%+v err=%v", due, err)
	}
}

func TestSQLFinishWatchDeliveryCleansResidualReservation(t *testing.T) {
	assistantTestEnv.TruncateAll(t, "watch_send_reservation", "watch_send_stat", "watch_delivery_bucket", "agent_run_event", "agent_run")
	ctx := context.Background()
	st := newAssistantTestStore()
	now := time.Date(2026, 9, 5, 10, 0, 0, 0, time.UTC).UnixMilli()
	dayStart := now / (24 * time.Hour).Milliseconds() * (24 * time.Hour).Milliseconds()
	hourStart := now / time.Hour.Milliseconds() * time.Hour.Milliseconds()
	bucket, err := st.UpsertDeliveryBucket(ctx, 97, 4001, now-2*time.Minute.Milliseconds(), now)
	if err != nil {
		t.Fatal(err)
	}
	allowed, _, err := st.ReserveWatchQuota(ctx, bucket.ID, bucket.UserID, []int64{77}, dayStart, hourStart, 1, 1)
	if err != nil || !allowed {
		t.Fatalf("reserve allowed=%v err=%v", allowed, err)
	}
	if err := st.DeferBucket(ctx, bucket.ID, now+time.Minute.Milliseconds()); err != nil {
		t.Fatal(err)
	}
	if err := st.FinishWatchDelivery(ctx, bucket.ID, bucket.UserID, 999, StatusCancelled, now); err != nil {
		t.Fatal(err)
	}
	next, err := st.UpsertDeliveryBucket(ctx, bucket.UserID, 4002, now, now)
	if err != nil {
		t.Fatal(err)
	}
	allowed, _, err = st.ReserveWatchQuota(ctx, next.ID, next.UserID, []int64{77}, dayStart, hourStart, 1, 1)
	if err != nil || !allowed {
		t.Fatalf("residual reservation was not released: allowed=%v err=%v", allowed, err)
	}
}

func TestSQLWatchFailureStopsAfterEightAttempts(t *testing.T) {
	assistantTestEnv.TruncateAll(t, "watch_send_reservation", "watch_send_stat", "watch_delivery_bucket", "agent_run_event", "agent_run")
	ctx := context.Background()
	st := newAssistantTestStore()
	now := time.Date(2026, 9, 5, 10, 0, 0, 0, time.UTC).UnixMilli()
	bucket, err := st.UpsertDeliveryBucket(ctx, 92, 2001, now-2*time.Minute.Milliseconds(), now)
	if err != nil {
		t.Fatal(err)
	}
	delays := []time.Duration{time.Minute, 2 * time.Minute, 4 * time.Minute, 8 * time.Minute, 16 * time.Minute, 30 * time.Minute, 30 * time.Minute}
	attemptAt := now
	for attempt := 1; attempt <= watchDeliveryMaxAttempts; attempt++ {
		run := scheduleSQLWatchAttempt(t, st, bucket, StatusError, attempt, attemptAt)
		if err := st.FinishWatchDelivery(ctx, bucket.ID, bucket.UserID, run.ID, StatusError, attemptAt); err != nil {
			t.Fatal(err)
		}
		fresh, err := st.GetBucket(ctx, bucket.ID)
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
		attemptAt = fresh.NotBeforeMs
	}
	if due, err := st.ListDueBuckets(ctx, attemptAt+watchDeliveryRetention.Milliseconds(), 2*time.Minute.Milliseconds()); err != nil || len(due) != 0 {
		t.Fatalf("discarded bucket became due: buckets=%+v err=%v", due, err)
	}
}

func TestSQLRequeueFailedWatchBucketsUsesRetryPolicy(t *testing.T) {
	assistantTestEnv.TruncateAll(t, "watch_send_reservation", "watch_send_stat", "watch_delivery_bucket", "agent_run_event", "agent_run")
	ctx := context.Background()
	st := newAssistantTestStore()
	now := time.Date(2026, 9, 5, 10, 0, 0, 0, time.UTC).UnixMilli()
	dayStart := now / (24 * time.Hour).Milliseconds() * (24 * time.Hour).Milliseconds()
	hourStart := now / time.Hour.Milliseconds() * time.Hour.Milliseconds()

	failed, err := st.UpsertDeliveryBucket(ctx, 93, 3001, now-2*time.Minute.Milliseconds(), now)
	if err != nil {
		t.Fatal(err)
	}
	allowed, _, err := st.ReserveWatchQuota(ctx, failed.ID, failed.UserID, []int64{73}, dayStart, hourStart, 1, 1)
	if err != nil || !allowed {
		t.Fatalf("reserve failed bucket allowed=%v err=%v", allowed, err)
	}
	scheduleSQLWatchAttempt(t, st, failed, StatusError, 1, now)
	if err := st.RequeueFailedBuckets(ctx, now); err != nil {
		t.Fatal(err)
	}
	fresh, err := st.GetBucket(ctx, failed.ID)
	if err != nil || fresh.Status != "deferred" || fresh.NotBeforeMs != now+time.Minute.Milliseconds() {
		t.Fatalf("requeued error bucket=%+v err=%v", fresh, err)
	}

	next, err := st.UpsertDeliveryBucket(ctx, failed.UserID, 3002, now, now)
	if err != nil {
		t.Fatal(err)
	}
	allowed, _, err = st.ReserveWatchQuota(ctx, next.ID, next.UserID, []int64{73}, dayStart, hourStart, 1, 1)
	if err != nil || !allowed {
		t.Fatalf("requeue did not release reservation: allowed=%v err=%v", allowed, err)
	}

	maxed, err := st.UpsertDeliveryBucket(ctx, 94, 3003, now-2*time.Minute.Milliseconds(), now)
	if err != nil {
		t.Fatal(err)
	}
	for attempt := 1; attempt <= watchDeliveryMaxAttempts; attempt++ {
		if attempt < watchDeliveryMaxAttempts {
			insertSQLWatchRun(t, st, maxed, StatusError, attempt, now)
			continue
		}
		scheduleSQLWatchAttempt(t, st, maxed, StatusError, attempt, now)
	}
	if err := st.RequeueFailedBuckets(ctx, now); err != nil {
		t.Fatal(err)
	}
	fresh, err = st.GetBucket(ctx, maxed.ID)
	if err != nil || fresh.Status != "discarded" || fresh.RunID != 0 || fresh.NotBeforeMs != 0 {
		t.Fatalf("maxed recovery bucket=%+v err=%v", fresh, err)
	}

	invalid, err := st.UpsertDeliveryBucket(ctx, 95, 3004, now-2*time.Minute.Milliseconds(), now)
	if err != nil {
		t.Fatal(err)
	}
	run, err := st.InsertRun(ctx, Run{
		UserID: invalid.UserID, SessionID: 1, RequestID: "watch-invalid-payload",
		Source: SourceWatch, Status: StatusError, Phase: PhaseDone, QueuedPayload: []byte(`{}`),
		ConsentVersion: 2, InputVersion: 1, CreatedAtMs: now, EndedAtMs: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.MarkBucketScheduled(ctx, invalid.ID, run.ID); err != nil {
		t.Fatal(err)
	}
	if err := st.RequeueFailedBuckets(ctx, now); err != nil {
		t.Fatal(err)
	}
	fresh, err = st.GetBucket(ctx, invalid.ID)
	if err != nil || fresh.Status != "discarded" || fresh.RunID != 0 {
		t.Fatalf("invalid-payload recovery bucket=%+v err=%v", fresh, err)
	}

	cancelled, err := st.UpsertDeliveryBucket(ctx, 96, 3005, now-2*time.Minute.Milliseconds(), now)
	if err != nil {
		t.Fatal(err)
	}
	scheduleSQLWatchAttempt(t, st, cancelled, StatusCancelled, 1, now)
	if err := st.RequeueFailedBuckets(ctx, now); err != nil {
		t.Fatal(err)
	}
	fresh, err = st.GetBucket(ctx, cancelled.ID)
	if err != nil || fresh.Status != "pending" || fresh.RunID != 0 || fresh.NotBeforeMs != 0 {
		t.Fatalf("cancelled recovery bucket=%+v err=%v", fresh, err)
	}

	invalidAssociation, err := st.UpsertDeliveryBucket(ctx, 98, 3006, now-2*time.Minute.Milliseconds(), now)
	if err != nil {
		t.Fatal(err)
	}
	nonWatchRun, err := st.InsertRun(ctx, Run{
		UserID: invalidAssociation.UserID, SessionID: 1, RequestID: "not-a-watch-run",
		Source: SourceUser, Status: StatusCancelled, Phase: PhaseDone, CreatedAtMs: now, EndedAtMs: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.MarkBucketScheduled(ctx, invalidAssociation.ID, nonWatchRun.ID); err != nil {
		t.Fatal(err)
	}
	if err := st.RequeueFailedBuckets(ctx, now); err != nil {
		t.Fatal(err)
	}
	fresh, err = st.GetBucket(ctx, invalidAssociation.ID)
	if err != nil || fresh.Status != "scheduled" || fresh.RunID != nonWatchRun.ID {
		t.Fatalf("invalid association was requeued: bucket=%+v err=%v", fresh, err)
	}

	crossUserBucket, err := st.UpsertDeliveryBucket(ctx, 99, 3007, now-2*time.Minute.Milliseconds(), now)
	if err != nil {
		t.Fatal(err)
	}
	payload, err := json.Marshal(map[string]any{"bucket_id": crossUserBucket.ID})
	if err != nil {
		t.Fatal(err)
	}
	crossUserRun, err := st.InsertRun(ctx, Run{
		UserID: 100, SessionID: 1, RequestID: "cross-user-watch-run", QueuedPayload: payload,
		Source: SourceWatch, Status: StatusCancelled, Phase: PhaseDone, CreatedAtMs: now, EndedAtMs: now,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := st.MarkBucketScheduled(ctx, crossUserBucket.ID, crossUserRun.ID); err != nil {
		t.Fatal(err)
	}
	if err := st.RequeueFailedBuckets(ctx, now); err != nil {
		t.Fatal(err)
	}
	fresh, err = st.GetBucket(ctx, crossUserBucket.ID)
	if err != nil || fresh.Status != "scheduled" || fresh.RunID != crossUserRun.ID {
		t.Fatalf("cross-user association was requeued: bucket=%+v err=%v", fresh, err)
	}
}

func scheduleSQLWatchAttempt(t *testing.T, st *SQLStore, bucket DeliveryBucket, status string, attempt int, nowMs int64) Run {
	t.Helper()
	run := insertSQLWatchRun(t, st, bucket, status, attempt, nowMs)
	if err := st.MarkBucketScheduled(context.Background(), bucket.ID, run.ID); err != nil {
		t.Fatal(err)
	}
	return run
}

func insertSQLWatchRun(t *testing.T, st *SQLStore, bucket DeliveryBucket, status string, attempt int, nowMs int64) Run {
	t.Helper()
	payload, err := json.Marshal(map[string]any{"bucket_id": bucket.ID, "hit_ids": bucket.HitIDs})
	if err != nil {
		t.Fatal(err)
	}
	run, err := st.InsertRun(context.Background(), Run{
		UserID: bucket.UserID, SessionID: 1, RequestID: fmt.Sprintf("watch-%d-attempt-%d", bucket.ID, attempt),
		Source: SourceWatch, Status: status, Phase: PhaseDone, QueuedPayload: payload,
		ConsentVersion: 2, InputVersion: 1, CreatedAtMs: nowMs, EndedAtMs: nowMs,
	})
	if err != nil {
		t.Fatal(err)
	}
	return run
}
