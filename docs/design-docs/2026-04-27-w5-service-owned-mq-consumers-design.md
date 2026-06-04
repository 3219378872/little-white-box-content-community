# W5 服务归属 MQ Consumer 集成设计

## 背景

`doc/phases/phase-2-interaction.md` 的 W5 要求引入独立 MQ consumer，用于搜索索引同步、推荐行为事件、Feed 写扩散、消息通知生成和媒体清理。

本轮采用服务归属式目录，而不是集中式 `app/mq/*-consumer`：

```text
app/feed/mq/
app/message/mq/
app/media/mq/
app/search/mq/
app/recommend/mq/
```

用户已确认：MQ consumer 独立编写 logic，不共享 RPC logic。也就是说，`app/*/mq` 不复用 `app/*/rpc/internal/logic` 或 `app/*/rpc/internal/mqs`，必要业务流程在 MQ 进程内独立实现。

## 目标

1. 按 W5 语义新增 5 个服务归属式独立 MQ consumer 入口。
2. 将现有 Feed、Message、Media 的消费职责从 RPC 进程迁移到对应 `app/*/mq`。
3. 每个 MQ consumer 拥有独立 `config`、`svc`、`logic`、`mqs` 和必要 `model/store/client`。
4. RPC 进程不再启动 MQ consumer，避免同一消费组被 RPC 和独立 consumer 混合运行。
5. 按消息错误类型返回 `ConsumeSuccess` 或 `ConsumeRetryLater`，避免毒消息无限重试。
6. 保持当前 `pkg/mqx` topic / group 命名，不迁移到 phase 文档中的大写 topic 名。

## 非目标

- 不复用 RPC logic 或 RPC `internal/mqs`。
- 不把 MQ logic 抽成和 RPC 共享的业务包。
- 不补完整 Search RPC 服务。
- 不补完整 Recommend RPC 服务。
- 不新增 Elasticsearch / Milvus / 推荐画像写入能力。
- 不追踪或提交本设计文档。

## 架构

每个 consumer 进程采用相同分层：

```text
app/<service>/mq/
├── etc/<service>-consumer.yaml
├── main.go
└── internal/
    ├── config/
    ├── svc/
    ├── mqs/
    ├── logic/
    └── model/ or store/ or client/
```

调用关系：

```text
RocketMQ topic
  -> internal/mqs consumer callback
  -> internal/logic independent business flow
  -> internal/svc injected dependency
  -> internal/model, Redis, S3, RPC client, or future adapter
```

允许复用跨服务公共库：

- `pkg/mqx`
- `pkg/cleanupx`
- `pkg/errx`
- `pkg/util`
- `pkg/interceptor`

禁止复用：

- `app/*/rpc/internal/logic`
- `app/*/rpc/internal/mqs`
- 任何因 Go `internal` 规则不能被 `app/*/mq` 合法导入的 RPC 私有包

如果 MQ consumer 需要数据库访问，优先在 `app/<service>/mq/internal/model` 写 MQ 自己的最小模型。自定义 SQL 放非 `*_gen.go` 文件；若使用 goctl 生成模型，不手工修改生成文件。

## Consumer 设计

### Feed Consumer

目录：`app/feed/mq`

订阅：

- Topic：`mqx.TopicPostCreate` (`post-create`)
- Group：`mqx.GroupFeedService`
- Tag：`mqx.TagDefault`

消息：

```json
{
  "post_id": 1,
  "author_id": 2,
  "created_at": 1710000000000
}
```

Logic 独立实现 Feed 写扩散：

1. 校验 `post_id`、`author_id`、`created_at`。
2. 调用 User RPC 获取作者粉丝数。
3. `INSERT IGNORE` 写 `xbh_feed.feed_outbox`。
4. 如果作者是大 V，只写 outbox，不写 inbox。
5. 如果作者不是大 V，分页调用 User RPC 获取粉丝列表。
6. 批量 `INSERT IGNORE` 写 `xbh_feed.feed_inbox`。

幂等边界：

- `feed_outbox.uk_author_post`
- `feed_inbox.uk_user_post`

### Message Consumer

目录：`app/message/mq`

订阅：

- Topic：`mqx.TopicMessagePush` (`message-push`)
- Group：`mqx.GroupMessageService`
- Tag：`mqx.TagDefault`

消息沿用现有通知事件字段：

```json
{
  "target_user_id": 1,
  "action_type": 1,
  "user_id": 2,
  "username": "alice",
  "target_id": 3,
  "target_type": 1,
  "content": "optional"
}
```

Logic 独立实现通知生成：

1. 校验 `target_user_id` 和 `action_type`。
2. 按 `action_type` 渲染点赞、评论、关注、系统通知内容。
3. 写 `xbh_message.notification`。
4. 删除目标用户 unread cache，使后续未读计数重新计算。

不复用 `app/message/rpc/internal/mqs`、`app/message/rpc/internal/model` 或 RPC logic。

### Media Consumer

目录：`app/media/mq`

订阅：

- Topic：`mqx.TopicMediaDelete` (`media-deleted`)
- Group：`mqx.GroupMediaService`
- Tag：`mqx.TagDefault`

消息：

```json
{
  "media_id": 1,
  "s3_object_key": "object/key",
  "bucket": "xbh-media",
  "deleted_at": 1710000000
}
```

Logic 独立实现对象清理：

1. 校验 `s3_object_key` 非空。
2. 使用 MQ 进程自己的 S3 / SeaweedFS adapter 删除对象。
3. 删除成功记录结构化日志。
4. 删除失败按临时错误重试。

不复用 `app/media/rpc/internal/storage` 或 RPC logic。

### Search Consumer

目录：`app/search/mq`

订阅：

- Topic：优先 `mqx.TopicSearchIndex` (`search-index`) 和 `mqx.TopicSearchDelete` (`search-delete`)
- Group：`mqx.GroupSearchService`
- Tag：`mqx.TagDefault`

当前仓库没有 Search 服务实现，也没有 ES / Milvus 写入链路。本轮只交付可启动、可订阅、可测试的 consumer 骨架：

1. 校验事件 JSON 和必要字段。
2. 记录结构化日志。
3. 通过本地 `Indexer` 接口承接未来索引写入。
4. 默认实现不声明真实索引同步已经完成。

本轮不让 Search consumer 订阅 `post-create`。`post-create` 归 Feed 写扩散使用；后续如果内容服务需要驱动搜索索引，应发布 `search-index` 或 `search-delete`。

### Recommend Consumer

目录：`app/recommend/mq`

订阅：

- Topic：`mqx.TopicUserBehavior` (`user-behavior`)
- Group：`mqx.GroupRecommendService`
- Tag：`mqx.TagDefault`

当前仓库没有 Recommend 服务实现，也没有画像或行为特征表。本轮只交付可启动、可订阅、可测试的 consumer 骨架：

1. 校验事件 JSON 和必要字段。
2. 记录结构化日志。
3. 通过本地 `BehaviorStore` 接口承接未来画像/特征写入。
4. 默认实现不声明真实画像更新已经完成。

## RPC 迁移

为完成“消费逻辑迁移至 MQ”，需要调整现有 RPC 入口：

- `app/feed/rpc/feed.go` 不再启动 `NewPostPublishConsumer`。
- `app/message/rpc/message.go` 不再启动 `NewRocketMQConsumer`。
- `app/media/rpc/media.go` 不再启动 `NewMediaCleanupConsumer`。

旧的 `app/*/rpc/internal/mqs` 在 MQ 等价测试通过后移除，避免同一职责保留两份实现。若某些测试仍引用旧包，迁移到 `app/*/mq/internal/...` 对应测试。

## 配置

每个 consumer 使用独立 yaml，敏感值继续使用环境变量占位。

Feed：

- `DataSource`
- `Redis`
- `UserRpc`
- `MQ`
- `BigVThreshold`
- `FanoutBatchSize`

Message：

- `DataSource`
- `Redis`
- `MQ`

Media：

- `S3Storage`
- `MQ`

Search：

- `MQ`

Recommend：

- `MQ`

`pkg/mqx.Consumer.Subscribe` 当前仍是未实现 panic，本轮统一使用 `SubscribeWithTopic`。

## 错误处理

永久错误返回 `consumer.ConsumeSuccess`，并记录带 ctx 的错误日志：

- JSON 解析失败
- 必填字段缺失
- 未知事件类型
- 空对象键
- Search / Recommend 骨架不支持的事件类型

临时错误返回 `consumer.ConsumeRetryLater`：

- MySQL 写入失败
- Redis 操作失败且影响主流程
- User RPC 调用失败
- S3 / SeaweedFS 删除失败
- RocketMQ consumer 初始化或启动失败

批量消息处理规则：

- 单条永久错误只跳过当前消息。
- 任一临时错误使整个 batch 返回 retry。

日志规则：

- 必须使用 `logx.WithContext(ctx)`。
- 禁止裸 `logx.Info()` / `logx.Error()`。
- 日志记录消息 ID、topic、关键业务 ID 和错误原因。
- 不记录敏感值。

## 部署

应用部署层新增 consumer 服务块，服务名与目录对应：

- `feed-consumer`
- `message-consumer`
- `media-consumer`
- `search-consumer`
- `recommend-consumer`

中间件 compose 继续只承载 MySQL、Redis、RocketMQ、Elasticsearch、Milvus、DTM 和可观测性组件。应用 consumer compose 依赖 RocketMQ NameServer，并按需依赖 MySQL、Redis、对象存储、Elasticsearch 或 Milvus。

当前 `deploy/rocketmq/init-topics.sh` 已覆盖 W5 需要的 topic 和 consumer group。实现时只需保持它与 `pkg/mqx/topics.go` 一致。

## 测试策略

按 TDD 执行，先写失败用例再写实现。

Feed：

- malformed JSON 返回 success skip。
- 缺少 `post_id` / `author_id` / `created_at` 返回 success skip。
- User RPC 失败返回 retry。
- 大 V 作者只写 outbox。
- 小 V 作者分页写完整 followers inbox。
- 重复消息依赖唯一键幂等。

Message：

- malformed JSON 返回 success skip。
- 缺少 `target_user_id` 或 `action_type` 返回 success skip。
- 不支持的 `action_type` 返回 success skip。
- 合法点赞、评论、关注、系统通知写入 notification。
- notification 写入失败返回 retry。
- unread cache 删除失败记录日志，不阻断通知写入成功。

Media：

- malformed JSON 返回 success skip。
- 空 `s3_object_key` 返回 success skip。
- S3 删除失败返回 retry。
- S3 删除成功返回 success。

Search：

- malformed JSON 返回 success skip。
- 缺必要字段返回 success skip。
- 合法 index/delete 事件调用 `Indexer` 接口并返回 success。
- `Indexer` 临时失败返回 retry。

Recommend：

- malformed JSON 返回 success skip。
- 缺必要字段返回 success skip。
- 合法 behavior 事件调用 `BehaviorStore` 接口并返回 success。
- `BehaviorStore` 临时失败返回 retry。

验证命令优先使用目标包：

```bash
GOCACHE=/tmp/go-build-little-white-box go test ./app/feed/mq/... ./app/message/mq/... ./app/media/mq/... ./app/search/mq/... ./app/recommend/mq/...
GOCACHE=/tmp/go-build-little-white-box go vet ./app/feed/mq/... ./app/message/mq/... ./app/media/mq/... ./app/search/mq/... ./app/recommend/mq/...
golangci-lint run ./app/feed/mq/... ./app/message/mq/... ./app/media/mq/... ./app/search/mq/... ./app/recommend/mq/...
```

完工前再尝试仓库级要求：

```bash
GOCACHE=/tmp/go-build-little-white-box go test ./... -race -cover
GOCACHE=/tmp/go-build-little-white-box go vet ./...
golangci-lint run
```

如果仓库级验证被无关 workspace 或历史问题阻塞，需要报告具体错误和已通过的目标包验证结果。

## 验收标准

- `app/feed/mq`、`app/message/mq`、`app/media/mq`、`app/search/mq`、`app/recommend/mq` 均存在独立入口。
- 每个 consumer 有独立 config、svc、logic、mqs。
- Feed、Message、Media 消费职责已从 RPC 进程迁出。
- MQ logic 不复用 RPC logic。
- Search、Recommend 可启动、可订阅、可测试，但验收口径明确为骨架能力。
- 永久错误不会无限重试，临时错误会 retry。
- 目标包 test/vet/lint 有明确结果。
