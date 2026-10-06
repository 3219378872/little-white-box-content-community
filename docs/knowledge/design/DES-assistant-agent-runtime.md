---
id: DES-assistant-agent-runtime
layer: design
title: 持久异步 Assistant Agent Runtime
status: active
owner: agent
updated_at: 2026-10-06
tracks:
- AGENT-001
- AGENT-002
- AGENT-003
- AGENT-004
- AGENT-010
- AGENT-011
- AGENT-012
- AGENT-013
- AGENT-014
- AGENT-015
- AGENT-020
- AGENT-021
- AGENT-022
- AGENT-023
- AGENT-024
- AGENT-025
- AGENT-026
- AGENT-050
- AGENT-051
- AGENT-052
- AGENT-053
- AGENT-054
- AGENT-060
- AGENT-061
- AGENT-062
- AGENT-063
- AGENT-080
- AGENT-081
- AGENT-082
- AGENT-083
- AGENT-090
- AGENT-A01
- AGENT-A02
- AGENT-A03
- AGENT-A04
- AGENT-A05
- AGENT-A06
- AGENT-A07
- AGENT-A08
- AGENT-A09
- MEM-001
- MEM-002
- MEM-003
- MEM-010
- MEM-011
- MEM-012
- MEM-013
- MEM-014
- MEM-020
- MEM-021
- MEM-022
- MEM-023
- MEM-024
- MEM-030
- MEM-031
- MEM-032
- MEM-033
- MEM-A01
- MEM-A02
- MEM-A03
- MEM-A04
- MEM-A05
- MEM-A06
---

# 持久异步 Assistant Agent Runtime

本设计将同步、双模式、Redis 会话的旧 Assistant 改为 MySQL 权威的长期异步 Agent。逐条实现状态和
当前证据分别见 [六域 IMP](../implementation/README.md) 与 [EVD](../evidence/README.md)；源码、契约、
SQL 与测试仍是事实权威。

> 2026-09-05：复杂需求、问答和结构化回答由[社区研究设计](DES-agent-community-research.md)承接。
> 本页继续记录基础运行机制及旧协议兼容路径。设计完成不等于实现验证完成，当前状态见实现层。
>
> 2026-10-06：`SPEC-agent-watch` 退役，Watch matcher、投递窗口、配额与主动消息已从本设计及实现中删除。

## 组件与所有权

```text
Gateway REST/SSE
  -> assistant-rpc (command acceptance + read model + event replay)
       -> MySQL xbh_assistant (authority)

assistant-agent worker
  -> MySQL lease queue -> provider -> tools -> journal/events/messages
  -> Elasticsearch Assistant history derivative
```

- `assistant-rpc`：鉴权、校验、接受 message/read/cancel/confirm/memory 命令，读取 thread、
  messages、events；不在请求内调用模型。不提供新 session API。
- `assistant-agent`：独立二进制，claim/renew/execute/recover run。进程可以横向扩展，数据库租约保证
  同一 run 同时只有一个 owner。
- MySQL：所有 Assistant 可见与运行状态权威；Redis：事件 channel 通知，可完全故障降级；ES：
  Assistant 历史派生索引，可 rebuild/delete。
- 普通 message RPC/库不新增 Agent 用户或数据，Gateway 并行合并两种 thread read model。

## 权威模型

### 代码组织

`app/assistant/internal/store/sql.go` 与 `app/assistant/internal/store/fake.go` 只保留构造、事务入口和共享状态；session/message/run/event、command
与 journal、source 和 outbox 分别由同包的 `sql_*` / `fake_*` 文件承接。SQL 方法仍使用
同一个事务绑定的 `exec`；内存实现仍共享原锁与 map，只保证串行，不模拟 MySQL 回滚。

`Engine.run` 只初始化本次执行并驱动 iteration。私有 `executionState` 按每次 Execute 分配，依次负责
读取运行状态、压缩、构造输入、模型请求、消费结果与工具轮；没有把可变 round 状态放入共享 Engine。
`iterationRestart` 保留 compact、redirect 和排他工具轮的重启位置。模型 streaming、工具 journal、
确认、compact 提交与终态发布分别在同包文件中实现；终态消息、来源快照和 outbox 仍使用原 step 事务。

工具 registry 按类型、catalog、policy、schema、source 和 result 分文件，仍从唯一 metadata 定义派生
广告与授权。这里只改变职责定位，不新增接口、表、配置、事件或全仓长度门禁。

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

破坏性迁移用 `assistant_runtime_v3` marker：首次执行清空并重建 `xbh_assistant`，清空 user 库 Agent
consent；marker 提交后重复 patch 不再清理。生产执行前必须绑定 MySQL `server_uuid`，分别备份并验证
Assistant 库与 consent，且提供精确确认值；补丁名和 SHA-256 写迁移 ledger。社区、普通私信和用户主体
表不在清理集合。Redis namespace 与 ES 派生索引按独立运维步骤治理，SQL patch 不虚假宣称跨存储清理。

## 接收、并发与输入处置

`POST /assistant/messages` 在一个事务中：先校验 consent 与 requestId 重放，再按 id 锁该用户全部开放
`agent_run`（只把 memory-review 标为取消），之后锁 thread、重查幂等结果并写 user message。这样输入
redirect/steer、worker 终态与后台抢占统一遵守 `agent_run -> assistant_thread` 锁序。随后按当前前台 run phase 决定 disposition，创建或更新 run；模型请求 phase 写 redirect，工具
phase 写 steer，compact/attachment/unsafe phase 写最多 32 条 FIFO。无活跃前台 run 则创建 queued
user run。数据库提交前不报告 accepted。

每用户一条永久前台 session：缺失则创建，遗留 `closed` 行 reopen，不再因用户操作关闭并另开一行。
线程 `last_message_at_ms` 距今不少于 30 分钟后，下一次新建 user run 在同一 session 上滚动
prompt epoch，重建 Safety/SOUL/工具规则/MEMORY，并保留 `compact_summary`。redirect、steer、FIFO
和崩溃恢复复用已保存快照。clear history 逻辑删除 message、写 ES
delete outbox、清 thread 可见摘要，不删 Memory。消息删除与 delete outbox 插入在同一事务内，
发件箱写入失败必须回滚删除。ES 删除仅在成功或 404 时确认完成，其他 HTTP 错误保留事件等待重试。
显式 Stop 只把 `cancel_requested` 置 1，该位
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

模型结果提交在相同 run 锁事务内再次比较本轮 `input_version`：普通文本、失败/不完整终态以及
工具意图与 `tool_executing` 阶段转换均不能越过已接受的 redirect。失配时不终止 run、不清空
active run，重置该轮已公开的 stream 并读取新输入重启。工具阶段已提交后继续沿用 steer/journal
语义，不给所有通用工具 step 增加输入版本拒绝。接受输入时以 `LockOpenRuns` 的当前锁定读决定
阶段，避免 MySQL REPEATABLE READ 下先前 consent/幂等读取留下的旧快照误判 disposition。

每次 Execute 固定 claim 时的 `(lease_owner, lease_generation)`。加载新 run 状态前先比较该 fence，
不能采用接管者的新身份；正常、取消、错误和恢复收尾都使用原 claim fence。取消监视器观察到 owner
变化只停止旧 work context，不替新 owner 修改状态。最终 step 事务仍作权威 fence 校验，覆盖读后接管。

用户 run 优先于 memory-review。claim 按 priority/created_at；前台消息可设置后台 run cancel。

## 事件与 SSE

每个事件先锁 run 分配 `seq=max+1` 并写 MySQL。SSE 始终按固定间隔查询 MySQL 的 `seq > cursor`，
不依赖 Redis 唤醒。
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
声明 WireAPI、模型、窗口、输出、流式与工具能力；启动 canary 强制调用无副作用虚拟工具。
两轮探针分别预留 256 个输出 token（包含 reasoning），受模型输出上限约束，避免 64/32 的小预算
在推理阶段耗尽；仍要求指定工具、正确 nonce 与非空确认，不能以预算不足为由放行。resilient
client 将错误分类后最多三次有界抖动退避，尊重上限内 Retry-After，并只向 capability/privacy 兼容的
route fallback。attempt 结果以内部 run event 审计，不向 SSE 暴露 provider 错误正文。

usage adapter 分离 input/output/cache-read/cache-write/reasoning，并按独立价格计算。可选
`BackgroundReview.Model` 只用于 memory-review，`LLM.AuxModel` 用于 compact；缺省使用冻结主 route。

compact 优先以上一次 provider prompt usage 为锚点，只估算后续新增消息；无 usage 时 ASCII 约四字符
一 token、非 ASCII 至少一字符一 token。达到窗口 50% 后选择最新 20% token 与所有未完成 tool/confirm；
摘要模型接收预算内的完整消息。压缩结果必须比输入小并
低于目标阈值，否则保留原消息并明确失败。事务成功后才提交摘要、新 prompt epoch/sidecar/capability
快照和 compact 标志。原 message 在 365 天保留期内通过 outbox 可检索；worker 启动及每小时执行有界
批次清理，物理删除与 ES delete outbox 同事务，旧 upsert payload 同时移除。
每次摘要输入都将旧 `compact_summary` 与本轮待压缩消息作为 JSON 历史材料送入模型，不进入 system；
旧摘要与序列化开销计入输入预算。第二次及后续 compact 不能仅摘要本轮新增消息后覆盖旧条件。
隐藏 sidecar（工具轮）只通过 `api_content` 进入 provider 历史，不写可见正文或 ES outbox。

## Memory Review

每个 session 记录连续成功未中断 user turn。达到 10 的倍数写 memory-review run，限制 16 provider round
与 600k input token，工具表只含 memory add/replace/remove/batch/read。前台 run claim/accept 时取消未完成
审查。成功 change 在 thread 插入 `kind=memory_changed, unread=false` 系统行及 undo action。
通知从已成功或成功重放的工具记录推导，统一在 run 的成功、取消、失败及资源终止事务中按 change ID
去重发布。取消剩余审查不撤销已完成变更，也不能隐藏撤销入口；没有成功 change 时不制造通知。

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

## Memory 全文身份与兼容

Memory add/replace/undo 在现有 `(user_id,target)` mutation lock 下读取当前有效全文，以 Go
`Normalize(content)` 精确比较全部字符；SQL 读取使用 `FOR UPDATE`，避免事务取得 target 锁前建立
的旧快照影响去重与容量。MEMORY/USER 总字符容量有界，因此比较整个有效集合不需要新增摘要索引。
旧 `content_norm VARCHAR(512)` 继续写入供旧版本兼容，但不再作为身份判断依据；现存行直接使用
已有 content 全文，不需迁移、补写或删除。新旧版本回滚不会改变存储格式，但旧版本仍有前缀去重
缺陷，不应混跑来宣称该缺陷已修复。MapStore 以私有副本提交整个 batch，并使用与 SQL 相同的
逐项 request id，失败不保留 entry/change，replace/undo 同样拒绝全文重复。

## 后台 run 取消收尾

Memory Review 已成功的变更即使随后取消，仍写入 `memory_changed` 撤销入口，但 run 以 cancelled
终止；未完成 tool call 及同 run、同 lease generation 的 pending journal 一并收口，已成功记录不得降级。

## 预算与观测

run counter 在每个 step 事务累计 rounds、tool calls、input/output/cache tokens、cost、elapsed/idle。warning /
critical 按时间、round、output 三个维度以唯一 `(run, level, dimension)` 记录指标和日志，并向下一模型 step
加入不可见 convergence instruction。硬上限检查在 claim、provider 前后和工具前；触顶写
`AGENT_RESOURCE_LIMIT` error，payload 包含 partial text 与完成 journal 摘要。
主模型与 compact 请求的 MaxTokens 还受 run 剩余总输出额度约束；辅助模型调用也累计 rounds/usage/cost，
即使摘要为空或无收益，已消费预算仍持久化。消费 usage 后、同轮每个工具执行前以及最终发布前再查硬限额，
结构化回答或等待问答不能绕开触顶终止。memory-review 发起请求前同时检查累计输入和本次估算预算。

Prometheus 覆盖 queue age、lease claim/recovery/renew failure、run phase/elapsed/idle、token/cost、journal hit、
confirmation、compact、BM25/outbox 和 review。

## 验证

- 纯逻辑：disposition state machine、预算、canonical digest、source handles、Memory 容量/version/undo。
- MySQL 集成：lease crash recovery、journal、confirm CAS、event replay、compact transaction。
- Redis/ES：通知故障轮询、history rebuild/delete、user isolation 与回源剔除。
- provider contract：Chat Completions 与 Responses 的非流式/流式 tool-call fixture、cache usage、错误分类、
  Retry-After、fallback、canary 与跨 chunk scrub。
- 根真实栈：授权、异步发送、断线重连、删除确认、memory-review、compact 新 epoch、history。

## 2026-09-25 历史删除与订阅失败边界

历史删除在同一事务中按 run → thread 的顺序锁定并取消所有活跃任务，关闭等待交互、清理队列和
历史派生数据，清除会话压缩摘要与 prompt snapshot 并推进 epoch，MEMORY/USER 保留。
取消终态使旧 worker 的 RunStep 失效，防止 compact 或回答写回。事务内读取 session 使用当前读，
防止并发请求的 repeatable-read 快照恢复删除前的历史。

订阅发现 run 终态后再次读取持久事件，以补齐两次查询之间提交的最终结果。网关在流尚未开始时
按公共错误映射返回 JSON 4xx/5xx；开始后发送无 id 的 transport_error，携带公共错误及 retryable。
该帧是连接结果，不是持久 run 事件，不推进恢复游标、不将 run 伪造为终止。写失败或客户端离开
取消该订阅上下文，不取消后台 run。
