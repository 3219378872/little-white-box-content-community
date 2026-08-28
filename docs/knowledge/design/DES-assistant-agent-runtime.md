---
id: DES-assistant-agent-runtime
layer: design
title: Assistant Agent Runtime（搜索、记忆、推荐、追踪）
status: active
owner: agent
upstream:
  - SPEC-grounded-assistant
  - SPEC-assistant-agent-mode
  - SPEC-agent-memory
  - SPEC-agent-watch
  - SPEC-content-discovery
  - SPEC-feedback-reliability
---

# Assistant Agent Runtime

本设计说明如何在现有 assistant RPC 内满足 Agent 分组工具、四层记忆、可解释推荐与
条件追踪。实现对齐以 [IMP-content-community-backend](../implementation/IMP-content-community-backend.md)
和源码、`.api`、`.proto`、SQL、测试为准。

总设计中的 Agent 小节见 [DES-content-community-backend](DES-content-community-backend.md)
的指向；细节以本页为准。

## 目标映射

| 规范 | 设计落点 |
| --- | --- |
| AGNT-001～003、060 | 仍走 `Chat` 流式 RPC；`enhanced_search` 缺省不变 |
| AGNT-007、010 | `consent_version` 存在 user 库；工具按分组注册，版本不足的分组不进工具表 |
| AGNT-011～019、ASST-002～012 | Search 回源 Content；评论必须挂在可见 published 父帖；web_search 独立来源类型 |
| MEM-* | `xbh_assistant` 四层表 + 回合后异步抽取 + REST |
| WCH-* | Watch 表 + `app/assistant/mq` 事件匹配 + 命中 REST / 下次对话注入 |
| DISC-001、ASST-004 | Recommend 工具只转发 RecommendRpc 真实 ID，回源后再解释 |
| REL-022 | agent_run 不落用户输入/回答全文 |

## 组件边界

- 仍只部署 assistant RPC，不新开业务服务。新增权威库 `xbh_assistant` 与可选 MQ 消费者
  `assistant-watch-matcher-group`。
- Gateway：SSE `Chat` 原样转发；新增记忆/Watch/反馈 REST，只绑定参数、鉴权、调 RPC。
- User RPC：`agent_capability_consent.consent_version`；查询同时返回服务端当前披露版本
  （常量 `2`）。
- Search / Content / Recommend / Interaction / Media：只被工具调用，不拥有记忆或 Watch。
- 会话历史仍 Redis（ASST-021）。记忆、Watch、运行审计进 MySQL。

## 方案

### Runtime 编排

`internal/agent` 对外保留 `Runner` 作为 Executor。新增 `Runtime`：

1. Intent Router：把用户话轮解析为 Query Plan（entity、intent、time_range、sources、
   filters）。无小模型时用当前 LLM；解析失败则 intent=`general`，工具表仍按 consent 分组。
2. Context Builder：按 intent 装载 Profile / 未衰减 Interest / 开放 Task，以及最多数条
   未读 Watch 命中。注入截断后的本会话历史（从最近话轮向前，受 `MaxContextRunes`
   约束，不含当前请求）。Episodic 只在追问历史时检索。
3. Policy：RPC 复核 `consent.Granted` 与 `consent_version`；未授权不得执行工具。
   预算与安全过滤仍在 Executor / ChatLogic。
4. Planner：把 plan 变成检索参数与本轮工具子集。
5. Executor：既有 openai-go function calling 循环；高危确认、软硬预算不变；空
   `Choices` 视为 LLM 不可用。终答中和模型自造 `[post:]`/`[comment:]`，只附加本轮
   已验证的 post/comment 来源。
6. 回合成功结束后异步 Memory Writer，不阻塞 `DONE`。`recommend` 意图把当前话轮
   upsert 为开放 Task；`continue_task` 不新建任务。

Intent 取值（内容域）：`community_opinion`、`factual_lookup`、`recommend`、`watch`、
`memory_query`、`continue_task`、`write_post`、`general`。

### 工具分组

注册表按 AGNT-010 分组。`AllowedTools` 配置可收缩。consent_version `< 2` 时只暴露
版本 1：`search_posts`、`web_search`、Write 三件套。

Search：ES 关键词（SearchRpc）+ 可选 Recommend 相似召回，RRF 合并后 Content 回源。
评论不对评论建倒排：对 Top-K 帖子 `GetCommentList`，有效评论作为 `comment` 证据。
`web_search` 未配置 key 时从工具表剔除。

UserState：Interaction/User/Content 已有读接口，显式传当前 userId。

Recommend：`recommend_posts` 调 GetRecommendPosts（scene=`agent`）；硬过滤来自 Memory
（suppressed、负向 Profile、Task 已排除 ID）；LLM 只对回源后的 Top-N 解释并选出最多 3
条卡片。禁止模型编造 post id。

Memory / Watch：见下。Write 保持现实现。

### 记忆

权威表：`user_preference`（Profile）、`user_interest`（Interest，读取时
`score * exp(-λ Δt)`，λ 配置）、`user_memory`（Episodic）、`memory_evidence`、
`task_memory`。唯一当前键 `(user_id, layer, dimension, value)`。冲突更新分值并追加
`history_json`。`suppressed=1` 表示“不要记住这个”。

写入：规则抽取（显式句式）优先，LLM 候选必须过 schema。来源仅 `behavior` /
`conversation` / `explicit`。关闭个性化后 behavior 来源不再用于约束（MEM-012）。

REST：`GET /assistant/memory`、`PATCH|DELETE /assistant/memory/:id`。

### 推荐反馈

`POST /assistant/recommend/feedback` 写入 `recommendation_feedback`，供 Memory Writer
抽取，不直接改 Recommend 模型。

### Watch

任务表 `watch_task`：`condition_type` 为 `author_new_post` | `tag_new_post` |
`keyword_new_post` | `post_revised` | `discussion_spike`。同一用户+条件+目标唯一。

Matcher 消费 `post-create` / `post-update` / `post-delete` 与 `user-behavior-v2`。
规则条件直接命中；`discussion_spike` 先看评论行为或 `comment_count` 增量，过阈值才
LLM 判定。`(task_id, event_key)` 去重写入 `watch_execution` / `watch_hit`。

投递：`GET /assistant/watch/hits`；Context Builder 注入未读摘要；聊天可发 `WATCH_HIT`
事件。不写私信、不做推送。

消费者组：`assistant-watch-matcher-group`。不新增 topic。

### 结构化事件

`ChatEventType` 增加 `CARD`、`ACTIONS`、`WATCH_HIT`。卡片 payload 只含本轮已验证标识。
旧客户端忽略未知类型（AGNT-060）。`ChatReq.context_post_id` 可选，缺省忽略。

### 观测

`agent_run` / `tool_call`：request_id、user_id、intent、model、latency、status、
工具名与结果状态。禁止正文、secret、提示词。

### Model routing

同一 OpenAI 兼容端点。可选 `LLM.ModelSmall`；未配置则全用 `LLM.Model`。Tier 0 纯读
工具可跳过额外生成。

## 接口与数据流

```text
Gateway /assistant/chat (mode=agent)
  → User.GetAgentCapabilityConsent（granted + consent_version）
  → Assistant.Chat
       Runtime → Executor(tools(version)) → SSE events
       async Memory Writer → xbh_assistant

Gateway /assistant/memory|watch|recommend/feedback
  → Assistant 对应 RPC → xbh_assistant

post-* / user-behavior-v2
  → assistant-mq Watch Matcher → watch_hit
```

SQL：基线 `deploy/sql/xbh_assistant.sql`；存量 `patches/20260827_assistant_runtime.sql`
与 `patches/20260827_agent_consent_version.sql`。

## 取舍

- 评论不做独立 ES 索引，避免扩张 DISC 搜索范围。
- Watch 不进入通知中心，避免打开 INT 非目标。
- 写帖工具保留，避免破坏已交付的 AGNT 验收。
- 记忆不迁会话 Redis，避免改 ASST-021 契约。
- 不新开服务、不引入新 LLM SDK。

## 失败模式

- 未授权：`AGENT_NOT_AUTHORIZED`，不降级执行工具。
- consent_version 过期：旧分组可用，新分组工具不可见；调用拒绝反馈模型。
- 检索/可见性失败：Search/Recommend 工具失败关闭，不编造证据。
- 记忆库不可用：对话可完成，Memory 工具与 REST 失败，不得谎报写入成功。
- Watch 匹配器不可用：任务仍可 CRUD；恢复后不补扫已不可见内容。
- LLM 不可用：不执行新的帖子写；规则 Watch 匹配可继续。
- 预算耗尽：既有 `AGENT_BUDGET_EXCEEDED`；已成功写入不回滚。

## 验证策略

- 单测：consent_version 分组裁剪、RRF、记忆冲突与衰减、Watch 去重与预筛选、未知事件兼容。
- 契约：gateway rest decision 覆盖新 REST；proto/api 经 `make generate`。
- 回归：AGNT-A01～A05 旧路径保持绿色。
- 联调：搜索综述带 comment 证据、记忆可见可删、推荐卡 ID 来自 RecommendRpc、Watch 命中。
- ASST-050 人类冻结集本设计不关闭。
