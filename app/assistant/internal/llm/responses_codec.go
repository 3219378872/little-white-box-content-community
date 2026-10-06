package llm

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"esx/app/assistant/internal/prompt"
)

// responsesCodec 实现 OpenAI Responses 协议。
type responsesCodec struct {
	cfg *Config
}

func (responsesCodec) endpointSuffix() string { return "/responses" }

// setHeaders 声明 Responses beta 版本，部分兼容上游据此启用该协议。
func (responsesCodec) setHeaders(header http.Header) {
	header.Set("OpenAI-Beta", responsesBeta)
}

// encode 编码 input 形式的请求；store=false 让上游不保存对话。
func (c responsesCodec) encode(messages []prompt.Turn, req Request, maxTokens int, stream bool) ([]byte, error) {
	body := map[string]any{
		"model":             c.cfg.Model,
		"input":             responsesInput(messages),
		"max_output_tokens": maxTokens,
		"stream":            stream,
		"store":             false,
	}
	if !req.DisableTools && len(req.Tools) > 0 {
		body["tools"] = responsesTools(req.Tools)
		if strings.TrimSpace(req.RequiredTool) != "" {
			body["tool_choice"] = map[string]any{"type": "function", "name": strings.TrimSpace(req.RequiredTool)}
		}
	}
	return marshalBody(body)
}

// decode 汇总 output 中的消息文本与函数调用（缺文本时退回 output_text）。incomplete 状态返回部分结果与原因，
// 其他非 completed 状态视为失败。
func (c responsesCodec) decode(raw []byte) (Result, error) {
	var parsed struct {
		Status            string `json:"status"`
		IncompleteDetails struct {
			Reason string `json:"reason"`
		} `json:"incomplete_details"`
		Output []struct {
			Type    string `json:"type"`
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
			ID        string          `json:"id"`
			CallID    string          `json:"call_id"`
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		} `json:"output"`
		OutputText string         `json:"output_text"`
		Usage      responsesUsage `json:"usage"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return Result{}, fmt.Errorf("decode responses: %w", err)
	}
	var texts []string
	var calls []ToolCall
	for _, item := range parsed.Output {
		switch item.Type {
		case "message":
			for _, part := range item.Content {
				if strings.TrimSpace(part.Text) != "" {
					texts = append(texts, part.Text)
				}
			}
		case "function_call", "tool_call":
			id := strings.TrimSpace(item.CallID)
			if id == "" {
				id = strings.TrimSpace(item.ID)
			}
			calls = append(calls, ToolCall{ID: id, Name: item.Name, Arguments: normalizeToolArguments(item.Arguments)})
		}
	}
	text := strings.TrimSpace(strings.Join(texts, ""))
	if text == "" {
		text = strings.TrimSpace(parsed.OutputText)
	}
	usable := text != "" || len(calls) > 0
	if parsed.Status != "" && parsed.Status != "completed" {
		if parsed.Status != "incomplete" {
			return Result{}, fmt.Errorf("assistant LLM response status=%s", parsed.Status)
		}
		reason := strings.TrimSpace(parsed.IncompleteDetails.Reason)
		if reason == "" {
			reason = "unknown"
		}
		return Result{
			Text: text, ToolCalls: calls, Model: c.cfg.Model, Raw: raw,
			Usage:            c.cfg.responsesUsage(parsed.Usage),
			IncompleteReason: reason,
		}, nil
	}
	if !usable {
		return Result{}, fmt.Errorf("assistant LLM returned an empty response")
	}
	return Result{Text: text, ToolCalls: calls, Model: c.cfg.Model, Raw: raw, Usage: c.cfg.responsesUsage(parsed.Usage)}, nil
}

// consumeEvent 处理 Responses 流事件：文本增量、函数调用的创建与参数增量、终态与错误事件。
// 终态事件携带完整响应，用于补齐计量，以及在没有增量时补发文本与工具调用。
func (c responsesCodec) consumeEvent(event string, data []byte, state *streamState, emit func(Delta) error) error {
	var envelope struct {
		Type        string          `json:"type"`
		Code        string          `json:"code"`
		Message     string          `json:"message"`
		Delta       string          `json:"delta"`
		Text        string          `json:"text"`
		ItemID      string          `json:"item_id"`
		OutputIndex int             `json:"output_index"`
		Name        string          `json:"name"`
		Arguments   json.RawMessage `json:"arguments"`
		Item        struct {
			Type      string          `json:"type"`
			ID        string          `json:"id"`
			CallID    string          `json:"call_id"`
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		} `json:"item"`
		Response json.RawMessage `json:"response"`
	}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return fmt.Errorf("decode responses stream: %w", err)
	}
	if event == "" {
		event = envelope.Type
	}
	switch event {
	case "response.output_text.delta":
		if visible := state.scrubber.Feed(envelope.Delta); visible != "" {
			return emitVisible(state, visible, emit)
		}
	case "response.output_item.added":
		if envelope.Item.Type == "function_call" || envelope.Item.Type == "tool_call" {
			key := envelope.Item.ID
			if key == "" {
				key = envelope.Item.CallID
			}
			item := state.ensureResponseCall(key, envelope.OutputIndex)
			item.id = firstNonEmpty(envelope.Item.CallID, envelope.Item.ID)
			item.name = envelope.Item.Name
			if args := normalizeToolArguments(envelope.Item.Arguments); args != "{}" {
				item.arguments.WriteString(args)
			}
		}
	case "response.function_call_arguments.delta":
		item := state.ensureResponseCall(envelope.ItemID, envelope.OutputIndex)
		item.arguments.WriteString(envelope.Delta)
	case "response.function_call_arguments.done":
		item := state.ensureResponseCall(envelope.ItemID, envelope.OutputIndex)
		if envelope.Name != "" {
			item.name = envelope.Name
		}
		if len(envelope.Arguments) > 0 {
			item.arguments.Reset()
			item.arguments.WriteString(normalizeToolArguments(envelope.Arguments))
		}
	case "response.completed", "response.incomplete":
		state.terminal = true
		if len(envelope.Response) > 0 {
			final, err := c.decode(envelope.Response)
			if err != nil {
				return err
			}
			state.usage = final.Usage
			state.model = final.Model
			state.incomplete = final.IncompleteReason
			if !state.emitted && final.Text != "" {
				if visible := state.scrubber.Feed(final.Text); visible != "" {
					if err := emitVisible(state, visible, emit); err != nil {
						return err
					}
				}
			}
			if len(state.calls) == 0 {
				for i, call := range final.ToolCalls {
					item := state.ensureCall(i, call.ID)
					item.name = call.Name
					item.arguments.WriteString(call.Arguments)
				}
			}
		}
	case "response.failed":
		var failed struct {
			Error struct {
				Code    string `json:"code"`
				Message string `json:"message"`
			} `json:"error"`
		}
		_ = json.Unmarshal(envelope.Response, &failed)
		return classifyResponsesStreamError(failed.Error.Code, failed.Error.Message)
	case "error":
		return classifyResponsesStreamError(envelope.Code, envelope.Message)
	}
	return nil
}

// classifyResponsesStreamError 把流内错误事件按关键字归类为可重试或不可重试的 ProviderError。
func classifyResponsesStreamError(code, message string) *ProviderError {
	signal := strings.ToLower(strings.TrimSpace(code + " " + message))
	kind, retryable := ErrorUnknown, true
	switch {
	case strings.Contains(signal, "invalid_api_key"), strings.Contains(signal, "authentication"),
		strings.Contains(signal, "unauthorized"), strings.Contains(signal, "permission_denied"):
		kind, retryable = ErrorAuth, false
	case strings.Contains(signal, "rate_limit"), strings.Contains(signal, "too many requests"):
		kind, retryable = ErrorRateLimit, true
	case strings.Contains(signal, "timeout"):
		kind, retryable = ErrorTimeout, true
	case strings.Contains(signal, "overload"), strings.Contains(signal, "capacity"):
		kind, retryable = ErrorOverloaded, true
	case strings.Contains(signal, "server_error"), strings.Contains(signal, "internal_error"):
		kind, retryable = ErrorServer, true
	case strings.Contains(signal, "content_policy"), strings.Contains(signal, "content filter"),
		strings.Contains(signal, "safety policy"):
		kind, retryable = ErrorContentPolicy, false
	case strings.Contains(signal, "context_length"), strings.Contains(signal, "context window"),
		strings.Contains(signal, "maximum context"), strings.Contains(signal, "too many tokens"):
		kind, retryable = ErrorContextOverflow, false
	case strings.Contains(signal, "invalid_prompt"), strings.Contains(signal, "invalid_request"),
		strings.Contains(signal, "invalid_value"), strings.Contains(signal, "unsupported_value"),
		strings.Contains(signal, "missing_required"):
		kind, retryable = ErrorInvalidRequest, false
	}
	return &ProviderError{
		Kind: kind, Retryable: retryable,
		Message: "assistant LLM stream failed: kind=" + string(kind),
	}
}

// responsesTools 把工具定义转换为 Responses 的扁平 function 工具。
func responsesTools(defs []prompt.ToolDef) []map[string]any {
	out := make([]map[string]any, 0, len(defs))
	for _, def := range defs {
		out = append(out, map[string]any{
			"type":        "function",
			"name":        def.Name,
			"description": def.Description,
			"parameters":  def.Parameters,
		})
	}
	return out
}

// responsesInput 把历史转换为 Responses input：工具调用与结果是独立的 function_call / function_call_output 项。
func responsesInput(turns []prompt.Turn) []map[string]any {
	out := make([]map[string]any, 0, len(turns))
	for _, turn := range turns {
		if len(turn.ToolCalls) > 0 {
			if strings.TrimSpace(turn.Content) != "" {
				out = append(out, map[string]any{
					"role":    "assistant",
					"content": []map[string]string{{"type": "output_text", "text": turn.Content}},
				})
			}
			for _, call := range turn.ToolCalls {
				args := call.Arguments
				if strings.TrimSpace(args) == "" {
					args = "{}"
				}
				out = append(out, map[string]any{
					"type":      "function_call",
					"call_id":   call.ID,
					"name":      call.Name,
					"arguments": args,
				})
			}
			continue
		}
		if isToolResult(turn) {
			out = append(out, map[string]any{
				"type":    "function_call_output",
				"call_id": turn.ToolCallID,
				"output":  turn.Content,
			})
			continue
		}
		role := strings.TrimSpace(turn.Role)
		partType := "input_text"
		if role == "assistant" {
			// Responses API rejects input_text on assistant items; history must be output_text.
			partType = "output_text"
		}
		out = append(out, map[string]any{
			"role": role, "content": []map[string]string{{"type": partType, "text": turn.Content}},
		})
	}
	return out
}

// responsesUsage 是 Responses 协议的计量字段。
type responsesUsage struct {
	InputTokens  int64 `json:"input_tokens"`
	OutputTokens int64 `json:"output_tokens"`
	TotalTokens  int64 `json:"total_tokens"`
	InputDetails struct {
		CachedTokens     int64 `json:"cached_tokens"`
		CacheWriteTokens int64 `json:"cache_write_tokens"`
	} `json:"input_tokens_details"`
	OutputDetails struct {
		ReasoningTokens int64 `json:"reasoning_tokens"`
	} `json:"output_tokens_details"`
}

// responsesUsage 把 Responses 计量换算为统一的 Usage。
func (cfg *Config) responsesUsage(raw responsesUsage) Usage {
	return cfg.usage(raw.InputTokens, raw.OutputTokens, raw.TotalTokens, raw.InputDetails.CachedTokens, raw.InputDetails.CacheWriteTokens, raw.OutputDetails.ReasoningTokens)
}

// firstNonEmpty 返回第一个非空白值。
func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}
