package store

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/elastic/go-elasticsearch/v8"
	"github.com/elastic/go-elasticsearch/v8/esapi"
)

type ElasticsearchStore struct {
	client *elasticsearch.Client
	index  string
}

type Option func(*elasticsearch.Config)

func WithBasicAuth(username, password string) Option {
	return func(c *elasticsearch.Config) {
		c.Username = username
		c.Password = password
	}
}

func NewElasticsearchStore(addresses []string, index string, opts ...Option) (*ElasticsearchStore, error) {
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	cfg := elasticsearch.Config{Addresses: addresses, Transport: transport}
	for _, opt := range opts {
		opt(&cfg)
	}
	client, err := elasticsearch.NewClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("new client: %w", err)
	}
	return &ElasticsearchStore{client: client, index: index}, nil
}

func (s *ElasticsearchStore) Health(ctx context.Context) error {
	res, err := s.client.Indices.Exists([]string{s.index}, s.client.Indices.Exists.WithContext(ctx))
	if err != nil {
		return fmt.Errorf("index health request: %w", err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.StatusCode == http.StatusOK {
		return nil
	}
	raw, _ := io.ReadAll(res.Body)
	return fmt.Errorf("index %q unavailable: status=%s body=%s", s.index, res.Status(), string(raw))
}

func (s *ElasticsearchStore) SearchPosts(ctx context.Context, query PostQuery) (PostResult, error) {
	from := int((query.Page - 1) * query.PageSize)
	body := map[string]any{
		"from":  from,
		"size":  query.PageSize,
		"query": postQuery(query.Keyword, query.Tags),
		"highlight": map[string]any{
			"fields":   map[string]any{"body": map[string]any{}},
			"pre_tags": []string{"<em>"}, "post_tags": []string{"</em>"},
		},
	}
	switch query.SortBy {
	case 2:
		body["sort"] = []any{map[string]any{"created_at": map[string]any{"order": "desc"}}}
	case 3:
		body["sort"] = []any{
			map[string]any{"like_count": map[string]any{"order": "desc"}},
			map[string]any{"comment_count": map[string]any{"order": "desc"}},
			map[string]any{"created_at": map[string]any{"order": "desc"}},
		}
	default:
		body["sort"] = []any{map[string]any{"_score": map[string]any{"order": "desc"}}, map[string]any{"created_at": map[string]any{"order": "desc"}}}
	}

	var response struct {
		Hits struct {
			Total totalHits `json:"total"`
			Hits  []struct {
				Source struct {
					PostID       int64  `json:"post_id"`
					AuthorID     int64  `json:"author_id"`
					Title        string `json:"title"`
					Body         string `json:"body"`
					LikeCount    int64  `json:"like_count"`
					CommentCount int64  `json:"comment_count"`
					CreatedAt    int64  `json:"created_at"`
				} `json:"_source"`
				Highlight map[string][]string `json:"highlight"`
			} `json:"hits"`
		} `json:"hits"`
	}
	if err := s.search(ctx, body, &response); err != nil {
		return PostResult{}, err
	}
	result := PostResult{Posts: make([]Post, 0, len(response.Hits.Hits)), Total: response.Hits.Total.Value}
	for _, hit := range response.Hits.Hits {
		highlight := hit.Source.Body
		if fragments := hit.Highlight["body"]; len(fragments) > 0 {
			highlight = strings.Join(fragments, " … ")
		}
		result.Posts = append(result.Posts, Post{
			ID: hit.Source.PostID, AuthorID: hit.Source.AuthorID, Title: hit.Source.Title,
			ContentHighlight: highlight, LikeCount: hit.Source.LikeCount,
			CommentCount: hit.Source.CommentCount, CreatedAt: hit.Source.CreatedAt,
		})
	}
	return result, nil
}

func (s *ElasticsearchStore) SearchTags(ctx context.Context, keyword string, limit int32) ([]Tag, error) {
	// xbh_posts has no independent tag index. Aggregate a bounded candidate set
	// from its keyword tags field, then apply case-insensitive matching locally.
	candidateSize := min(int(limit)*20, 1000)
	body := map[string]any{
		"size": 0,
		"aggs": map[string]any{"tags": map[string]any{"terms": map[string]any{
			"field": "tags", "size": candidateSize, "order": map[string]string{"_count": "desc"},
		}}},
	}
	buckets, err := s.tagBuckets(ctx, body)
	if err != nil {
		return nil, err
	}
	needle := strings.ToLower(keyword)
	result := make([]Tag, 0, limit)
	for _, tag := range buckets {
		if !strings.Contains(strings.ToLower(tag.Name), needle) {
			continue
		}
		result = append(result, tag)
		if len(result) == int(limit) {
			break
		}
	}
	return result, nil
}

func (s *ElasticsearchStore) HotSearches(ctx context.Context, limit int32) ([]string, error) {
	body := map[string]any{
		"size": 0,
		"aggs": map[string]any{"tags": map[string]any{"terms": map[string]any{
			"field": "tags", "size": limit, "order": map[string]string{"_count": "desc"},
		}}},
	}
	buckets, err := s.tagBuckets(ctx, body)
	if err != nil {
		return nil, err
	}
	keywords := make([]string, 0, len(buckets))
	for _, tag := range buckets {
		keywords = append(keywords, tag.Name)
		if len(keywords) == int(limit) {
			break
		}
	}
	return keywords, nil
}

func postQuery(keyword string, tags []string) map[string]any {
	must := []any{map[string]any{"multi_match": map[string]any{
		"query": keyword, "fields": []string{"title^2", "body"}, "operator": "and",
	}}}
	if len(tags) == 0 {
		return map[string]any{"bool": map[string]any{"must": must}}
	}
	return map[string]any{"bool": map[string]any{
		"must": must, "filter": []any{map[string]any{"terms": map[string]any{"tags": tags}}},
	}}
}

func (s *ElasticsearchStore) tagBuckets(ctx context.Context, body map[string]any) ([]Tag, error) {
	var response struct {
		Aggregations struct {
			Tags struct {
				Buckets []struct {
					Key      string `json:"key"`
					DocCount int64  `json:"doc_count"`
				} `json:"buckets"`
			} `json:"tags"`
		} `json:"aggregations"`
	}
	if err := s.search(ctx, body, &response); err != nil {
		return nil, err
	}
	result := make([]Tag, 0, len(response.Aggregations.Tags.Buckets))
	for _, bucket := range response.Aggregations.Tags.Buckets {
		result = append(result, Tag{Name: bucket.Key, PostCount: bucket.DocCount})
	}
	return result, nil
}

func (s *ElasticsearchStore) search(ctx context.Context, body map[string]any, target any) error {
	raw, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal query: %w", err)
	}
	req := esapi.SearchRequest{Index: []string{s.index}, Body: bytes.NewReader(raw), TrackTotalHits: true}
	res, err := req.Do(ctx, s.client)
	if err != nil {
		return fmt.Errorf("search request: %w", err)
	}
	defer func() { _ = res.Body.Close() }()
	if res.IsError() {
		body, _ := io.ReadAll(res.Body)
		return fmt.Errorf("search failed status=%s body=%s", res.Status(), string(body))
	}
	if err := json.NewDecoder(res.Body).Decode(target); err != nil {
		return fmt.Errorf("decode search response: %w", err)
	}
	return nil
}

type totalHits struct {
	Value int64
}

func (t *totalHits) UnmarshalJSON(data []byte) error {
	var object struct {
		Value int64 `json:"value"`
	}
	if len(data) > 0 && data[0] == '{' {
		if err := json.Unmarshal(data, &object); err != nil {
			return err
		}
		t.Value = object.Value
		return nil
	}
	return json.Unmarshal(data, &t.Value)
}
