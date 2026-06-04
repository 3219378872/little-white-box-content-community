# Phase 3: 搜索系统

## 概述

### 阶段目标
构建完整的多路召回、精排、重排搜索系统，展示 Go 并发模型的最佳实践。

### 预计周期
6 周

### 前置条件
- Phase 1-2 已完成
- Elasticsearch 8.x 正常运行
- Milvus 2.x 正常运行
- 所有核心服务正常运行

---

## 详细任务清单

### W1: ES 索引设计

#### 任务 1.1: 创建 Search RPC 服务
**涉及模块**: `app/search/rpc/`

**生成命令**:
```bash
cd app/search/rpc
goctl rpc protoc ../../proto/search/search.proto --go_out=. --go-grpc_out=. --zrpc_out=. --style go_zero
```

**search.proto 定义**:
```protobuf
syntax = "proto3";
package search;
option go_package = "./pb";

service SearchService {
  rpc Search(SearchReq) returns (SearchResp);
  rpc SyncPostIndex(SyncPostIndexReq) returns (SyncPostIndexResp);
  rpc GetHotSearch(GetHotSearchReq) returns (GetHotSearchResp);
  rpc Suggest(SuggestReq) returns (SuggestResp);
}

message SearchReq {
  string query = 1;
  int64 user_id = 2;
  int32 page = 3;
  int32 page_size = 4;
  repeated string filters = 5;  // 筛选条件
}

message SearchResult {
  int64 post_id = 1;
  string title = 2;
  string content_highlight = 3;
  float score = 4;
  int64 created_at = 5;
}

message SearchResp {
  repeated SearchResult results = 1;
  int64 total = 2;
  bool has_more = 3;
}
```

**验收标准**:
- [ ] Search RPC 服务启动成功

---

#### 任务 1.2: 设计 ES 索引 Mapping
**涉及模块**: `deploy/es/`

**帖子索引 Mapping**:
```json
{
  "mappings": {
    "properties": {
      "id": { "type": "long" },
      "title": {
        "type": "text",
        "analyzer": "ik_max_word",
        "search_analyzer": "ik_smart",
        "fields": {
          "keyword": { "type": "keyword" }
        }
      },
      "content": {
        "type": "text",
        "analyzer": "ik_max_word",
        "search_analyzer": "ik_smart"
      },
      "author_id": { "type": "long" },
      "author_name": {
        "type": "text",
        "analyzer": "ik_max_word"
      },
      "tags": { "type": "keyword" },
      "status": { "type": "integer" },
      "like_count": { "type": "long" },
      "comment_count": { "type": "long" },
      "created_at": { "type": "date" },
      "updated_at": { "type": "date" }
    }
  }
}
```

**验收标准**:
- [ ] 索引创建成功
- [ ] 中文分词正常

---

#### 任务 1.3: 集成 ES 客户端
**涉及模块**: `pkg/esx/`, `app/search/rpc/internal/svc/`

**ES 客户端封装**:
```go
// pkg/esx/client.go
type ESClient struct {
    client *elastic.Client
}

func NewESClient(urls []string) (*ESClient, error) {
    client, err := elastic.NewClient(
        elastic.SetURL(urls...),
        elastic.SetSniff(false),
    )
    return &ESClient{client: client}, err
}

func (c *ESClient) Index(ctx context.Context, index string, id string, doc interface{}) error {
    _, err := c.client.Index().
        Index(index).
        Id(id).
        BodyJson(doc).
        Do(ctx)
    return err
}

func (c *ESClient) Search(ctx context.Context, index string, query elastic.Query, from, size int) (*elastic.SearchResult, error) {
    return c.client.Search().
        Index(index).
        Query(query).
        From(from).
        Size(size).
        Highlight(elastic.NewHighlight().
            Field("title").
            Field("content")).
        Do(ctx)
}
```

**验收标准**:
- [ ] ES 连接正常
- [ ] 索引/搜索操作正常

---

### W2: 向量检索

#### 任务 2.1: 集成 Milvus 客户端
**涉及模块**: `pkg/milvusx/`, `app/search/rpc/internal/svc/`

**Milvus 客户端封装**:
```go
// pkg/milvusx/client.go
type MilvusClient struct {
    client *milvus.Client
}

func NewMilvusClient(addr string) (*MilvusClient, error) {
    client, err := milvus.NewClient(context.Background(),
        milvus.Config{
            Address: addr,
        },
    )
    return &MilvusClient{client: client}, err
}

func (c *MilvusClient) Search(ctx context.Context, collection string, vector []float32, topK int) ([]int64, error) {
    result, err := c.client.Search(ctx, &milvus.SearchRequest{
        CollectionName: collection,
        Vectors:        []entity.Vector{entity.FloatVector(vector)},
        TopK:           int32(topK),
    })
    if err != nil {
        return nil, err
    }

    var ids []int64
    for _, hit := range result {
        ids = append(ids, hit.IDs...)
    }
    return ids, nil
}
```

**验收标准**:
- [ ] Milvus 连接正常
- [ ] 向量搜索正常

---

#### 任务 2.2: 创建向量 Collection
**涉及模块**: `deploy/milvus/`

**Collection 设计**:
```go
// 帖子向量 Collection
func CreatePostVectorCollection(client *milvus.Client) error {
    schema := &entity.Schema{
        CollectionName: "post_vectors",
        AutoID:         false,
        Fields: []*entity.Field{
            {
                Name:       "post_id",
                DataType:   entity.FieldTypeInt64,
                PrimaryKey: true,
            },
            {
                Name:     "vector",
                DataType: entity.FieldTypeFloatVector,
                TypeParams: map[string]string{
                    "dim": "768",  // 嵌入向量维度
                },
            },
        },
    }

    return client.CreateCollection(context.Background(), schema, 2) // 2 分片
}
```

**验收标准**:
- [ ] Collection 创建成功
- [ ] 索引创建成功

---

#### 任务 2.3: 实现文本嵌入
**涉及模块**: `app/search/rpc/internal/logic/`

**嵌入服务集成**:
```go
// 使用开源嵌入模型或外部 API
type EmbeddingService interface {
    Embed(ctx context.Context, text string) ([]float32, error)
    EmbedBatch(ctx context.Context, texts []string) ([][]float32, error)
}

// 实现：使用本地模型或远程 API
type OpenAIEmbedding struct {
    apiKey string
    model  string
}

func (e *OpenAIEmbedding) Embed(ctx context.Context, text string) ([]float32, error) {
    // 调用嵌入 API
    resp, err := e.client.CreateEmbeddings(ctx, &openai.EmbeddingRequest{
        Model: e.model,
        Input: text,
    })
    if err != nil {
        return nil, err
    }
    return resp.Data[0].Embedding, nil
}
```

**验收标准**:
- [ ] 文本嵌入正常
- [ ] 向量维度正确（768）

---

### W3: 多路召回

#### 任务 3.1: 实现召回接口
**涉及模块**: `app/search/rpc/internal/logic/`

**召回器接口设计**:
```go
// 召回器统一接口
type Recaller interface {
    Recall(ctx context.Context, query string, userId int64, limit int) ([]*RecallItem, error)
    Name() string
}

type RecallItem struct {
    PostId    int64
    Score     float64
    Source    string  // 召回来源
    Extra     map[string]interface{}
}
```

**验收标准**:
- [ ] 召回接口定义清晰
- [ ] 各召回器实现该接口

---

#### 任务 3.2: 实现 ES 文本召回
**涉及模块**: `app/search/rpc/internal/logic/recall/`

**ES 召回器**:
```go
type ESRecaller struct {
    esClient *esx.ESClient
}

func (r *ESRecaller) Recall(ctx context.Context, query string, userId int64, limit int) ([]*RecallItem, error) {
    // BM25 文本匹配
    boolQuery := elastic.NewBoolQuery().
        Should(
            elastic.NewMatchQuery("title", query).Boost(2.0),
            elastic.NewMatchQuery("content", query),
            elastic.NewMatchQuery("author_name", query).Boost(0.5),
        )

    result, err := r.esClient.Search(ctx, "posts", boolQuery, 0, limit)
    if err != nil {
        return nil, err
    }

    var items []*RecallItem
    for _, hit := range result.Hits.Hits {
        var post struct {
            ID int64 `json:"id"`
        }
        json.Unmarshal(hit.Source, &post)
        items = append(items, &RecallItem{
            PostId: post.ID,
            Score:  *hit.Score,
            Source: "es_text",
        })
    }
    return items, nil
}

func (r *ESRecaller) Name() string { return "es_text" }
```

**验收标准**:
- [ ] ES 召回正常
- [ ] BM25 评分正确

---

#### 任务 3.3: 实现向量召回
**涉及模块**: `app/search/rpc/internal/logic/recall/`

**向量召回器**:
```go
type VectorRecaller struct {
    milvusClient *milvusx.MilvusClient
    embedding    EmbeddingService
}

func (r *VectorRecaller) Recall(ctx context.Context, query string, userId int64, limit int) ([]*RecallItem, error) {
    // 1. 文本转向量
    vector, err := r.embedding.Embed(ctx, query)
    if err != nil {
        return nil, err
    }

    // 2. Milvus 相似度搜索
    ids, err := r.milvusClient.Search(ctx, "post_vectors", vector, limit)
    if err != nil {
        return nil, err
    }

    // 3. 构造召回结果
    var items []*RecallItem
    for i, id := range ids {
        items = append(items, &RecallItem{
            PostId: id,
            Score:  float64(limit - i), // 距离转换为分数
            Source: "vector",
        })
    }
    return items, nil
}

func (r *VectorRecaller) Name() string { return "vector" }
```

**验收标准**:
- [ ] 向量召回正常
- [ ] 相似度排序正确

---

#### 任务 3.4: 实现热门召回
**涉及模块**: `app/search/rpc/internal/logic/recall/`

**热门召回器**:
```go
type HotRecaller struct {
    redis *redis.Client
}

func (r *HotRecaller) Recall(ctx context.Context, query string, userId int64, limit int) ([]*RecallItem, error) {
    // 从 Redis 热门榜获取
    ids, err := r.redis.ZRevRange(ctx, "hot_posts", 0, int64(limit-1)).Result()
    if err != nil {
        return nil, err
    }

    var items []*RecallItem
    for i, idStr := range ids {
        id, _ := strconv.ParseInt(idStr, 10, 64)
        items = append(items, &RecallItem{
            PostId: id,
            Score:  float64(limit - i),
            Source: "hot",
        })
    }
    return items, nil
}

func (r *HotRecaller) Name() string { return "hot" }
```

**验收标准**:
- [ ] 热门召回正常
- [ ] 热榜数据正确

---

#### 任务 3.5: 实现标签召回
**涉及模块**: `app/search/rpc/internal/logic/recall/`

**标签召回器**:
```go
type TagRecaller struct {
    esClient *esx.ESClient
}

func (r *TagRecaller) Recall(ctx context.Context, query string, userId int64, limit int) ([]*RecallItem, error) {
    // 标签精确匹配
    query := elastic.NewTermQuery("tags", query)

    result, err := r.esClient.Search(ctx, "posts", query, 0, limit)
    if err != nil {
        return nil, err
    }

    var items []*RecallItem
    for _, hit := range result.Hits.Hits {
        var post struct {
            ID int64 `json:"id"`
        }
        json.Unmarshal(hit.Source, &post)
        items = append(items, &RecallItem{
            PostId: post.ID,
            Score:  *hit.Score,
            Source: "tag",
        })
    }
    return items, nil
}

func (r *TagRecaller) Name() string { return "tag" }
```

**验收标准**:
- [ ] 标签召回正常

---

#### 任务 3.6: 实现个性化召回
**涉及模块**: `app/search/rpc/internal/logic/recall/`

**个性化召回器**:
```go
type PersonalRecaller struct {
    redis       *redis.Client
    milvus      *milvusx.MilvusClient
    embedding   EmbeddingService
}

func (r *PersonalRecaller) Recall(ctx context.Context, query string, userId int64, limit int) ([]*RecallItem, error) {
    if userId == 0 {
        return nil, nil // 未登录无个性化
    }

    // 1. 获取用户兴趣向量
    userVector, err := r.redis.Get(ctx, fmt.Sprintf("user_vector:%d", userId)).Result()
    if err != nil {
        return nil, err
    }

    // 2. 向量搜索
    vector := parseVector(userVector)
    ids, err := r.milvus.Search(ctx, "post_vectors", vector, limit)
    if err != nil {
        return nil, err
    }

    var items []*RecallItem
    for i, id := range ids {
        items = append(items, &RecallItem{
            PostId: id,
            Score:  float64(limit - i),
            Source: "personal",
        })
    }
    return items, nil
}

func (r *PersonalRecaller) Name() string { return "personal" }
```

**验收标准**:
- [ ] 个性化召回正常
- [ ] 用户兴趣匹配

---

#### 任务 3.7: 实现并发多路召回
**涉及模块**: `app/search/rpc/internal/logic/`

**核心并发逻辑**:
```go
func (l *SearchLogic) multiRecall(ctx context.Context, query string, userId int64) ([]*RecallItem, error) {
    g, ctx := errgroup.WithContext(ctx)

    var (
        esItems       []*RecallItem
        vectorItems   []*RecallItem
        hotItems      []*RecallItem
        tagItems      []*RecallItem
        personalItems []*RecallItem
    )

    // 5 路召回并行执行
    g.Go(func() error {
        var err error
        esItems, err = l.esRecaller.Recall(ctx, query, userId, 200)
        return err
    })

    g.Go(func() error {
        var err error
        vectorItems, err = l.vectorRecaller.Recall(ctx, query, userId, 200)
        return err
    })

    g.Go(func() error {
        var err error
        hotItems, err = l.hotRecaller.Recall(ctx, query, userId, 50)
        return err
    })

    g.Go(func() error {
        var err error
        tagItems, err = l.tagRecaller.Recall(ctx, query, userId, 100)
        return err
    })

    g.Go(func() error {
        if userId == 0 {
            return nil // 未登录跳过
        }
        var err error
        personalItems, err = l.personalRecaller.Recall(ctx, query, userId, 100)
        return err
    })

    if err := g.Wait(); err != nil {
        logx.Errorf("partial recall failed: %v", err)
        // 部分失败降级：用已成功的结果继续
    }

    return l.mergeAndDedup(esItems, vectorItems, hotItems, tagItems, personalItems), nil
}
```

**验收标准**:
- [ ] 并发召回正常
- [ ] 超时控制正确
- [ ] 去重逻辑正确

---

### W4: 精排模型

#### 任务 4.1: 设计排序特征
**涉及模块**: `app/search/rpc/internal/logic/rank/`

**特征列表**:

| 特征 | 类型 | 说明 |
|------|------|------|
| text_score | float | 文本匹配分 |
| vector_score | float | 向量相似度 |
| like_count | int | 点赞数 |
| comment_count | int | 评论数 |
| author_level | int | 作者等级 |
| recency | float | 时效性 |
| user_affinity | float | 用户亲和度 |

**特征提取**:
```go
type RankFeatures struct {
    TextScore     float64
    VectorScore   float64
    LikeCount     int64
    CommentCount  int64
    AuthorLevel   int32
    Recency       float64
    UserAffinity  float64
}

func (l *SearchLogic) extractFeatures(ctx context.Context, item *RecallItem, userId int64) (*RankFeatures, error) {
    // 并发获取各项特征
    g, ctx := errgroup.WithContext(ctx)

    var post *model.Post
    var author *user.UserInfo

    g.Go(func() error {
        var err error
        post, err = l.svcCtx.PostModel.FindOne(ctx, item.PostId)
        return err
    })

    g.Go(func() error {
        if post == nil {
            return nil
        }
        resp, err := l.svcCtx.UserRpc.GetUser(ctx, &user.GetUserReq{UserId: post.AuthorId})
        author = resp
        return err
    })

    if err := g.Wait(); err != nil {
        return nil, err
    }

    return &RankFeatures{
        TextScore:    item.Score,
        LikeCount:    post.LikeCount,
        CommentCount: post.CommentCount,
        AuthorLevel:  author.Level,
        Recency:      calculateRecency(post.CreatedAt),
    }, nil
}
```

**验收标准**:
- [ ] 特征提取完整
- [ ] 并发获取正常

---

#### 任务 4.2: 实现排序模型
**涉及模块**: `app/search/rpc/internal/logic/rank/`

**排序器接口**:
```go
type Ranker interface {
    Score(ctx context.Context, item *RankItem) float64
    Name() string
}

// 加权融合排序器
type WeightedRanker struct {
    rankers []struct {
        ranker Ranker
        weight float64
    }
}

func (w *WeightedRanker) Score(ctx context.Context, item *RankItem) float64 {
    var score float64
    for _, r := range w.rankers {
        score += r.weight * r.ranker.Score(ctx, item)
    }
    return score
}
```

**验收标准**:
- [ ] 排序器接口定义
- [ ] 加权融合实现

---

#### 任务 4.3: 实现各排序策略
**涉及模块**: `app/search/rpc/internal/logic/rank/`

**相关性排序**:
```go
type RelevanceRanker struct{}

func (r *RelevanceRanker) Score(ctx context.Context, item *RankItem) float64 {
    return item.Features.TextScore * 0.6 + item.Features.VectorScore * 0.4
}

func (r *RelevanceRanker) Name() string { return "relevance" }
```

**热度排序**:
```go
type PopularityRanker struct{}

func (p *PopularityRanker) Score(ctx context.Context, item *RankItem) float64 {
    // Hacker News 算法
    gravity := 1.8
    return float64(item.Features.LikeCount+item.Features.CommentCount*2) /
        math.Pow(item.Features.Recency+2, gravity)
}

func (p *PopularityRanker) Name() string { return "popularity" }
```

**个性化排序**:
```go
type PersonalRanker struct{}

func (p *PersonalRanker) Score(ctx context.Context, item *RankItem) float64 {
    return item.Features.UserAffinity * 0.3 +
        item.Features.TextScore * 0.4 +
        float64(item.Features.AuthorLevel) * 0.01
}

func (p *PersonalRanker) Name() string { return "personal" }
```

**验收标准**:
- [ ] 各排序策略实现
- [ ] 分数计算正确

---

### W5: 重排优化

#### 任务 5.1: 实现多样性重排
**涉及模块**: `app/search/rpc/internal/logic/rerank/`

**多样性算法 (MMR)**:
```go
func (l *SearchLogic) diversify(items []*RankItem, topK int) []*RankItem {
    result := make([]*RankItem, 0, topK)
    selected := make(map[int64]bool)

    for len(result) < topK && len(items) > 0 {
        var bestItem *RankItem
        var bestScore float64 = -1

        for _, item := range items {
            if selected[item.PostId] {
                continue
            }

            // MMR 分数 = 相关性 - λ * 与已选的最大相似度
            mmrScore := item.Score - 0.5*l.maxSimilarity(item, result)
            if mmrScore > bestScore {
                bestScore = mmrScore
                bestItem = item
            }
        }

        if bestItem == nil {
            break
        }

        result = append(result, bestItem)
        selected[bestItem.PostId] = true
    }

    return result
}

func (l *SearchLogic) maxSimilarity(item *RankItem, selected []*RankItem) float64 {
    var maxSim float64
    for _, s := range selected {
        sim := cosineSimilarity(item.Vector, s.Vector)
        if sim > maxSim {
            maxSim = sim
        }
    }
    return maxSim
}
```

**验收标准**:
- [ ] 多样性重排正常
- [ ] 结果不重复

---

#### 任务 5.2: 实现业务规则重排
**涉及模块**: `app/search/rpc/internal/logic/rerank/`

**业务规则**:
```go
func (l *SearchLogic) applyBusinessRules(items []*RankItem) []*RankItem {
    // 规则 1: 置顶内容
    items = l.pinTopContent(items)

    // 规则 2: 过滤敏感内容
    items = l.filterSensitive(items)

    // 规则 3: 保证作者多样性（同一作者最多 2 条）
    items = l.limitAuthorPosts(items, 2)

    return items
}

func (l *SearchLogic) limitAuthorPosts(items []*RankItem, maxPerAuthor int) []*RankItem {
    result := make([]*RankItem, 0, len(items))
    authorCount := make(map[int64]int)

    for _, item := range items {
        if authorCount[item.AuthorId] < maxPerAuthor {
            result = append(result, item)
            authorCount[item.AuthorId]++
        }
    }
    return result
}
```

**验收标准**:
- [ ] 业务规则生效
- [ ] 作者多样性保证

---

### W6: 搜索集成

#### 任务 6.1: 实现完整搜索流程
**涉及模块**: `app/search/rpc/internal/logic/`

**完整搜索流程**:
```go
func (l *SearchLogic) Search(ctx context.Context, req *search.SearchReq) (*search.SearchResp, error) {
    // 设置总超时
    ctx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
    defer cancel()

    // Phase 1: 召回（限时 100ms）
    recallCtx, recallCancel := context.WithTimeout(ctx, 100*time.Millisecond)
    defer recallCancel()

    candidates, err := l.multiRecall(recallCtx, req.Query, req.UserId)
    if err != nil {
        return nil, err
    }

    // Phase 2: 精排（用剩余时间）
    rankedItems, err := l.rank(ctx, candidates, req.UserId)
    if err != nil {
        return nil, err
    }

    // Phase 3: 重排
    rerankedItems := l.rerank(ctx, rankedItems, int(req.PageSize))

    // 构造响应
    return l.buildResponse(rerankedItems), nil
}
```

**验收标准**:
- [ ] 搜索流程完整
- [ ] 超时控制正确

---

#### 任务 6.2: 实现热搜功能
**涉及模块**: `app/search/rpc/internal/logic/`

**热搜实现**:
```go
func (l *GetHotSearchLogic) GetHotSearch(ctx context.Context, req *search.GetHotSearchReq) (*search.GetHotSearchResp, error) {
    // 从 Redis 获取热搜榜
    result, err := l.svcCtx.Redis.ZRevRangeWithScores(ctx, "hot_search", 0, 50).Result()
    if err != nil {
        return nil, err
    }

    var items []*search.HotSearchItem
    for i, item := range result {
        items = append(items, &search.HotSearchItem{
            Rank:       int32(i + 1),
            Keyword:    item.Member.(string),
            SearchCount: int64(item.Score),
        })
    }

    return &search.GetHotSearchResp{Items: items}, nil
}

// 搜索词统计（在搜索时调用）
func (l *SearchLogic) recordSearchKeyword(ctx context.Context, keyword string) {
    l.svcCtx.Redis.ZIncrBy(ctx, "hot_search", 1, keyword)
}
```

**验收标准**:
- [ ] 热搜榜显示正常
- [ ] 搜索词统计正确

---

#### 任务 6.3: 实现搜索建议
**涉及模块**: `app/search/rpc/internal/logic/`

**搜索建议**:
```go
func (l *SuggestLogic) Suggest(ctx context.Context, req *search.SuggestReq) (*search.SuggestResp, error) {
    // ES Completion Suggester
    suggestResult, err := l.svcCtx.ESClient.Suggest(ctx, "posts", req.Query)
    if err != nil {
        return nil, err
    }

    var suggestions []string
    for _, option := range suggestResult {
        suggestions = append(suggestions, option.Text)
    }

    return &search.SuggestResp{Suggestions: suggestions}, nil
}
```

**验收标准**:
- [ ] 搜索建议正常
- [ ] 响应时间 < 50ms

---

#### 任务 6.4: Gateway 搜索接口
**涉及模块**: `app/gateway/internal/logic/`

**搜索 API**:
```go
func (l *SearchLogic) Search(req *types.SearchReq) (*types.SearchResp, error) {
    resp, err := l.svcCtx.SearchRpc.Search(l.ctx, &search.SearchReq{
        Query:    req.Query,
        UserId:   l.ctx.Value("userId").(int64),
        Page:     req.Page,
        PageSize: req.PageSize,
    })
    if err != nil {
        return nil, err
    }

    // 转换响应
    return &types.SearchResp{
        Results:  convertResults(resp.Results),
        Total:    resp.Total,
        HasMore:  resp.HasMore,
    }, nil
}
```

**验收标准**:
- [ ] 搜索 API 正常
- [ ] 响应格式正确

---

## 技术要点

### goroutine 并发模型

**errgroup 并发模式**:
```go
g, ctx := errgroup.WithContext(ctx)

// 并发执行多个任务
g.Go(func() error { return task1(ctx) })
g.Go(func() error { return task2(ctx) })
g.Go(func() error { return task3(ctx) })

// 等待所有任务完成
if err := g.Wait(); err != nil {
    // 处理错误
}
```

**优势**:
- goroutine 初始栈仅 2KB（vs Java 线程 1MB）
- 可轻松创建上万并发
- 零成本创建，无需线程池管理

### context 超时级联

```go
// 总超时
ctx, cancel := context.WithTimeout(ctx, 500*time.Millisecond)
defer cancel()

// 子阶段独立超时
recallCtx, recallCancel := context.WithTimeout(ctx, 100*time.Millisecond)
defer recallCancel()

// context 自动传播取消信号
```

### Go interface 组合

```go
// 统一召回接口
type Recaller interface {
    Recall(ctx context.Context, query string, userId int64, limit int) ([]*RecallItem, error)
    Name() string
}

// 统一排序接口
type Ranker interface {
    Score(ctx context.Context, item *RankItem) float64
    Name() string
}

// 组合多个实现
type CompositeRecaller struct {
    recallers []Recaller
}
```

---

## 依赖与风险

### 外部依赖
| 依赖 | 用途 |
|------|------|
| Elasticsearch | 文本索引/搜索 |
| Milvus | 向量检索 |
| Redis | 缓存/热搜榜 |

### 潜在风险

| 风险 | 等级 | 缓解措施 |
|------|------|---------|
| Milvus Go SDK 文档少 | MEDIUM | 参考 Python 示例，原型验证 |
| 嵌入模型性能 | MEDIUM | 本地缓存嵌入向量 |
| 召回层延迟 | MEDIUM | 设置独立超时，降级处理 |

---

## 验收标准

### 功能验收
- [ ] 搜索返回相关结果
- [ ] 多路召回正常工作
- [ ] 精排/重排生效
- [ ] 热搜榜正常
- [ ] 搜索建议正常

### 性能验收
- [ ] 搜索 P99 < 100ms
- [ ] 召回阶段 < 100ms
- [ ] 排序阶段 < 200ms

### 测试验收
- [ ] 单元测试覆盖率 > 80%
- [ ] 搜索相关性测试通过

---

## 交付物清单

| 交付物 | 路径 |
|--------|------|
| Search RPC | `app/search/rpc/` |
| ES 索引配置 | `deploy/es/` |
| Milvus Collection | `deploy/milvus/` |
| 召回器实现 | `app/search/rpc/internal/logic/recall/` |
| 排序器实现 | `app/search/rpc/internal/logic/rank/` |
| 重排器实现 | `app/search/rpc/internal/logic/rerank/` |

---

## 下一步

Phase 3 完成后，进入 [Phase 4: 推荐系统](phase-4-recommend.md)。
