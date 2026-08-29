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

func TestChatCompletionsToolCall(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var payload map[string]any
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Errorf("payload: %v", err)
		}
		if _, ok := payload["tools"]; !ok {
			t.Error("chat request missing tools")
		}
		_ = json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{
				"message": map[string]any{
					"content": nil,
					"tool_calls": []map[string]any{{
						"id": "call_1", "type": "function",
						"function": map[string]any{"name": "search_posts", "arguments": `{"keyword":"go"}`},
					}},
				},
			}},
			"usage": map[string]any{"prompt_tokens": 10, "completion_tokens": 4, "total_tokens": 14},
		})
	}))
	defer server.Close()
	client, err := New(Config{Enabled: true, WireAPI: WireAPIChatCompletions, Endpoint: server.URL + "/v1", Model: "m", Timeout: time.Second, MaxOutputTokens: 128})
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.Complete(context.Background(), Request{
		Messages: []prompt.Turn{{Role: "user", Content: "hi"}},
		Tools:    []prompt.ToolDef{{Name: "search_posts", Description: "search", Parameters: map[string]any{"type": "object"}}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.ToolCalls) != 1 || result.ToolCalls[0].Name != "search_posts" {
		t.Fatalf("tool calls: %+v", result.ToolCalls)
	}
}

func TestResponsesToolCall(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "completed",
			"output": []map[string]any{
				{"type": "function_call", "call_id": "c1", "name": "present_sources", "arguments": `{"handles":["h1"]}`},
			},
			"usage": map[string]any{"input_tokens": 8, "output_tokens": 3, "total_tokens": 11},
		})
	}))
	defer server.Close()
	client, err := New(Config{Enabled: true, WireAPI: WireAPIResponses, Endpoint: server.URL + "/v1", Model: "m", Timeout: time.Second, MaxOutputTokens: 128})
	if err != nil {
		t.Fatal(err)
	}
	result, err := client.Complete(context.Background(), Request{
		Messages: []prompt.Turn{{Role: "user", Content: "hi"}},
		Tools:    []prompt.ToolDef{{Name: "present_sources", Parameters: map[string]any{"type": "object"}}},
	})
	if err != nil || len(result.ToolCalls) != 1 || result.ToolCalls[0].Name != "present_sources" {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if result.ToolCalls[0].Arguments != `{"handles":["h1"]}` {
		t.Fatalf("arguments still wrapped: %q", result.ToolCalls[0].Arguments)
	}
}

func TestDecodeResponsesArgumentsObjectOrString(t *testing.T) {
	client := &HTTPClient{cfg: Config{Model: "m"}}
	rawObject, _ := json.Marshal(map[string]any{
		"status": "completed",
		"output": []map[string]any{
			{"type": "function_call", "call_id": "c1", "name": "search_posts", "arguments": map[string]any{"keyword": "go"}},
		},
	})
	got, err := client.decodeResponses(rawObject)
	if err != nil || len(got.ToolCalls) != 1 || got.ToolCalls[0].Arguments != `{"keyword":"go"}` {
		t.Fatalf("object args=%+v err=%v", got, err)
	}
	rawString := []byte(`{"status":"completed","output":[{"type":"function_call","call_id":"c1","name":"search_posts","arguments":"{\"keyword\":\"go\"}"}]}`)
	got, err = client.decodeResponses(rawString)
	if err != nil || len(got.ToolCalls) != 1 || got.ToolCalls[0].Arguments != `{"keyword":"go"}` {
		t.Fatalf("string args=%+v err=%v", got, err)
	}
}

func TestResponsesInputAssistantUsesOutputText(t *testing.T) {
	got := responsesInput([]prompt.Turn{
		{Role: "system", Content: "sys"},
		{Role: "user", Content: "hi"},
		{Role: "assistant", Content: "hello"},
		{Role: "user", Content: "again"},
	})
	if len(got) != 4 {
		t.Fatalf("len=%d", len(got))
	}
	assertPartType := func(i int, wantRole, wantType string) {
		t.Helper()
		if got[i]["role"] != wantRole {
			t.Fatalf("item %d role=%v want %s", i, got[i]["role"], wantRole)
		}
		content, ok := got[i]["content"].([]map[string]string)
		if !ok || len(content) != 1 {
			t.Fatalf("item %d content=%T %#v", i, got[i]["content"], got[i]["content"])
		}
		if content[0]["type"] != wantType {
			t.Fatalf("item %d type=%s want %s", i, content[0]["type"], wantType)
		}
	}
	assertPartType(0, "system", "input_text")
	assertPartType(1, "user", "input_text")
	assertPartType(2, "assistant", "output_text")
	assertPartType(3, "user", "input_text")
}

func TestResponsesInputEmitsFunctionCallAndOutput(t *testing.T) {
	got := responsesInput([]prompt.Turn{
		{Role: "user", Content: "查猫粮"},
		{Role: "assistant", ToolCalls: []prompt.ToolCall{{ID: "c1", Name: "search_posts", Arguments: `{"keyword":"猫粮"}`}}},
		{Role: "tool", ToolCallID: "c1", Name: "search_posts", Content: "没有可展示的已发布帖子。"},
	})
	if len(got) != 3 {
		t.Fatalf("len=%d %#v", len(got), got)
	}
	if got[1]["type"] != "function_call" || got[1]["call_id"] != "c1" || got[1]["arguments"] != `{"keyword":"猫粮"}` {
		t.Fatalf("function_call=%#v", got[1])
	}
	if _, ok := got[1]["role"]; ok {
		t.Fatalf("function_call should not have role: %#v", got[1])
	}
	if got[2]["type"] != "function_call_output" || got[2]["call_id"] != "c1" || got[2]["output"] != "没有可展示的已发布帖子。" {
		t.Fatalf("function_call_output=%#v", got[2])
	}
}

func TestChatMessagesEmitsToolCallsAndToolRole(t *testing.T) {
	got := chatMessages([]prompt.Turn{
		{Role: "user", Content: "查猫粮"},
		{Role: "assistant", ToolCalls: []prompt.ToolCall{{ID: "c1", Name: "search_posts", Arguments: `{"keyword":"猫粮"}`}}},
		{Role: "tool", ToolCallID: "c1", Name: "search_posts", Content: "none"},
	})
	if len(got) != 3 {
		t.Fatalf("len=%d", len(got))
	}
	if got[1]["role"] != "assistant" {
		t.Fatalf("assistant=%#v", got[1])
	}
	if got[1]["content"] != nil {
		t.Fatalf("empty tool-call content want nil, got %#v", got[1]["content"])
	}
	calls, ok := got[1]["tool_calls"].([]map[string]any)
	if !ok || len(calls) != 1 || calls[0]["id"] != "c1" {
		t.Fatalf("tool_calls=%#v", got[1]["tool_calls"])
	}
	if got[2]["role"] != "tool" || got[2]["tool_call_id"] != "c1" || got[2]["content"] != "none" {
		t.Fatalf("tool=%#v", got[2])
	}
}

func TestCompleteSurfacesHTTPErrorBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"error":{"code":"invalid_value","message":"Invalid value: 'input_text'"}}`))
	}))
	defer server.Close()
	client, err := New(Config{Enabled: true, WireAPI: WireAPIResponses, Endpoint: server.URL + "/v1", Model: "m", Timeout: time.Second, MaxOutputTokens: 128})
	if err != nil {
		t.Fatal(err)
	}
	_, err = client.Complete(context.Background(), Request{Messages: []prompt.Turn{{Role: "user", Content: "hi"}}})
	if err == nil || !strings.Contains(err.Error(), "400") || !strings.Contains(err.Error(), "invalid_value") {
		t.Fatalf("err=%v", err)
	}
}

func TestCompleteSetsResponsesClientHeaders(t *testing.T) {
	var gotUA, gotBeta, gotAccept string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotUA = r.Header.Get("User-Agent")
		gotBeta = r.Header.Get("OpenAI-Beta")
		gotAccept = r.Header.Get("Accept")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"status": "completed",
			"output": []map[string]any{
				{"type": "message", "content": []map[string]string{{"type": "output_text", "text": "ok"}}},
			},
		})
	}))
	defer server.Close()
	client, err := New(Config{Enabled: true, WireAPI: WireAPIResponses, Endpoint: server.URL + "/v1", Model: "m", Timeout: time.Second, MaxOutputTokens: 128})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := client.Complete(context.Background(), Request{Messages: []prompt.Turn{{Role: "user", Content: "hi"}}}); err != nil {
		t.Fatal(err)
	}
	if gotUA != responsesUserAgent {
		t.Fatalf("User-Agent=%q", gotUA)
	}
	if gotBeta != responsesBeta {
		t.Fatalf("OpenAI-Beta=%q", gotBeta)
	}
	if gotAccept != "application/json" {
		t.Fatalf("Accept=%q", gotAccept)
	}
}

func TestDecodeResponsesIncompleteWithText(t *testing.T) {
	raw, _ := json.Marshal(map[string]any{
		"status": "incomplete",
		"output": []map[string]any{
			{"type": "reasoning", "content": []map[string]string{}},
			{"type": "message", "role": "assistant", "content": []map[string]string{{"type": "output_text", "text": "pong"}}},
		},
	})
	got, err := (&HTTPClient{cfg: Config{Model: "m"}}).decodeResponses(raw)
	if err != nil {
		t.Fatal(err)
	}
	if got.Text != "pong" {
		t.Fatalf("text=%q", got.Text)
	}
}

func TestDecodeResponsesIncompleteEmptyFails(t *testing.T) {
	raw, _ := json.Marshal(map[string]any{
		"status": "incomplete",
		"output": []map[string]any{{"type": "reasoning"}},
	})
	_, err := (&HTTPClient{cfg: Config{Model: "m"}}).decodeResponses(raw)
	if err == nil || !strings.Contains(err.Error(), "incomplete") {
		t.Fatalf("err=%v", err)
	}
}

func TestReadinessFailsIfToolsUnsupported(t *testing.T) {
	if err := Ready(Unsupported(), true); err == nil {
		t.Fatal("expected readiness failure")
	}
	if err := Ready(Unsupported(), false); err != nil {
		t.Fatal(err)
	}
}

func TestCompleteHonorsCanceledContext(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("canceled complete must not hit the provider")
	}))
	defer server.Close()
	client, err := New(Config{Enabled: true, WireAPI: WireAPIChatCompletions, Endpoint: server.URL + "/v1", Model: "m", Timeout: time.Second, MaxOutputTokens: 128})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = client.Complete(ctx, Request{Messages: []prompt.Turn{{Role: "user", Content: "hi"}}})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("err=%v", err)
	}
}
