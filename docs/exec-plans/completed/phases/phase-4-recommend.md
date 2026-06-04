# Phase 4: 推荐系统

## 概述

### 阶段目标
实现基于 channel Pipeline 的推荐漏斗系统，包含用户画像、多路召回、精排重排和冷启动策略。

### 预计周期
5 周

### 前置条件
- Phase 1-3 已完成
- 用户行为数据积累足够
- Elasticsearch/Milvus 正常运行

---

## 详细任务清单

### W1: 用户画像

#### 任务 1.1: 创建 Recommend RPC 服务
**涉及模块**: `app/recommend/rpc/`

**生成命令**:
```bash
cd app/recommend/rpc
goctl rpc protoc ../../proto/recommend/recommend.proto --go_out=. --go-grpc_out=. --zrpc_out=. --style go_zero
```

**recommend.proto 定义**:
```protobuf
syntax = "proto3";
package recommend;
option go_package = "./pb";

service RecommendService {
  rpc GetFeed(GetFeedReq) returns (GetFeedResp);
  rpc GetUserProfile(GetUserProfileReq) returns (GetUserProfileResp);
  rpc UpdateUserProfile(UpdateUserProfileReq) returns (UpdateUserProfileResp);
  rpc GetColdStartFeed(GetColdStartFeedReq) returns (GetColdStartFeedResp);
}

message GetFeedReq {
  int64 user_id = 1;
  int32 page = 2;
  int32 page_size = 3;
  string scene = 4;  // home, discover, profile
}

message FeedItem {
  int64 post_id = 1;
  float score = 2;
  string recall_source = 3;
}

message GetFeedResp {
  repeated FeedItem items = 1;
  bool has_more = 2;
}
```

**验收标准**:
- [ ] Recommend RPC 服务启动成功

---

#### 任务 1.2: 设计用户画像结构
**涉及模块**: `app/recommend/rpc/internal/model/`

**画像维度**:

| 维度 | 字段 | 说明 |
|------|------|------|
| 兴趣标签 | interest_tags | 用户兴趣标签权重 |
| 作者偏好 | author_prefs | 偏好作者 ID 列表 |
| 内容偏好 | content_prefs | 内容类型偏好 |
| 活跃时段 | active_hours | 活跃时间段分布 |
| 社交偏好 | social_prefs | 关注/互动偏好 |

**Redis 存储结构**:
```go
// 用户画像 Redis Key
const (
    UserInterestTags = "user:profile:%d:tags"      // Hash: tag -> weight
    UserAuthorPrefs  = "user:profile:%d:authors"   // ZSet: author_id -> score
    UserContentPrefs = "user:profile:%d:content"   // Hash: type -> weight
    UserActiveHours  = "user:profile:%d:hours"     // List: hour counts
    UserVector       = "user:profile:%d:vector"    // String: JSON vector
)
```

**验收标准**:
- [ ] 画像结构设计完成
- [ ] Redis Key 定义清晰

---

#### 任务 1.3: 实现特征提取
**涉及模块**: `app/recommend/rpc/internal/logic/feature/`

**特征提取器**:
```go
type FeatureExtractor struct {
    db    *sqlx.DB
    redis *redis.Client
}

// 从用户行为提取特征
func (e *FeatureExtractor) ExtractFromBehavior(ctx context.Context, userId int64, behaviors []*UserBehavior) (*UserProfile, error) {
    profile := &UserProfile{UserId: userId}

    for _, b := range behaviors {
        weight := e.behaviorWeight(b.ActionType)

        // 更新兴趣标签
        for _, tag := range b.Tags {
            profile.InterestTags[tag] += weight
        }

        // 更新作者偏好
        profile.AuthorPrefs[b.AuthorId] += weight

        // 更新内容类型偏好
        profile.ContentPrefs[b.ContentType] += weight
    }

    // 归一化
    e.normalize(profile)

    return profile, nil
}

// 行为权重
func (e *FeatureExtractor) behaviorWeight(actionType int) float64 {
    weights := map[int]float64{
        1: 1.0,   // 点赞
        2: 2.0,   // 收藏
        3: 0.5,   // 浏览
        4: 3.0,   // 分享
        5: 2.5,   // 评论
    }
    return weights[actionType]
}
```

**验收标准**:
- [ ] 特征提取正常
- [ ] 权重计算正确

---

#### 任务 1.4: 实现画像更新
**涉及模块**: `app/recommend/rpc/internal/logic/`

**实时更新**:
```go
// MQ 消费者：实时更新用户画像
func (c *RecommendConsumer) Consume(msg *primitive.MessageExt) error {
    var event UserActionEvent
    json.Unmarshal(msg.Body, &event)

    ctx := context.Background()
    pipe := c.redis.Pipeline()

    // 更新兴趣标签
    for _, tag := range event.Tags {
        key := fmt.Sprintf("user:profile:%d:tags", event.UserId)
        pipe.HIncrByFloat(ctx, key, tag, c.weight(event.ActionType))
    }

    // 更新作者偏好
    authorKey := fmt.Sprintf("user:profile:%d:authors", event.UserId)
    pipe.ZIncrBy(ctx, authorKey, c.weight(event.ActionType), strconv.FormatInt(event.AuthorId, 10))

    // 更新内容类型偏好
    contentKey := fmt.Sprintf("user:profile:%d:content", event.UserId)
    pipe.HIncrByFloat(ctx, contentKey, event.ContentType, c.weight(event.ActionType))

    _, err := pipe.Exec(ctx)
    return err
}
```

**验收标准**:
- [ ] 画像实时更新正常
- [ ] Redis 数据正确

---

### W2: 召回层

#### 任务 2.1: 设计召回 Pipeline
**涉及模块**: `app/recommend/rpc/internal/logic/recall/`

**Pipeline 架构**:
```
用户请求 → 多路召回 → channel 流式传递 → 合并去重 → 粗排
              ↓
         ┌─────────────────────────────────────┐
         │  协同过滤 → content_ch               │
         │  内容相似 → content_ch               │
         │  热门内容 → content_ch               │
         │  关注链   → content_ch               │
         │  新鲜内容 → content_ch               │
         └─────────────────────────────────────┘
                          ↓
                    Fan-in 合并
```

**验收标准**:
- [ ] Pipeline 架构设计完成

---

#### 任务 2.2: 实现协同过滤召回
**涉及模块**: `app/recommend/rpc/internal/logic/recall/`

**User-CF 实现**:
```go
type CFRecaller struct {
    redis *redis.Client
}

func (r *CFRecaller) Recall(ctx context.Context, userId int64, limit int) <-chan *RecallItem {
    out := make(chan *RecallItem, limit)

    go func() {
        defer close(out)

        // 1. 获取相似用户
        similarUsers, _ := r.getSimilarUsers(ctx, userId, 50)

        // 2. 获取相似用户的喜欢内容
        pipe := r.redis.Pipeline()
        cmds := make(map[int64]*redis.StringSliceCmd)

        for _, uid := range similarUsers {
            cmds[uid] = pipe.ZRevRange(ctx, fmt.Sprintf("user:%d:likes", uid), 0, int64(limit/len(similarUsers)))
        }
        pipe.Exec(ctx)

        // 3. 发送到 channel
        for uid, cmd := range cmds {
            postIds, _ := cmd.Result()
            for _, pid := range postIds {
                id, _ := strconv.ParseInt(pid, 10, 64)
                select {
                case out <- &RecallItem{PostId: id, Source: "cf", Score: float64(uid)}:
                case <-ctx.Done():
                    return
                }
            }
        }
    }()

    return out
}

// 获取相似用户（基于 Jaccard 相似度）
func (r *CFRecaller) getSimilarUsers(ctx context.Context, userId int64, limit int) ([]int64, error) {
    // 获取当前用户的喜欢集合
    userLikes, _ := r.redis.SMembers(ctx, fmt.Sprintf("user:%d:likes_set", userId)).Result()

    // 找到喜欢相同内容的其他用户
    otherUsers := make(map[int64]int)
    for _, postId := range userLikes {
        users, _ := r.redis.SMembers(ctx, fmt.Sprintf("post:%s:liked_users", postId)).Result()
        for _, u := range users {
            uid, _ := strconv.ParseInt(u, 10, 64)
            if uid != userId {
                otherUsers[uid]++
            }
        }
    }

    // 按重叠数量排序
    var result []int64
    for uid, count := range otherUsers {
        if count >= 3 { // 最小重叠阈值
            result = append(result, uid)
        }
    }

    return result[:min(limit, len(result))], nil
}
```

**验收标准**:
- [ ] 协同过滤召回正常
- [ ] 相似用户计算正确

---

#### 任务 2.3: 实现内容相似召回
**涉及模块**: `app/recommend/rpc/internal/logic/recall/`

**基于向量相似度**:
```go
type ContentRecaller struct {
    milvus    *milvusx.MilvusClient
    embedding EmbeddingService
}

func (r *ContentRecaller) Recall(ctx context.Context, userId int64, limit int) <-chan *RecallItem {
    out := make(chan *RecallItem, limit)

    go func() {
        defer close(out)

        // 1. 获取用户兴趣向量
        userVector, err := r.getUserVector(ctx, userId)
        if err != nil {
            return
        }

        // 2. Milvus 相似度搜索
        ids, _ := r.milvus.Search(ctx, "post_vectors", userVector, limit)

        // 3. 发送到 channel
        for i, id := range ids {
            select {
            case out <- &RecallItem{PostId: id, Source: "content", Score: float64(limit - i)}:
            case <-ctx.Done():
                return
            }
        }
    }()

    return out
}

func (r *ContentRecaller) getUserVector(ctx context.Context, userId int64) ([]float32, error) {
    // 聚合用户喜欢的帖子向量
    likedPosts, _ := r.redis.ZRevRange(ctx, fmt.Sprintf("user:%d:likes", userId), 0, 99).Result()

    if len(likedPosts) == 0 {
        return nil, errors.New("no liked posts")
    }

    // 获取帖子向量并平均
    vectors, _ := r.milvus.GetVectors(ctx, "post_vectors", likedPosts)

    avgVector := make([]float32, 768)
    for _, v := range vectors {
        for i, val := range v {
            avgVector[i] += val / float32(len(vectors))
        }
    }

    return avgVector, nil
}
```

**验收标准**:
- [ ] 内容相似召回正常
- [ ] 向量平均计算正确

---

#### 任务 2.4: 实现热门召回
**涉及模块**: `app/recommend/rpc/internal/logic/recall/`

**热门召回器**:
```go
type HotRecaller struct {
    redis *redis.Client
}

func (r *HotRecaller) Recall(ctx context.Context, limit int) <-chan *RecallItem {
    out := make(chan *RecallItem, limit)

    go func() {
        defer close(out)

        // 从 Redis 热门榜获取
        result, _ := r.redis.ZRevRangeWithScores(ctx, "hot_posts", 0, int64(limit-1)).Result()

        for i, item := range result {
            postId, _ := strconv.ParseInt(item.Member.(string), 10, 64)
            select {
            case out <- &RecallItem{
                PostId: postId,
                Source: "hot",
                Score:  item.Score,
            }:
            case <-ctx.Done():
                return
            }
        }
    }()

    return out
}
```

**验收标准**:
- [ ] 热门召回正常

---

#### 任务 2.5: 实现关注链召回
**涉及模块**: `app/recommend/rpc/internal/logic/recall/`

**关注链召回**:
```go
type FollowChainRecaller struct {
    userRpc user.UserServiceClient
    redis   *redis.Client
}

func (r *FollowChainRecaller) Recall(ctx context.Context, userId int64, limit int) <-chan *RecallItem {
    out := make(chan *RecallItem, limit)

    go func() {
        defer close(out)

        // 1. 获取关注的人
        followings, _ := r.userRpc.GetFollowing(ctx, &user.GetFollowingReq{UserId: userId})

        // 2. 获取关注的人的喜欢
        pipe := r.redis.Pipeline()
        cmds := make([]*redis.StringSliceCmd, 0)

        for _, f := range followings.Users {
            cmd := pipe.ZRevRange(ctx, fmt.Sprintf("user:%d:likes", f.Id), 0, 5)
            cmds = append(cmds, cmd)
        }
        pipe.Exec(ctx)

        // 3. 发送到 channel
        seen := make(map[int64]bool)
        for _, cmd := range cmds {
            postIds, _ := cmd.Result()
            for _, pid := range postIds {
                id, _ := strconv.ParseInt(pid, 10, 64)
                if !seen[id] {
                    seen[id] = true
                    select {
                    case out <- &RecallItem{PostId: id, Source: "follow_chain"}:
                    case <-ctx.Done():
                        return
                    }
                }
            }
        }
    }()

    return out
}
```

**验收标准**:
- [ ] 关注链召回正常

---

#### 任务 2.6: 实现新鲜内容池
**涉及模块**: `app/recommend/rpc/internal/logic/recall/`

**新鲜内容召回**:
```go
type NewContentRecaller struct {
    redis *redis.Client
}

func (r *NewContentRecaller) Recall(ctx context.Context, limit int) <-chan *RecallItem {
    out := make(chan *RecallItem, limit)

    go func() {
        defer close(out)

        // 从新内容池获取（最近 24 小时）
        now := time.Now().Unix()
        result, _ := r.redis.ZRangeByScore(ctx, "new_posts", &redis.ZRangeBy{
            Min:   fmt.Sprintf("%d", now-86400),
            Max:   "+inf",
            Count: int64(limit),
        }).Result()

        for _, pid := range result {
            id, _ := strconv.ParseInt(pid, 10, 64)
            select {
            case out <- &RecallItem{PostId: id, Source: "new"}:
            case <-ctx.Done():
                return
            }
        }
    }()

    return out
}
```

**验收标准**:
- [ ] 新鲜内容召回正常

---

#### 任务 2.7: 实现 Fan-in 合并
**涉及模块**: `app/recommend/rpc/internal/logic/recall/`

**Fan-in 模式**:
```go
func (l *RecallLogic) mergeRecall(ctx context.Context, channels []<-chan *RecallItem) <-chan *RecallItem {
    out := make(chan *RecallItem, 1000)

    go func() {
        defer close(out)

        g, ctx := errgroup.WithContext(ctx)

        // 每个 channel 一个 goroutine 读取
        for _, ch := range channels {
            ch := ch
            g.Go(func() error {
                for item := range ch {
                    select {
                    case out <- item:
                    case <-ctx.Done():
                        return ctx.Err()
                    }
                }
                return nil
            })
        }

        g.Wait()
    }()

    return out
}
```

**验收标准**:
- [ ] 合并正常
- [ ] 无数据丢失

---

### W3: 排序层

#### 任务 3.1: 实现粗排
**涉及模块**: `app/recommend/rpc/internal/logic/rank/`

**粗排策略**:
```go
type RoughRanker struct {
    redis *redis.Client
}

func (r *RoughRanker) Rank(ctx context.Context, in <-chan *RecallItem) <-chan *RankItem {
    out := make(chan *RankItem, 500)

    go func() {
        defer close(out)

        for item := range in {
            // 简单特征：召回分数 + 热度
            hotScore, _ := r.redis.ZScore(ctx, "hot_posts", strconv.FormatInt(item.PostId, 10)).Result()

            rankItem := &RankItem{
                PostId:     item.PostId,
                Score:      item.Score + hotScore*0.1,
                Source:     item.Source,
            }

            select {
            case out <- rankItem:
            case <-ctx.Done():
                return
            }
        }
    }()

    return out
}
```

**验收标准**:
- [ ] 粗排正常
- [ ] 筛选效率符合预期

---

#### 任务 3.2: 实现精排
**涉及模块**: `app/recommend/rpc/internal/logic/rank/`

**精排特征**:
```go
type FineRanker struct {
    userRpc    user.UserServiceClient
    contentRpc content.ContentServiceClient
    redis      *redis.Client
}

func (r *FineRanker) Rank(ctx context.Context, in <-chan *RankItem, userId int64) <-chan *RankItem {
    out := make(chan *RankItem, 50)

    go func() {
        defer close(out)

        // 批量获取用户画像
        userProfile, _ := r.getUserProfile(ctx, userId)

        for item := range in {
            // 获取帖子特征
            postFeatures := r.getPostFeatures(ctx, item.PostId)

            // 计算精排分数
            score := r.computeScore(userProfile, postFeatures, item)

            item.Score = score

            select {
            case out <- item:
            case <-ctx.Done():
                return
            }
        }
    }()

    return out
}

func (r *FineRanker) computeScore(userProfile *UserProfile, postFeatures *PostFeatures, item *RankItem) float64 {
    score := item.Score

    // 兴趣匹配
    for _, tag := range postFeatures.Tags {
        score += userProfile.InterestTags[tag] * 0.3
    }

    // 作者偏好
    score += userProfile.AuthorPrefs[postFeatures.AuthorId] * 0.2

    // 内容类型偏好
    score += userProfile.ContentPrefs[postFeatures.ContentType] * 0.1

    // 时效性
    recency := time.Since(time.Unix(postFeatures.CreatedAt, 0)).Hours()
    score *= math.Exp(-recency / 168) // 7 天衰减

    return score
}
```

**验收标准**:
- [ ] 精排正常
- [ ] 特征计算正确

---

### W4: 重排层

#### 任务 4.1: 实现多样性重排
**涉及模块**: `app/recommend/rpc/internal/logic/rerank/`

**多样性策略**:
```go
type DiversityReranker struct{}

func (r *DiversityReranker) Rerank(items []*RankItem, topK int) []*RankItem {
    result := make([]*RankItem, 0, topK)
    tagCount := make(map[string]int)
    authorCount := make(map[int64]int)

    for _, item := range items {
        if len(result) >= topK {
            break
        }

        // 标签多样性：每个标签最多 3 条
        if tagCount[item.MainTag] >= 3 {
            continue
        }

        // 作者多样性：每个作者最多 2 条
        if authorCount[item.AuthorId] >= 2 {
            continue
        }

        result = append(result, item)
        tagCount[item.MainTag]++
        authorCount[item.AuthorId]++
    }

    return result
}
```

**验收标准**:
- [ ] 多样性保证
- [ ] 作者不重复

---

#### 任务 4.2: 实现业务规则
**涉及模块**: `app/recommend/rpc/internal/logic/rerank/`

**业务规则**:
```go
func (l *RerankLogic) applyBusinessRules(items []*RankItem, userId int64) []*RankItem {
    // 规则 1: 过滤已读内容
    items = l.filterRead(items, userId)

    // 规则 2: 过滤低质量内容
    items = l.filterLowQuality(items)

    // 规则 3: 插入广告位（每 10 条）
    items = l.insertAds(items)

    // 规则 4: 保证时效性混合
    items = l.mixFreshContent(items)

    return items
}

func (l *RerankLogic) filterRead(items []*RankItem, userId int64) []*RankItem {
    // 获取已读帖子
    readKey := fmt.Sprintf("user:%d:read", userId)
    readPosts, _ := l.redis.SMembers(l.ctx, readKey).Result()
    readSet := make(map[int64]bool)
    for _, pid := range readPosts {
        id, _ := strconv.ParseInt(pid, 10, 64)
        readSet[id] = true
    }

    var result []*RankItem
    for _, item := range items {
        if !readSet[item.PostId] {
            result = append(result, item)
        }
    }
    return result
}
```

**验收标准**:
- [ ] 已读过滤生效
- [ ] 业务规则正确

---

### W5: 冷启动

#### 任务 5.1: 实现冷启动策略
**涉及模块**: `app/recommend/rpc/internal/logic/coldstart/`

**冷启动流程**:
```go
func (l *GetColdStartFeedLogic) GetColdStartFeed(ctx context.Context, req *recommend.GetColdStartFeedReq) (*recommend.GetColdStartFeedResp, error) {
    // 新用户无历史行为，使用以下策略：
    // 1. 热门内容（60%）
    // 2. 高质量新人内容（20%）
    // 3. 编辑推荐（20%）

    hotItems := l.getHotItems(ctx, int(float64(req.PageSize)*0.6))
    newCreatorItems := l.getNewCreatorItems(ctx, int(float64(req.PageSize)*0.2))
    editorPicks := l.getEditorPicks(ctx, int(float64(req.PageSize)*0.2))

    items := append(hotItems, newCreatorItems...)
    items = append(items, editorPicks...)

    // 打乱顺序
    rand.Shuffle(len(items), func(i, j int) {
        items[i], items[j] = items[j], items[i]
    })

    return &recommend.GetColdStartFeedResp{Items: items}, nil
}
```

**验收标准**:
- [ ] 冷启动推荐正常
- [ ] 内容配比正确

---

#### 任务 5.2: 实现兴趣引导
**涉及模块**: `app/recommend/rpc/internal/logic/`

**兴趣引导流程**:
```go
// 新用户注册时，引导选择兴趣标签
func (l *OnboardingLogic) SaveInterests(userId int64, tags []string) error {
    pipe := l.redis.Pipeline()

    // 初始化兴趣标签权重
    key := fmt.Sprintf("user:profile:%d:tags", userId)
    for _, tag := range tags {
        pipe.HSet(l.ctx, key, tag, 10.0) // 初始权重
    }

    return pipe.Exec(l.ctx).Err()
}
```

**验收标准**:
- [ ] 兴趣选择正常
- [ ] 初始画像创建

---

#### 任务 5.3: 实现探索机制
**涉及模块**: `app/recommend/rpc/internal/logic/`

**探索策略**:
```go
// 在推荐中插入探索内容
func (l *RecommendLogic) addExploration(items []*RankItem, ratio float64) []*RankItem {
    exploreCount := int(float64(len(items)) * ratio)
    if exploreCount == 0 {
        return items
    }

    // 随机获取探索内容
    exploreItems := l.getRandomItems(l.ctx, exploreCount)

    // 随机位置插入
    result := make([]*RankItem, 0, len(items)+exploreCount)
    result = append(result, items...)

    for _, item := range exploreItems {
        pos := rand.Intn(len(result) + 1)
        result = insertAt(result, item, pos)
    }

    return result
}
```

**验收标准**:
- [ ] 探索内容插入正常

---

## 技术要点

### channel Pipeline 模式

```go
func (l *RecommendLogic) GetFeed(ctx context.Context, userId int64, size int) ([]*FeedItem, error) {
    // Stage 1: 多路召回 → channel
    recallCh := l.recall(ctx, userId)       // 输出 ~5000 候选

    // Stage 2: 粗排 → channel
    roughCh := l.roughRank(ctx, recallCh)   // 筛选 ~500

    // Stage 3: 精排 → channel
    fineCh := l.fineRank(ctx, userId, roughCh)  // 筛选 ~50

    // Stage 4: 重排（消费最终结果）
    return l.rerank(ctx, userId, fineCh, size)   // 输出 20
}

// 召回层：多路并行，结果汇入同一个 channel
func (l *RecommendLogic) recall(ctx context.Context, userId int64) <-chan *RecallItem {
    out := make(chan *RecallItem, 1000)

    go func() {
        defer close(out)
        g, ctx := errgroup.WithContext(ctx)

        channels := make([]<-chan *RecallItem, 0, 5)
        channels = append(channels,
            l.cfRecaller.Recall(ctx, userId),
            l.contentRecaller.Recall(ctx, userId),
            l.hotRecaller.Recall(ctx),
            l.followChainRecaller.Recall(ctx, userId),
            l.newContentRecaller.Recall(ctx),
        )

        // Fan-in: 合并所有 channel 到 out
        for _, ch := range channels {
            ch := ch
            g.Go(func() error {
                for item := range ch {
                    select {
                    case out <- item:
                    case <-ctx.Done():
                        return ctx.Err()
                    }
                }
                return nil
            })
        }
        g.Wait()
    }()

    return out
}
```

### Pipeline 优势

1. **流式处理**：数据通过 channel 流式传递，无需一次性加载所有数据
2. **背压控制**：channel 缓冲区满时自动阻塞，防止内存溢出
3. **并发安全**：每个 stage 独立 goroutine，无锁竞争
4. **可扩展**：添加新的召回/排序策略只需增加 channel

---

## 依赖与风险

### 外部依赖
| 依赖 | 用途 |
|------|------|
| Redis | 用户画像/缓存 |
| Milvus | 向量检索 |
| User RPC | 用户数据 |
| Content RPC | 内容数据 |

### 潜在风险

| 风险 | 等级 | 缓解措施 |
|------|------|---------|
| 冷启动效果 | MEDIUM | 引导兴趣选择 + 热门内容 |
| 召回层延迟 | MEDIUM | 设置独立超时 |
| 数据稀疏 | MEDIUM | 探索机制补充 |

---

## 验收标准

### 功能验收
- [ ] 推荐结果个性化
- [ ] 多路召回正常
- [ ] Pipeline 流程完整
- [ ] 冷启动推荐正常
- [ ] 兴趣引导正常

### 性能验收
- [ ] 推荐响应时间 < 200ms
- [ ] 召回阶段 < 100ms
- [ ] 排序阶段 < 100ms

### 质量验收
- [ ] 推荐多样性满足要求
- [ ] 用户满意度 > 80%

---

## 交付物清单

| 交付物 | 路径 |
|--------|------|
| Recommend RPC | `app/recommend/rpc/` |
| 召回器实现 | `app/recommend/rpc/internal/logic/recall/` |
| 排序器实现 | `app/recommend/rpc/internal/logic/rank/` |
| 重排器实现 | `app/recommend/rpc/internal/logic/rerank/` |
| 冷启动逻辑 | `app/recommend/rpc/internal/logic/coldstart/` |

---

## 下一步

Phase 4 完成后，进入 [Phase 5: 运维监控](phase-5-ops.md)。
