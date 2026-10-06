package llm

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"esx/app/assistant/internal/canonical"
	"esx/app/assistant/internal/prompt"
)

// chatCodec 实现 OpenAI Chat Completions 协议。
type chatCodec struct {
	cfg *Config
}

// endpointSuffix 是 Chat Completions 接口路径。
func (chatCodec) endpointSuffix() string { return "/chat/completions" }

// Chat Completions 没有协议特有的请求头。
func (chatCodec) setHeaders(http.Header) {}

// encode 编码 messages 形式的请求；流式请求要求上游在末尾附带 usage。
func (c chatCodec) encode(messages []prompt.Turn, req Request, maxTokens int, stream bool) ([]byte, error) {
	body := map[string]any{
		"model":      c.cfg.Model,
		"messages":   chatMessages(messages),
		"max_tokens": maxTokens,
		"stream":     stream,
	}
	if stream {
		body["stream_options"] = map[string]any{"include_usage": true}
	}
	if !req.DisableTools && len(req.Tools) > 0 {
		body["tools"] = chatTools(req.Tools)
		if strings.TrimSpace(req.RequiredTool) != "" {
			body["tool_choice"] = map[string]any{
				"type": "function", "function": map[string]any{"name": strings.TrimSpace(req.RequiredTool)},
			}
		}
	}
	return marshalBody(body)
}

// decode 取第一个 choice 的文本与工具调用；两者皆空视为空响应。
func (c chatCodec) decode(raw []byte) (Result, error) {
	var parsed struct {
		Choices []struct {
			Message struct {
				Content   string `json:"content"`
				ToolCalls []struct {
					ID       string `json:"id"`
					Type     string `json:"type"`
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"message"`
		} `json:"choices"`
		Usage chatUsage `json:"usage"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return Result{}, fmt.Errorf("decode chat completions: %w", err)
	}
	if len(parsed.Choices) == 0 {
		return Result{}, fmt.Errorf("assistant LLM returned an empty response")
	}
	msg := parsed.Choices[0].Message
	calls := make([]ToolCall, 0, len(msg.ToolCalls))
	for _, call := range msg.ToolCalls {
		if strings.TrimSpace(call.Function.Name) == "" {
			continue
		}
		calls = append(calls, ToolCall{
			ID:        call.ID,
			Name:      call.Function.Name,
			Arguments: canonical.UnwrapArgsJSON(call.Function.Arguments),
		})
	}
	text := strings.TrimSpace(msg.Content)
	if text == "" && len(calls) == 0 {
		return Result{}, fmt.Errorf("assistant LLM returned an empty response")
	}
	return Result{Text: text, ToolCalls: calls, Model: c.cfg.Model, Raw: raw, Usage: c.cfg.chatUsage(parsed.Usage)}, nil
}

// consumeEvent 累积增量文本与按 index 拼接的工具调用参数；finish_reason 标志流结束，
// length / content_filter 记为截断原因。
func (c chatCodec) consumeEvent(_ string, data []byte, state *streamState, emit func(Delta) error) error {
	var chunk struct {
		Model   string `json:"model"`
		Choices []struct {
			Delta struct {
				Content   string `json:"content"`
				ToolCalls []struct {
					Index    int    `json:"index"`
					ID       string `json:"id"`
					Function struct {
						Name      string `json:"name"`
						Arguments string `json:"arguments"`
					} `json:"function"`
				} `json:"tool_calls"`
			} `json:"delta"`
			FinishReason string `json:"finish_reason"`
		} `json:"choices"`
		Usage chatUsage `json:"usage"`
	}
	if err := json.Unmarshal(data, &chunk); err != nil {
		return fmt.Errorf("decode chat completions stream: %w", err)
	}
	if chunk.Model != "" {
		state.model = chunk.Model
	}
	for _, choice := range chunk.Choices {
		if visible := state.scrubber.Feed(choice.Delta.Content); visible != "" {
			if err := emitVisible(state, visible, emit); err != nil {
				return err
			}
		}
		for _, call := range choice.Delta.ToolCalls {
			item := state.ensureCall(call.Index, call.ID)
			if call.ID != "" {
				item.id = call.ID
			}
			item.name += call.Function.Name
			item.arguments.WriteString(call.Function.Arguments)
		}
		if choice.FinishReason != "" {
			state.terminal = true
		}
		switch choice.FinishReason {
		case "length":
			state.incomplete = "max_output_tokens"
		case "content_filter":
			state.incomplete = "content_filter"
		}
	}
	if chunk.Usage.PromptTokens > 0 || chunk.Usage.CompletionTokens > 0 {
		state.usage = c.cfg.chatUsage(chunk.Usage)
	}
	return nil
}

// chatMessages 把历史转换为 Chat 消息：工具结果用 tool 角色，带工具调用的助手消息无正文时 content 为 null。
func chatMessages(turns []prompt.Turn) []map[string]any {
	out := make([]map[string]any, 0, len(turns))
	for _, turn := range turns {
		if isToolResult(turn) {
			item := map[string]any{
				"role":         "tool",
				"tool_call_id": turn.ToolCallID,
				"content":      turn.Content,
			}
			if strings.TrimSpace(turn.Name) != "" {
				item["name"] = turn.Name
			}
			out = append(out, item)
			continue
		}
		item := map[string]any{"role": turn.Role}
		if len(turn.ToolCalls) > 0 {
			calls := make([]map[string]any, 0, len(turn.ToolCalls))
			for _, call := range turn.ToolCalls {
				args := call.Arguments
				if strings.TrimSpace(args) == "" {
					args = "{}"
				}
				calls = append(calls, map[string]any{
					"id":   call.ID,
					"type": "function",
					"function": map[string]any{
						"name":      call.Name,
						"arguments": args,
					},
				})
			}
			item["tool_calls"] = calls
			if strings.TrimSpace(turn.Content) == "" {
				item["content"] = nil
			} else {
				item["content"] = turn.Content
			}
			out = append(out, item)
			continue
		}
		item["content"] = turn.Content
		out = append(out, item)
	}
	return out
}

// chatTools 把工具定义包装为 function 工具。
func chatTools(defs []prompt.ToolDef) []map[string]any {
	out := make([]map[string]any, 0, len(defs))
	for _, def := range defs {
		out = append(out, map[string]any{
			"type": "function",
			"function": map[string]any{
				"name":        def.Name,
				"description": def.Description,
				"parameters":  def.Parameters,
			},
		})
	}
	return out
}

// chatUsage 是 Chat 协议的计量字段；缓存写入兼容 Anthropic 风格的 cache_creation_input_tokens。
type chatUsage struct {
	PromptTokens     int64 `json:"prompt_tokens"`
	CompletionTokens int64 `json:"completion_tokens"`
	TotalTokens      int64 `json:"total_tokens"`
	PromptDetails    struct {
		CachedTokens     int64 `json:"cached_tokens"`
		CacheWriteTokens int64 `json:"cache_write_tokens"`
	} `json:"prompt_tokens_details"`
	CompletionDetails struct {
		ReasoningTokens int64 `json:"reasoning_tokens"`
	} `json:"completion_tokens_details"`
	CacheCreationInputTokens int64 `json:"cache_creation_input_tokens"`
}

// chatUsage 把 Chat 计量换算为统一的 Usage。
func (cfg *Config) chatUsage(raw chatUsage) Usage {
	cacheWrite := raw.PromptDetails.CacheWriteTokens
	if cacheWrite == 0 {
		cacheWrite = raw.CacheCreationInputTokens
	}
	return cfg.usage(raw.PromptTokens, raw.CompletionTokens, raw.TotalTokens, raw.PromptDetails.CachedTokens, cacheWrite, raw.CompletionDetails.ReasoningTokens)
}
