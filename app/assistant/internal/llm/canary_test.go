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
