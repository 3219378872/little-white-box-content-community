package llm

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"

	"esx/app/assistant/internal/canonical"
	"esx/app/assistant/internal/prompt"
)

type streamedToolCall struct {
	index     int
	id        string
	name      string
	arguments strings.Builder
}

type streamState struct {
	text       strings.Builder
	scrubber   prompt.StreamingScrubber
	calls      map[int]*streamedToolCall
	callKeys   map[string]int
	usage      Usage
	model      string
	incomplete string
	raw        []byte
	emitted    bool
	terminal   bool
}

func (c *HTTPClient) SupportsStreaming() bool { return c != nil }

func (c *HTTPClient) CompleteStream(ctx context.Context, req Request, emit func(Delta) error) (Result, error) {
	if c == nil {
		return Result{}, fmt.Errorf("llm client is nil")
	}
	maxTokens := req.MaxTokens
	if maxTokens <= 0 || maxTokens > c.cfg.MaxOutputTokens {
		maxTokens = c.cfg.MaxOutputTokens
	}
	payload, err := c.marshal(req, maxTokens, true)
	if err != nil {
		return Result{}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.Endpoint, bytes.NewReader(payload))
	if err != nil {
		return Result{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "text/event-stream")
	httpReq.Header.Set("User-Agent", responsesUserAgent)
	if c.cfg.WireAPI == WireAPIResponses {
		httpReq.Header.Set("OpenAI-Beta", responsesBeta)
	}
	if c.cfg.APIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)
	}
	resp, err := c.client.Do(httpReq)
	if err != nil {
		return Result{}, ClassifyError(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		raw, readErr := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
		if readErr != nil {
			return Result{}, readErr
		}
		return Result{}, classifyHTTPError(resp.StatusCode, resp.Header, raw)
	}
	if !strings.Contains(strings.ToLower(resp.Header.Get("Content-Type")), "text/event-stream") {
		raw, readErr := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
		if readErr != nil {
			return Result{}, readErr
		}
		if len(raw) > maxResponseBytes {
			return Result{}, fmt.Errorf("assistant LLM response exceeds the byte limit")
		}
		var result Result
		if c.cfg.WireAPI == WireAPIResponses {
			result, err = c.decodeResponses(raw)
		} else {
			result, err = c.decodeChat(raw)
		}
		if err != nil {
			return Result{}, err
		}
		result.Text = strings.TrimSpace(prompt.SanitizeOutput(result.Text))
		if emit != nil && result.Text != "" {
			if err := emit(Delta{Text: result.Text}); err != nil {
				return Result{}, err
			}
		}
		result.Streamed = false
		return result, nil
	}

	state := &streamState{calls: map[int]*streamedToolCall{}, callKeys: map[string]int{}, model: c.cfg.Model}
	err = scanSSE(resp.Body, func(event string, data []byte) error {
		if bytes.Equal(bytes.TrimSpace(data), []byte("[DONE]")) {
			state.terminal = true
			return nil
		}
		if len(data) > maxResponseBytes {
			return fmt.Errorf("assistant LLM stream event exceeds the byte limit")
		}
		state.raw = append(state.raw[:0], data...)
		if c.cfg.WireAPI == WireAPIResponses {
			return c.consumeResponsesEvent(event, data, state, emit)
		}
		return c.consumeChatEvent(data, state, emit)
	})
	if err != nil {
		return Result{}, err
	}
	if !state.terminal {
		return Result{}, &ProviderError{
			Kind: ErrorUnknown, Retryable: true,
			Message: "assistant LLM stream ended before a terminal event",
		}
	}
	if tail := state.scrubber.Flush(); tail != "" {
		if err := emitVisible(state, tail, emit); err != nil {
			return Result{}, err
		}
	}
	result := Result{
		Text: strings.TrimSpace(state.text.String()), ToolCalls: state.toolCalls(), Model: state.model,
		Raw: append([]byte(nil), state.raw...), Usage: state.usage, IncompleteReason: state.incomplete, Streamed: true,
	}
	if result.Text == "" && len(result.ToolCalls) == 0 && result.IncompleteReason == "" {
		return Result{}, fmt.Errorf("assistant LLM returned an empty response")
	}
	return result, nil
}

func scanSSE(reader io.Reader, consume func(event string, data []byte) error) error {
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 64<<10), maxResponseBytes)
	var event string
	var data strings.Builder
	flush := func() error {
		if data.Len() == 0 {
			event = ""
			return nil
		}
		raw := []byte(strings.TrimSuffix(data.String(), "\n"))
		data.Reset()
		err := consume(event, raw)
		event = ""
		return err
	}
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			if err := flush(); err != nil {
				return err
			}
			continue
		}
		if strings.HasPrefix(line, ":") {
			continue
		}
		if strings.HasPrefix(line, "event:") {
			event = strings.TrimSpace(strings.TrimPrefix(line, "event:"))
			continue
		}
		if strings.HasPrefix(line, "data:") {
			data.WriteString(strings.TrimSpace(strings.TrimPrefix(line, "data:")))
			data.WriteByte('\n')
		}
	}
	if err := scanner.Err(); err != nil {
		return ClassifyError(err)
	}
	return flush()
}

func (c *HTTPClient) consumeChatEvent(data []byte, state *streamState, emit func(Delta) error) error {
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
		state.usage = c.chatUsage(chunk.Usage)
	}
	return nil
}

func (c *HTTPClient) consumeResponsesEvent(event string, data []byte, state *streamState, emit func(Delta) error) error {
	var envelope struct {
		Type        string          `json:"type"`
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
			final, err := c.decodeResponses(envelope.Response)
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
	case "response.failed", "error":
		return &ProviderError{Kind: ErrorUnknown, Retryable: true, Message: "assistant LLM stream failed"}
	}
	return nil
}

func emitVisible(state *streamState, text string, emit func(Delta) error) error {
	if text == "" {
		return nil
	}
	state.text.WriteString(text)
	state.emitted = true
	if emit != nil {
		return emit(Delta{Text: text})
	}
	return nil
}

func (s *streamState) ensureCall(index int, id string) *streamedToolCall {
	if item := s.calls[index]; item != nil {
		return item
	}
	item := &streamedToolCall{index: index, id: id}
	s.calls[index] = item
	return item
}

func (s *streamState) ensureResponseCall(key string, outputIndex int) *streamedToolCall {
	if key != "" {
		if index, ok := s.callKeys[key]; ok {
			return s.ensureCall(index, key)
		}
	}
	index := outputIndex
	for s.calls[index] != nil && (key == "" || s.calls[index].id != key) {
		index++
	}
	if key != "" {
		s.callKeys[key] = index
	}
	return s.ensureCall(index, key)
}

func (s *streamState) toolCalls() []ToolCall {
	indexes := make([]int, 0, len(s.calls))
	for index := range s.calls {
		indexes = append(indexes, index)
	}
	sort.Ints(indexes)
	out := make([]ToolCall, 0, len(indexes))
	for _, index := range indexes {
		item := s.calls[index]
		if strings.TrimSpace(item.name) == "" {
			continue
		}
		args := canonical.UnwrapArgsJSON(item.arguments.String())
		if strings.TrimSpace(args) == "" {
			args = "{}"
		}
		id := item.id
		if id == "" {
			id = "call_" + strconv.Itoa(index)
		}
		out = append(out, ToolCall{ID: id, Name: item.name, Arguments: args})
	}
	return out
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

var _ StreamingClient = (*HTTPClient)(nil)
