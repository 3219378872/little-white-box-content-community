package llm

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"esx/app/assistant/internal/prompt"
)

const canaryTool = "assistant_capability_canary"

func Canary(ctx context.Context, client Client) error {
	if client == nil || !client.SupportsTools() {
		return fmt.Errorf("selected WireAPI must support tool schema/call/result")
	}
	def := prompt.ToolDef{
		Name: canaryTool, Description: "Return the supplied nonce without side effects.",
		Parameters: map[string]any{
			"type": "object", "additionalProperties": false,
			"properties": map[string]any{"nonce": map[string]any{"type": "string", "enum": []string{"agent-canary"}}},
			"required":   []string{"nonce"},
		},
	}
	first, err := client.Complete(ctx, Request{
		Messages: []prompt.Turn{{Role: "user", Content: "Call the capability canary exactly once with nonce agent-canary."}},
		Tools:    []prompt.ToolDef{def}, RequiredTool: canaryTool, MaxTokens: 64,
	})
	if err != nil {
		return fmt.Errorf("assistant LLM tool canary call: %w", err)
	}
	var call *ToolCall
	for i := range first.ToolCalls {
		if first.ToolCalls[i].Name == canaryTool {
			call = &first.ToolCalls[i]
			break
		}
	}
	if call == nil {
		return fmt.Errorf("assistant LLM tool canary did not return the forced tool call")
	}
	var args struct {
		Nonce string `json:"nonce"`
	}
	if json.Unmarshal([]byte(call.Arguments), &args) != nil || args.Nonce != "agent-canary" {
		return fmt.Errorf("assistant LLM tool canary returned invalid arguments")
	}
	callID := strings.TrimSpace(call.ID)
	if callID == "" {
		callID = "canary-call"
	}
	second, err := client.Complete(ctx, Request{
		Messages: []prompt.Turn{
			{Role: "user", Content: "Call the capability canary exactly once with nonce agent-canary."},
			{Role: "assistant", ToolCalls: []prompt.ToolCall{{ID: callID, Name: canaryTool, Arguments: call.Arguments}}},
			{Role: "tool", ToolCallID: callID, Name: canaryTool, Content: `{"ok":true,"nonce":"agent-canary"}`},
		},
		DisableTools: true, MaxTokens: 32,
	})
	if err != nil {
		return fmt.Errorf("assistant LLM tool-result canary: %w", err)
	}
	if strings.TrimSpace(second.Text) == "" {
		return fmt.Errorf("assistant LLM tool-result canary returned no acknowledgement")
	}
	return nil
}
