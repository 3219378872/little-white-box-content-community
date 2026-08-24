package model

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	milvusclient "github.com/milvus-io/milvus-sdk-go/v2/client"
	"github.com/milvus-io/milvus-sdk-go/v2/entity"
	"google.golang.org/grpc"
)

const maxExternalRecallSeeds = 5

type ElasticsearchPostRecallSource struct {
	endpoint       string
	index          string
	username       string
	password       string
	featureVersion string
	redis          redisClient
	client         *http.Client
	timeout        time.Duration
}

func NewElasticsearchPostRecallSource(
	addresses []string,
	index string,
	username string,
	password string,
	featureVersion string,
	redis redisClient,
	timeout time.Duration,
) (*ElasticsearchPostRecallSource, error) {
	if len(addresses) == 0 || strings.TrimSpace(addresses[0]) == "" || strings.TrimSpace(index) == "" {
		return nil, fmt.Errorf("elasticsearch recall address and index are required")
	}
	endpoint := strings.TrimRight(strings.TrimSpace(addresses[0]), "/")
	if _, err := url.ParseRequestURI(endpoint); err != nil {
		return nil, fmt.Errorf("parse elasticsearch recall address: %w", err)
	}
	if timeout <= 0 {
		return nil, fmt.Errorf("elasticsearch recall timeout must be positive")
	}
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	return &ElasticsearchPostRecallSource{
		endpoint: endpoint, index: index, username: username, password: password,
		featureVersion: featureVersion, redis: redis,
		client: &http.Client{Transport: transport}, timeout: timeout,
	}, nil
}

func (s *ElasticsearchPostRecallSource) Name() string { return "es" }

func (s *ElasticsearchPostRecallSource) Recall(ctx context.Context, req RecallRequest) ([]PostCandidate, error) {
	if req.Limit <= 0 {
		return nil, fmt.Errorf("elasticsearch recall limit must be positive")
	}
	seedIDs, err := externalRecallSeedIDs(ctx, s.redis, s.featureVersion, req)
	if err != nil {
		return nil, fmt.Errorf("load elasticsearch recall seeds: %w", err)
	}
	if len(seedIDs) == 0 {
		return nil, ErrNotApplicable
	}
	like := make([]map[string]string, 0, len(seedIDs))
	excluded := make([]string, 0, len(seedIDs))
	for _, postID := range seedIDs {
		value := strconv.FormatInt(postID, 10)
		like = append(like, map[string]string{"_index": s.index, "_id": value})
		excluded = append(excluded, value)
	}
	body, err := json.Marshal(map[string]any{
		"size":    req.Limit,
		"_source": []string{"post_id"},
		"query": map[string]any{"bool": map[string]any{
			"must": map[string]any{"more_like_this": map[string]any{
				"fields": []string{"title", "body"}, "like": like,
				"min_term_freq": 1, "min_doc_freq": 1,
			}},
			"must_not": map[string]any{"ids": map[string]any{"values": excluded}},
		}},
	})
	if err != nil {
		return nil, fmt.Errorf("marshal elasticsearch recall query: %w", err)
	}
	requestCtx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	httpRequest, err := http.NewRequestWithContext(requestCtx, http.MethodPost,
		s.endpoint+"/"+url.PathEscape(s.index)+"/_search", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create elasticsearch recall request: %w", err)
	}
	httpRequest.Header.Set("Content-Type", "application/json")
	if s.username != "" {
		httpRequest.SetBasicAuth(s.username, s.password)
	}
	response, err := s.client.Do(httpRequest)
	if err != nil {
		return nil, fmt.Errorf("execute elasticsearch recall: %w", err)
	}
	defer func() { _ = response.Body.Close() }()
	if response.StatusCode < http.StatusOK || response.StatusCode >= http.StatusMultipleChoices {
		raw, _ := io.ReadAll(io.LimitReader(response.Body, 4096))
		return nil, fmt.Errorf("elasticsearch recall status=%s body=%s", response.Status, strings.TrimSpace(string(raw)))
	}
	var payload struct {
		Hits struct {
			Hits []struct {
				ID     string  `json:"_id"`
				Score  float64 `json:"_score"`
				Source struct {
					PostID int64 `json:"post_id"`
				} `json:"_source"`
			} `json:"hits"`
		} `json:"hits"`
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 4<<20)).Decode(&payload); err != nil {
		return nil, fmt.Errorf("decode elasticsearch recall: %w", err)
	}
	reason := "based on recent interests"
	if req.SeedPostID > 0 {
		reason = "similar content"
	}
	result := make([]PostCandidate, 0, len(payload.Hits.Hits))
	seen := make(map[int64]struct{}, len(payload.Hits.Hits))
	for _, hit := range payload.Hits.Hits {
		postID := hit.Source.PostID
		if postID <= 0 {
			postID, _ = strconv.ParseInt(hit.ID, 10, 64)
		}
		if postID <= 0 || containsInt64(seedIDs, postID) {
			continue
		}
		if _, exists := seen[postID]; exists {
			continue
		}
		seen[postID] = struct{}{}
		result = append(result, PostCandidate{PostID: postID, RecallScore: hit.Score, RecallSource: s.Name(), Reason: reason})
	}
	return result, nil
}

type milvusRecallClient interface {
	Query(context.Context, string, []string, string, []string, ...milvusclient.SearchQueryOptionFunc) (milvusclient.ResultSet, error)
	Search(context.Context, string, []string, string, []string, []entity.Vector, string, entity.MetricType, int, entity.SearchParam, ...milvusclient.SearchQueryOptionFunc) ([]milvusclient.SearchResult, error)
	Close() error
}

type milvusClientFactory func(context.Context, milvusclient.Config) (milvusRecallClient, error)

type MilvusPostRecallSource struct {
	address        string
	collection     string
	username       string
	password       string
	database       string
	featureVersion string
	nprobe         int
	timeout        time.Duration
	redis          redisClient
	factory        milvusClientFactory

	mu     sync.Mutex
	client milvusRecallClient
	closed bool
}

func NewMilvusPostRecallSource(
	address string,
	collection string,
	username string,
	password string,
	database string,
	featureVersion string,
	nprobe int,
	redis redisClient,
	timeout time.Duration,
) *MilvusPostRecallSource {
	return &MilvusPostRecallSource{
		address: address, collection: collection, username: username, password: password,
		database: database, featureVersion: featureVersion, nprobe: nprobe, redis: redis, timeout: timeout,
		factory: func(ctx context.Context, config milvusclient.Config) (milvusRecallClient, error) {
			return milvusclient.NewClient(ctx, config)
		},
	}
}

func (s *MilvusPostRecallSource) Name() string { return "milvus" }

func (s *MilvusPostRecallSource) Recall(ctx context.Context, req RecallRequest) ([]PostCandidate, error) {
	if req.Limit <= 0 {
		return nil, fmt.Errorf("milvus recall limit must be positive")
	}
	seedIDs, err := externalRecallSeedIDs(ctx, s.redis, s.featureVersion, req)
	if err != nil {
		return nil, fmt.Errorf("load milvus recall seeds: %w", err)
	}
	if len(seedIDs) == 0 {
		return nil, ErrNotApplicable
	}
	requestCtx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	client, err := s.getClient(requestCtx)
	if err != nil {
		return nil, fmt.Errorf("connect milvus recall: %w", err)
	}
	rows, err := client.Query(requestCtx, s.collection, nil,
		"post_id in ["+joinInt64(seedIDs)+"]", []string{"embedding"})
	if err != nil {
		return nil, fmt.Errorf("query milvus recall seed vectors: %w", err)
	}
	column, ok := rows.GetColumn("embedding").(*entity.ColumnFloatVector)
	if !ok || column.Len() == 0 {
		return nil, ErrNotApplicable
	}
	vectors := make([]entity.Vector, 0, column.Len())
	for _, vector := range column.Data() {
		vectors = append(vectors, entity.FloatVector(vector))
	}
	searchParams, err := entity.NewIndexIvfFlatSearchParam(s.nprobe)
	if err != nil {
		return nil, fmt.Errorf("create milvus recall search parameters: %w", err)
	}
	results, err := client.Search(requestCtx, s.collection, nil,
		"post_id not in ["+joinInt64(seedIDs)+"]", nil, vectors,
		"embedding", entity.L2, req.Limit, searchParams)
	if err != nil {
		return nil, fmt.Errorf("search milvus recall neighbors: %w", err)
	}
	reason := "based on semantic interests"
	if req.SeedPostID > 0 {
		reason = "semantically similar"
	}
	merged := make(map[int64]PostCandidate)
	for _, searchResult := range results {
		if searchResult.Err != nil {
			return nil, fmt.Errorf("decode milvus recall result: %w", searchResult.Err)
		}
		for index := 0; index < searchResult.ResultCount; index++ {
			postID, err := searchResult.IDs.GetAsInt64(index)
			if err != nil || postID <= 0 || containsInt64(seedIDs, postID) {
				continue
			}
			distance := float64(searchResult.Scores[index])
			if distance < 0 {
				distance = 0
			}
			candidate := PostCandidate{
				PostID: postID, RecallScore: 1 / (1 + distance), RecallSource: s.Name(), Reason: reason,
			}
			if current, exists := merged[postID]; !exists || candidate.RecallScore > current.RecallScore {
				merged[postID] = candidate
			}
		}
	}
	result := make([]PostCandidate, 0, len(merged))
	for _, candidate := range merged {
		result = append(result, candidate)
	}
	sort.SliceStable(result, func(i, j int) bool {
		if result[i].RecallScore == result[j].RecallScore {
			return result[i].PostID < result[j].PostID
		}
		return result[i].RecallScore > result[j].RecallScore
	})
	if len(result) > req.Limit {
		result = result[:req.Limit]
	}
	return result, nil
}

func (s *MilvusPostRecallSource) getClient(ctx context.Context) (milvusRecallClient, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil, fmt.Errorf("milvus recall source is closed")
	}
	if s.client != nil {
		return s.client, nil
	}
	client, err := s.factory(ctx, milvusclient.Config{
		Address: s.address, Username: s.username, Password: s.password, DBName: s.database,
		DialOptions: []grpc.DialOption{grpc.WithNoProxy()},
	})
	if err != nil {
		return nil, err
	}
	s.client = client
	return client, nil
}

func (s *MilvusPostRecallSource) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	if s.client == nil {
		return nil
	}
	return s.client.Close()
}

func externalRecallSeedIDs(ctx context.Context, redis redisClient, featureVersion string, req RecallRequest) ([]int64, error) {
	if req.SeedPostID > 0 {
		return []int64{req.SeedPostID}, nil
	}
	if req.Identity == "" || redis == nil {
		return nil, nil
	}
	recent, err := redis.LrangeCtx(ctx, "feature:"+featureVersion+":"+req.Identity+":recent", 0, 49)
	if err != nil {
		return nil, err
	}
	positive := map[string]struct{}{
		"click": {}, "dwell": {}, "like": {}, "favorite": {}, "comment": {}, "share": {},
	}
	result := make([]int64, 0, maxExternalRecallSeeds)
	seen := make(map[int64]struct{}, maxExternalRecallSeeds)
	for _, raw := range recent {
		var item struct {
			Action     string `json:"action"`
			TargetID   int64  `json:"target_id"`
			TargetType string `json:"target_type"`
		}
		if err := json.Unmarshal([]byte(raw), &item); err != nil {
			return nil, fmt.Errorf("decode recent behavior: %w", err)
		}
		if item.TargetType != "post" || item.TargetID <= 0 {
			continue
		}
		if _, ok := positive[item.Action]; !ok {
			continue
		}
		if _, ok := seen[item.TargetID]; ok {
			continue
		}
		seen[item.TargetID] = struct{}{}
		result = append(result, item.TargetID)
		if len(result) == maxExternalRecallSeeds {
			break
		}
	}
	return result, nil
}

func joinInt64(values []int64) string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		result = append(result, strconv.FormatInt(value, 10))
	}
	return strings.Join(result, ",")
}

func containsInt64(values []int64, wanted int64) bool {
	return slices.Contains(values, wanted)
}
