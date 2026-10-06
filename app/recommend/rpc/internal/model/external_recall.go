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

	"esx/app/recommend/featurekey"
	"esx/pkg/vectorprojection"

	milvusclient "github.com/milvus-io/milvus-sdk-go/v2/client"
	"github.com/milvus-io/milvus-sdk-go/v2/entity"
	"google.golang.org/grpc"
)

const maxExternalRecallSeeds = 5

// ElasticsearchPostRecallSource 以观看者最近浏览或指定种子帖为参照，用 ES more_like_this 召回相似帖子。
type ElasticsearchPostRecallSource struct {
	endpoint string
	index    string
	username string
	password string
	features featurekey.Space
	redis    redisClient
	client   *http.Client
	timeout  time.Duration
}

// ElasticsearchRecallOptions 是 Elasticsearch 相似帖召回的连接参数；Redis 用于读取观看者近期行为作为种子。
type ElasticsearchRecallOptions struct {
	Addresses      []string
	Index          string
	Username       string
	Password       string
	FeatureVersion string
	Redis          redisClient
	Timeout        time.Duration
}

// NewElasticsearchPostRecallSource 校验地址、索引与超时；只使用第一个地址，且不走环境代理。
func NewElasticsearchPostRecallSource(opts ElasticsearchRecallOptions) (*ElasticsearchPostRecallSource, error) {
	if len(opts.Addresses) == 0 || strings.TrimSpace(opts.Addresses[0]) == "" || strings.TrimSpace(opts.Index) == "" {
		return nil, fmt.Errorf("elasticsearch recall address and index are required")
	}
	endpoint := strings.TrimRight(strings.TrimSpace(opts.Addresses[0]), "/")
	if _, err := url.ParseRequestURI(endpoint); err != nil {
		return nil, fmt.Errorf("parse elasticsearch recall address: %w", err)
	}
	if opts.Timeout <= 0 {
		return nil, fmt.Errorf("elasticsearch recall timeout must be positive")
	}
	// 召回请求只访问内网 ES，不能被环境变量里的 HTTP 代理劫持。
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.Proxy = nil
	return &ElasticsearchPostRecallSource{
		endpoint: endpoint, index: opts.Index, username: opts.Username, password: opts.Password,
		features: featurekey.New(opts.FeatureVersion), redis: opts.Redis,
		client: &http.Client{Transport: transport}, timeout: opts.Timeout,
	}, nil
}

// Name 返回召回来源标识。
func (s *ElasticsearchPostRecallSource) Name() string { return "es" }

// Recall 用种子帖召回相似帖子；没有种子时返回空结果。
func (s *ElasticsearchPostRecallSource) Recall(ctx context.Context, req RecallRequest) ([]PostCandidate, error) {
	if req.Limit <= 0 {
		return nil, fmt.Errorf("elasticsearch recall limit must be positive")
	}
	seedIDs, err := externalRecallSeedIDs(ctx, s.redis, s.features, req)
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

// milvusRecallClient 是向量召回用到的 Milvus 客户端子集，便于测试替换。
type milvusRecallClient interface {
	Query(context.Context, string, []string, string, []string, ...milvusclient.SearchQueryOptionFunc) (milvusclient.ResultSet, error)
	Search(context.Context, string, []string, string, []string, []entity.Vector, string, entity.MetricType, int, entity.SearchParam, ...milvusclient.SearchQueryOptionFunc) ([]milvusclient.SearchResult, error)
	Close() error
}

// milvusClientFactory 创建 Milvus 客户端，测试可注入假实现。
type milvusClientFactory func(context.Context, milvusclient.Config) (milvusRecallClient, error)

// MilvusPostRecallSource 以种子帖的向量在 Milvus 中检索相近帖子；连接按需建立。
type MilvusPostRecallSource struct {
	address    string
	collection string
	username   string
	password   string
	database   string
	features   featurekey.Space
	nprobe     int
	timeout    time.Duration
	redis      redisClient
	factory    milvusClientFactory

	mu     sync.Mutex
	client milvusRecallClient
	closed bool
}

// MilvusRecallOptions 是 Milvus 向量召回的连接与检索参数；Redis 用于读取观看者近期行为作为种子。
type MilvusRecallOptions struct {
	Address        string
	Collection     string
	Username       string
	Password       string
	Database       string
	FeatureVersion string
	NProbe         int
	Redis          redisClient
	Timeout        time.Duration
}

// NewMilvusPostRecallSource 只保存参数，连接在首次召回时惰性建立。
func NewMilvusPostRecallSource(opts MilvusRecallOptions) *MilvusPostRecallSource {
	return &MilvusPostRecallSource{
		address: opts.Address, collection: opts.Collection, username: opts.Username, password: opts.Password,
		database: opts.Database, features: featurekey.New(opts.FeatureVersion), nprobe: opts.NProbe,
		redis: opts.Redis, timeout: opts.Timeout,
		factory: func(ctx context.Context, config milvusclient.Config) (milvusRecallClient, error) {
			return milvusclient.NewClient(ctx, config)
		},
	}
}

// Name 返回召回来源标识。
func (s *MilvusPostRecallSource) Name() string { return "milvus" }

// Recall 取种子帖向量做近邻检索，排除种子本身；没有种子时返回空结果。
func (s *MilvusPostRecallSource) Recall(ctx context.Context, req RecallRequest) ([]PostCandidate, error) {
	if req.Limit <= 0 {
		return nil, fmt.Errorf("milvus recall limit must be positive")
	}
	seedIDs, err := externalRecallSeedIDs(ctx, s.redis, s.features, req)
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
	seeds, err := vectorprojection.Latest(requestCtx, client, s.collection, seedIDs, true)
	if err != nil {
		return nil, fmt.Errorf("query latest seed vectors: %w", err)
	}
	vectors := make([]entity.Vector, 0, len(seeds))
	for _, id := range seedIDs {
		if row, ok := seeds[id]; ok && !row.Deleted {
			vectors = append(vectors, entity.FloatVector(row.Vector))
		}
	}
	if len(vectors) == 0 {
		return nil, ErrNotApplicable
	}

	searchParams, err := entity.NewIndexIvfFlatSearchParam(s.nprobe)
	if err != nil {
		return nil, fmt.Errorf("create milvus recall search parameters: %w", err)
	}
	results, err := client.Search(requestCtx, s.collection, nil,
		"deleted == false && post_id not in ["+joinInt64(seedIDs)+"]", []string{"post_id", "revision"}, vectors,
		"embedding", entity.L2, req.Limit, searchParams, milvusclient.WithSearchQueryConsistencyLevel(entity.ClStrong))
	if err != nil {
		return nil, fmt.Errorf("search milvus recall neighbors: %w", err)
	}
	reason := "based on semantic interests"
	if req.SeedPostID > 0 {
		reason = "semantically similar"
	}
	type hit struct {
		id, revision int64
		distance     float64
	}
	var hits []hit
	candidateIDs := make([]int64, 0)
	seenIDs := map[int64]bool{}
	for _, searchResult := range results {
		if searchResult.Err != nil {
			return nil, fmt.Errorf("decode milvus recall result: %w", searchResult.Err)
		}
		ids, ok := searchResult.Fields.GetColumn("post_id").(*entity.ColumnInt64)
		revisions, revOK := searchResult.Fields.GetColumn("revision").(*entity.ColumnInt64)
		if !ok || !revOK || ids.Len() != searchResult.ResultCount || revisions.Len() != searchResult.ResultCount || len(searchResult.Scores) != searchResult.ResultCount {
			return nil, fmt.Errorf("versioned Milvus recall metadata missing; rebuild required")
		}
		for i, id := range ids.Data() {
			if id <= 0 || containsInt64(seedIDs, id) {
				continue
			}
			hits = append(hits, hit{id: id, revision: revisions.Data()[i], distance: float64(searchResult.Scores[i])})
			if !seenIDs[id] {
				candidateIDs = append(candidateIDs, id)
				seenIDs[id] = true
			}
		}
	}
	latest, err := vectorprojection.Latest(requestCtx, client, s.collection, candidateIDs, false)
	if err != nil {
		return nil, fmt.Errorf("verify latest recall revisions: %w", err)
	}
	merged := make(map[int64]PostCandidate)
	for _, hit := range hits {
		row, ok := latest[hit.id]
		if !ok || row.Deleted || row.Revision != hit.revision {
			continue
		}
		distance := max(hit.distance, 0)
		candidate := PostCandidate{PostID: hit.id, RecallScore: 1 / (1 + distance), RecallSource: s.Name(), Reason: reason}
		if current, exists := merged[hit.id]; !exists || candidate.RecallScore > current.RecallScore {
			merged[hit.id] = candidate
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

// getClient 懒加载 Milvus 连接，启动时 Milvus 不可用不阻塞服务；关闭后拒绝再建立连接。
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

// Close 关闭 Milvus 连接，并阻止之后再建立连接。
func (s *MilvusPostRecallSource) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closed = true
	if s.client == nil {
		return nil
	}
	return s.client.Close()
}

// externalRecallSeedIDs 返回外部召回的种子帖：显式指定的帖子，或观看者最近浏览的帖子（最多 50 个）。
func externalRecallSeedIDs(ctx context.Context, redis redisClient, features featurekey.Space, req RecallRequest) ([]int64, error) {
	if req.SeedPostID > 0 {
		return []int64{req.SeedPostID}, nil
	}
	if req.Identity == "" || redis == nil {
		return nil, nil
	}
	recent, err := redis.LrangeCtx(ctx, features.ViewerRecent(req.Identity), 0, 49)
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

// joinInt64 把 ID 列表拼成逗号分隔的字符串。
func joinInt64(values []int64) string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		result = append(result, strconv.FormatInt(value, 10))
	}
	return strings.Join(result, ",")
}

// containsInt64 判断列表是否包含 ID。
func containsInt64(values []int64, wanted int64) bool {
	return slices.Contains(values, wanted)
}
