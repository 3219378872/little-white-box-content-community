# Iter 1 = Phase 2 — L1 索引/向量/清理 实施计划

> 父 spec：[../specs/2026-04-29-data-foundation-design.md](../specs/2026-04-29-data-foundation-design.md) §2-§3
> 迭代索引：[../specs/2026-05-20-data-foundation-iteration-plan.md](../specs/2026-05-20-data-foundation-iteration-plan.md)

## 现状关键发现

1. **没有 producer**：`mqx.NewProducer` 在 Content RPC svcCtx 已注入，但 logic 层从未调用
2. **DTM 路径主导**：`CreatePostLogic` 用 DTM saga 调用 `feed.FanoutPost` gRPC，**不发** `post-create` MQ
3. **feed-fanout 是死消费**：`app/feed/mq` 订阅 `TopicPostCreate` 但生产侧无消息
4. **`create_post_event_test.go:90` 显式断言** DTM 路径不发 MQ — 这条断言需要按 spec 反转

## 任务序列（按依赖与风险递增排序）

### T1. 扩展 `pkg/event` — 新增 PostEvent / InteractionEvent

- 复用现有 `BehaviorEvent` 风格（JSON tag + Validate）
- 三个事件 struct：`PostCreatedEvent`、`PostUpdatedEvent`、`PostDeletedEvent`
- 字段：`event_id` (Snowflake)、`post_id`、`author_id`、`title`、`body_excerpt`、`tags`、`event_time`
- `InteractionEvent`：`user_id`、`action` (like/unlike/favorite/unfavorite/comment-create)、`target_id`、`target_type`
- 单测：序列化/反序列化对称 + Validate 覆盖

**Commit:** `feat(event): add post and interaction event types`

### T2. ES Indexer 实现 + search-index-consumer 接入

**为什么先做这个**：与现有 producer 解耦，可用 `mqx.Producer` 在集成测试里手动发消息验证消费者，不依赖 Content RPC 改造。

- `pkg/searchx/es_indexer.go`：`NewESIndexer(addr, index)`，实现 `Index/Delete` 接口
- 索引 Mapping（父 spec §7.4 隐含 + Phase 3 doc）：`title/body` IK 分词、`tags/author_id/category_id` keyword、`created_at` date
- `app/search/mq/internal/svc/service_context.go`：根据配置选择 `ESIndexer` or `NoopIndexer`
- `app/search/mq/etc/search-mq.yaml`：加 ES 地址、索引名
- testcontainers 集成测试：发消息 → ES 可查
- 单测：mapping 校验、错误重试路径

**依赖**：`github.com/elastic/go-elasticsearch/v8`

**Commit:** `feat(searchx): add ES indexer and wire into search-mq`

### T3. embedding-consumer 新建（app/embedding/mq）

- 目录结构对齐 search/mq：`internal/{config,svc,mqs,embedder,milvus}`
- `Embedder` 接口 + `NoopEmbedder`（产出全 0 向量，dim=256）
- `MilvusWriter`：collection `post_embeddings`，schema `post_id (PK Int64) + embedding (FloatVector dim=256)`
- 订阅 `TopicPostCreate` / `TopicPostUpdate` / `TopicPostDelete`
- testcontainers Milvus 2.2 集成测试
- 单测：Noop 路径 + 失败重试

**依赖**：`github.com/milvus-io/milvus-sdk-go/v2`

**Commit:** `feat(embedding): add embedding-mq consumer with NoopEmbedder + Milvus`

### T4. content-cleanup-consumer 新建（app/content/mq/cleanup）

- 订阅 `TopicPostDelete`
- 清理目标：
  - Redis `post:{pid}:stats` / `post:{pid}:quality`
  - Redis `hot:posts:24h` / `hot:posts:7d` ZSET 移除该 post
  - Redis `tag:{name}:posts` 各标签 ZSET 移除（需 post.tags 字段，事件携带）
  - Feed Redis：调用 `feed.RPC.RemovePost` 或直接清理 `user:{uid}:feed` ZSET（设计选择见 §决策）
- 决策：cleanup-consumer 不直接操作 feed 表，发新事件 `feed-cleanup` 由 feed/mq 消费 — 保持服务边界
- testcontainers Redis 集成测试

**Commit:** `feat(content-cleanup): add post-delete cleanup consumer`

### T5. feed-fanout 重构 + 与 L1 对齐

- 当前 `app/feed/mq/internal/mqs/post_publish_consumer.go` 已是单消费者结构，符合新约定
- 检查项：
  - 抽 `OutboxStore` / `InboxStore` 接口（便于单测 mock）— 已有 Model 但缺接口层
  - 加 `feed-cleanup` 消费者订阅 `TopicPostDelete`（从 T4 receive cleanup 事件）或直接订阅 `TopicPostDelete`
- 决策：T4 cleanup-consumer 直接订阅 post-delete + feed cleanup 也直接订阅 post-delete，**多路并行消费同一 topic**（RocketMQ 支持，不同 consumer group）

**Commit:** `refactor(feed-mq): align store interfaces + add feed-cleanup consumer`

### T6. Content RPC 加 MQ 发布

- `CreatePostLogic` DTM commit 成功后 → `MQProducer.SendSyncWithTag(TopicPostCreate, ...)` 发 `PostCreatedEvent`
- `UpdatePostLogic` → `TopicPostUpdate`
- `DeletePostLogic` → `TopicPostDelete`
- **反转** `TestCreatePostLogic_DTMPathDoesNotPublishPostCreatedEvent`：应改为断言发布
- 父 spec §3.4 幂等保障：消费者 upsert 天然幂等，生产者无需事务消息（先走 best-effort）
- 错误处理：MQ 发送失败仅打日志，不回滚 DB（异步路径，最终一致）

**Commit:** `feat(content-rpc): publish post lifecycle events to MQ`

### T7. Interaction RPC 加 MQ 发布

- LikeLogic / UnlikeLogic / FavoriteLogic / UnfavoriteLogic
- 发 `TopicLike`/`TopicUnlike`/`TopicFavorite`/`TopicUnfavorite`（topic 已在 `pkg/mqx/topics.go` 定义）
- 同时发 `TopicUserBehavior` 给 behavior-log（已有 pipeline 消费）和未来 recommend
- 单测覆盖：成功路径发 MQ、DB 失败不发 MQ

**Commit:** `feat(interaction-rpc): publish interaction events to MQ`

### T8. 端到端集成测试

- `app/integration_test/iter1_e2e_test.go`：
  - 起 testcontainers：RocketMQ + ES + Milvus + Redis
  - 用 mqx.Producer 直接发 `post-create` 消息（模拟 Content RPC）
  - 断言 ES 可查、Milvus 可查
  - 发 `post-delete` → 断言 Redis 各 key 清理
- 跳过条件：缺 docker 时 `t.Skip`

**Commit:** `test(iter1): add end-to-end consumer integration tests`

### T9. 文档收尾

- 更新 `2026-05-20-data-foundation-iteration-plan.md` 进度
- 更新 memory：标记 search/recommend skeleton 已转为实质实现

**Commit:** `docs: mark iter 1 phase 2 complete`

## 风险与对应

| 风险 | 应对 |
|------|------|
| ES 8.8 镜像拉取慢 / testcontainers 启动 >2min | 用 `WithStartupTimeout(3min)`；CI 缓存镜像 |
| Milvus 2.2 单机版需要 etcd+MinIO 一并启动 | 用 milvus-standalone 单容器（不是 milvus-cluster） |
| RocketMQ 双 broker+nameserver 集成测试启动慢 | 用 mqx 现有 testcontainers helper，复用 |
| DTM 路径与 MQ 路径双重发布导致重复 | 不重复：DTM 只负责 feed.FanoutPost 远程调用；MQ 是另一条独立链路（search/embedding/cleanup） |
| 反转 `create_post_event_test.go` 破坏 DTM 契约测试 | 拆分：DTM contract 测试不变，新增 MQ publish 测试 |

## 验收

- [ ] 所有 task commit 完成
- [ ] `go test -race -tags=integration ./...` 通过
- [ ] 各 consumer 包覆盖率 ≥80%
- [ ] iteration plan Iter 1 复选框打勾
- [ ] memory 更新
