package runtime

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"esx/app/assistant/internal/llm"
	"esx/app/assistant/internal/prompt"
	"esx/app/assistant/internal/store"
	"esx/app/assistant/internal/tool"
	"esx/app/assistant/watch"
	"esx/app/content/rpc/contentservice"

	"google.golang.org/grpc"
)

func allowAllWatchPosts(_ context.Context, _ int64, postIDs []int64) (map[int64]bool, error) {
	visible := make(map[int64]bool, len(postIDs))
	for _, postID := range postIDs {
		visible[postID] = true
	}
	return visible, nil
}

type watchFinalStream struct{}

func (*watchFinalStream) Complete(context.Context, llm.Request) (llm.Result, error) {
	return llm.Result{}, errors.New("unexpected non-stream call")
}

func (*watchFinalStream) CompleteStream(_ context.Context, _ llm.Request, emit func(llm.Delta) error) (llm.Result, error) {
	if err := emit(llm.Delta{Text: "must not remain"}); err != nil {
		return llm.Result{}, err
	}
	return llm.Result{Text: "must not remain", Streamed: true}, nil
}

func (*watchFinalStream) SupportsTools() bool      { return true }
func (*watchFinalStream) WireAPI() string          { return llm.WireAPIResponses }
func (*watchFinalStream) MaxOutputTokens() int     { return 128 }
func (*watchFinalStream) ContextWindowTokens() int { return 128000 }
func (*watchFinalStream) RouteID() string          { return "primary" }
func (*watchFinalStream) ModelName() string        { return "watch-final" }
func (*watchFinalStream) Boundary() string         { return "same" }
func (*watchFinalStream) SupportsStreaming() bool  { return true }

func TestWatchRunReceivesHitsAndCountsOnlyAfterSuccess(t *testing.T) {
	ctx := context.Background()
	mem := store.NewMemoryStore()
	watchStore, bucket, taskID := watchFixture(t, mem)
	now := store.NowMs()
	if err := scheduleBucket(ctx, mem, nil, watchStore, func(context.Context, int64) (bool, error) { return true, nil }, allowAllWatchPosts, bucket, now); err != nil {
		t.Fatal(err)
	}
	scheduled, _ := mem.GetBucket(ctx, bucket.ID)
	run, err := mem.Claim(ctx, "watch-worker", now, 60_000)
	if err != nil || run == nil || run.ID != scheduled.RunID {
		t.Fatalf("claim=%+v err=%v", run, err)
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
	engine := &Engine{Store: mem, Watch: watchStore, Tools: reg, LLM: script, WatchPosts: allowAllWatchPosts, Window: 128000}
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
	if err := scheduleBucket(ctx, mem, nil, watchStore, func(context.Context, int64) (bool, error) { return true, nil }, allowAllWatchPosts, bucket, store.NowMs()); err != nil {
		t.Fatal(err)
	}
	scheduled, _ := mem.GetBucket(ctx, bucket.ID)
	run, err := mem.Claim(ctx, "watch-worker", store.NowMs(), 60_000)
	if err != nil || run == nil || run.ID != scheduled.RunID {
		t.Fatalf("claim=%+v err=%v", run, err)
	}
	reg, _ := tool.NewRegistry(tool.Clients{Store: mem}, []string{tool.PresentSources})
	engine := &Engine{Store: mem, Watch: watchStore, Tools: reg, LLM: &scriptedLLM{}, WatchPosts: allowAllWatchPosts, Window: 128000}
	engine.Execute(ctx, *run, false)
	reset, _ := mem.GetBucket(ctx, bucket.ID)
	if reset.Status != "pending" || reset.RunID != 0 {
		t.Fatalf("bucket=%+v", reset)
	}
}

func TestWatchRepeatedReadTerminatesWithNoProgress(t *testing.T) {
	ctx := context.Background()
	mem := store.NewMemoryStore()
	watchStore, bucket, _ := watchFixture(t, mem)
	if err := scheduleBucket(ctx, mem, nil, watchStore, func(context.Context, int64) (bool, error) { return true, nil }, allowAllWatchPosts, bucket, store.NowMs()); err != nil {
		t.Fatal(err)
	}
	scheduled, _ := mem.GetBucket(ctx, bucket.ID)
	run, err := mem.Claim(ctx, "watch-worker", store.NowMs(), 60_000)
	if err != nil || run == nil || run.ID != scheduled.RunID {
		t.Fatalf("claim=%+v scheduled=%+v err=%v", run, scheduled, err)
	}
	reg, err := tool.NewRegistry(tool.Clients{Store: mem}, []string{tool.GetPost})
	if err != nil {
		t.Fatal(err)
	}
	call := llm.ToolCall{ID: "read-1", Name: tool.GetPost, Arguments: `{"post_id":99}`}
	model := &scriptedLLM{replies: []llm.Result{
		{ToolCalls: []llm.ToolCall{call}},
		{ToolCalls: []llm.ToolCall{{ID: "read-2", Name: tool.GetPost, Arguments: `{"post_id":99}`}}},
		{ToolCalls: []llm.ToolCall{{ID: "read-3", Name: tool.GetPost, Arguments: `{"post_id":99}`}}},
	}}
	engine := &Engine{Store: mem, Watch: watchStore, Tools: reg, LLM: model, WatchPosts: allowAllWatchPosts, Window: 128000}
	engine.Execute(ctx, *run, false)
	fresh, err := mem.GetRun(ctx, run.ID)
	if err != nil || fresh.Status != store.StatusError || fresh.ErrorCode != "TOOL_NO_PROGRESS" {
		t.Fatalf("run=%+v err=%v", fresh, err)
	}
	messages, err := mem.ListSessionMessages(ctx, run.UserID, run.SessionID, true)
	if err != nil {
		t.Fatal(err)
	}
	hints := 0
	for _, message := range messages {
		if !message.Visible && message.Role == store.RoleSystem && strings.Contains(string(message.APIContent), "工具无进展") {
			hints++
		}
	}
	if hints != 1 {
		t.Fatalf("convergence hints=%d all=%+v", hints, messages)
	}
}

type watchPostContent struct {
	contentservice.ContentService
}

func (*watchPostContent) GetPost(_ context.Context, in *contentservice.GetPostReq, _ ...grpc.CallOption) (*contentservice.GetPostResp, error) {
	id := int64(99)
	if in != nil && in.PostId > 0 {
		id = in.PostId
	}
	return &contentservice.GetPostResp{Post: &contentservice.PostInfo{Id: id, Status: 1, Revision: 1, Title: "命中帖子", Content: "回源正文"}}, nil
}

func TestWatchToolRoundKeepsHitsBeforeResults(t *testing.T) {
	ctx := context.Background()
	mem := store.NewMemoryStore()
	watchStore, bucket, _ := watchFixture(t, mem)
	if err := scheduleBucket(ctx, mem, nil, watchStore, func(context.Context, int64) (bool, error) { return true, nil }, allowAllWatchPosts, bucket, store.NowMs()); err != nil {
		t.Fatal(err)
	}
	scheduled, _ := mem.GetBucket(ctx, bucket.ID)
	run, err := mem.Claim(ctx, "watch-worker", store.NowMs(), 60_000)
	if err != nil || run == nil || run.ID != scheduled.RunID {
		t.Fatalf("claim=%+v scheduled=%+v err=%v", run, scheduled, err)
	}
	reg, err := tool.NewRegistry(tool.Clients{Store: mem, Content: &watchPostContent{}}, nil)
	if err != nil {
		t.Fatal(err)
	}
	script := &scriptedLLM{replies: []llm.Result{
		{ToolCalls: []llm.ToolCall{{ID: "c1", Name: tool.GetPost, Arguments: `{"post_id":99}`}}},
		{Text: "已核验命中帖子"},
	}}
	engine := &Engine{Store: mem, Watch: watchStore, Tools: reg, LLM: script, WatchPosts: allowAllWatchPosts, Window: 128000}
	engine.Execute(ctx, *run, false)
	if len(script.reqs) != 2 {
		t.Fatalf("complete calls=%d", len(script.reqs))
	}
	first := script.reqs[0].Messages
	second := script.reqs[1].Messages
	if n := countWatchHits(first); n != 1 {
		t.Fatalf("first request watch hits=%d roles=%v", n, roles(first))
	}
	if n := countWatchHits(second); n != 1 {
		t.Fatalf("second request watch hits=%d roles=%v", n, roles(second))
	}
	if !hasToolCall(second, "c1", tool.GetPost) {
		t.Fatalf("second request missing function_call: %v", roles(second))
	}
	if !hasToolResult(second, "c1", "回源正文") && !hasToolResult(second, "c1", "命中帖子") {
		t.Fatalf("second request missing tool result: %+v", second)
	}
	hitIdx, callIdx, resultIdx := -1, -1, -1
	for i, turn := range second {
		if strings.Contains(turn.Content, watchHitsMarker) {
			hitIdx = i
		}
		if hasToolCall([]prompt.Turn{turn}, "c1", tool.GetPost) {
			callIdx = i
		}
		if turn.ToolCallID == "c1" {
			resultIdx = i
		}
	}
	if hitIdx < 0 || callIdx <= hitIdx || resultIdx <= callIdx {
		t.Fatalf("watch context order want hits < call < result, got hit=%d call=%d result=%d roles=%v", hitIdx, callIdx, resultIdx, roles(second))
	}
	messages, err := mem.ListSessionMessages(ctx, run.UserID, run.SessionID, true)
	if err != nil {
		t.Fatal(err)
	}
	hiddenInput, visibleUser := 0, 0
	for _, message := range messages {
		if message.Kind == store.KindWatchInput {
			if message.Visible {
				t.Fatalf("watch input leaked into visible history: %+v", message)
			}
			hiddenInput++
		}
		if message.Visible && message.Role == store.RoleUser {
			visibleUser++
		}
	}
	if hiddenInput != 1 {
		t.Fatalf("hidden watch input=%d messages=%+v", hiddenInput, messages)
	}
	if visibleUser != 0 {
		t.Fatalf("visible user messages=%d", visibleUser)
	}
}

func TestWatchInputSidecarIsIdempotentOnResume(t *testing.T) {
	ctx := context.Background()
	mem := store.NewMemoryStore()
	watchStore, bucket, _ := watchFixture(t, mem)
	if err := scheduleBucket(ctx, mem, nil, watchStore, func(context.Context, int64) (bool, error) { return true, nil }, allowAllWatchPosts, bucket, store.NowMs()); err != nil {
		t.Fatal(err)
	}
	scheduled, _ := mem.GetBucket(ctx, bucket.ID)
	run, err := mem.Claim(ctx, "watch-worker", store.NowMs(), 60_000)
	if err != nil || run == nil || run.ID != scheduled.RunID {
		t.Fatalf("claim=%+v err=%v", run, err)
	}
	engine := &Engine{Store: mem, Watch: watchStore, WatchPosts: allowAllWatchPosts}
	if err := engine.ensureWatchInput(ctx, *run); err != nil {
		t.Fatal(err)
	}
	if err := engine.ensureWatchInput(ctx, *run); err != nil {
		t.Fatal(err)
	}
	messages, err := mem.ListSessionMessages(ctx, run.UserID, run.SessionID, true)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, message := range messages {
		if message.Kind == store.KindWatchInput {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("watch input copies=%d messages=%+v", count, messages)
	}
}

func countWatchHits(turns []prompt.Turn) int {
	n := 0
	for _, turn := range turns {
		if strings.Contains(turn.Content, watchHitsMarker) {
			n++
		}
	}
	return n
}

func TestWatchBucketWithoutCurrentConsentIsDeferred(t *testing.T) {
	ctx := context.Background()
	mem := store.NewMemoryStore()
	watchStore, bucket, _ := watchFixture(t, mem)
	now := store.NowMs()
	if err := scheduleBucket(ctx, mem, nil, watchStore, func(context.Context, int64) (bool, error) { return false, nil }, allowAllWatchPosts, bucket, now); err != nil {
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

func TestWatchScheduleFiltersPostsThatAreNoLongerVisible(t *testing.T) {
	ctx := context.Background()
	mem := store.NewMemoryStore()
	watchStore, bucket, taskID := watchFixture(t, mem)
	if err := watchStore.RecordHit(ctx, watch.Hit{UserID: 7, TaskID: taskID, PostID: 100, Title: "removed"}, "event-2"); err != nil {
		t.Fatal(err)
	}
	hits, _ := watchStore.ListHits(ctx, 7, false)
	var secondHitID int64
	for _, hit := range hits {
		if hit.PostID == 100 {
			secondHitID = hit.ID
		}
	}
	bucket, _ = mem.UpsertDeliveryBucket(ctx, 7, secondHitID, bucket.WindowStartMs, store.NowMs())
	visibleOnly99 := func(_ context.Context, _ int64, _ []int64) (map[int64]bool, error) {
		return map[int64]bool{99: true}, nil
	}
	if err := scheduleBucket(ctx, mem, nil, watchStore, nil, visibleOnly99, bucket, store.NowMs()); err != nil {
		t.Fatal(err)
	}
	scheduled, _ := mem.GetBucket(ctx, bucket.ID)
	run, err := mem.GetRun(ctx, scheduled.RunID)
	if err != nil {
		t.Fatal(err)
	}
	payload := decodeWatchRunPayload(run.QueuedPayload)
	if len(payload.HitIDs) != 1 || len(payload.PostIDs) != 1 || payload.PostIDs[0] != 99 {
		t.Fatalf("payload=%+v", payload)
	}
}

func TestWatchScheduleFailsClosedAndDiscardsEmptyBucket(t *testing.T) {
	ctx := context.Background()
	mem := store.NewMemoryStore()
	watchStore, bucket, _ := watchFixture(t, mem)
	noneVisible := func(context.Context, int64, []int64) (map[int64]bool, error) { return map[int64]bool{}, nil }
	if err := scheduleBucket(ctx, mem, nil, watchStore, nil, noneVisible, bucket, store.NowMs()); err != nil {
		t.Fatal(err)
	}
	fresh, _ := mem.GetBucket(ctx, bucket.ID)
	if fresh.Status != "discarded" || fresh.RunID != 0 {
		t.Fatalf("bucket=%+v", fresh)
	}

	mem = store.NewMemoryStore()
	watchStore, bucket, _ = watchFixture(t, mem)
	authorityErr := errors.New("content unavailable")
	failing := func(context.Context, int64, []int64) (map[int64]bool, error) { return nil, authorityErr }
	if err := scheduleBucket(ctx, mem, nil, watchStore, nil, failing, bucket, store.NowMs()); !errors.Is(err, authorityErr) {
		t.Fatalf("err=%v", err)
	}
	fresh, _ = mem.GetBucket(ctx, bucket.ID)
	if fresh.Status != "pending" {
		t.Fatalf("failed authority check mutated bucket: %+v", fresh)
	}

	mem = store.NewMemoryStore()
	bucket, err := mem.UpsertDeliveryBucket(ctx, 7, 999, store.NowMs()-watchWindow.Milliseconds(), store.NowMs())
	if err != nil {
		t.Fatal(err)
	}
	if err := scheduleBucket(ctx, mem, nil, watch.NewMapStore(), nil, allowAllWatchPosts, bucket, store.NowMs()); err != nil {
		t.Fatal(err)
	}
	fresh, _ = mem.GetBucket(ctx, bucket.ID)
	if fresh.Status != "discarded" {
		t.Fatalf("expired hits left bucket retrying: %+v", fresh)
	}
}

func TestRecoveredWatchRunRechecksVisibilityBeforeModel(t *testing.T) {
	ctx := context.Background()
	mem := store.NewMemoryStore()
	watchStore, bucket, taskID := watchFixture(t, mem)
	if err := scheduleBucket(ctx, mem, nil, watchStore, nil, allowAllWatchPosts, bucket, store.NowMs()); err != nil {
		t.Fatal(err)
	}
	scheduled, _ := mem.GetBucket(ctx, bucket.ID)
	run, err := mem.Claim(ctx, "watch-worker", store.NowMs(), 60_000)
	if err != nil || run == nil || run.ID != scheduled.RunID {
		t.Fatalf("run=%+v err=%v", run, err)
	}
	model := &scriptedLLM{replies: []llm.Result{{Text: "must not run"}}}
	noneVisible := func(context.Context, int64, []int64) (map[int64]bool, error) { return map[int64]bool{}, nil }
	registry, err := tool.NewRegistry(tool.Clients{Store: mem}, []string{tool.PresentSources})
	if err != nil {
		t.Fatal(err)
	}
	engine := &Engine{Store: mem, Watch: watchStore, Tools: registry, LLM: model, WatchPosts: noneVisible, Window: 128000}
	engine.Execute(ctx, *run, true)
	if len(model.reqs) != 0 {
		t.Fatalf("model requests=%d", len(model.reqs))
	}
	freshRun, _ := mem.GetRun(ctx, run.ID)
	freshBucket, _ := mem.GetBucket(ctx, bucket.ID)
	if freshRun.Status != store.StatusDone || freshBucket.Status != "discarded" {
		t.Fatalf("run=%+v bucket=%+v", freshRun, freshBucket)
	}
	next, _ := mem.UpsertDeliveryBucket(ctx, 7, 999, bucket.WindowStartMs+watchWindow.Milliseconds(), store.NowMs())
	now := store.NowMs()
	dayStart := now / int64((24 * time.Hour).Milliseconds()) * int64((24 * time.Hour).Milliseconds())
	hourStart := now / int64(time.Hour.Milliseconds()) * int64(time.Hour.Milliseconds())
	allowed, _, err := mem.ReserveWatchQuota(ctx, next.ID, 7, []int64{taskID}, dayStart, hourStart, 1, 1)
	if err != nil || !allowed {
		t.Fatalf("discarded run leaked quota reservation: allowed=%v err=%v", allowed, err)
	}
}

func TestWatchRunRechecksVisibilityBeforeFinalCommit(t *testing.T) {
	ctx := context.Background()
	mem := store.NewMemoryStore()
	watchStore, bucket, _ := watchFixture(t, mem)
	checks := 0
	visibleUntilCompletion := func(_ context.Context, _ int64, postIDs []int64) (map[int64]bool, error) {
		checks++
		if checks >= 3 {
			return map[int64]bool{}, nil
		}
		return allowAllWatchPosts(ctx, 7, postIDs)
	}
	if err := scheduleBucket(ctx, mem, nil, watchStore, nil, visibleUntilCompletion, bucket, store.NowMs()); err != nil {
		t.Fatal(err)
	}
	scheduled, _ := mem.GetBucket(ctx, bucket.ID)
	run, err := mem.Claim(ctx, "watch-worker", store.NowMs(), 60_000)
	if err != nil || run == nil || run.ID != scheduled.RunID {
		t.Fatalf("run=%+v err=%v", run, err)
	}
	registry, err := tool.NewRegistry(tool.Clients{Store: mem}, []string{tool.PresentSources})
	if err != nil {
		t.Fatal(err)
	}
	engine := &Engine{Store: mem, Watch: watchStore, Tools: registry, LLM: &watchFinalStream{}, WatchPosts: visibleUntilCompletion, Window: 128000}
	engine.Execute(ctx, *run, false)
	freshRun, _ := mem.GetRun(ctx, run.ID)
	freshBucket, _ := mem.GetBucket(ctx, bucket.ID)
	if checks != 3 || freshRun.Status != store.StatusDone || freshBucket.Status != "discarded" {
		t.Fatalf("checks=%d run=%+v bucket=%+v", checks, freshRun, freshBucket)
	}
	messages, err := mem.ListSessionMessages(ctx, run.UserID, run.SessionID, true)
	if err != nil {
		t.Fatal(err)
	}
	for _, message := range messages {
		if message.Kind == store.KindWatch && message.Visible {
			t.Fatalf("invisible hit produced watch message: %+v", message)
		}
	}
	events, err := mem.ListEventsAfter(ctx, run.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	tokenIndex, resetIndex, doneIndex := -1, -1, -1
	for index, event := range events {
		switch event.Type {
		case store.EventToken:
			tokenIndex = index
		case store.EventResponseReset:
			resetIndex = index
		case store.EventDone:
			doneIndex = index
		}
	}
	if tokenIndex < 0 || resetIndex <= tokenIndex || doneIndex <= resetIndex {
		t.Fatalf("events=%+v", events)
	}
}

func TestWatchRunRechecksVisibilityBeforeEveryModelRound(t *testing.T) {
	ctx := context.Background()
	mem := store.NewMemoryStore()
	watchStore, bucket, _ := watchFixture(t, mem)
	checks := 0
	visibleUntilSecondRound := func(_ context.Context, _ int64, postIDs []int64) (map[int64]bool, error) {
		checks++
		if checks >= 3 {
			return map[int64]bool{}, nil
		}
		return allowAllWatchPosts(ctx, 7, postIDs)
	}
	if err := scheduleBucket(ctx, mem, nil, watchStore, nil, visibleUntilSecondRound, bucket, store.NowMs()); err != nil {
		t.Fatal(err)
	}
	scheduled, _ := mem.GetBucket(ctx, bucket.ID)
	run, err := mem.Claim(ctx, "watch-worker", store.NowMs(), 60_000)
	if err != nil || run == nil || run.ID != scheduled.RunID {
		t.Fatalf("run=%+v err=%v", run, err)
	}
	content := &watchPostContent{}
	registry, err := tool.NewRegistry(tool.Clients{Store: mem, Content: content}, []string{tool.GetPost})
	if err != nil {
		t.Fatal(err)
	}
	model := &scriptedLLM{replies: []llm.Result{
		{ToolCalls: []llm.ToolCall{{ID: "read", Name: tool.GetPost, Arguments: `{"post_id":99}`}}},
		{Text: "must not be delivered"},
	}}
	engine := &Engine{Store: mem, Watch: watchStore, Tools: registry, LLM: model, WatchPosts: visibleUntilSecondRound, Window: 128000}
	engine.Execute(ctx, *run, false)
	if len(model.reqs) != 1 {
		t.Fatalf("model requests=%d visibility checks=%d", len(model.reqs), checks)
	}
	freshRun, _ := mem.GetRun(ctx, run.ID)
	freshBucket, _ := mem.GetBucket(ctx, bucket.ID)
	if freshRun.Status != store.StatusDone || freshBucket.Status != "discarded" {
		t.Fatalf("run=%+v bucket=%+v", freshRun, freshBucket)
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
