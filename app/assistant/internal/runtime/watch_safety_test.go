package runtime

import (
	"context"
	"testing"
	"time"

	"esx/app/assistant/internal/llm"
	"esx/app/assistant/internal/store"
	"esx/app/assistant/internal/tool"
)

func TestRevokedConsentDefersWatchScheduling(t *testing.T) {
	ctx := context.Background()
	mem := store.NewMemoryStore()
	mem.SetAgentConsent(7, 0)
	now := store.NowMs()
	bucket, err := mem.UpsertDeliveryBucket(ctx, 7, 101, now-watchWindow.Milliseconds(), now)
	if err != nil {
		t.Fatal(err)
	}
	if err := scheduleBucket(ctx, mem, nil, nil, bucket, now); err != nil {
		t.Fatal(err)
	}
	fresh, err := mem.GetBucket(ctx, bucket.ID)
	if err != nil || fresh.Status != "deferred" || fresh.RunID != 0 || fresh.NotBeforeMs <= now {
		t.Fatalf("revoked bucket=%+v err=%v", fresh, err)
	}
}

func TestWatchTerminalCommitMarksBucketAndDailyStat(t *testing.T) {
	ctx := context.Background()
	mem := store.NewMemoryStore()
	now := store.NowMs()
	watchStore, bucket, _ := watchFixture(t, mem)
	if err := scheduleBucket(ctx, mem, watchStore, nil, bucket, now); err != nil {
		t.Fatal(err)
	}
	scheduled, _ := mem.GetBucket(ctx, bucket.ID)
	if scheduled == nil || scheduled.Status != "scheduled" || scheduled.RunID == 0 {
		t.Fatalf("scheduled bucket=%+v", scheduled)
	}
	run, err := mem.Claim(ctx, "watch-worker", now, 60_000)
	if err != nil || run == nil || run.ID != scheduled.RunID {
		t.Fatalf("claim=%+v err=%v", run, err)
	}
	registry, err := tool.NewRegistry(tool.Clients{Store: mem}, []string{tool.PresentSources})
	if err != nil {
		t.Fatal(err)
	}
	engine := &Engine{Store: mem, Watch: watchStore, Tools: registry, LLM: &scriptedLLM{replies: []llm.Result{{Text: "watch update"}}}, Window: 128000}
	engine.Execute(ctx, *run, false)
	fresh, err := mem.GetBucket(ctx, bucket.ID)
	if err != nil || fresh.Status != "sent" {
		t.Fatalf("finished bucket=%+v err=%v", fresh, err)
	}
	dayStart := now / int64((24 * time.Hour).Milliseconds()) * int64((24 * time.Hour).Milliseconds())
	count, err := mem.CountSent(ctx, 7, 0, "day", dayStart)
	if err != nil || count != 1 {
		t.Fatalf("daily count=%d err=%v", count, err)
	}
}
