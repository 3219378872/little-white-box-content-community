package llm

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"esx/app/assistant/internal/canonical"
	"esx/app/assistant/internal/prompt"
)

const (
	WireAPIChatCompletions = "chat_completions"
	WireAPIResponses       = "responses"
	maxResponseBytes       = 8 << 20
	// GLM's mine upstream 403s Go's default client signature; match a browser UA
	// plus the Responses beta header that Codex/other tools send.
	responsesUserAgent = "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/128.0.0.0 Safari/537.36"
	responsesBeta      = "responses=v1"
)

type ToolCall struct {
	ID           string
	Name         string
	Arguments    string
	Prepared     bool
	PrepareError error
}

type Usage struct {
	PromptTokens     int64
	CompletionTokens int64
	CacheTokens      int64
	CacheWriteTokens int64
	ReasoningTokens  int64
	TotalTokens      int64
	CostUSD          float64
	Estimated        bool
}

type Request struct {
	SuppressText  bool
	Messages      []prompt.Turn
	Tools         []prompt.ToolDef
	MaxTokens     int
	Convergence   string
	DisableTools  bool
	RequiredTool  string
	AttemptPrefix string
	Observer      AttemptObserver
}

type Result struct {
	Text             string
	ToolCalls        []ToolCall
	Model            string
	Raw              []byte
	Usage            Usage
	IncompleteReason string
	StreamID         string
	Attempts         int
	Streamed         bool
}

type Client interface {
	Complete(ctx context.Context, req Request) (Result, error)
	SupportsTools() bool
	WireAPI() string
	MaxOutputTokens() int
	ContextWindowTokens() int
}

type Config struct {
	Enabled                        bool
	RouteID                        string
	Boundary                       string
	WireAPI                        string
	Endpoint                       string
	APIKey                         string
	Model                          string
	Timeout                        time.Duration
	MaxOutputTokens                int
	ContextWindowTokens            int
	PromptCostPerMillionTokens     float64
	CompletionCostPerMillionTokens float64
	CacheReadCostPerMillionTokens  float64
	CacheWriteCostPerMillionTokens float64
	ReasoningCostPerMillionTokens  float64
}

type HTTPClient struct {
	cfg    Config
	client *http.Client
}

func New(cfg Config) (*HTTPClient, error) {
	if !cfg.Enabled {
		return nil, nil
	}
	wire := strings.TrimSpace(cfg.WireAPI)
	if wire == "" {
		wire = WireAPIChatCompletions
	}
	if wire != WireAPIChatCompletions && wire != WireAPIResponses {
		return nil, fmt.Errorf("assistant LLM wire API must be %q or %q", WireAPIChatCompletions, WireAPIResponses)
	}
	endpoint, err := normalizeEndpoint(cfg.Endpoint, wire)
	if err != nil {
		return nil, err
	}
	if strings.TrimSpace(cfg.Model) == "" {
		return nil, fmt.Errorf("assistant LLM model is required")
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 90 * time.Second
	}
	if cfg.MaxOutputTokens <= 0 {
		cfg.MaxOutputTokens = 32768
	}
	if cfg.MaxOutputTokens > 65536 {
		cfg.MaxOutputTokens = 65536
	}
	if cfg.ContextWindowTokens <= 0 {
		cfg.ContextWindowTokens = 128000
	}
	if cfg.CacheReadCostPerMillionTokens == 0 {
		cfg.CacheReadCostPerMillionTokens = cfg.PromptCostPerMillionTokens
	}
	if cfg.CacheWriteCostPerMillionTokens == 0 {
		cfg.CacheWriteCostPerMillionTokens = cfg.PromptCostPerMillionTokens
	}
	if cfg.ReasoningCostPerMillionTokens == 0 {
		cfg.ReasoningCostPerMillionTokens = cfg.CompletionCostPerMillionTokens
	}
	cfg.WireAPI = wire
	cfg.Endpoint = endpoint
	cfg.Model = strings.TrimSpace(cfg.Model)
	return &HTTPClient{cfg: cfg, client: &http.Client{Timeout: cfg.Timeout}}, nil
}

func (c *HTTPClient) SupportsTools() bool { return c != nil }
func (c *HTTPClient) WireAPI() string {
	if c == nil {
		return ""
	}
	return c.cfg.WireAPI
}
func (c *HTTPClient) MaxOutputTokens() int {
	if c == nil {
		return 0
	}
	return c.cfg.MaxOutputTokens
}
func (c *HTTPClient) ContextWindowTokens() int {
	if c == nil {
		return 0
	}
	return c.cfg.ContextWindowTokens
}

func (c *HTTPClient) RouteID() string {
	if c == nil {
		return ""
	}
	if strings.TrimSpace(c.cfg.RouteID) == "" {
		return "primary"
	}
	return strings.TrimSpace(c.cfg.RouteID)
}

func (c *HTTPClient) ModelName() string {
	if c == nil {
		return ""
	}
	return c.cfg.Model
}

func (c *HTTPClient) Boundary() string {
	if c == nil {
		return ""
	}
	return strings.TrimSpace(c.cfg.Boundary)
}

type capabilityClient interface {
	RouteID() string
	ModelName() string
	Boundary() string
	SupportsStreaming() bool
}

type fallbackCapabilityClient interface {
	FallbackRouteIDs() []string
}

func Capability(client Client) prompt.ProviderCapability {
	if client == nil {
		return prompt.ProviderCapability{}
	}
	capability := prompt.ProviderCapability{
		RouteID: "primary", WireAPI: client.WireAPI(), ContextTokens: client.ContextWindowTokens(),
		MaxOutputTokens: client.MaxOutputTokens(), Tools: client.SupportsTools(),
	}
	if detailed, ok := client.(capabilityClient); ok {
		capability.RouteID = detailed.RouteID()
		capability.Model = detailed.ModelName()
		capability.Boundary = detailed.Boundary()
		capability.Streaming = detailed.SupportsStreaming()
	}
	if fallbacks, ok := client.(fallbackCapabilityClient); ok {
		capability.FallbackRouteIDs = append([]string(nil), fallbacks.FallbackRouteIDs()...)
	}
	return capability
}

type RouteSelector interface {
	ForRoute(routeID string) (Client, bool)
}

type exactRouteSelector interface {
	ExactRoute(routeID string) (Client, bool)
}

type capabilityRouteSelector interface {
	ForCapability(capability prompt.ProviderCapability) (Client, bool)
}

func SelectRoute(client Client, routeID string) (Client, bool) {
	if client == nil {
		return nil, false
	}
	if selector, ok := client.(RouteSelector); ok {
		return selector.ForRoute(routeID)
	}
	capability := Capability(client)
	return client, routeID == "" || capability.RouteID == routeID
}

func SelectExactRoute(client Client, routeID string) (Client, bool) {
	if client == nil {
		return nil, false
	}
	if selector, ok := client.(exactRouteSelector); ok {
		return selector.ExactRoute(routeID)
	}
	capability := Capability(client)
	return client, routeID == "" || capability.RouteID == routeID
}

func SelectCapability(client Client, frozen prompt.ProviderCapability) (Client, bool) {
	if client == nil || strings.TrimSpace(frozen.RouteID) == "" {
		return nil, false
	}
	var selected Client
	var ok bool
	if selector, selectable := client.(capabilityRouteSelector); selectable {
		selected, ok = selector.ForCapability(frozen)
	} else {
		selected, ok = SelectRoute(client, frozen.RouteID)
	}
	if !ok || !supportsFrozenCapability(selected, frozen) {
		return nil, false
	}
	bound := &frozenClient{base: selected, capability: frozen}
	if frozen.Streaming {
		return &frozenStreamingClient{frozenClient: bound}, true
	}
	return bound, true
}

func supportsFrozenCapability(client Client, frozen prompt.ProviderCapability) bool {
	actual := Capability(client)
	if actual.RouteID != frozen.RouteID || (frozen.Tools && !actual.Tools) ||
		(frozen.Streaming && !actual.Streaming) {
		return false
	}
	if frozen.WireAPI != "" && actual.WireAPI != frozen.WireAPI {
		return false
	}
	if frozen.Model != "" && actual.Model != frozen.Model {
		return false
	}
	if strings.TrimSpace(actual.Boundary) != strings.TrimSpace(frozen.Boundary) {
		return false
	}
	if frozen.ContextTokens > 0 && actual.ContextTokens < frozen.ContextTokens {
		return false
	}
	return frozen.MaxOutputTokens <= 0 || actual.MaxOutputTokens >= frozen.MaxOutputTokens
}

type frozenClient struct {
	base       Client
	capability prompt.ProviderCapability
}

func (c *frozenClient) Complete(ctx context.Context, req Request) (Result, error) {
	return c.base.Complete(ctx, req)
}
func (c *frozenClient) SupportsTools() bool      { return c.capability.Tools }
func (c *frozenClient) WireAPI() string          { return c.capability.WireAPI }
func (c *frozenClient) MaxOutputTokens() int     { return c.capability.MaxOutputTokens }
func (c *frozenClient) ContextWindowTokens() int { return c.capability.ContextTokens }
func (c *frozenClient) RouteID() string          { return c.capability.RouteID }
func (c *frozenClient) ModelName() string        { return c.capability.Model }
func (c *frozenClient) Boundary() string         { return c.capability.Boundary }
func (*frozenClient) SupportsStreaming() bool    { return false }

type frozenStreamingClient struct {
	*frozenClient
}

func (c *frozenStreamingClient) CompleteStream(ctx context.Context, req Request, emit func(Delta) error) (Result, error) {
	stream, ok := c.base.(StreamingClient)
	if !ok {
		return Result{}, fmt.Errorf("frozen assistant LLM route no longer supports streaming")
	}
	return stream.CompleteStream(ctx, req, emit)
}

func (*frozenStreamingClient) SupportsStreaming() bool { return true }

func Ready(client Client, enabled bool) error {
	if !enabled {
		return nil
	}
	if client == nil || !client.SupportsTools() {
		return fmt.Errorf("selected WireAPI must support tool schema/call/result")
	}
	return nil
}

func (c *HTTPClient) Complete(ctx context.Context, req Request) (Result, error) {
	if c == nil {
		return Result{}, fmt.Errorf("llm client is nil")
	}
	maxTokens := req.MaxTokens
	if maxTokens <= 0 || maxTokens > c.cfg.MaxOutputTokens {
		maxTokens = c.cfg.MaxOutputTokens
	}
	if maxTokens > 65536 {
		maxTokens = 65536
	}
	payload, err := c.marshal(req, maxTokens, false)
	if err != nil {
		return Result{}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.Endpoint, bytes.NewReader(payload))
	if err != nil {
		return Result{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")
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
	raw, err := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes+1))
	if err != nil {
		return Result{}, err
	}
	if len(raw) > maxResponseBytes {
		return Result{}, fmt.Errorf("assistant LLM response exceeds the byte limit")
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Result{}, classifyHTTPError(resp.StatusCode, resp.Header, raw)
	}
	var result Result
	if c.cfg.WireAPI == WireAPIResponses {
		result, err = c.decodeResponses(raw)
	} else {
		result, err = c.decodeChat(raw)
	}
	if err == nil {
		result.Text = strings.TrimSpace(prompt.SanitizeOutput(result.Text))
	}
	return result, err
}

func (c *HTTPClient) marshal(req Request, maxTokens int, stream bool) ([]byte, error) {
	messages := req.Messages
	if strings.TrimSpace(req.Convergence) != "" {
		messages = append(append([]prompt.Turn{}, messages...), prompt.Turn{Role: "system", Content: req.Convergence})
	}
	if c.cfg.WireAPI == WireAPIResponses {
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
		return json.Marshal(body)
	}
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
	return json.Marshal(body)
}

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

func isToolResult(turn prompt.Turn) bool {
	return strings.TrimSpace(turn.ToolCallID) != "" || strings.TrimSpace(turn.Role) == "tool"
}

func normalizeToolArguments(raw json.RawMessage) string {
	if len(bytes.TrimSpace(raw)) == 0 {
		return "{}"
	}
	return canonical.UnwrapArgsJSON(string(raw))
}

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

func (c *HTTPClient) decodeChat(raw []byte) (Result, error) {
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
	return Result{Text: text, ToolCalls: calls, Model: c.cfg.Model, Raw: raw, Usage: c.chatUsage(parsed.Usage)}, nil
}

func (c *HTTPClient) decodeResponses(raw []byte) (Result, error) {
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
			Usage:            c.responsesUsage(parsed.Usage),
			IncompleteReason: reason,
		}, nil
	}
	if !usable {
		return Result{}, fmt.Errorf("assistant LLM returned an empty response")
	}
	return Result{Text: text, ToolCalls: calls, Model: c.cfg.Model, Raw: raw, Usage: c.responsesUsage(parsed.Usage)}, nil
}

func (c *HTTPClient) chatUsage(raw chatUsage) Usage {
	cacheWrite := raw.PromptDetails.CacheWriteTokens
	if cacheWrite == 0 {
		cacheWrite = raw.CacheCreationInputTokens
	}
	return c.usage(raw.PromptTokens, raw.CompletionTokens, raw.TotalTokens, raw.PromptDetails.CachedTokens, cacheWrite, raw.CompletionDetails.ReasoningTokens)
}

func (c *HTTPClient) responsesUsage(raw responsesUsage) Usage {
	return c.usage(raw.InputTokens, raw.OutputTokens, raw.TotalTokens, raw.InputDetails.CachedTokens, raw.InputDetails.CacheWriteTokens, raw.OutputDetails.ReasoningTokens)
}

func (c *HTTPClient) usage(inputTokens, outputTokens, totalTokens, cacheRead, cacheWrite, reasoning int64) Usage {
	if totalTokens == 0 {
		totalTokens = inputTokens + outputTokens
	}
	regularInput := inputTokens - cacheRead - cacheWrite
	if regularInput < 0 {
		regularInput = 0
	}
	regularOutput := outputTokens - reasoning
	if regularOutput < 0 {
		regularOutput = 0
	}
	cost := (float64(regularInput)*c.cfg.PromptCostPerMillionTokens +
		float64(cacheRead)*c.cfg.CacheReadCostPerMillionTokens +
		float64(cacheWrite)*c.cfg.CacheWriteCostPerMillionTokens +
		float64(regularOutput)*c.cfg.CompletionCostPerMillionTokens +
		float64(reasoning)*c.cfg.ReasoningCostPerMillionTokens) / 1_000_000
	return Usage{
		PromptTokens: inputTokens, CompletionTokens: outputTokens, CacheTokens: cacheRead,
		CacheWriteTokens: cacheWrite, ReasoningTokens: reasoning, TotalTokens: totalTokens,
		CostUSD: cost, Estimated: inputTokens == 0 && outputTokens == 0,
	}
}

func normalizeEndpoint(endpoint, wireAPI string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(endpoint))
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return "", fmt.Errorf("assistant LLM endpoint must be an absolute HTTP URL")
	}
	path := strings.TrimRight(parsed.Path, "/")
	suffix := "/chat/completions"
	if wireAPI == WireAPIResponses {
		suffix = "/responses"
	}
	if path == "" || strings.HasSuffix(path, "/v1") {
		path += suffix
	}
	parsed.Path = path
	return parsed.String(), nil
}

type unsupportedClient struct{}

func (unsupportedClient) Complete(context.Context, Request) (Result, error) {
	return Result{}, fmt.Errorf("tools unsupported")
}
func (unsupportedClient) SupportsTools() bool      { return false }
func (unsupportedClient) WireAPI() string          { return "none" }
func (unsupportedClient) MaxOutputTokens() int     { return 0 }
func (unsupportedClient) ContextWindowTokens() int { return 0 }

func Unsupported() Client { return unsupportedClient{} }
