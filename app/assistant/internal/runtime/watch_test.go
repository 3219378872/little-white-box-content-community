package runtime

import (
	"context"
	"strings"
	"testing"
	"time"

	"esx/app/assistant/internal/llm"
	"esx/app/assistant/internal/store"
	"esx/app/assistant/internal/tool"
	"esx/app/assistant/watch"
)

func TestWatchRunReceivesHitsAndCountsOnlyAfterSuccess(t *testing.T) {
	ctx := context.Background()
	mem := store.NewMemoryStore()
	watchStore, bucket, taskID := watchFixture(t, mem)
	now := store.NowMs()
	if err := scheduleBucket(ctx, mem, watchStore, func(context.Context, int64) (bool, error) { return true, nil }, bucket, now); err != nil {
		t.Fatal(err)
	}
	scheduled, _ := mem.GetBucket(ctx, bucket.ID)
	run, err := mem.GetRun(ctx, scheduled.RunID)
	if err != nil {
		t.Fatal(err)
	}
	dayStart := now / int64((24 * time.Hour).Milliseconds()) * int64((24 * time.Hour).Milliseconds())
	hourStart := now / int64(time.Hour.Milliseconds()) * int64(time.Hour.Milliseconds())
	if daily, _ := mem.CountSent(ctx, 7, 0, "day", dayStart); daily != 0 {
		t.Fatalf("daily count advanced before delivery: %d", daily)
	}
	reg, err := tool.NewRegistry(tool.Clients{Store: mem}, []string{tool.PresentSources})
	if err != nil {
		t.Fatal(err)
	}
	script := &scriptedLLM{replies: []llm.Result{{Text: "Watch 主动消息"}}}
	engine := &Engine{Store: mem, Watch: watchStore, Tools: reg, LLM: script, Window: 128000}
	engine.Execute(ctx, *run, false)
	if len(script.reqs) != 1 || !containsPromptText(script.reqs[0], "UNTRUSTED_WATCH_HITS_JSON") || !containsPromptText(script.reqs[0], `"post_id":99`) {
		t.Fatalf("watch prompt=%+v", script.reqs)
	}
	finished, _ := mem.GetBucket(ctx, bucket.ID)
	if finished.Status != "sent" {
		t.Fatalf("bucket=%+v", finished)
	}
	if daily, _ := mem.CountSent(ctx, 7, 0, "day", dayStart); daily != 1 {
		t.Fatalf("daily=%d", daily)
	}
	if hourly, _ := mem.CountSent(ctx, 7, taskID, "hour", hourStart); hourly != 1 {
		t.Fatalf("hourly=%d", hourly)
	}
	thread, _ := mem.GetThread(ctx, 7)
	if thread.UnreadCount != 1 || thread.LastMessagePreview != "Watch 主动消息" {
		t.Fatalf("thread=%+v", thread)
	}
	events, _ := mem.ListEventsAfter(ctx, run.ID, 0)
	if len(events) < 3 || events[len(events)-2].Type != store.EventToken || events[len(events)-1].Type != store.EventDone {
		t.Fatalf("events=%+v", events)
	}
	outbox, _ := mem.ListUnpublishedOutbox(ctx, 10)
	if len(outbox) != 1 || !strings.Contains(outbox[0].PayloadJSON, "Watch 主动消息") {
		t.Fatalf("outbox=%+v", outbox)
	}
}

func TestWatchRunFailureRequeuesBucket(t *testing.T) {
	ctx := context.Background()
	mem := store.NewMemoryStore()
	watchStore, bucket, _ := watchFixture(t, mem)
	if err := scheduleBucket(ctx, mem, watchStore, func(context.Context, int64) (bool, error) { return true, nil }, bucket, store.NowMs()); err != nil {
		t.Fatal(err)
	}
	scheduled, _ := mem.GetBucket(ctx, bucket.ID)
	run, _ := mem.GetRun(ctx, scheduled.RunID)
	reg, _ := tool.NewRegistry(tool.Clients{Store: mem}, []string{tool.PresentSources})
	engine := &Engine{Store: mem, Watch: watchStore, Tools: reg, LLM: &scriptedLLM{}, Window: 128000}
	engine.Execute(ctx, *run, false)
	reset, _ := mem.GetBucket(ctx, bucket.ID)
	if reset.Status != "pending" || reset.RunID != 0 {
		t.Fatalf("bucket=%+v", reset)
	}
}

func TestWatchBucketWithoutCurrentConsentIsDeferred(t *testing.T) {
	ctx := context.Background()
	mem := store.NewMemoryStore()
	watchStore, bucket, _ := watchFixture(t, mem)
	now := store.NowMs()
	if err := scheduleBucket(ctx, mem, watchStore, func(context.Context, int64) (bool, error) { return false, nil }, bucket, now); err != nil {
		t.Fatal(err)
	}
	deferred, _ := mem.GetBucket(ctx, bucket.ID)
	if deferred.Status != "deferred" || deferred.RunID != 0 || deferred.NotBeforeMs <= now {
		t.Fatalf("bucket=%+v", deferred)
	}
}

func TestDeferredBucketDoesNotCollideWithNextMergeWindow(t *testing.T) {
	ctx := context.Background()
	mem := store.NewMemoryStore()
	first, _ := mem.UpsertDeliveryBucket(ctx, 7, 1, 120000, 1)
	second, _ := mem.UpsertDeliveryBucket(ctx, 7, 2, 240000, 2)
	if err := mem.DeferBucket(ctx, first.ID, 240000); err != nil {
		t.Fatal(err)
	}
	gotFirst, _ := mem.GetBucket(ctx, first.ID)
	gotSecond, _ := mem.GetBucket(ctx, second.ID)
	if gotFirst.ID == gotSecond.ID || gotFirst.WindowStartMs != 120000 || gotSecond.WindowStartMs != 240000 {
		t.Fatalf("first=%+v second=%+v", gotFirst, gotSecond)
	}
}

func watchFixture(t *testing.T, mem *store.MemoryStore) (*watch.MapStore, store.DeliveryBucket, int64) {
	t.Helper()
	ctx := context.Background()
	watchStore := watch.NewMapStore()
	task, err := watchStore.Create(ctx, watch.Task{UserID: 7, ConditionType: watch.AuthorNewPost, TargetType: "author", TargetID: 8})
	if err != nil {
		t.Fatal(err)
	}
	if err := watchStore.RecordHit(ctx, watch.Hit{UserID: 7, TaskID: task.ID, PostID: 99, Title: "命中帖子", Summary: "作者发帖"}, "event-1"); err != nil {
		t.Fatal(err)
	}
	hits, _ := watchStore.ListHits(ctx, 7, false)
	if len(hits) != 1 {
		t.Fatalf("hits=%+v", hits)
	}
	bucket, err := mem.UpsertDeliveryBucket(ctx, 7, hits[0].ID, store.NowMs()-watchWindow.Milliseconds(), store.NowMs())
	if err != nil {
		t.Fatal(err)
	}
	return watchStore, bucket, task.ID
}

func containsPromptText(req llm.Request, want string) bool {
	for _, turn := range req.Messages {
		if strings.Contains(turn.Content, want) {
			return true
		}
	}
	return false
}
