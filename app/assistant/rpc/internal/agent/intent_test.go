package agent

import (
	"context"
	"strings"
	"testing"
	"time"

	"esx/app/assistant/watch"
)

func TestClassifyIntent(t *testing.T) {
	t.Parallel()
	cases := []struct {
		message string
		intent  string
	}{
		{"还有吗", IntentContinueTask},
		{"帮我盯这个作者", IntentWatch},
		{"推荐几个帖子", IntentRecommend},
		{"最近黑神话讨论风向怎么样", IntentCommunityOpinion},
		{"帮我发一篇帖子", IntentWritePost},
		{"hello", IntentGeneral},
	}
	for _, tc := range cases {
		plan := ClassifyIntent(tc.message)
		if plan.Intent != tc.intent {
			t.Fatalf("%q: got %s want %s", tc.message, plan.Intent, tc.intent)
		}
	}
}

func TestRestrictToolsForConsentKeepsV1Set(t *testing.T) {
	registry, err := NewToolRegistry(Clients{}, Version1Tools())
	if err != nil {
		t.Fatal(err)
	}
	if got := RestrictToolsForConsent(registry, 1); got == nil || !got.Has(ToolSearchPosts) || got.Has("recommend_posts") {
		t.Fatalf("v1 filter: %+v", got.Definitions())
	}
	if got := RestrictToolsForConsent(registry, CurrentConsentVersion); got != registry {
		t.Fatal("current version should keep original registry")
	}
}

type fakeRunner struct {
	session *Session
}

func (f *fakeRunner) Run(_ context.Context, session *Session) (*Result, error) {
	f.session = session
	return &Result{Text: "ok"}, nil
}

func TestRuntimeClassifiesAndRestricts(t *testing.T) {
	registry, err := NewToolRegistry(Clients{}, Version1Tools())
	if err != nil {
		t.Fatal(err)
	}
	executor := &fakeRunner{}
	runtime := NewRuntime(executor, nil)
	result, err := runtime.Run(context.Background(), &Session{
		UserMessage:    "还有吗",
		Tools:          registry,
		ConsentVersion: 1,
	})
	if err != nil || result.Text != "ok" {
		t.Fatalf("run: %v %+v", err, result)
	}
	if executor.session.Plan.Intent != IntentContinueTask {
		t.Fatalf("intent=%s", executor.session.Plan.Intent)
	}
	if executor.session.Tools == nil || !executor.session.Tools.Has(ToolSearchPosts) {
		t.Fatal("expected v1 tools to remain")
	}
}

func TestRuntimeInjectsUnreadWatchHits(t *testing.T) {
	store := watch.NewMapStore()
	if err := store.RecordHit(context.Background(), watch.Hit{
		UserID: 2, TaskID: 1, PostID: 11, Title: "怪猎", Summary: "新帖",
	}, "k1"); err != nil {
		t.Fatal(err)
	}
	executor := &fakeRunner{}
	runtime := NewRuntime(executor, nil)
	runtime.Watch = store
	_, err := runtime.Run(context.Background(), &Session{UserID: 2, UserMessage: "hello", Tools: mustRegistry(t)})
	if err != nil {
		t.Fatal(err)
	}
	if executor.session.SystemPrompt != "" {
		t.Fatalf("watch data must not become system instructions: %q", executor.session.SystemPrompt)
	}
	if !strings.Contains(executor.session.WatchContext, "未读的条件追踪命中") || strings.Contains(executor.session.WatchContext, "怪猎") {
		t.Fatalf("watch context must be fail-closed without content lookup: %q", executor.session.WatchContext)
	}
	if turn := executor.session.userTurnText(); !strings.Contains(turn, "UNTRUSTED_PERSONAL_CONTEXT_JSON=") {
		t.Fatalf("watch context was not serialized as untrusted user data: %q", turn)
	}
}

func TestRuntimePersistsAuditWithoutUserText(t *testing.T) {
	audit := NewMapAuditStore()
	registry, err := NewToolRegistry(Clients{}, []string{ToolSearchPosts})
	if err != nil {
		t.Fatal(err)
	}
	runtime := NewRuntime(&fakeRunner{}, nil)
	runtime.Audit = audit
	runtime.Model = "test-model"
	session := &Session{UserID: 2, RequestID: "req-9", UserMessage: "secret prompt", Tools: registry, Plan: QueryPlan{Intent: IntentRecommend}}
	_, _, _ = registry.Call(context.Background(), session, ToolSearchPosts, "c1", `{"keyword":"secret"}`)
	_, err = runtime.Run(context.Background(), session)
	if err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(time.Second)
	var runs []RunRecord
	for time.Now().Before(deadline) {
		runs = audit.Snapshot()
		if len(runs) == 1 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if len(runs) != 1 {
		t.Fatalf("runs=%d", len(runs))
	}
	if runs[0].UserID != 2 || runs[0].RequestID != "req-9" || runs[0].Status != "ok" || runs[0].Model != "test-model" {
		t.Fatalf("%+v", runs[0])
	}
	if strings.Contains(runs[0].Intent, "secret") {
		t.Fatal("intent leaked user text")
	}
	if len(runs[0].Tools) == 0 || runs[0].Tools[0].ArgDigest == `{"keyword":"secret"}` {
		t.Fatalf("expected hashed args, got %+v", runs[0].Tools)
	}
	if strings.Contains(runs[0].Tools[0].ArgDigest, "secret") {
		t.Fatalf("raw args stored: %s", runs[0].Tools[0].ArgDigest)
	}
}

func TestRuntimeNilAuditSkips(t *testing.T) {
	runtime := NewRuntime(&fakeRunner{}, nil)
	if _, err := runtime.Run(context.Background(), &Session{UserID: 1, Tools: mustRegistry(t)}); err != nil {
		t.Fatal(err)
	}
}

func mustRegistry(t *testing.T) *ToolRegistry {
	t.Helper()
	registry, err := NewToolRegistry(Clients{}, Version1Tools())
	if err != nil {
		t.Fatal(err)
	}
	return registry
}
