---
id: DES-content-community-backend
layer: design
title: 小白盒内容社区后端设计
status: active
owner: agent
upstream:
  - SPEC-community-core
  - SPEC-content-discovery
  - SPEC-grounded-assistant
  - SPEC-feedback-reliability
---

# 小白盒内容社区后端设计

本设计说明如何以 go-zero 工程结构满足四份已批准规范。实现对齐状态以
[IMP-content-community-backend](../implementation/IMP-content-community-backend.md)
和源码、`.api`、`.proto`、SQL、测试为准；本文不覆盖代码事实。

## 组件边界

- Gateway（`app/gateway`）：HTTP 绑定、鉴权、SSE、BFF 组合。详情和列表的访问者点赞/收藏
  状态在 Gateway 回填 Interaction，不把 Interaction 客户端放进 Content。收藏隐私、回源过滤
  也在 Gateway 组合，因为没有独立 favorites 读模型跨库。
- RPC：user / content / interaction / media / message / feed / search / recommend /
  assistant / behavior，各自持有权威库与事务。
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

### Assistant

只对认证用户开放。工具检索 published 候选后必须 Content 重读正文。无正文证据则固定拒答。
事实段落强制 `[post:id]`，对外 SOURCE 只含回源验证过来源。LLM 不可用返回证据摘要；
检索或回源失败关闭。会话按用户隔离，Redis 限流 20/60s，历史 100 条 / 30 天。

### Assistant Agent 模式（AGNT-*）

编排放在 assistant RPC 内新增的 `internal/agent` 包，不开新服务：复用会话存储、安全
过滤、SSE 管道与指标。`Chat` 流式 RPC 增加 `mode` 与 `attachments` 字段分流：
enhanced_search 走既有单轮管线，agent 走多轮 Runner。对外新增 `TOOL_CALL` /
`CONFIRM_REQUIRED` 事件与 `ConfirmToolCall` 回调、REST `tool/confirm` 转发。

- **Runner 抽象**：`agent.Runner` 接口隔离编排引擎，默认实现基于 openai-go 官方 SDK
  的 chat completions + tools（经 `option.WithBaseURL` 指向现有 OpenAI 兼容端点），
  未来可换其他 agent SDK 而不动工具层与事件层。
- **工具层**：`search_posts`→SearchRpc、`web_search`→Tavily（`net/http` 直连，key 走
  env，未配置时从工具表剔除）、`create_post`/`update_post`/`delete_post`→ContentRpc v2
  写语义。全部显式传 userId（AGNT-003），不引入新的身份通道。create/update 引用的
  mediaId 必须 ⊆ 本次请求附件并先经 MediaRpc `BatchGetMedia` 校验 owner+status
  （AGNT-013），Content 侧 `validatePostMedia` 二次校验兜底；幂等键由 requestId 派生
  （AGNT-015）。
- **高危确认**：Runner 发出 `CONFIRM_REQUIRED(call_id)` 后挂起，等待 Redis pub/sub 唤醒
  （待确认项存 `{prefix}:pending_confirm:{requestId}:{callId}`，TTL=ConfirmTimeoutSeconds，
  默认 120s）；网关 `POST /api/v2/assistant/tool/confirm` → `ConfirmToolCall` 校验凭据
  一次性绑定后发布结果。超时/拒绝按 AGNT-020~022 反馈模型继续。
- **预算**：软限 MaxStepsSoft(8)/硬限 MaxStepsHard(12)（AGNT-030/031）：超软限后在每个
  工具结果尾部注入剩余轮数系统通知；达硬限剥离 tools 强制收尾一次，失败报
  `AGENT_BUDGET_EXCEEDED`。独立配额默认 10 req/min（AGNT-032）。
- **授权存储**：user 模块新增 `agent_capability_consent` 表与 Get/Set RPC（照抄
  personalization_preference 模式，含 granted_at/revoked_at 审计字段）；SQL 变更同时进
  基线与幂等补丁。网关在 mode=agent 时先查授权，未授权返回
  `errx.AgentNotAuthorized`（AGNT-002/004/006）。

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
- LLM 不可用但有证据：证据摘要降级。
- Agent 轮内预算耗尽且无法收尾：结构化 `AGENT_BUDGET_EXCEEDED` 错误事件，已成功写入保留。
- Agent 确认等待超时：视为拒绝并反馈模型，流不挂起。
- Assistant 状态存储不可用：一次性降级、不续接。
- 指标后端不可用：业务继续。

## 取舍

- `Total` 不重扫全库，换可实现的可见性保证。
- 详情互动状态放在 Gateway 而不是 Content，避免 Content 依赖 Interaction。
- 点赞前同步问 Content，换 CORE-034/016，增加 Interaction→Content 边。
- 不把赞/评/关通知做成产品，避免死消费者冒充完整通知系统。
- 曝光视口不在服务端复测，换可实现的去重边界。
- 算法旁路可选；未达 DISC-062 门槛不宣称学习改善。

## 验收策略

代码行为类（CORE-A*、DISC-A01~A05、ASST-A01~A04、REL-A01~A04 的接口部分）用 Go 测试
落地，每个改动的 Logic 至少一条失败路径。DISC-A06、ASST-050/051、REL-A05 需要人类冻结
集与真实观测，由 `IMP-todo-blocked-gates` 登记，禁止标 `aligned`。
