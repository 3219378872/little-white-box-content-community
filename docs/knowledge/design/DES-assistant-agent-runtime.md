---
id: DES-assistant-agent-runtime
layer: design
title: 持久异步 Assistant Agent Runtime
status: active
owner: agent
upstream:
  - SPEC-assistant-agent
  - SPEC-agent-memory
  - SPEC-agent-watch
  - SPEC-content-discovery
  - SPEC-feedback-reliability
---

# 持久异步 Assistant Agent Runtime

本设计将同步、双模式、Redis 会话的旧 Assistant 改为 MySQL 权威的长期异步 Agent。实现映射和实际
证据以 `IMP-content-community-backend`、源码、契约、SQL 与测试为准。

## 组件与所有权

```text
Gateway REST/SSE
  -> assistant-rpc (command acceptance + read model + event replay)
       -> MySQL xbh_assistant (authority)
       -> Redis publish (best-effort wake-up only)

assistant-agent worker
  -> MySQL lease queue -> provider -> tools -> journal/events/messages
  -> Elasticsearch Assistant history derivative

assistant-watch matcher
  -> content/behavior events -> two-minute buckets -> Watch run queue
```

- `assistant-rpc`：鉴权、校验、接受 message/session/read/cancel/confirm/memory/watch 命令，读取 thread、
  messages、events；不在请求内调用模型。
- `assistant-agent`：独立二进制，claim/renew/execute/recover run。进程可以横向扩展，数据库租约保证
  同一 run 同时只有一个 owner。
- `assistant-watch`：保留事件匹配，写内部 execution/hit 并按用户两分钟 bucket 调度只读 Watch run；
  不直接产生用户可见 hit。
- MySQL：所有 Assistant 可见与运行状态权威；Redis：事件 channel 通知，可完全故障降级；ES：
  Assistant 历史派生索引，可 rebuild/delete。
- 普通 message RPC/库不新增 Agent 用户或数据，Gateway 并行合并两种 thread read model。

## 权威模型

- `assistant_thread(user_id PK)`：当前 session、未读、最后可见消息、活跃前台 run。
- `assistant_session(id, user_id, prompt_epoch, prompt_snapshot, tool_snapshot, compact_summary, status)`。
- `assistant_message(id, user_id, session_id, run_id, role, kind, content, api_content, visible, unread,
  compacted, deleted_at_ms, created_at_ms)`；`api_content` 保存 provider-bound 原字节。
- `agent_run(id, user_id, session_id, request_id, source, status, phase, priority, queued_payload,
  lease_owner, lease_until_ms, heartbeat_at_ms, cancel_requested, counters..., prompt_epoch)`。
- `agent_run_event(run_id, seq, type, payload_json)`：`UNIQUE(run_id, seq)`，终止事件唯一。
- `agent_tool_call`：call、规范化参数摘要、状态、结果与 source handles。
- `agent_command_journal`：`UNIQUE(user_id, request_id, tool, canonical_args_digest)`，缓存副作用结果。
- `core_memory_entry` / `memory_change`：双 target 自然语言条目、version、变更前后快照。
- `assistant_index_outbox`：message upsert/delete 到 ES；MySQL 消息永远是回源权威。
- Watch task/execution/hit 保留；hit 只作内部 bucket 输入与 90 天审计。

破坏性迁移用 `assistant_runtime_v3` marker：首次执行清空并重建 `xbh_assistant`，清空 user 库 Agent
consent、旧 Redis namespace 与旧 ES 索引；marker 提交后重复 patch 不再清理。社区、普通私信和用户
主体表不在清理集合。

## 接收、并发与输入处置

`POST /assistant/messages` 在一个事务中：锁 thread，校验 consent/session，写 user message，按当前
前台 run phase 决定 disposition，创建或更新 run，提交后返回 ids。模型请求 phase 写 redirect；工具
phase 写 steer；compact/attachment/unsafe phase 写最多 32 条 FIFO。无活跃前台 run 则创建 queued
user run。数据库提交前不报告 accepted。

新 session 只结束当前 session、滚动 epoch 和创建空 session。clear history 逻辑删除 message、写 ES
delete outbox、清 thread 可见摘要，不删 Memory/Watch。显式 Stop 设置 cancel_requested；worker 在模型
和工具安全点检查，终止事件写库后才释放 active run。

## Lease、步骤提交与恢复

worker 用 `SELECT ... FOR UPDATE SKIP LOCKED` claim `queued` 或租约过期 `running` run，将租约设为 60 秒；
独立续租循环每 10 秒 CAS `lease_owner`。每个 provider 回答、工具请求/结果、compact 和终止均作为 step
事务提交。恢复从最后完整 step 重建 provider messages，使用 session prompt 快照与 message
`api_content` 原字节；未完整提交的 provider 调用可重试，副作用由 journal 去重。

用户 run 优先于 Watch、后者优先于 memory-review。claim 按 priority/created_at；前台消息可设置后台
run cancel。Watch 取消前尚未投递的 hit bucket 重置为 pending。

## 事件与 SSE

每个事件先锁 run 分配 `seq=max+1` 并写 MySQL，然后 best-effort `PUBLISH assistant:run:<id>`。SSE 先按
`Last-Event-ID` 查询 `seq > cursor`，之后通知唤醒再查询；没有通知时每秒轮询。客户端断线只结束读
循环，不写 cancel。运行中超过 30 秒没有业务事件时 worker 写内部 heartbeat event；API 可用它维持
流活跃，但 thread 不渲染为消息。

## Prompt、模型与 Compact

仓库 `app/assistant/agent/SOUL.md` 是 human-owned 默认温暖伙伴资产。Prompt builder 固定拼接平台安全
规则、SOUL、Agent/tool 规则、按 target/id 排序的 MEMORY/USER、会话历史，结果与工具定义序列化后
保存在 session。普通恢复绝不重新生成。

Provider adapter 实现 Chat Completions 与 Responses 的统一 message/tool-call step。启动时执行协议能力
检查；未能表达工具 schema/call/result 时 readiness false。旧 `LLM.ModelSmall` 删除；可选
`BackgroundReview.Model` 只用于 memory-review，缺省主模型。

token estimator 达窗口 50% 时进入 compact phase：选择最新 20% token、所有未完成 tool/confirm，模型
生成摘要；事务提交摘要、新 prompt epoch 和 compact 标志。提交后重新载入 SOUL、Memory 与工具快照。
原 message 保留并通过 outbox 可检索。

## Memory Review

每个 session 记录连续成功未中断 user turn。达到 10 的倍数写 memory-review run，限制 16 provider round
与 600k input token，工具表只含 memory add/replace/remove/batch/read。前台 run claim/accept 时取消未完成
审查。成功 change 在 thread 插入 `kind=memory_changed, unread=false` 系统行及 undo action。

## History Search

outbox relay 为 user/assistant 可见消息写 `assistant-history-v1`，字段包含 userId、sessionId、messageId、
role、content、timestamps、deleted/compacted；CJK analyzer + BM25。工具执行四种 shape：keywords、around、
session、recent。ES 查询必须带 userId，但仍逐条以 messageId 回源 MySQL 校验 user、365 天和删除状态。
当前 session live context、tool、review 与普通 message 库从未进入索引。

## 工具、Journal、确认与来源

模型直接看到按 run source 裁剪的 registry，不再有 `ClassifyIntent`/`QueryPlan`/Planner。user tools 根据
consent version；Watch 只读；review 只 Memory。每次 call 先严格 JSON schema 和 canonical JSON；有副作用
则 reserve journal，已成功行直接回放结果，执行后在同一业务边界提交结果。create/update 继续使用下游
幂等、revision 与 ownership。

delete_post 在执行前写数据库 confirmation，绑定 user/session/run/call/tool/digest/revision；confirm API
使用 `pending -> approved|rejected` CAS。worker 只消费一次 approved，并在执行前复核 revision。

搜索、推荐、web executor 对验证结果生成随机 handle，写 `agent_source_ledger(run_id, handle, kind,
authority_id, revision, payload)`。工具结果只给模型 handle 和安全摘要。`present_sources` 复核同 run 最多
10 个 handle，写 source_card event；普通最终文本不做 ID/URL 解析。

## Watch 投递

matcher 将命中写 2 分钟 user bucket。同任务近一小时 sent < 3 且用户当天 sent < 20 才调度；否则 bucket
保持 deferred 并在下一允许窗口摘要。Watch worker 读取 bucket，使用只读 registry 形成回答，最终 assistant
message 与 thread unread 在同一事务提交。用户 run 抢占时 bucket 回 pending。

## 预算与观测

run counter 在每个 step 事务累计 rounds、tool calls、input/output/cache tokens、cost、elapsed/idle。warning /
critical 按时间、round、output 三个维度以唯一 `(run, level, dimension)` 记录指标和日志，并向下一模型 step
加入不可见 convergence instruction。硬上限检查在 claim、provider 前后和工具前；触顶写
`AGENT_RESOURCE_LIMIT` error，payload 包含 partial text 与完成 journal 摘要。

Prometheus 覆盖 queue age、lease claim/recovery/renew failure、run phase/elapsed/idle、token/cost、journal hit、
confirmation、compact、BM25/outbox、Watch bucket/rate、review、Redis notify failure 和 SSE poll fallback。

## 验证

- 纯逻辑：disposition state machine、预算、canonical digest、source handles、Memory 容量/version/undo。
- MySQL 集成：lease crash recovery、journal、confirm CAS、event replay、compact transaction、Watch bucket。
- Redis/ES：通知故障轮询、history rebuild/delete、user isolation 与回源剔除。
- provider contract：Chat Completions 与 Responses 真实 tool-call fixture/readiness。
- 根真实栈：授权、异步发送、断线重连、删除确认、memory-review、compact 新 epoch、history、Watch 主动消息。
