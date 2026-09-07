package runtime

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"esx/app/assistant/internal/llm"
	"esx/app/assistant/internal/store"
	"esx/app/assistant/internal/tool"
)

func TestWatchResearchDelivery(t *testing.T) {
	for _, tc := range []struct {
		name   string
		status string
		bucket string
	}{
		{"published", store.StatusDone, "sent"},
		{"unstructured", store.StatusError, "deferred"},
		{"cancelled", store.StatusCancelled, "pending"},
		{"invisible", store.StatusDone, "discarded"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			mem := store.NewMemoryStore()
			watches, bucket, taskID := watchFixture(t, mem)
			now := store.NowMs()
			if err := scheduleBucket(ctx, mem, nil, watches, nil, allowAllWatchPosts, bucket, now); err != nil {
				t.Fatal(err)
			}
			run, err := mem.Claim(ctx, "watch-research", now, 60_000)
			if err != nil || run == nil {
				t.Fatalf("claim=%+v err=%v", run, err)
			}
			registry, err := tool.NewRegistry(tool.Clients{Store: mem, Content: &watchPostContent{}}, nil)
			if err != nil {
				t.Fatal(err)
			}
			visible := true
			round := 0
			model := scriptLLM{complete: func(ctx context.Context, req llm.Request) (llm.Result, error) {
				round++
				if !req.SuppressText {
					t.Fatal("Watch v2 exposed unvalidated model text")
				}
				for _, def := range req.Tools {
					if def.Name == tool.PresentSources || def.Name == tool.AskQuestions {
						t.Fatalf("unexpected Watch v2 tool %s", def.Name)
					}
				}
				if round == 1 {
					for _, turn := range req.Messages {
						if strings.Contains(turn.Content, watchHitsMarker) && strings.Contains(turn.Content, tool.PresentSources) {
							t.Fatal("Watch input requires an unavailable legacy presentation tool")
						}
					}
					return llm.Result{ToolCalls: []llm.ToolCall{{ID: "get", Name: tool.GetPost, Arguments: `{"post_id":99}`}}}, nil
				}
				sources, err := mem.ListSources(ctx, run.ID)
				if err != nil || len(sources) != 1 {
					t.Fatalf("sources=%+v err=%v", sources, err)
				}
				if round == 2 {
					args, _ := json.Marshal(map[string]any{"handle": sources[0].Handle})
					return llm.Result{ToolCalls: []llm.ToolCall{{ID: "read", Name: tool.ReadSource, Arguments: string(args)}}}, nil
				}
				if round != 3 {
					t.Fatalf("unexpected model round %d: last turn=%+v", round, req.Messages[len(req.Messages)-1])
				}
				if tc.name == "unstructured" {
					return llm.Result{Text: "unvalidated answer"}, nil
				}
				if tc.name == "cancelled" {
					if err := mem.RequestCancel(ctx, run.UserID, run.ID); err != nil {
						t.Fatal(err)
					}
				}
				visible = tc.name != "invisible"
				evidence, err := mem.ListEvidence(ctx, run.ID, sources[0].Handle)
				if err != nil || len(evidence) == 0 {
					t.Fatalf("evidence=%+v err=%v", evidence, err)
				}
				args, _ := json.Marshal(map[string]any{"blocks": []map[string]any{{
					"kind": "fact", "text": evidence[0].Text,
					"citations": []store.AnswerCitation{{Handle: sources[0].Handle, EvidenceIDs: []string{evidence[0].ID}}},
				}}})
				return llm.Result{ToolCalls: []llm.ToolCall{{ID: "publish", Name: tool.PublishAnswer, Arguments: string(args)}}}, nil
			}}
			engine := &Engine{Store: mem, Watch: watches, Tools: registry, LLM: model, Window: 128000,
				WatchPosts: func(ctx context.Context, userID int64, ids []int64) (map[int64]bool, error) {
					if !visible {
						return map[int64]bool{}, nil
					}
					return allowAllWatchPosts(ctx, userID, ids)
				}}
			engine.Execute(ctx, *run, false)
			fresh, err := mem.GetRun(ctx, run.ID)
			if err != nil || fresh.Status != tc.status || round != 3 {
				t.Fatalf("run=%+v rounds=%d err=%v", fresh, round, err)
			}
			if tc.name == "unstructured" && fresh.ErrorCode != "ANSWER_VALIDATION_FAILED" {
				t.Fatalf("error code=%s", fresh.ErrorCode)
			}
			finished, err := mem.GetBucket(ctx, bucket.ID)
			if err != nil || finished.Status != tc.bucket {
				t.Fatalf("bucket=%+v err=%v", finished, err)
			}
			wantCount := int64(0)
			if tc.name == "published" {
				wantCount = 1
			}
			thread, err := mem.GetThread(ctx, run.UserID)
			if err != nil || int64(thread.UnreadCount) != wantCount {
				t.Fatalf("thread=%+v err=%v", thread, err)
			}
			messages, err := mem.ListSessionMessages(ctx, run.UserID, run.SessionID, true)
			if err != nil {
				t.Fatal(err)
			}
			var delivered int64
			for _, message := range messages {
				if !message.Visible {
					continue
				}
				delivered++
				answer, err := mem.GetPresentation(ctx, message.ID)
				if err != nil || answer == nil || len(answer.Sources) != 1 || message.Kind != store.KindWatch || !message.Unread {
					t.Fatalf("message=%+v answer=%+v err=%v", message, answer, err)
				}
			}
			if delivered != wantCount {
				t.Fatalf("delivered=%d want=%d", delivered, wantCount)
			}
			outbox, err := mem.ListUnpublishedOutbox(ctx, 10)
			if err != nil || int64(len(outbox)) != wantCount {
				t.Fatalf("outbox=%+v err=%v", outbox, err)
			}
			events, err := mem.ListEventsAfter(ctx, run.ID, 0)
			if err != nil {
				t.Fatal(err)
			}
			var committed int64
			for _, event := range events {
				if event.Type == store.EventAnswerCommitted {
					committed++
				}
				if event.Type == store.EventToken {
					t.Fatal("Watch v2 emitted uncommitted answer text")
				}
			}
			if committed != wantCount {
				t.Fatalf("answer_committed=%d want=%d", committed, wantCount)
			}
			for _, quota := range []struct {
				taskID int64
				kind   string
				window time.Duration
			}{{0, "day", 24 * time.Hour}, {taskID, "hour", time.Hour}} {
				start := now / quota.window.Milliseconds() * quota.window.Milliseconds()
				count, err := mem.CountSent(ctx, run.UserID, quota.taskID, quota.kind, start)
				if err != nil || int64(count) != wantCount {
					t.Fatalf("%s count=%d err=%v", quota.kind, count, err)
				}
			}
			if wantCount == 0 {
				next, err := mem.UpsertDeliveryBucket(ctx, run.UserID, 999, bucket.WindowStartMs+watchWindow.Milliseconds(), now)
				if err != nil {
					t.Fatal(err)
				}
				day := now / (24 * time.Hour).Milliseconds() * (24 * time.Hour).Milliseconds()
				hour := now / time.Hour.Milliseconds() * time.Hour.Milliseconds()
				allowed, _, err := mem.ReserveWatchQuota(ctx, next.ID, run.UserID, []int64{taskID}, day, hour, 1, 1)
				if err != nil || !allowed {
					t.Fatalf("failed delivery leaked reservation: allowed=%v err=%v", allowed, err)
				}
			}
		})
	}
}
