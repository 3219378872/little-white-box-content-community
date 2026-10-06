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

// ToolCall 是模型请求的一次工具调用；Prepared/PrepareError 由 runtime 在执行前填写。
type ToolCall struct {
	ID           string
	Name         string
	Arguments    string
	Prepared     bool
	PrepareError error
}

// Usage 是一次模型调用的 token 计量与按单价折算的成本；Estimated 表示上游未返回计量。
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

// Request 是与协议无关的模型请求；Convergence 作为最后一条 system 消息追加，用于收敛提示。
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

// Result 是与协议无关的模型输出；IncompleteReason 非空表示上游截断（长度、内容过滤等）。
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

// Client 是 runtime 使用的模型客户端抽象，HTTPClient、韧性包装与冻结路由都实现它。
type Client interface {
	Complete(ctx context.Context, req Request) (Result, error)
	SupportsTools() bool
	WireAPI() string
	MaxOutputTokens() int
	ContextWindowTokens() int
}

// Config 是单个模型路由的连接、限额与计价配置。
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

// HTTPClient 通过 HTTP 调用 OpenAI 兼容上游。协议差异全部由 codec 处理，这里只负责传输与限额。
type HTTPClient struct {
	cfg    Config
	client *http.Client
	codec  wireCodec
}

// New 校验并补全配置：协议缺省为 Chat Completions，端点补全协议路径，输出上限限制在 64k 以内，
// 缓存与推理单价缺省沿用输入/输出单价。未启用时返回 nil 客户端。
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
	client := &HTTPClient{cfg: cfg, client: &http.Client{Timeout: cfg.Timeout}}
	client.codec = newWireCodec(&client.cfg)
	return client, nil
}

// 以下访问器允许 nil 接收者，未配置的路由报告零能力。
func (c *HTTPClient) SupportsTools() bool { return c != nil }

// WireAPI 返回配置的协议。
func (c *HTTPClient) WireAPI() string {
	if c == nil {
		return ""
	}
	return c.cfg.WireAPI
}

// MaxOutputTokens 返回配置的单次输出上限。
func (c *HTTPClient) MaxOutputTokens() int {
	if c == nil {
		return 0
	}
	return c.cfg.MaxOutputTokens
}

// ContextWindowTokens 返回配置的上下文窗口。
func (c *HTTPClient) ContextWindowTokens() int {
	if c == nil {
		return 0
	}
	return c.cfg.ContextWindowTokens
}

// RouteID 是路由标识，缺省为 primary。
func (c *HTTPClient) RouteID() string {
	if c == nil {
		return ""
	}
	if strings.TrimSpace(c.cfg.RouteID) == "" {
		return "primary"
	}
	return strings.TrimSpace(c.cfg.RouteID)
}

// ModelName 返回配置的模型名。
func (c *HTTPClient) ModelName() string {
	if c == nil {
		return ""
	}
	return c.cfg.Model
}

// Boundary 返回配置的数据边界，备用路由必须与主路由一致。
func (c *HTTPClient) Boundary() string {
	if c == nil {
		return ""
	}
	return strings.TrimSpace(c.cfg.Boundary)
}

// Ready 在助手启用时要求所选协议支持工具调用。
func Ready(client Client, enabled bool) error {
	if !enabled {
		return nil
	}
	if client == nil || !client.SupportsTools() {
		return fmt.Errorf("selected WireAPI must support tool schema/call/result")
	}
	return nil
}

// Complete 发送一次非流式请求并解码完整响应。
func (c *HTTPClient) Complete(ctx context.Context, req Request) (Result, error) {
	if c == nil {
		return Result{}, fmt.Errorf("llm client is nil")
	}
	httpReq, err := c.newHTTPRequest(ctx, req, false)
	if err != nil {
		return Result{}, err
	}
	resp, err := c.client.Do(httpReq)
	if err != nil {
		return Result{}, ClassifyError(err)
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := readLimitedBody(resp.Body)
	if err != nil {
		return Result{}, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return Result{}, classifyHTTPError(resp.StatusCode, resp.Header, raw)
	}
	return c.decodeBody(raw)
}

// newHTTPRequest 用当前协议编码请求体并设置公共头。输出 token 取请求值，但不超过路由上限。
func (c *HTTPClient) newHTTPRequest(ctx context.Context, req Request, stream bool) (*http.Request, error) {
	maxTokens := req.MaxTokens
	if maxTokens <= 0 || maxTokens > c.cfg.MaxOutputTokens {
		maxTokens = c.cfg.MaxOutputTokens
	}
	if maxTokens > 65536 {
		maxTokens = 65536
	}
	messages := req.Messages
	if strings.TrimSpace(req.Convergence) != "" {
		messages = append(append([]prompt.Turn{}, messages...), prompt.Turn{Role: "system", Content: req.Convergence})
	}
	payload, err := c.codec.encode(messages, req, maxTokens, stream)
	if err != nil {
		return nil, err
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.cfg.Endpoint, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if stream {
		httpReq.Header.Set("Accept", "text/event-stream")
	} else {
		httpReq.Header.Set("Accept", "application/json")
	}
	httpReq.Header.Set("User-Agent", responsesUserAgent)
	c.codec.setHeaders(httpReq.Header)
	if c.cfg.APIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+c.cfg.APIKey)
	}
	return httpReq, nil
}

// readLimitedBody 读取响应体，超过 maxResponseBytes 时报错，防止异常上游耗尽内存。
func readLimitedBody(body io.Reader) ([]byte, error) {
	raw, err := io.ReadAll(io.LimitReader(body, maxResponseBytes+1))
	if err != nil {
		return nil, err
	}
	if len(raw) > maxResponseBytes {
		return nil, fmt.Errorf("assistant LLM response exceeds the byte limit")
	}
	return raw, nil
}

// decodeBody 解码完整响应，并去掉模型回显的平台上下文封装（记忆、摘要等提示块）。
func (c *HTTPClient) decodeBody(raw []byte) (Result, error) {
	result, err := c.codec.decode(raw)
	if err != nil {
		return Result{}, err
	}
	result.Text = strings.TrimSpace(prompt.SanitizeOutput(result.Text))
	return result, nil
}

// isToolResult 判断一条历史是否是工具结果；两种协议都以 tool_call_id 或 tool 角色识别。
func isToolResult(turn prompt.Turn) bool {
	return strings.TrimSpace(turn.ToolCallID) != "" || strings.TrimSpace(turn.Role) == "tool"
}

// normalizeToolArguments 把上游给出的参数（对象或被转义的字符串）统一为 JSON 对象文本，缺省为 {}。
func normalizeToolArguments(raw json.RawMessage) string {
	if len(bytes.TrimSpace(raw)) == 0 {
		return "{}"
	}
	return canonical.UnwrapArgsJSON(string(raw))
}

// usage 按计费口径拆分 token：缓存读写与推理 token 各自计价，其余按普通输入/输出计价；
// 上游未给出 total 时用输入加输出补齐。
func (cfg *Config) usage(inputTokens, outputTokens, totalTokens, cacheRead, cacheWrite, reasoning int64) Usage {
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
	cost := (float64(regularInput)*cfg.PromptCostPerMillionTokens +
		float64(cacheRead)*cfg.CacheReadCostPerMillionTokens +
		float64(cacheWrite)*cfg.CacheWriteCostPerMillionTokens +
		float64(regularOutput)*cfg.CompletionCostPerMillionTokens +
		float64(reasoning)*cfg.ReasoningCostPerMillionTokens) / 1_000_000
	return Usage{
		PromptTokens: inputTokens, CompletionTokens: outputTokens, CacheTokens: cacheRead,
		CacheWriteTokens: cacheWrite, ReasoningTokens: reasoning, TotalTokens: totalTokens,
		CostUSD: cost, Estimated: inputTokens == 0 && outputTokens == 0,
	}
}

// normalizeEndpoint 要求绝对 HTTP(S) 地址；只给到根路径或 /v1 时按协议补全接口路径。
func normalizeEndpoint(endpoint, wireAPI string) (string, error) {
	parsed, err := url.Parse(strings.TrimSpace(endpoint))
	if err != nil || parsed.Host == "" || (parsed.Scheme != "http" && parsed.Scheme != "https") {
		return "", fmt.Errorf("assistant LLM endpoint must be an absolute HTTP URL")
	}
	path := strings.TrimRight(parsed.Path, "/")
	if path == "" || strings.HasSuffix(path, "/v1") {
		path += newWireCodec(&Config{WireAPI: wireAPI}).endpointSuffix()
	}
	parsed.Path = path
	return parsed.String(), nil
}

// unsupportedClient 是未配置模型时的占位客户端，所有调用都失败。
type unsupportedClient struct{}

// Complete 总是失败。
func (unsupportedClient) Complete(context.Context, Request) (Result, error) {
	return Result{}, fmt.Errorf("tools unsupported")
}

// SupportsTools 为 false。
func (unsupportedClient) SupportsTools() bool { return false }

// WireAPI 报告 none。
func (unsupportedClient) WireAPI() string { return "none" }

// MaxOutputTokens 为 0。
func (unsupportedClient) MaxOutputTokens() int { return 0 }

// ContextWindowTokens 为 0。
func (unsupportedClient) ContextWindowTokens() int { return 0 }

// Unsupported 返回未配置模型时使用的占位客户端。
func Unsupported() Client { return unsupportedClient{} }
