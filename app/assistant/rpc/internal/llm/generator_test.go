package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestOpenAICompatibleGeneratesFromIsolatedToolContext(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer secret" {
			t.Fatalf("authorization header=%q", r.Header.Get("Authorization"))
		}
		var payload struct {
			Model    string `json:"model"`
			Messages []struct {
				Role    string `json:"role"`
				Content string `json:"content"`
			} `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
			t.Fatal(err)
		}
		if payload.Model != "model-v1" || len(payload.Messages) != 2 ||
			payload.Messages[0].Role != "system" || !strings.Contains(payload.Messages[1].Content, "TOOL_CONTEXT_JSON=") {
			t.Fatalf("unexpected payload: %#v", payload)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"grounded answer"}}],"usage":{"prompt_tokens":120,"completion_tokens":30,"total_tokens":150}}`))
	}))
	defer server.Close()
	generator, err := NewOpenAICompatible(server.URL, "secret", "model-v1", time.Second, 100, 100, 2, 8)
	if err != nil {
		t.Fatal(err)
	}
	result, err := generator.Generate(context.Background(), Request{
		UserMessage: "question", ToolName: "search", ToolResult: "trusted facts",
	})
	if err != nil || result.Text != "grounded answer" || result.Model != "model-v1" {
		t.Fatalf("result=%+v err=%v", result, err)
	}
	if result.Usage.PromptTokens != 120 || result.Usage.CompletionTokens != 30 ||
		result.Usage.TotalTokens != 150 || result.Usage.CostUSD != 0.00048 {
		t.Fatalf("unexpected usage: %+v", result.Usage)
	}
}

func TestOpenAICompatibleRejectsUpstreamAndOversizedResponses(t *testing.T) {
	t.Run("upstream failure", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
		}))
		defer server.Close()
		generator, err := NewOpenAICompatible(server.URL, "", "model-v1", time.Second, 100, 100, 0, 0)
		if err != nil {
			t.Fatal(err)
		}
		_, err = generator.Generate(context.Background(), Request{UserMessage: "q", ToolName: "search", ToolResult: "r"})
		if err == nil || !strings.Contains(err.Error(), "503") {
			t.Fatalf("error=%v want upstream status", err)
		}
	})

	t.Run("oversized response", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"too long"}}]}`))
		}))
		defer server.Close()
		generator, err := NewOpenAICompatible(server.URL, "", "model-v1", time.Second, 100, 3, 0, 0)
		if err != nil {
			t.Fatal(err)
		}
		_, err = generator.Generate(context.Background(), Request{UserMessage: "q", ToolName: "search", ToolResult: "r"})
		if err == nil || !strings.Contains(err.Error(), "output limit") {
			t.Fatalf("error=%v want output limit", err)
		}
	})
}

func TestOpenAICompatibleRejectsNegativeTokenPricing(t *testing.T) {
	_, err := NewOpenAICompatible("https://example.test", "", "model-v1", time.Second, 100, 100, -1, 0)
	if err == nil {
		t.Fatal("expected negative token pricing to be rejected")
	}
}
