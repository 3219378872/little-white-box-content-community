---
id: IMP-architecture
layer: implementation
title: 服务架构与模块清单
status: aligned
owner: agent
upstream:
  - DES-content-community-backend
tracks:
  - app/
  - pkg/
verified_at: 2026-09-06
verified_commit: 3ac98c6
---

# 服务架构与模块清单

本文档取代旧 ARCHITECTURE 服务架构文档与 generated 旧快照索引的定位作用。
它只描述"当前代码如何组织"，不覆盖源码、配置、`.api`、`.proto` 与测试事实。

## 概览

esx 是基于 go-zero 的社交内容平台微服务集群：

```
Client → Gateway (REST :8888) → User RPC (:9090)
                              → Content RPC (:8088)
                              → Media RPC (:9008)
                              → Interaction / Feed / Message / Search /
                                Recommend / Assistant / Behavior RPC
```

异步链路经 RocketMQ 驱动：内容变更 → search 索引 / embedding / feed fanout /
count-sync / 清理 / Watch 匹配；客户端行为 → behavior RPC → 行为日志管道（ClickHouse）与
推荐特征；权威业务事务通过 outbox 同事务投递。Assistant 记忆与 Watch 权威库是
`xbh_assistant`（DSN `DB_ASSISTANT`）。`assistant-rpc` 只接受命令并读权威库；
`assistant-agent` worker 通过 MySQL lease 执行 run。Watch matcher 进程订阅
`post-create` / `post-update` / `post-delete` / `user-behavior-v2`，消费组
`assistant-watch-matcher-group`，把命中写入内部 bucket 并调度 Watch run。

## 服务清单

| 模块 | 类型 | 入口 | 定义文件 |
| --- | --- | --- | --- |
| Gateway | REST API 网关 | `app/gateway/gateway.go` | `app/gateway/gateway.api` |
| User | RPC | `app/user/rpc/user.go` | `proto/user/user.proto` |
| Content | RPC | `app/content/rpc/content.go` | `proto/content/content.proto` |
| Media | RPC + MQ | `app/media/rpc/media.go`、`app/media/mq/main.go` | `proto/media/media.proto` |
| Interaction | RPC | `app/interaction/rpc/interaction.go` | `proto/interaction/interaction.proto` |
| Feed | RPC + MQ | `app/feed/rpc/feed.go`、`app/feed/mq/main.go` | `proto/feed/feed.proto` |
| Message | RPC | `app/message/rpc/message.go` | `proto/message/message.proto` |
| Search | RPC + MQ | `app/search/rpc/search.go`、`app/search/mq/main.go` | `proto/search/search.proto` |
| Recommend | RPC + MQ | `app/recommend/rpc/recommend.go`、`app/recommend/mq/main.go` | `proto/recommend/recommend.proto` |
| Embedding | MQ + service | `app/embedding/mq/main.go`、`app/embedding/service` | `proto/embedding/embedding.proto` |
| Assistant | RPC（命令/读模型/SSE replay） | `app/assistant/rpc/assistant.go` | `proto/assistant/assistant.proto` |
| Assistant Agent worker | 异步 run 执行器 | `app/assistant/worker/main.go` | `app/assistant/worker/etc/agent.yaml` |
| Assistant Watch matcher | MQ 消费者 | `app/assistant/mq/main.go` | — |
| Behavior | RPC | `app/behavior/rpc/behavior.go` | `proto/behavior/behavior.proto` |
| Pipeline | 行为日志管道 | `app/pipeline/behaviorlog/main.go` | — |
| Content cleanup | MQ 消费者 | `app/content/mq/cleanup/main.go` | — |

## RPC 服务分层

每个 RPC 服务遵循 go-zero 标准分层：

```
internal/config/   → 配置结构体
internal/svc/      → 依赖注入容器（ServiceContext）
internal/server/   → gRPC server 实现
internal/logic/    → 业务逻辑
internal/model/    → 数据访问层
```

## 共享库（pkg/）

仓库为单一 Go module（根 `esx`），无嵌套 `go.mod`、无 `go.work`。日常脚本按
`go.mod` 发现目录遍历，当前只有根模块。跨服务只共享真正跨界的能力；业务专属
逻辑留在所属服务内部。

- `pkg/errx` — 业务错误码与 HTTP/gRPC 转换；框架错误不泄露原始消息。赞/藏重复
  请求按成功 no-op，不再保留从未返回的 AlreadyLiked / NotLikedYet 一类死码。
- `pkg/jwtx` — JWT 签发/校验与 context 透传。Handler/Logic 直接读 `jwtx`，不经
  middleware 包装函数。
- `pkg/middleware` — HTTP Required/Optional 鉴权与 CORS。行为接收与 Trace 在
  `app/gateway/internal/middleware/`；RPC 侧在 `pkg/interceptor/`。
- `pkg/interceptor` — gRPC 业务错误拦截器、trace_id 透传与服务间 HMAC 内部鉴权。
- `pkg/mqx` — RocketMQ 生产者/消费者封装与主题常量。
- `pkg/outboxx` — 事务发件箱与可靠投递 relay（含延迟/积压指标）。
- `pkg/event` — 跨服务事件载荷（PostEvent / InteractionEvent / BehaviorEvent）。
- `pkg/pageutil` / `pkg/cursorx` / `pkg/idempotencyx` / `pkg/cleanupx` /
  `pkg/validator` / `pkg/visibilityx` — 分页、游标、幂等、清理、输入边界、可见性。
- `pkg/util` — 仅 Snowflake ID。密码哈希在 `app/user/rpc/internal/password`；
  JSON 列编码在 `app/content/rpc/internal/model`。
- `pkg/testutil` — 集成测试容器与连接辅助。

## 包边界（内聚）

- Assistant 工具执行器按职责拆在 `app/assistant/internal/tool/exec_*.go`（搜索、
  内容、推荐、社交、记忆、Watch、来源、Web），注册表仍在 `registry.go`。
- Content 权威可见性：`pkg/visibilityx` 是纯策略；`app/content/visibility` 只适配
  Content RPC 客户端。Feed/Search/Recommend/Assistant 回源走适配层。
- SQL 基线里仍保留但无运行时 Model 的表（`category`、`media_task`、
  `xbh_message.verify_code`、`user_login_log`）是历史脚手架，不构成当前能力。
  删除生产 schema 需要独立迁移决策，本实现不通过空 Model 暴露它们。

## 服务间通信

- **Gateway → RPC**：zrpc 客户端 + etcd 服务发现；trace_id 经 gRPC metadata 透传。
- **RPC → RPC**：Interaction 写赞/藏前问 Content 校验 published；Assistant 经 Content
  重读正文并验证 published。详情/列表的访问者互动状态由 Gateway 回填 Interaction。
  记忆与 Watch 的权威在 assistant RPC（`xbh_assistant`），不新开业务服务。
- **算法旁路**：`algorithm/online_infer` 与 `offline_train` 可选；推荐超时则规则降级。

### 行为事件双轨可靠性模型

同一 `user-behavior-v2` 主题上存在两种投递保证，属有意设计（REL-001）：

- **权威业务动作**（like/unlike/favorite/unfavorite/comment/follow/unfollow）只能由
  user/content/interaction 的事务 outbox 生成，丢失即计数漂移，必须可靠投递。
- **客户端遥测动作**（exposure/click/dwell/view/play/share/hide/dislike）经 behavior-rpc
  直发 RocketMQ，仅服务推荐信号与分析，崩溃丢窗口可容忍；白名单在
  `pkg/event/behavior.go` 强制，count-sync 只应用权威动作，两条来源不交叉。
- **RPC → MQ**：权威业务事务与 outbox 同事务提交，relay 投递 RocketMQ。
- **写入路径**：权威写入走事务 outbox，不依赖 DTM。Content 契约已删除 `QueryPrepared`。

## 注意事项

- 本文档是事实快照；模块、端口、主题与配置以代码和 `etc/*.yaml` 为准。
- 定位具体模块或流程时，按 [DES-content-community-backend](../design/DES-content-community-backend.md)
  与 [IMP-content-community-backend](IMP-content-community-backend.md) 的追踪表逐条对照。
