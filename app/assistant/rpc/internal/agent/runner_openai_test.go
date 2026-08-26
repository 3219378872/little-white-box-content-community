package agent

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"esx/app/assistant/rpc/xiaobaihe/assistant/pb"
)

// fakeOpenAI 模拟 chat completions：按调用次序回放预设响应，并记录请求体。
type fakeOpenAI struct {
	server    *httptest.Server
	requests  []map[string]any
	responses []string
}

func newFakeOpenAI(t *testing.T, responses []string) *fakeOpenAI {
	fake := &fakeOpenAI{responses: responses}
	mux := http.NewServeMux()
	mux.HandleFunc("/v1/chat/completions", func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var decoded map[string]any
		_ = json.Unmarshal(body, &decoded)
		fake.requests = append(fake.requests, decoded)
		index := len(fake.requests) - 1
		if index >= len(fake.responses) {
			http.Error(w, "no scripted response", http.StatusInternalServerError)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(fake.responses[index]))
	})
	fake.server = httptest.NewServer(mux)
	t.Cleanup(fake.server.Close)
	return fake
}

func toolCallResponse(id, name, args string) string {
	payload, _ := json.Marshal(map[string]any{
		"id": "chatcmpl-1", "object": "chat.completion", "created": 1, "model": "test-model",
		"choices": []any{map[string]any{
			"index": 0,
			"message": map[string]any{
				"role": "assistant", "content": "",
				"tool_calls": []any{map[string]any{
					"id": id, "type": "function",
					"function": map[string]any{"name": name, "arguments": args},
				}},
			},
			"finish_reason": "tool_calls",
		}},
		"usage": map[string]any{"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2},
	})
	return string(payload)
}

func textResponse(text string) string {
	payload, _ := json.Marshal(map[string]any{
		"id": "chatcmpl-2", "object": "chat.completion", "created": 2, "model": "test-model",
		"choices": []any{map[string]any{
			"index": 0, "message": map[string]any{"role": "assistant", "content": text},
			"finish_reason": "stop",
		}},
		"usage": map[string]any{"prompt_tokens": 1, "completion_tokens": 1, "total_tokens": 2},
	})
	return string(payload)
}

func (f *fakeOpenAI) runner(t *testing.T) *OpenAIRunner {
	t.Helper()
	endpoint := f.server.URL + "/v1/chat/completions"
	runner, err := NewOpenAIRunner(endpoint, "key", "test-model", 8000, 256)
	if err != nil {
		t.Fatal(err)
	}
	return runner
}

func TestRunnerExecutesToolThenAnswers(t *testing.T) {
	fake := newFakeOpenAI(t, []string{
		toolCallResponse("c1", ToolSearchPosts, `{"keyword":"golang"}`),
		textResponse("社区里有相关帖子。"),
	})
	registry, err := NewToolRegistry(Clients{}, []string{ToolSearchPosts})
	if err != nil {
		t.Fatal(err)
	}
	events := make([]*pb.ChatEvent, 0, 4)
	session := &Session{
		UserID: 7, RequestID: "r", UserMessage: "找找 go 帖子",
		Budget: Budget{MaxStepsSoft: 8, MaxStepsHard: 12, MaxToolCallsPerTurn: 12, StepTimeout: 5000},
		Emit:   func(event *pb.ChatEvent) error { events = append(events, event); return nil },
		Tools:  registry,
	}
	result, err := fake.runner(t).Run(context.Background(), session)
	if err != nil {
		t.Fatal(err)
	}
	if result.Text != "社区里有相关帖子。" {
		t.Fatalf("unexpected answer %q", result.Text)
	}
	if len(events) != 1 || events[0].Type != pb.ChatEventType_CHAT_EVENT_TYPE_TOOL_CALL ||
		events[0].ToolCall.Tool != ToolSearchPosts {
		t.Fatalf("expected one TOOL_CALL event, got %+v", events)
	}
}

func TestRunnerInjectsSoftLimitNotice(t *testing.T) {
	fake := newFakeOpenAI(t, []string{
		toolCallResponse("c1", ToolSearchPosts, `{"keyword":"x"}`),
		textResponse("done"),
	})
	registry, err := NewToolRegistry(Clients{}, []string{ToolSearchPosts})
	if err != nil {
		t.Fatal(err)
	}
	session := &Session{
		UserID: 7, RequestID: "r", UserMessage: "q",
		Budget: Budget{MaxStepsSoft: 1, MaxStepsHard: 3, StepTimeout: 5000},
		Emit:   func(*pb.ChatEvent) error { return nil },
		Tools:  registry,
	}
	if _, err := fake.runner(t).Run(context.Background(), session); err != nil {
		t.Fatal(err)
	}
	if len(fake.requests) < 2 {
		t.Fatalf("expected at least two model calls, got %d", len(fake.requests))
	}
	raw, _ := json.Marshal(fake.requests[1]["messages"])
	if !strings.Contains(string(raw), "[SYSTEM] 步数预算提醒") ||
		!strings.Contains(string(raw), "还剩 2 步") {
		t.Fatalf("soft-limit notice not injected: %s", raw)
	}
}

func TestRunnerFinalizesWithoutToolsAtHardLimit(t *testing.T) {
	toolReply := toolCallResponse("c1", ToolSearchPosts, `{"keyword":"x"}`)
	fake := newFakeOpenAI(t, []string{toolReply, toolReply, textResponse("预算内收尾")})
	registry, err := NewToolRegistry(Clients{}, []string{ToolSearchPosts})
	if err != nil {
		t.Fatal(err)
	}
	session := &Session{
		UserID: 7, RequestID: "r", UserMessage: "q",
		Budget: Budget{MaxStepsSoft: 8, MaxStepsHard: 2, StepTimeout: 5000},
		Emit:   func(*pb.ChatEvent) error { return nil },
		Tools:  registry,
	}
	result, err := fake.runner(t).Run(context.Background(), session)
	if err != nil {
		t.Fatal(err)
	}
	if result.Text != "预算内收尾" {
		t.Fatalf("unexpected finalize answer %q", result.Text)
	}
	final := fake.requests[len(fake.requests)-1]
	if _, has := final["tools"]; has {
		t.Fatalf("finalize call must not carry tools")
	}
}

func TestNormalizeBaseURLMatchesLLMConvention(t *testing.T) {
	cases := map[string]string{
		"http://llm:8000":                     "http://llm:8000/v1",
		"http://llm:8000/v1":                  "http://llm:8000/v1",
		"http://llm:8000/v1/chat/completions": "http://llm:8000/v1",
	}
	for input, want := range cases {
		got, err := normalizeBaseURL(input)
		if err != nil || got != want {
			t.Fatalf("normalizeBaseURL(%q)=%q,%v want %q", input, got, err, want)
		}
	}
}
