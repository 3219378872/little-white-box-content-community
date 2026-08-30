package runtime

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"esx/app/assistant/internal/llm"
	"esx/app/assistant/internal/memory"
	"esx/app/assistant/internal/prompt"
	"esx/app/assistant/internal/store"
	"esx/app/assistant/internal/tool"
)

type retryingStream struct {
	calls int
}

type toolRoundStream struct {
	calls int
}

func (*toolRoundStream) Complete(context.Context, llm.Request) (llm.Result, error) {
	return llm.Result{}, errors.New("unexpected non-stream call")
}

func (s *toolRoundStream) CompleteStream(_ context.Context, _ llm.Request, emit func(llm.Delta) error) (llm.Result, error) {
	s.calls++
	if s.calls == 1 {
		if err := emit(llm.Delta{Text: "temporary preamble"}); err != nil {
			return llm.Result{}, err
		}
		return llm.Result{
			Text: "temporary preamble", Streamed: true,
			ToolCalls: []llm.ToolCall{{ID: "memory-1", Name: tool.GetMemory, Arguments: `{}`}},
		}, nil
	}
	if err := emit(llm.Delta{Text: "final answer"}); err != nil {
		return llm.Result{}, err
	}
	return llm.Result{Text: "final answer", Streamed: true}, nil
}

func (*toolRoundStream) SupportsTools() bool      { return true }
func (*toolRoundStream) WireAPI() string          { return llm.WireAPIResponses }
func (*toolRoundStream) MaxOutputTokens() int     { return 128 }
func (*toolRoundStream) ContextWindowTokens() int { return 128000 }
func (*toolRoundStream) RouteID() string          { return "primary" }
func (*toolRoundStream) ModelName() string        { return "m" }
func (*toolRoundStream) Boundary() string         { return "same" }
func (*toolRoundStream) SupportsStreaming() bool  { return true }

func (*retryingStream) Complete(context.Context, llm.Request) (llm.Result, error) {
	return llm.Result{}, errors.New("unexpected non-stream call")
}

func (s *retryingStream) CompleteStream(_ context.Context, _ llm.Request, emit func(llm.Delta) error) (llm.Result, error) {
	s.calls++
	if s.calls == 1 {
		if err := emit(llm.Delta{Text: "discarded"}); err != nil {
			return llm.Result{}, err
		}
		return llm.Result{}, &llm.ProviderError{Kind: llm.ErrorRateLimit, Retryable: true, Message: "retry"}
	}
	if err := emit(llm.Delta{Text: "winner"}); err != nil {
		return llm.Result{}, err
	}
	return llm.Result{Text: "winner", Streamed: true}, nil
}

func (*retryingStream) SupportsTools() bool      { return true }
func (*retryingStream) WireAPI() string          { return llm.WireAPIResponses }
func (*retryingStream) MaxOutputTokens() int     { return 128 }
func (*retryingStream) ContextWindowTokens() int { return 128000 }
func (*retryingStream) RouteID() string          { return "primary" }
func (*retryingStream) ModelName() string        { return "m" }
func (*retryingStream) Boundary() string         { return "same" }
func (*retryingStream) SupportsStreaming() bool  { return true }

func TestStreamingRetryReplayKeepsOnlyWinningAttempt(t *testing.T) {
	mem := store.NewMemoryStore()
	_, run, _ := mustStartRun(t, mem, "hello")
	registry, err := tool.NewRegistry(tool.Clients{Store: mem}, []string{tool.PresentSources})
	if err != nil {
		t.Fatal(err)
	}
	provider := &retryingStream{}
	client, err := llm.NewResilient([]llm.Route{{ID: "primary", Boundary: "same", Client: provider}}, llm.RetryOptions{
		MaxAttempts: 2, BaseDelay: time.Nanosecond, MaxDelay: time.Nanosecond,
		Sleep: func(context.Context, time.Duration) error { return nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	engine := &Engine{Store: mem, Tools: registry, LLM: client, Window: 128000}
	engine.Execute(context.Background(), run, false)

	events, err := mem.ListEventsAfter(context.Background(), run.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	var assembled strings.Builder
	tokens, resets := 0, 0
	for _, event := range events {
		var payload store.EventPayload
		_ = json.Unmarshal(event.PayloadJSON, &payload)
		switch event.Type {
		case store.EventToken:
			tokens++
			assembled.WriteString(payload.Text)
			if payload.StreamID == "" {
				t.Fatalf("token without stream id: %+v", event)
			}
		case store.EventResponseReset:
			resets++
			assembled.Reset()
		}
	}
	if assembled.String() != "winner" || tokens != 2 || resets != 1 {
		t.Fatalf("assembled=%q tokens=%d resets=%d events=%+v", assembled.String(), tokens, resets, events)
	}
	messages, _ := mem.ListSessionMessages(context.Background(), run.UserID, run.SessionID, true)
	visible := 0
	for _, message := range messages {
		if message.Role == store.RoleAssistant && message.Visible {
			visible++
			if message.Content != "winner" {
				t.Fatalf("message=%+v", message)
			}
			turn, ok := prompt.DecodeTurn(message.APIContent)
			if !ok || turn.Content != "winner" {
				t.Fatalf("api content=%s", message.APIContent)
			}
		}
	}
	if visible != 1 {
		t.Fatalf("visible=%d messages=%+v", visible, messages)
	}
}

func TestStreamingToolRoundResetsPreambleBeforeFinalAnswer(t *testing.T) {
	mem := store.NewMemoryStore()
	_, run, _ := mustStartRun(t, mem, "remember this")
	registry, err := tool.NewRegistry(tool.Clients{Memory: memory.NewMapStore()}, []string{tool.GetMemory})
	if err != nil {
		t.Fatal(err)
	}
	provider := &toolRoundStream{}
	engine := &Engine{Store: mem, Tools: registry, LLM: provider, Window: 128000}
	engine.Execute(context.Background(), run, false)

	events, err := mem.ListEventsAfter(context.Background(), run.ID, 0)
	if err != nil {
		t.Fatal(err)
	}
	var assembled strings.Builder
	resets := 0
	for _, event := range events {
		var payload store.EventPayload
		_ = json.Unmarshal(event.PayloadJSON, &payload)
		switch event.Type {
		case store.EventToken:
			assembled.WriteString(payload.Text)
		case store.EventResponseReset:
			assembled.Reset()
			resets++
		}
	}
	if assembled.String() != "final answer" || resets != 1 || provider.calls != 2 {
		t.Fatalf("assembled=%q resets=%d calls=%d events=%+v", assembled.String(), resets, provider.calls, events)
	}
}

func TestStreamWriterFencesChangedInputAndResetsPublishedText(t *testing.T) {
	mem := store.NewMemoryStore()
	_, run, _ := mustStartRun(t, mem, "hello")
	engine := &Engine{Store: mem}
	writer := newModelStreamWriter(engine, context.Background(), run)
	if err := writer.Observe(llm.AttemptEvent{Kind: llm.AttemptStart, Attempt: 1, RouteID: "primary", StreamID: "s1"}); err != nil {
		t.Fatal(err)
	}
	if err := writer.Delta(llm.Delta{Text: "old"}); err != nil {
		t.Fatal(err)
	}
	if err := mem.SetRunInput(context.Background(), run.ID, []byte(`{"text":"new"}`), store.NowMs()); err != nil {
		t.Fatal(err)
	}
	if err := writer.Delta(llm.Delta{Text: "stale"}); err != nil {
		t.Fatal(err)
	}
	if err := writer.Finish(); !errors.Is(err, errRunRedirected) {
		t.Fatalf("err=%v", err)
	}
	fresh, _ := mem.GetRun(context.Background(), run.ID)
	if err := writer.ResetWithRun(*fresh); err != nil {
		t.Fatal(err)
	}
	events, _ := mem.ListEventsAfter(context.Background(), run.ID, 0)
	if events[len(events)-1].Type != store.EventResponseReset {
		t.Fatalf("events=%+v", events)
	}
}
