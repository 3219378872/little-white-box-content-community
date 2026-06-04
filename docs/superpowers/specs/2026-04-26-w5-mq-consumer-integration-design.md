# W5 MQ 消费者集成设计

## 背景

`doc/phases/phase-2-interaction.md` 的 W5 要求新增独立 MQ 消费者进程：

- `search-consumer`：消费帖子事件，同步搜索索引与向量。
- `recommend-consumer`：消费行为事件，更新用户画像与行为特征。
- `feed-consumer`：消费帖子发布事件，执行 Feed 写扩散。
- `message-consumer`：消费通知事件，生成站内通知。
- `media-consumer`：消费媒体事件，处理对象存储清理。

当前仓库已经有部分消费逻辑，但它们挂在各自 RPC 服务进程内：

- `app/feed/rpc/internal/mqs/post_publish_consumer.go`
- `app/message/rpc/internal/mqs/message_consumer.go`
- `app/media/rpc/internal/mqs/media_cleanup_consumer.go`

Search 和 Recommend 目前只有 proto 与中间件基础设施，没有对应服务实现。因此本设计把 W5 范围限定为“独立消费者进程集成”，不在本轮补完整搜索服务、推荐服务、Elasticsearch 写入实现或 Milvus 向量生成。

## 目标

1. 新增 `app/mq/*-consumer` 独立入口，满足 W5 目录与启动形态。
2. Feed、Message、Media consumer 复用已有业务处理，不复制核心逻辑。
3. Search、Recommend consumer 提供可启动、可订阅、可测试的消费骨架。
4. 明确永久错误和临时错误的消费结果，避免毒消息无限重试。
5. 给部署层补应用 consumer 服务块，与 middleware compose 分离。

## 非目标

- 不实现完整 Search RPC 服务。
- 不实现完整 Recommend RPC 服务。
- 不新增 Elasticsearch / Milvus 客户端依赖。
- 不改写已有 Feed / Message / Media 业务语义。
- 不把 superpowers 设计文档加入 git 跟踪。

## 方案

采用独立进程入口 + 共享 handler 的方式。

目录结构：

```text
app/mq/
├── feed-consumer/
│   ├── etc/feed-consumer.yaml
│   └── main.go
├── message-consumer/
│   ├── etc/message-consumer.yaml
│   └── main.go
├── media-consumer/
│   ├── etc/media-consumer.yaml
│   └── main.go
├── search-consumer/
│   ├── etc/search-consumer.yaml
│   └── main.go
└── recommend-consumer/
    ├── etc/recommend-consumer.yaml
    └── main.go
```

Feed、Message、Media 独立入口直接加载对应 RPC 服务的 config，构造对应 `ServiceContext`，并调用现有 `internal/mqs` consumer factory。这样能保留现有数据库、Redis、S3、fanout 逻辑和测试覆盖。

Search、Recommend 独立入口使用轻量 config 与本地 handler：

- Search 订阅 `post-create`、`post-update`、`post-delete` 或 `search-index/search-delete` 中当前仓库已有 topic。事件合法时记录结构化日志并返回成功；后续 Search 服务实现后再替换为 RPC 或客户端调用。
- Recommend 订阅 `user-behavior`。事件合法时记录结构化日志并返回成功；下游画像计算尚未实现时不伪造成功写入。

## 事件与 Topic

沿用 `pkg/mqx/topics.go` 当前定义：

- Feed：`TopicPostCreate` + `TagDefault`
- Message：`TopicMessagePush` + `TagDefault`
- Media：`TopicMediaDelete` + `TagDefault`
- Search：优先使用 `TopicSearchIndex` / `TopicSearchDelete`；必要时兼容 `TopicPostCreate`。
- Recommend：`TopicUserBehavior`

W5 文档中的 `POST_PUBLISH`、`USER_ACTION` 等大写 topic 不直接替换当前仓库 topic，避免破坏已实现的 producer / consumer 契约。若后续需要统一命名，应单独做迁移设计。

## 错误处理

Consumer handler 按错误类型决定 RocketMQ 消费结果：

- JSON 无法解析、缺少必要字段、未知事件类型：永久错误，记录 `logx.WithContext(ctx).Errorw`，返回 `ConsumeSuccess`。
- DB / Redis / S3 / RPC 临时失败：返回 `ConsumeRetryLater`。
- 批量消息中任一临时失败：本批返回 `ConsumeRetryLater`。

所有日志必须绑定消费回调传入的 `ctx`。进程关闭使用 `cleanupx.Shutdown`。

## 配置

每个 consumer 使用独立 `etc/*.yaml`，字段只包含该进程需要的配置：

- 通用：`MQ.NameServer`、`MQ.GroupName`、`MQ.Topic`、`MQ.Tag`
- Feed：`DataSource`、`Redis`、`UserRpc`、`ContentRpc`、`BigVThreshold`、`FanoutBatchSize`
- Message：`DataSource`、`Redis`
- Media：`DataSource`、`S3Storage`
- Search / Recommend：先只需要 MQ 配置

敏感值保留 `${ENV_VAR}` 占位，不硬编码真实密钥。

## 部署

新增或补齐应用级 `deploy/docker-compose.yml`，包含五个 consumer 服务。当前 `deploy/docker-compose.middleware.yml` 继续只承载 MySQL、Redis、RocketMQ、Elasticsearch、Milvus、DTM、可观测性等中间件。

应用 compose 中每个 consumer：

- build 对应 `app/mq/<name>-consumer`
- 依赖 `rocketmq-namesrv`
- 按需依赖 mysql / redis / seaweedfs / elasticsearch / milvus
- 注入 `MQ_NAMESERVER`、数据库 DSN、Redis 密码、S3 凭据等环境变量

如果仓库没有通用服务 Dockerfile，本轮补最小可构建 Dockerfile 或先将 compose 指向统一构建上下文，避免写出不可用配置。

## 测试

按 TDD 执行：

1. Search / Recommend handler 单测：
   - malformed JSON 返回成功并记录永久错误。
   - 缺必要字段返回成功。
   - 合法事件返回成功。
2. Feed / Message / Media 独立入口相关测试：
   - consumer config 能正确传给现有 factory。
   - 现有 handler 的成功、永久错误、临时重试路径保持通过。
3. 部署配置测试：
   - compose 文件能被解析。
   - consumer 服务块包含 build、depends_on、必要环境变量。

验证命令优先使用目标包：

```bash
GOCACHE=/tmp/codex-gocache go test ./app/mq/... ./app/feed/rpc/internal/mqs ./app/message/rpc/internal/mqs ./app/media/rpc/internal/mqs
GOCACHE=/tmp/codex-gocache go vet ./app/mq/... ./app/feed/rpc/internal/mqs ./app/message/rpc/internal/mqs ./app/media/rpc/internal/mqs
golangci-lint run ./app/mq/... ./app/feed/rpc/internal/mqs/... ./app/message/rpc/internal/mqs/... ./app/media/rpc/internal/mqs/...
```

完工前再按仓库要求尝试全量验证；若全量 lint 暴露无关历史债务，报告具体阻塞并保留目标包验证结果。

## 风险与约束

- Search / Recommend 的真实业务能力未实现，本轮只能交付消费进程骨架，不能宣称索引同步或画像更新已经真实落库。
- 现有 `pkg/mqx.Consumer.Subscribe` 仍是 panic，本轮使用已实现的 `SubscribeWithTopic`。
- Feed / Message / Media 现有 RPC 进程仍可能启动内嵌 consumer；若独立 consumer 投产，需要通过配置关闭内嵌消费，避免同一消费组部署形态混乱。
- `go.work` 当前未包含 `app/feed`、`app/message`、`app/media` 等子模块，本轮实现前要重新确认当前模块布局，必要时补 workspace 配置。

## 交付验收

- `app/mq/*-consumer` 目录存在并可编译。
- Feed / Message / Media 独立 consumer 可复用现有 handler。
- Search / Recommend consumer 可启动、可订阅、可处理永久错误和合法事件。
- 应用部署配置包含五个 consumer 服务。
- 目标包测试、vet、lint 有明确结果。
