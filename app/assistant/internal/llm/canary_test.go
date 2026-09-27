package llm

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestCanaryExercisesToolCallAndToolResultReplay(t *testing.T) {
	for _, wireAPI := range []string{WireAPIChatCompletions, WireAPIResponses} {
		t.Run(wireAPI, func(t *testing.T) {
			calls := 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls++
				var body map[string]any
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Fatal(err)
				}
				w.Header().Set("Content-Type", "application/json")
				budget := body["max_tokens"]
				if wireAPI == WireAPIResponses {
					budget = body["max_output_tokens"]
				}
				// A reasoning provider may spend the first 100 tokens internally.
				// Returning no visible payload models the live token-exhaustion failure.
				if n, _ := budget.(float64); n < 128 {
					_ = json.NewEncoder(w).Encode(map[string]any{"status": "incomplete", "output": []any{}, "choices": []any{map[string]any{"message": map[string]any{"content": ""}}}})
					return
				}
				switch calls {
				case 1:
					if _, ok := body["tools"]; !ok {
						t.Fatal("first canary round did not advertise the forced tool")
					}
					if wireAPI == WireAPIResponses {
						_ = json.NewEncoder(w).Encode(map[string]any{
							"status": "completed",
							"output": []map[string]any{{
								"type": "function_call", "call_id": "canary-1", "name": canaryTool,
								"arguments": `{"nonce":"agent-canary"}`,
							}},
						})
						return
					}
					_ = json.NewEncoder(w).Encode(map[string]any{
						"choices": []map[string]any{{"message": map[string]any{
							"tool_calls": []map[string]any{{
								"id": "canary-1", "type": "function",
								"function": map[string]any{"name": canaryTool, "arguments": `{"nonce":"agent-canary"}`},
							}},
						}}},
					})
				case 2:
					if _, ok := body["tools"]; ok {
						t.Fatal("tool-result replay round must not allow another tool call")
					}
					if wireAPI == WireAPIResponses {
						input, _ := json.Marshal(body["input"])
						if !containsJSONText(input, "function_call_output") {
							t.Fatalf("responses replay input=%s", input)
						}
						_ = json.NewEncoder(w).Encode(map[string]any{
							"status": "completed",
							"output": []map[string]any{{
								"type": "message", "content": []map[string]string{{"type": "output_text", "text": "ack"}},
							}},
						})
						return
					}
					messages, _ := json.Marshal(body["messages"])
					if !containsJSONText(messages, `"role":"tool"`) {
						t.Fatalf("chat replay messages=%s", messages)
					}
					_ = json.NewEncoder(w).Encode(map[string]any{
						"choices": []map[string]any{{"message": map[string]any{"content": "ack"}}},
					})
				default:
					t.Fatalf("unexpected canary request %d", calls)
				}
			}))
			defer server.Close()

			client := mustHTTPClient(t, Config{
				Enabled: true, WireAPI: wireAPI, Endpoint: server.URL + "/v1", Model: "m",
				Timeout: time.Second, MaxOutputTokens: 128,
			})
			if err := Canary(context.Background(), client); err != nil {
				t.Fatal(err)
			}
			if calls != 2 {
				t.Fatalf("canary calls=%d", calls)
			}
		})
	}
}

func containsJSONText(raw []byte, want string) bool {
	for i := 0; i+len(want) <= len(raw); i++ {
		if string(raw[i:i+len(want)]) == want {
			return true
		}
	}
	return false
}

type canaryResultClient struct {
	Client
	results []Result
}

func (c *canaryResultClient) SupportsTools() bool { return true }
func (c *canaryResultClient) Complete(context.Context, Request) (Result, error) {
	r := c.results[0]
	c.results = c.results[1:]
	return r, nil
}
func TestCanaryStillRejectsMissingOrInvalidProtocol(t *testing.T) {
	valid := Result{ToolCalls: []ToolCall{{ID: "call", Name: canaryTool, Arguments: `{"nonce":"agent-canary"}`}}}
	for _, tc := range []struct {
		name    string
		results []Result
	}{
		{"missing tool", []Result{{Text: "ack"}}},
		{"wrong nonce", []Result{{ToolCalls: []ToolCall{{Name: canaryTool, Arguments: `{"nonce":"wrong"}`}}}}},
		{"empty acknowledgement", []Result{valid, {Text: " "}}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if Canary(context.Background(), &canaryResultClient{results: tc.results}) == nil {
				t.Fatal("invalid protocol passed readiness")
			}
		})
	}
}
