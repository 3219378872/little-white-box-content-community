package llm

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"esx/app/assistant/internal/prompt"
)

func TestChatStreamAggregatesTextToolsUsage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var request map[string]any
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			t.Fatal(err)
		}
		if request["stream"] != true {
			t.Fatalf("stream=%v", request["stream"])
		}
		w.Header().Set("Content-Type", "text/event-stream")
		writeSSE(t, w, "", map[string]any{"model": "m-live", "choices": []map[string]any{{"delta": map[string]any{
			"content": "hel", "tool_calls": []map[string]any{{"index": 0, "id": "c1", "function": map[string]any{"name": "search_", "arguments": `{"key"`}}},
		}}}})
		writeSSE(t, w, "", map[string]any{"choices": []map[string]any{{"delta": map[string]any{
			"content": "lo", "tool_calls": []map[string]any{{"index": 0, "function": map[string]any{"name": "posts", "arguments": `:"go"}`}}},
		}, "finish_reason": "tool_calls"}}, "usage": map[string]any{
			"prompt_tokens": 10, "completion_tokens": 6, "total_tokens": 16,
			"prompt_tokens_details":     map[string]any{"cached_tokens": 4, "cache_write_tokens": 2},
			"completion_tokens_details": map[string]any{"reasoning_tokens": 3},
		}})
		_, _ = io.WriteString(w, "data: [DONE]\n\n")
	}))
	defer server.Close()
	client := mustHTTPClient(t, Config{Enabled: true, WireAPI: WireAPIChatCompletions, Endpoint: server.URL + "/v1", Model: "m", MaxOutputTokens: 128})
	var visible strings.Builder
	result, err := client.CompleteStream(context.Background(), Request{Messages: []prompt.Turn{{Role: "user", Content: "hi"}}}, func(delta Delta) error {
		visible.WriteString(delta.Text)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if visible.String() != "hello" || result.Text != "hello" || result.Model != "m-live" {
		t.Fatalf("visible=%q result=%+v", visible.String(), result)
	}
	if len(result.ToolCalls) != 1 || result.ToolCalls[0].Name != "search_posts" || result.ToolCalls[0].Arguments != `{"key":"go"}` {
		t.Fatalf("calls=%+v", result.ToolCalls)
	}
	if result.Usage.CacheTokens != 4 || result.Usage.CacheWriteTokens != 2 || result.Usage.ReasoningTokens != 3 {
		t.Fatalf("usage=%+v", result.Usage)
	}
}

func TestResponsesStreamScrubsSplitSidecarAndToolArguments(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		writeSSE(t, w, "response.output_text.delta", map[string]any{"type": "response.output_text.delta", "delta": "before\n<untrusted-mem"})
		writeSSE(t, w, "response.output_text.delta", map[string]any{"type": "response.output_text.delta", "delta": "ory-context>\nsecret"})
		writeSSE(t, w, "response.output_text.delta", map[string]any{"type": "response.output_text.delta", "delta": "\n</untrusted-memory-context>\nafter"})
		writeSSE(t, w, "response.output_item.added", map[string]any{"type": "response.output_item.added", "output_index": 1, "item": map[string]any{"type": "function_call", "id": "i1", "call_id": "c1", "name": "get_post", "arguments": ""}})
		writeSSE(t, w, "response.function_call_arguments.delta", map[string]any{"type": "response.function_call_arguments.delta", "item_id": "i1", "output_index": 1, "delta": `{"post_`})
		writeSSE(t, w, "response.function_call_arguments.delta", map[string]any{"type": "response.function_call_arguments.delta", "item_id": "i1", "output_index": 1, "delta": `id":9}`})
		writeSSE(t, w, "response.completed", map[string]any{"type": "response.completed", "response": map[string]any{
			"status": "completed", "output": []map[string]any{{"type": "function_call", "call_id": "c1", "name": "get_post", "arguments": `{"post_id":9}`}},
			"usage": map[string]any{"input_tokens": 8, "output_tokens": 3, "total_tokens": 11, "input_tokens_details": map[string]any{"cached_tokens": 2}, "output_tokens_details": map[string]any{"reasoning_tokens": 1}},
		}})
	}))
	defer server.Close()
	client := mustHTTPClient(t, Config{Enabled: true, WireAPI: WireAPIResponses, Endpoint: server.URL + "/v1", Model: "m", MaxOutputTokens: 128})
	var visible strings.Builder
	result, err := client.CompleteStream(context.Background(), Request{Messages: []prompt.Turn{{Role: "user", Content: "hi"}}}, func(delta Delta) error {
		visible.WriteString(delta.Text)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if visible.String() != "before\n\nafter" || strings.Contains(visible.String(), "secret") {
		t.Fatalf("visible=%q", visible.String())
	}
	if len(result.ToolCalls) != 1 || result.ToolCalls[0].Arguments != `{"post_id":9}` {
		t.Fatalf("calls=%+v", result.ToolCalls)
	}
	if result.Usage.CacheTokens != 2 || result.Usage.ReasoningTokens != 1 {
		t.Fatalf("usage=%+v", result.Usage)
	}
}

func TestHTTPErrorClassifiesRetryAfter(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Retry-After", "2")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = io.WriteString(w, `{"error":{"message":"slow down"}}`)
	}))
	defer server.Close()
	client := mustHTTPClient(t, Config{Enabled: true, Endpoint: server.URL + "/v1", Model: "m", MaxOutputTokens: 16})
	_, err := client.Complete(context.Background(), Request{Messages: []prompt.Turn{{Role: "user", Content: "hi"}}})
	var providerErr *ProviderError
	if !errors.As(err, &providerErr) || providerErr.Kind != ErrorRateLimit || !providerErr.Retryable || providerErr.RetryAfter != 2*time.Second {
		t.Fatalf("err=%T %+v", err, providerErr)
	}
}

func TestHTTPErrorTaxonomy(t *testing.T) {
	tests := []struct {
		name      string
		status    int
		body      string
		kind      ErrorKind
		retryable bool
	}{
		{name: "auth", status: http.StatusUnauthorized, body: `{"error":"bad token"}`, kind: ErrorAuth},
		{name: "invalid", status: http.StatusUnprocessableEntity, body: `{"error":"bad field"}`, kind: ErrorInvalidRequest},
		{name: "context", status: http.StatusBadRequest, body: `{"error":"context_length_exceeded"}`, kind: ErrorContextOverflow},
		{name: "policy", status: http.StatusForbidden, body: `{"error":"content_policy"}`, kind: ErrorContentPolicy},
		{name: "timeout", status: http.StatusRequestTimeout, body: `{}`, kind: ErrorTimeout, retryable: true},
		{name: "server", status: http.StatusInternalServerError, body: `{"error":"context window backend failed"}`, kind: ErrorServer, retryable: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := classifyHTTPError(tt.status, nil, []byte(tt.body))
			if err.Kind != tt.kind || err.Retryable != tt.retryable || strings.Contains(err.Error(), tt.body) {
				t.Fatalf("error=%+v", err)
			}
		})
	}
}

func TestRetryAfterParsesHTTPDate(t *testing.T) {
	now := time.Date(2026, time.August, 30, 12, 0, 0, 0, time.UTC)
	value := now.Add(7 * time.Second).Format(http.TimeFormat)
	if got := parseRetryAfter(value, now); got != 7*time.Second {
		t.Fatalf("retry after=%v", got)
	}
}

func TestTruncatedChatStreamIsRetryable(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		writeSSE(t, w, "", map[string]any{"choices": []map[string]any{{"delta": map[string]any{"content": "partial"}}}})
	}))
	defer server.Close()
	client := mustHTTPClient(t, Config{Enabled: true, WireAPI: WireAPIChatCompletions, Endpoint: server.URL + "/v1", Model: "m", MaxOutputTokens: 128})
	var visible strings.Builder
	_, err := client.CompleteStream(context.Background(), Request{}, func(delta Delta) error {
		visible.WriteString(delta.Text)
		return nil
	})
	var providerErr *ProviderError
	if !errors.As(err, &providerErr) || !providerErr.Retryable || visible.String() != "partial" {
		t.Fatalf("visible=%q err=%+v", visible.String(), providerErr)
	}
}

type streamStub struct {
	id       string
	boundary string
	model    string
	results  []error
	text     string
	calls    int
}

func (s *streamStub) Complete(context.Context, Request) (Result, error) {
	return Result{}, errors.New("unexpected non-stream call")
}
func (s *streamStub) CompleteStream(_ context.Context, _ Request, emit func(Delta) error) (Result, error) {
	s.calls++
	if s.text != "" {
		if err := emit(Delta{Text: s.text}); err != nil {
			return Result{}, err
		}
	}
	if s.calls <= len(s.results) && s.results[s.calls-1] != nil {
		return Result{}, s.results[s.calls-1]
	}
	return Result{Text: s.text, Streamed: true}, nil
}
func (*streamStub) SupportsTools() bool      { return true }
func (*streamStub) WireAPI() string          { return WireAPIResponses }
func (*streamStub) MaxOutputTokens() int     { return 128 }
func (*streamStub) ContextWindowTokens() int { return 128000 }
func (s *streamStub) RouteID() string        { return s.id }
func (s *streamStub) ModelName() string {
	if s.model == "" {
		return "m"
	}
	return s.model
}
func (s *streamStub) Boundary() string      { return s.boundary }
func (*streamStub) SupportsStreaming() bool { return true }

func TestResilientStreamResetsFailedAttemptsAndFallsBack(t *testing.T) {
	transient := &ProviderError{Kind: ErrorRateLimit, Retryable: true, RetryAfter: time.Second, Message: "rate limited"}
	primary := &streamStub{id: "primary", boundary: "same", text: "old", results: []error{transient, transient}}
	fallback := &streamStub{id: "fallback", boundary: "same", text: "new"}
	var waits []time.Duration
	client, err := NewResilient([]Route{{ID: "primary", Boundary: "same", Client: primary}, {ID: "fallback", Boundary: "same", Client: fallback}}, RetryOptions{
		MaxAttempts: 3, BaseDelay: time.Millisecond, MaxDelay: time.Millisecond, MaxRetryAfter: 2 * time.Second,
		Sleep: func(_ context.Context, delay time.Duration) error { waits = append(waits, delay); return nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	var visible strings.Builder
	result, err := client.CompleteStream(context.Background(), Request{AttemptPrefix: "run", Observer: func(event AttemptEvent) error {
		if event.Kind == AttemptReset {
			visible.Reset()
		}
		return nil
	}}, func(delta Delta) error { visible.WriteString(delta.Text); return nil })
	if err != nil {
		t.Fatal(err)
	}
	if visible.String() != "new" || result.Text != "new" || result.Attempts != 3 || primary.calls != 2 || fallback.calls != 1 {
		t.Fatalf("visible=%q result=%+v calls=%d/%d", visible.String(), result, primary.calls, fallback.calls)
	}
	if len(waits) != 2 || waits[0] < time.Second {
		t.Fatalf("waits=%v", waits)
	}
}

func TestFrozenCapabilityUsesOnlySnapshottedFallbacks(t *testing.T) {
	transient := &ProviderError{Kind: ErrorServer, Retryable: true, Message: "retry"}
	primary := &streamStub{id: "primary", boundary: "same", results: []error{transient, transient}}
	addedLater := &streamStub{id: "new-route", boundary: "same", text: "wrong"}
	frozenFallback := &streamStub{id: "frozen-fallback", boundary: "same", text: "winner"}
	router, err := NewResilient([]Route{
		{ID: "primary", Boundary: "same", Client: primary},
		{ID: "new-route", Boundary: "same", Client: addedLater},
		{ID: "frozen-fallback", Boundary: "same", Client: frozenFallback},
	}, RetryOptions{
		MaxAttempts: 3, BaseDelay: time.Nanosecond, MaxDelay: time.Nanosecond,
		Sleep: func(context.Context, time.Duration) error { return nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	client, ok := SelectCapability(router, prompt.ProviderCapability{
		RouteID: "primary", FallbackRouteIDs: []string{"frozen-fallback"}, WireAPI: WireAPIResponses,
		Model: "m", ContextTokens: 128000, MaxOutputTokens: 128, Streaming: true, Tools: true, Boundary: "same",
	})
	if !ok {
		t.Fatal("frozen capability was not selectable")
	}
	result, err := client.(StreamingClient).CompleteStream(context.Background(), Request{}, func(Delta) error { return nil })
	if err != nil || result.Text != "winner" || addedLater.calls != 0 || frozenFallback.calls != 1 {
		t.Fatalf("result=%+v err=%v calls new/frozen=%d/%d", result, err, addedLater.calls, frozenFallback.calls)
	}
}

func TestFrozenCapabilityRejectsChangedModelOrBoundary(t *testing.T) {
	for _, route := range []*streamStub{
		{id: "primary", model: "changed", boundary: "same"},
		{id: "primary", model: "m", boundary: "changed"},
	} {
		frozen := prompt.ProviderCapability{
			RouteID: "primary", WireAPI: WireAPIResponses, Model: "m", ContextTokens: 128000,
			MaxOutputTokens: 128, Streaming: true, Tools: true, Boundary: "same",
		}
		if _, ok := SelectCapability(route, frozen); ok {
			t.Fatalf("changed route was accepted: %+v", route)
		}
	}
}

func TestDeterministicProviderErrorDoesNotRetry(t *testing.T) {
	invalid := &ProviderError{Kind: ErrorInvalidRequest, Retryable: false, Message: "invalid"}
	primary := &streamStub{id: "primary", boundary: "same", results: []error{invalid}}
	client, err := NewResilient([]Route{{ID: "primary", Boundary: "same", Client: primary}}, RetryOptions{
		MaxAttempts: 3, BaseDelay: time.Nanosecond, MaxDelay: time.Nanosecond,
		Sleep: func(context.Context, time.Duration) error { return nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.CompleteStream(context.Background(), Request{}, func(Delta) error { return nil })
	if err == nil || primary.calls != 1 {
		t.Fatalf("err=%v calls=%d", err, primary.calls)
	}
}

func mustHTTPClient(t *testing.T, cfg Config) *HTTPClient {
	t.Helper()
	if cfg.Timeout == 0 {
		cfg.Timeout = time.Second
	}
	client, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	return client
}

func writeSSE(t *testing.T, w io.Writer, event string, payload any) {
	t.Helper()
	raw, err := json.Marshal(payload)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.WriteString(w, "event: "+event+"\ndata: "+string(raw)+"\n\n")
}
