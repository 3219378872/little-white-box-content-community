package runtime

import (
	"context"
	"encoding/json"
	"time"

	"esx/app/assistant/internal/store"
	"esx/app/assistant/watch"

	"github.com/zeromicro/go-zero/core/logx"
)

const watchWindow = 2 * time.Minute

func ScheduleDueWatchRuns(ctx context.Context, st store.Store, watchStore watch.Store) {
	if st == nil {
		return
	}
	now := store.NowMs()
	buckets, err := st.ListDueBuckets(ctx, now, watchWindow.Milliseconds())
	if err != nil || len(buckets) == 0 {
		return
	}
	for _, bucket := range buckets {
		if err := scheduleBucket(ctx, st, watchStore, bucket, now); err != nil {
			logx.WithContext(ctx).Errorw("schedule watch run failed", logx.Field("bucket", bucket.ID), logx.Field("err", err.Error()))
		}
	}
}

func scheduleBucket(ctx context.Context, st store.Store, watchStore watch.Store, bucket store.DeliveryBucket, now int64) error {
	hourStart := now / int64(time.Hour.Milliseconds()) * int64(time.Hour.Milliseconds())
	dayStart := now / int64((24 * time.Hour).Milliseconds()) * int64((24 * time.Hour).Milliseconds())
	daily, err := st.CountSent(ctx, bucket.UserID, 0, "day", dayStart)
	if err != nil {
		return err
	}
	if daily >= 20 {
		return nil
	}
	taskHourOK := true
	if watchStore != nil && len(bucket.HitIDs) > 0 {
		hits, _ := watchStore.ListHits(ctx, bucket.UserID, false)
		byID := map[int64]watch.Hit{}
		for _, hit := range hits {
			byID[hit.ID] = hit
		}
		seen := map[int64]struct{}{}
		for _, id := range bucket.HitIDs {
			hit, ok := byID[id]
			if !ok {
				continue
			}
			if _, dup := seen[hit.TaskID]; dup {
				continue
			}
			seen[hit.TaskID] = struct{}{}
			hourly, err := st.CountSent(ctx, bucket.UserID, hit.TaskID, "hour", hourStart)
			if err != nil {
				return err
			}
			if hourly >= 3 {
				taskHourOK = false
				break
			}
		}
	}
	if !taskHourOK {
		return nil
	}
	thread, err := st.GetThread(ctx, bucket.UserID)
	if err != nil {
		return err
	}
	sessionID := thread.SessionID
	if sessionID == 0 {
		created, err := st.CreateSession(ctx, store.Session{UserID: bucket.UserID, PromptEpoch: 1, Status: store.SessionOpen, CreatedAtMs: now})
		if err != nil {
			return err
		}
		sessionID = created.ID
		thread.SessionID = sessionID
		_ = st.SaveThread(ctx, *thread)
	}
	payload, _ := json.Marshal(map[string]any{"hit_ids": bucket.HitIDs, "bucket_id": bucket.ID})
	run, err := st.InsertRun(ctx, store.Run{
		UserID: bucket.UserID, SessionID: sessionID, RequestID: "watch-" + itoa(bucket.ID) + "-" + itoa(now),
		Source: store.SourceWatch, Status: store.StatusQueued, Phase: store.PhaseQueued, Priority: store.PriorityWatch,
		QueuedPayload: payload, CreatedAtMs: now, LastActivityAtMs: now,
	})
	if err != nil {
		return err
	}
	if err := st.MarkBucketScheduled(ctx, bucket.ID, run.ID); err != nil {
		return err
	}
	_ = st.IncrSent(ctx, bucket.UserID, 0, "day", dayStart)
	return nil
}
