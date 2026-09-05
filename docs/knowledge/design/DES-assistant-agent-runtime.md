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

> 2026-09-05：复杂需求、问答和结构化回答由[社区研究设计](DES-agent-community-research.md)承接。
> 本页继续记录基础运行机制及旧协议兼容路径。设计完成不等于实现验证完成，当前状态见实现层。

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

- `assistant-rpc`：鉴权、校验、接受 message/read/cancel/confirm/memory/watch 命令，读取 thread、
  messages、events；不在请求内调用模型。不提供新 session API。
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
- `memory_target_lock(user_id, target)`：按用户和 MEMORY/USER target 串行容量、规范化去重、replace/remove
  与 undo，避免并发锁升级和超容量提交。
- `assistant_index_outbox`：message upsert/delete 到 ES；MySQL 消息永远是回源权威。
- Watch task 保留；execution/hit 只作内部 bucket 输入与 90 天审计，到期分批物理删除；
  `watch_send_reservation` 与 `watch_send_stat.reserved_count` 在调度事务中原子预留小时/日配额，只有成功
  投递才转为 sent，失败、抢占和 discard 均释放 reservation。

破坏性迁移用 `assistant_runtime_v3` marker：首次执行清空并重建 `xbh_assistant`，清空 user 库 Agent
consent；marker 提交后重复 patch 不再清理。生产执行前必须绑定 MySQL `server_uuid`，分别备份并验证
Assistant 库与 consent，且提供精确确认值；补丁名和 SHA-256 写迁移 ledger。社区、普通私信和用户主体
表不在清理集合。Redis namespace 与 ES 派生索引按独立运维步骤治理，SQL patch 不虚假宣称跨存储清理。

## 接收、并发与输入处置

`POST /assistant/messages` 在一个事务中：先校验 consent 与 requestId 重放，再按 id 锁该用户全部开放
`agent_run`（只把 Watch/review 标为取消），之后锁 thread、重查幂等结果并写 user message。这样输入
redirect/steer、worker 终态与 Watch 抢占统一遵守 `agent_run -> assistant_thread -> Watch bucket/quota`
锁序。随后按当前前台 run phase 决定 disposition，创建或更新 run；模型请求 phase 写 redirect，工具
phase 写 steer，compact/attachment/unsafe phase 写最多 32 条 FIFO。无活跃前台 run 则创建 queued
user run。数据库提交前不报告 accepted。

每用户一条永久前台 session：缺失则创建，遗留 `closed` 行 reopen，不再因用户操作关闭并另开一行。
线程 `last_message_at_ms` 距今不少于 30 分钟后，下一次新建 user 或 Watch run 在同一 session 上滚动
prompt epoch，重建 Safety/SOUL/工具规则/MEMORY，并保留 `compact_summary`。redirect、steer、FIFO
和崩溃恢复复用已保存快照。clear history 逻辑删除 message、写 ES
delete outbox、清 thread 可见摘要，不删 Memory/Watch。显式 Stop 只把 `cancel_requested` 置 1，该位
一旦置位就不能被后续 `UpdateRun` 清掉。worker 为 in-flight 模型/工具请求单独派生 work context：
轮询到取消位后立即 cancel 该 context（HTTP 随 request context 中止），并在每个模型/工具安全点
重新读库；已取消则写 `CANCELLED` 终止事件，不得把随后返回的模型正文当 `done`。持久化用未取消的
parent context，避免 Stop 把收尾 SQL 一并打断。

## Lease、步骤提交与恢复

worker 用 `SELECT ... FOR UPDATE SKIP LOCKED` claim `queued` 或租约过期 `running` run，将租约设为 60 秒；
独立续租循环每 10 秒 CAS `lease_owner`。每个 provider 回答、工具请求/结果、compact 和终止均作为 step
事务提交。恢复从最后完整 step 重建 provider messages，使用 session prompt 快照与 message
`api_content` 原字节；未完整提交的 provider 调用可重试，副作用由 journal 去重。provider stream 的
writer identity 为 run/lease/input/model-round/attempt；token 以小批次提交。重试、redirect 与
lease 接管前若已有公开 delta，先写 `response_reset(streamId)`，恢复时顺序重放 token/reset 后只得到
获胜 attempt。模型在已流式输出用户可见正文后只调用 `present_sources` 时不写 `response_reset`，
该正文作为可见 assistant 消息保留；`present_sources` 不是失败 attempt。

用户 run 优先于 Watch、后者优先于 memory-review。claim 按 priority/created_at；前台消息可设置后台
run cancel。Watch 取消前尚未投递的 hit bucket 重置为 pending。

## 事件与 SSE

每个事件先锁 run 分配 `seq=max+1` 并写 MySQL，然后 best-effort `PUBLISH assistant:run:<id>`。SSE 始终按
固定间隔查询 MySQL 的 `seq > cursor`；Redis wake 只缩短下一次查询等待，读取不得阻塞等待 wake token。
客户端断线只结束读循环，不写 cancel。`token` 与 `response_reset` 携带 streamId；前者追加，后者清空
指定 run 的临时回答。
运行中超过 30 秒没有业务事件时 worker 写内部 heartbeat event；API 可用它维持
流活跃，但 thread 不渲染为消息。

## Prompt、模型与 Compact

仓库 `app/assistant/agent/SOUL.md` 是 human-owned 默认温暖伙伴资产。Prompt builder 把平台安全规则、
SOUL 与 Agent/tool 规则冻结为 system 原字节；按 target/id 排序的 MEMORY/USER 经 JSON 编码后进入独立
`<untrusted-memory-context>` user sidecar，Go JSON 的 HTML 转义阻止条目伪造闭合标签。system、sidecar、
工具 schema 与 provider capability 一并序列化到 session；普通恢复绝不重新生成。旧格式 snapshot 按旧
字节恢复，冷对话拼接与 compact 成功提交才升级格式。

Provider adapter 实现 Chat Completions 与 Responses 的统一 message/tool-call/stream step。route profile
声明 WireAPI、模型、窗口、输出、流式与工具能力；启动 canary 强制调用无副作用虚拟工具。resilient
client 将错误分类后最多三次有界抖动退避，尊重上限内 Retry-After，并只向 capability/privacy 兼容的
route fallback。attempt 结果以内部 run event 审计，不向 SSE 暴露 provider 错误正文。

usage adapter 分离 input/output/cache-read/cache-write/reasoning，并按独立价格计算。可选
`BackgroundReview.Model` 只用于 memory-review，`LLM.AuxModel` 用于 compact；缺省使用冻结主 route。

compact 优先以上一次 provider prompt usage 为锚点，只估算后续新增消息；无 usage 时 ASCII 约四字符
一 token、非 ASCII 至少一字符一 token。达到窗口 50% 后选择最新 20% token、所有未完成 tool/confirm，
以及当前 Watch 的隐藏 `watch_input` sidecar；摘要模型接收预算内的完整消息。压缩结果必须比输入小并
低于目标阈值，否则保留原消息并明确失败。事务成功后才提交摘要、新 prompt epoch/sidecar/capability
快照和 compact 标志。原 message 在 365 天保留期内通过 outbox 可检索；worker 启动及每小时执行有界
批次清理，物理删除与 ES delete outbox 同事务，旧 upsert payload 同时移除。
隐藏 sidecar（工具轮与 Watch 注入）只通过 `api_content` 进入 provider 历史，不写可见正文或 ES outbox。

## Memory Review

每个 session 记录连续成功未中断 user turn。达到 10 的倍数写 memory-review run，限制 16 provider round
与 600k input token，工具表只含 memory add/replace/remove/batch/read。前台 run claim/accept 时取消未完成
审查。成功 change 在 thread 插入 `kind=memory_changed, unread=false` 系统行及 undo action。

## History Search

outbox relay 为 user/assistant 可见消息写 `assistant-history-v1`，字段包含 userId、sessionId、messageId、
role、content、timestamps、deleted/compacted；CJK analyzer + BM25。工具执行四种 shape：keywords、around、
session、recent。ES 查询必须带 userId，但仍逐条以 messageId 回源 MySQL 校验 user、365 天和删除状态。
当前 provider live context 的 message ids、tool、review 与普通 message 库从未进入结果；同 session 已
compacted 或不在 live window 的历史仍可检索。

## 工具、Journal、确认与来源

模型只看到 prompt epoch 冻结的 registry snapshot，不再有 `ClassifyIntent`/`QueryPlan`/Planner。单一
metadata 声明 effect、source、consent、confirmation、availability、幂等类型与最大输出；它派生广告、
授权、journal 与 guard。每次 call 严格拒绝未知字段和尾随 JSON，再 canonicalize；有副作用则 reserve
journal，已成功行直接回放结果，执行后在同一业务边界提交结果。相同规范参数与规范结果/错误连续出现
时第二次注入收敛提示，第三次以 `TOOL_NO_PROGRESS` 终止。create/update 继续使用下游幂等、revision
与 ownership。

delete_post 在执行前写数据库 confirmation，绑定 user/session/run/call/tool/digest/revision；confirm API
使用 `pending -> approved|rejected` CAS。worker 只消费一次 approved，并在执行前复核 revision。

搜索、推荐、web executor 对验证结果生成随机 handle，写 `agent_source_ledger(run_id, handle, kind,
authority_id, revision, payload)`。需要来源的 executor 在 ledger 不可用或任一 handle 写失败时整体失败，
不得把未登记结果返回给模型。工具结果只给模型 handle 和安全摘要。`present_sources` 复核同 run 最多
10 个 handle，写 source_card event；普通最终文本不做 ID/URL 解析。

## Watch 投递

matcher 先按事件 revision 回源当前 published 状态，再将命中写 2 分钟 user bucket。调度事务锁 thread、
bucket 和 quota 行，原子预留同任务每小时 3 条与每用户每日 20 条额度；超额 bucket 保持 deferred 并在
下一允许窗口摘要。Watch worker 读取精确 hit ids，把命中 JSON 写成当前 run 的隐藏 `watch_input`
sidecar（`visible=false`，`api_content` 为 provider user turn），再使用只读 registry 形成回答。恢复、每个
模型轮之前及最终消息提交之前都重新回源全部命中的当前可见性；缺失、过期或任一不可见时 fail-closed
discard，已流式正文先写 `response_reset`。sidecar 重放放在本 run 工具消息之前，不得每轮追加到上下文
末尾。最终 assistant message、thread unread、bucket sent、reservation 转 sent 与 run 终态在同一事务
提交。用户 run 抢占时 bucket 回 pending 并释放 reservation。

Watch run 的 `error` 与用户抢占/取消分开处理：失败先释放 reservation，再用 `not_before_ms` 从 1 分钟
开始指数退避，单次最多 30 分钟；同一用户、同一 bucket 的 error run 总尝试最多 8 次，对应重试间隔
1/2/4/8/16/30/30 分钟。历史次数只通过结构化 `queued_payload.bucket_id` 关联并排除当前 run；当前
run 的关联缺失或不一致时 fail-closed discard。第 8 次失败，或下一次重试将越过 bucket 创建后 90 天
保留边界时，bucket 转为 discarded，不再创建 run。`cancelled`、Stop、撤权和用户输入抢占不消耗失败
次数，仍立即回 pending 并释放 reservation。worker 异常退出后，调度器对遗留终态 run 执行同一转换，
且只接受 user 与 source 均匹配的 Watch run 关联。

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
- provider contract：Chat Completions 与 Responses 的非流式/流式 tool-call fixture、cache usage、错误分类、
  Retry-After、fallback、canary 与跨 chunk scrub。
- 根真实栈：授权、异步发送、断线重连、删除确认、memory-review、compact 新 epoch、history、Watch 主动消息。
