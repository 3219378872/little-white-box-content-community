package llm

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"io"
	"sort"
	"strconv"
	"strings"

	"esx/app/assistant/internal/canonical"
	"esx/app/assistant/internal/prompt"
)

// streamedToolCall 是流式拼接中的一个工具调用。
type streamedToolCall struct {
	index     int
	id        string
	name      string
	arguments strings.Builder
}

// streamState 是一次流式响应的累积状态；text 只包含经清洗器去掉上下文封装后的可见文本。
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

// SupportsStreaming 表示 HTTPClient 支持流式调用。
func (c *HTTPClient) SupportsStreaming() bool { return c != nil }

// CompleteStream 发送流式请求并逐段推送可见文本。上游未返回 SSE 时按完整响应解码并一次性推送；
// 流在终态事件前结束视为可重试错误。
func (c *HTTPClient) CompleteStream(ctx context.Context, req Request, emit func(Delta) error) (Result, error) {
	if c == nil {
		return Result{}, fmt.Errorf("llm client is nil")
	}
	httpReq, err := c.newHTTPRequest(ctx, req, true)
	if err != nil {
		return Result{}, err
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
		return c.decodeNonStreamingResponse(resp.Body, emit)
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
		return c.codec.consumeEvent(event, data, state, emit)
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
	// 清洗器可能还缓存着跨分片的尾部文本，结束前一并推送。
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

// scanSSE 按 SSE 规则解析事件：空行分隔事件，多行 data 以换行拼接，注释行忽略。
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

// emitVisible 记录并推送一段可见文本。
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

// ensureCall 按 index 取得或创建工具调用。
func (s *streamState) ensureCall(index int, id string) *streamedToolCall {
	if item := s.calls[index]; item != nil {
		return item
	}
	item := &streamedToolCall{index: index, id: id}
	s.calls[index] = item
	return item
}

// ensureResponseCall 按 Responses 的 item ID 定位工具调用；ID 未知时从 output_index 起找空位，
// 避免不同调用因 output_index 相同而被合并。
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

// toolCalls 按 index 输出已命名的工具调用；缺 ID 的按序号生成，参数缺省为 {}。
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

var _ StreamingClient = (*HTTPClient)(nil)

// decodeNonStreamingResponse 处理请求了流式但上游返回完整 JSON 的情况：解码后把全文作为一次增量推送。
func (c *HTTPClient) decodeNonStreamingResponse(body io.Reader, emit func(Delta) error) (Result, error) {
	raw, err := readLimitedBody(body)
	if err != nil {
		return Result{}, err
	}
	result, err := c.decodeBody(raw)
	if err != nil {
		return Result{}, err
	}
	if emit != nil && result.Text != "" {
		if err := emit(Delta{Text: result.Text}); err != nil {
			return Result{}, err
		}
	}
	result.Streamed = false
	return result, nil
}
