package llm

import (
	"context"
	"encoding/json"
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

func TestReadinessFailsIfToolsUnsupported(t *testing.T) {
	if err := Ready(Unsupported(), true); err == nil {
		t.Fatal("expected readiness failure")
	}
	if err := Ready(Unsupported(), false); err != nil {
		t.Fatal(err)
	}
}
