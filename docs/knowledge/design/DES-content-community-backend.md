---
id: DES-content-community-backend
layer: design
title: 小白盒内容社区后端设计
status: active
owner: agent
upstream:
  - SPEC-community-core
  - SPEC-content-discovery
  - SPEC-assistant-agent
  - SPEC-agent-memory
  - SPEC-agent-watch
  - SPEC-feedback-reliability
---

# 小白盒内容社区后端设计

本设计说明如何以 go-zero 工程结构满足社区核心、发现、持久异步 Assistant Agent 与
反馈可靠性规范。Agent Runtime、记忆与 Watch 的细节以
[DES-assistant-agent-runtime](DES-assistant-agent-runtime.md) 为准。实现对齐状态以
[IMP-content-community-backend](../implementation/IMP-content-community-backend.md)
和源码、`.api`、`.proto`、SQL、测试为准；本文不覆盖代码事实。

## 组件边界

- Gateway（`app/gateway`）：HTTP 绑定、鉴权、SSE、BFF 组合。详情和列表的访问者点赞/收藏
  状态在 Gateway 回填 Interaction，不把 Interaction 客户端放进 Content。收藏隐私、回源过滤
  也在 Gateway 组合，因为没有独立 favorites 读模型跨库。
- RPC：user / content / interaction / media / message / feed / search / recommend /
  assistant / behavior，各自持有权威库与事务；独立 assistant-agent worker 只拥有运行执行职责。
- MQ：search 索引、embedding、feed fanout、行为管道、推荐特征、媒体清理、内容计数同步。
- 算法旁路（`algorithm/`）：可选在线推理与离线训练。推荐在超时或不可用时规则降级
  （DISC-036）；算法不拥有可见性。
- 可靠写入：权威业务与 outbox 同事务，relay 投递 RocketMQ。不再使用 DTM。
- 行为闭环：客户端事件 → behavior-rpc（校验+去重）→ RocketMQ → behaviorlog
  （ClickHouse）→ recommend-mq 特征。
- 权威可见性：Content 是帖子状态唯一权威。Feed、Search、Recommend、Assistant 和
  收藏列表把索引/inbox/召回当候选，返回前经 `app/content/visibility` + `pkg/visibilityx`
  回源；验证失败整次请求失败关闭。

## 方案

### 可见性与详情状态

Content 持有状态机与正文。`FindPostById`/`FindByIds` 不读缓存，避免 CORE-053 允许的
失效失败把已取消发布内容继续当成 published。公开列表 SQL `status=1` 后再丢弃非
published 行。

已认证 GetPost/列表的 `isLiked`/`isFavorited` 由 Gateway 调 Interaction
`BatchCheckLiked`/`BatchCheckFavorited` 回填（CORE-032）。Content proto 可保留这两个
字段但 Content Logic 不再填写；对外以 Gateway 为准。

`Total` 只按本页过滤回减（CORE-015 / DISC-001）。全库精确计数需要索引与权威完全同步，
当前不做。

### 互动写路径

点赞/收藏在 Interaction 写入前经 Content `AssertInteractable` 确认目标可互动
（CORE-034）：帖子必须 published；评论必须有效且父帖 published。Content 不可用则失败关闭，
不写入关系。对不可用目标返回 `ContentNotFound`，与 CORE-016 一致。关注只校验用户身份。

公开计数：关系以 Interaction 为准；`post.like_count`/`favorite_count` 由 count-sync
异步收敛，目标 30 秒（CORE-032）。评论计数在 Content 事务内更新。

### 关注流

`/api/v2/feed/follow` 仅认证用户。先分页拉取当前 following（页大小 100），空关注立即
返回空列表，不混入推荐。inbox 只保留当前关注作者；outbox 按作者分批 `IN` 后归并。
读路径回源 Content；inbox 不在取消发布时主动撤回，新请求靠回源排除（DISC-011）。

### 搜索

ES 只索引 published，取消发布时尽力删文档。`post-update` 按 `post_id` 投递，载荷带
`revision`；索引写入用 external version，旧快照 409 丢弃。查询再回源 Content：丢掉
不可见 ID，标题与摘要改用权威正文，`Total` 按本页回减。用户/标签失败可降级并列出
`unavailableTypes`；帖子可见性或索引不可用不能降级成空成功。

### 推荐

候选来自规则召回，可选 OnlineInfer。匿名或关闭个性化只走规则冷启动（DISC-031）。
游标 HMAC 绑定身份/请求/场景/会话/实验/页大小，TTL 600s。作者配额、负反馈 30 天、
曝光 7 天按 DISC-034/035。返回前 `visibilityx` 过滤；可见性失败关闭，推理失败规则降级。
推荐可直连 ES/Milvus 作召回源，但仍必须回源 Content。候选特征按 `revision` 单调覆盖，
旧快照不回写。

### Assistant Agent

消息页虚拟线程由 assistant RPC 提供独立 read model，不创建 message 用户。所有用户输入先写
`xbh_assistant`，独立 worker 通过 MySQL lease 执行；断线不取消，事件从 MySQL 按序重放，Redis 仅
作通知。模型可直接对话并自主调用受授权工具，不再有 enhanced_search、模式字段或 Intent Router。
检索结果必须回源，只有 `present_sources` 选择的 run-local handle 成为来源卡，但来源不是回答门禁。
完整运行、Memory、Watch、历史 BM25、compact 与预算设计见
[DES-assistant-agent-runtime](DES-assistant-agent-runtime.md)。

### 写入与私信

帖子写路径只有 `/api/v2/post*`，强制 `expectedRevision`（CORE-013/062）。帖子/评论/
媒体/互动/关注走事务 outbox。私信权威写入以 message 库提交为成功（CORE-044）；不实现
赞/评/关通知生产者。`message-push` 消费者不是当前产品路径，部署可不启动；主题保留不
构成对外能力。

### 行为与隐私

Gateway 接收白名单动作。曝光的 50%/1s 由客户端判定，服务端只强制 `(requestId, postId)`
去重（REL-004）。完整 IP 在写入 ClickHouse 前哈希。业务日志不写手机号、验证码、正文或
私信。关闭个性化后 recommend-mq 定时清特征，24 小时内完成（REL-023）。

### 健康与观测

`/health` 存活，`/health/ready` 列出依赖；搜索/Assistant 可选，故障只标 `degraded`。
MQ 消费者与 outbox relay 暴露 outcome 与延迟。SLO 报告由 `scripts/spec_evals.py slo`
按 REL-030~033 口径计算；正式关闭依赖真实月度数据。

## 失败模式

对应 `REL-054`：

- 权威库不可用：写/读返回 503。
- Redis 不可用：回源；已提交写仍成功。
- outbox relay 不可用：业务成功，异步延迟并告警。
- 行为 Broker 不可用：行为接收 503。
- 部分推荐来源不可用：规则降级并标记。
- 可见性不可用：发现与 Assistant 失败关闭。
- 帖子搜索索引不可用：搜索 503。
- LLM 不可用：run 明确错误终止，保留已提交文本、来源卡与副作用摘要。
- Agent 触达任一硬预算：`AGENT_RESOURCE_LIMIT`，已成功副作用保留且可审计。
- Agent 确认等待超时：数据库 CAS 拒绝，run 可继续收尾或明确终止。
- Assistant MySQL 不可用：拒绝接受新 run，不同步执行；Redis 不可用则 SSE 轮询 MySQL。
- 指标后端不可用：业务继续。

## 取舍

- `Total` 不重扫全库，换可实现的可见性保证。
- 详情互动状态放在 Gateway 而不是 Content，避免 Content 依赖 Interaction。
- 点赞前同步问 Content，换 CORE-034/016，增加 Interaction→Content 边。
- 不把赞/评/关通知做成产品，避免死消费者冒充完整通知系统。
- 曝光视口不在服务端复测，换可实现的去重边界。
- 算法旁路可选；未达 DISC-062 门槛不宣称学习改善。

## 验收策略

代码行为类（CORE-A*、DISC-A01~A05、AGENT-A01~A06、REL-A01~A04 的接口部分）用 Go 测试
落地，每个改动的 Logic 至少一条失败路径。DISC-A06、ASST-050/051、REL-A05 需要人类冻结
集与真实观测，由 `IMP-todo-blocked-gates` 登记，禁止标 `aligned`。
