// Package websearch 提供网络搜索工具实现。当前仅支持 Tavily API
// （AGNT-012：结果只作研究素材，不作为社区事实证据）。
package websearch

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"
)

// Result 是单条网络搜索结果。
type Result struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Content string `json:"content"`
}

// Searcher 是网络搜索能力抽象。
type Searcher interface {
	Search(ctx context.Context, query string, maxResults int) ([]Result, error)
}

// Config 是 Tavily 客户端配置。
type Config struct {
	APIKey     string
	Endpoint   string        // 缺省 https://api.tavily.com
	Timeout    time.Duration // 缺省 8s
	MaxResults int           // 缺省 5
}

type tavilyClient struct {
	cfg    Config
	client *http.Client
}

// New 返回 Tavily 搜索器；APIKey 为空返回 nil（调用方据此从工具表剔除 web_search，
// AGNT-010 允许按配置收缩工具集合）。
func New(cfg Config) Searcher {
	if strings.TrimSpace(cfg.APIKey) == "" {
		return nil
	}
	if cfg.Endpoint == "" {
		cfg.Endpoint = "https://api.tavily.com"
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 8 * time.Second
	}
	if cfg.MaxResults <= 0 {
		cfg.MaxResults = 5
	}
	return &tavilyClient{cfg: cfg, client: &http.Client{Timeout: cfg.Timeout}}
}

type tavilyRequest struct {
	APIKey        string `json:"api_key"`
	Query         string `json:"query"`
	SearchDepth   string `json:"search_depth"`
	MaxResults    int    `json:"max_results"`
	IncludeAnswer bool   `json:"include_answer"`
}

type tavilyResponse struct {
	Results []struct {
		Title   string `json:"title"`
		URL     string `json:"url"`
		Content string `json:"content"`
	} `json:"results"`
}

func (c *tavilyClient) Search(ctx context.Context, query string, maxResults int) ([]Result, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("websearch: query is required")
	}
	limit := maxResults
	if limit <= 0 || limit > c.cfg.MaxResults {
		limit = c.cfg.MaxResults
	}
	payload, err := json.Marshal(tavilyRequest{
		APIKey: c.cfg.APIKey, Query: query,
		SearchDepth: "basic", MaxResults: limit, IncludeAnswer: false,
	})
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		strings.TrimRight(c.cfg.Endpoint, "/")+"/search", bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("websearch: provider returned status %d", resp.StatusCode)
	}
	var decoded tavilyResponse
	if err := json.NewDecoder(resp.Body).Decode(&decoded); err != nil {
		return nil, fmt.Errorf("websearch: decode response: %w", err)
	}
	results := make([]Result, 0, len(decoded.Results))
	for _, item := range decoded.Results {
		results = append(results, Result{Title: item.Title, URL: item.URL, Content: item.Content})
	}
	return results, nil
}
