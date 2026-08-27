package agent

import (
	"context"
	"testing"
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
