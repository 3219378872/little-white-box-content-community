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

	"esx/app/assistant/internal/prompt"
)

const (
	WireAPIChatCompletions = "chat_completions"
	WireAPIResponses       = "responses"
	maxResponseBytes       = 8 << 20
)

type ToolCall struct {
	ID        string
	Name      string
	Arguments string
}

type Usage struct {
	PromptTokens     int64
	CompletionTokens int64
	CacheTokens      int64
	TotalTokens      int64
	CostUSD          float64
}

type Request struct {
	Messages     []prompt.Turn
	Tools        []prompt.ToolDef
	MaxTokens    int
	Convergence  string
	DisableTools bool
}

type Result struct {
	Text      string
	ToolCalls []ToolCall
	Model     string
	Raw       []byte
	Usage     Usage
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
	WireAPI                        string
	Endpoint                       string
	APIKey                         string
	Model                          string
	Timeout                        time.Duration
	MaxOutputTokens                int
	ContextWindowTokens            int
	PromptCostPerMillionTokens     float64
	CompletionCostPerMillionTokens float64
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
	payload, err := c.marshal(req, maxTokens)
	if err != nil {
		return Result{}, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.Endpoint, bytes.NewReader(payload))
	if err != nil {
		return Result{}, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if c.cfg.APIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)
	}
	resp, err := c.client.Do(httpReq)
	if err != nil {
		return Result{}, err
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
		return Result{}, fmt.Errorf("assistant LLM status=%s body=%s", resp.Status, truncateForLog(raw))
	}
	if c.cfg.WireAPI == WireAPIResponses {
		return c.decodeResponses(raw)
	}
	return c.decodeChat(raw)
}

func (c *HTTPClient) marshal(req Request, maxTokens int) ([]byte, error) {
	messages := req.Messages
	if strings.TrimSpace(req.Convergence) != "" {
		messages = append(append([]prompt.Turn{}, messages...), prompt.Turn{Role: "system", Content: req.Convergence})
	}
	if c.cfg.WireAPI == WireAPIResponses {
		body := map[string]any{
			"model":             c.cfg.Model,
			"input":             responsesInput(messages),
			"max_output_tokens": maxTokens,
			"stream":            false,
			"store":             false,
		}
		if !req.DisableTools && len(req.Tools) > 0 {
			body["tools"] = responsesTools(req.Tools)
		}
		return json.Marshal(body)
	}
	body := map[string]any{
		"model":      c.cfg.Model,
		"messages":   chatMessages(messages),
		"max_tokens": maxTokens,
		"stream":     false,
	}
	if !req.DisableTools && len(req.Tools) > 0 {
		body["tools"] = chatTools(req.Tools)
	}
	return json.Marshal(body)
}

func chatMessages(turns []prompt.Turn) []map[string]any {
	out := make([]map[string]any, 0, len(turns))
	for _, turn := range turns {
		item := map[string]any{"role": turn.Role, "content": turn.Content}
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

func truncateForLog(raw []byte) string {
	const n = 400
	s := strings.Join(strings.Fields(string(raw)), " ")
	if len(s) > n {
		return s[:n] + "..."
	}
	return s
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
		Usage struct {
			PromptTokens     int64 `json:"prompt_tokens"`
			CompletionTokens int64 `json:"completion_tokens"`
			TotalTokens      int64 `json:"total_tokens"`
		} `json:"usage"`
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
		calls = append(calls, ToolCall{ID: call.ID, Name: call.Function.Name, Arguments: call.Function.Arguments})
	}
	text := strings.TrimSpace(msg.Content)
	if text == "" && len(calls) == 0 {
		return Result{}, fmt.Errorf("assistant LLM returned an empty response")
	}
	return Result{Text: text, ToolCalls: calls, Model: c.cfg.Model, Raw: raw, Usage: c.usage(parsed.Usage.PromptTokens, parsed.Usage.CompletionTokens, parsed.Usage.TotalTokens)}, nil
}

func (c *HTTPClient) decodeResponses(raw []byte) (Result, error) {
	var parsed struct {
		Status string `json:"status"`
		Output []struct {
			Type    string `json:"type"`
			Content []struct {
				Type string `json:"type"`
				Text string `json:"text"`
			} `json:"content"`
			CallID    string          `json:"call_id"`
			Name      string          `json:"name"`
			Arguments json.RawMessage `json:"arguments"`
		} `json:"output"`
		OutputText string `json:"output_text"`
		Usage      struct {
			InputTokens  int64 `json:"input_tokens"`
			OutputTokens int64 `json:"output_tokens"`
			TotalTokens  int64 `json:"total_tokens"`
		} `json:"usage"`
	}
	if err := json.Unmarshal(raw, &parsed); err != nil {
		return Result{}, fmt.Errorf("decode responses: %w", err)
	}
	if parsed.Status != "" && parsed.Status != "completed" {
		return Result{}, fmt.Errorf("assistant LLM response status=%s", parsed.Status)
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
			args := string(item.Arguments)
			if args == "" {
				args = "{}"
			}
			calls = append(calls, ToolCall{ID: item.CallID, Name: item.Name, Arguments: args})
		}
	}
	text := strings.TrimSpace(strings.Join(texts, ""))
	if text == "" {
		text = strings.TrimSpace(parsed.OutputText)
	}
	if text == "" && len(calls) == 0 {
		return Result{}, fmt.Errorf("assistant LLM returned an empty response")
	}
	return Result{Text: text, ToolCalls: calls, Model: c.cfg.Model, Raw: raw, Usage: c.usage(parsed.Usage.InputTokens, parsed.Usage.OutputTokens, parsed.Usage.TotalTokens)}, nil
}

func (c *HTTPClient) usage(promptTokens, completionTokens, totalTokens int64) Usage {
	if totalTokens == 0 {
		totalTokens = promptTokens + completionTokens
	}
	cost := (float64(promptTokens)*c.cfg.PromptCostPerMillionTokens + float64(completionTokens)*c.cfg.CompletionCostPerMillionTokens) / 1_000_000
	return Usage{PromptTokens: promptTokens, CompletionTokens: completionTokens, TotalTokens: totalTokens, CostUSD: cost}
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
