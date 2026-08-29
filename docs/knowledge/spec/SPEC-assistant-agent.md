---
id: SPEC-assistant-agent
layer: spec
title: 持久异步 Assistant Agent 规范
status: approved
owner: human
upstream:
  - INT-content-community-backend
---

# 持久异步 Assistant Agent 规范

## 范围与身份

本规范统一约束消息页中的“小白盒 Agent”虚拟线程、异步运行、平台工具、历史召回、来源展示、
授权、预算和恢复语义。它接替已 retired/deprecated 的 `SPEC-grounded-assistant` 与
`SPEC-assistant-agent-mode`。Memory 与 Watch 的专门行为分别由 `SPEC-agent-memory` 和
`SPEC-agent-watch` 约束。

- `AGENT-001`：Assistant 是每个认证用户拥有的一条虚拟私信线程，不创建机器人用户，不写入普通
  私信 conversation/message 模型，也不改变普通 Message API。
- `AGENT-002`：Assistant 只有一个通用 Agent 行为，不存在 enhanced_search/agent 模式开关；
  `/api/v2/assistant/chat` 与请求 `mode` 字段硬删除，不提供兼容适配层。
- `AGENT-003`：Agent 可以直接对话，也可以根据 system prompt、SOUL、冻结的 MEMORY/USER、当前
  会话和工具 schema 自主选择工具及参数；服务端不得先用关键词或小模型对话轮做 Intent Router。
- `AGENT-004`：用户 run 以当前认证用户身份执行，只能访问该用户直接可访问的数据，只能更新或
  删除本人帖子；Watch run 只读，memory-review run 只能读写 MEMORY/USER。

## 线程、会话与消息

- `AGENT-010`：`GET /api/v2/assistant/thread` 返回虚拟线程摘要、当前 session、未读数和活跃 run；
  `GET /api/v2/assistant/messages` 只返回该用户的 Assistant 消息。
- `AGENT-011`：`POST /api/v2/assistant/messages` 先持久化用户消息与 run，再返回 `messageId`、
  `sessionId`、`runId` 和 `disposition=started|redirected|steered|queued`，不等待模型完成。
- `AGENT-012`：每用户同时最多一个前台 run。新消息在模型请求阶段可安全 redirect，在工具阶段
  steer；compact、附件处理或其它不能安全注入的阶段进入最多 32 条 FIFO，超限明确拒绝。
- `AGENT-013`：`POST /api/v2/assistant/sessions` 开始新会话并滚动 prompt epoch；不删除历史、
  MEMORY/USER 或 Watch。`DELETE /api/v2/assistant/history` 删除 Assistant 历史，不影响后三者。
- `AGENT-014`：`POST /api/v2/assistant/thread/read` 更新 Assistant 未读；主动 Watch 消息计入未读，
  `memory_changed` 系统行不计未读。
- `AGENT-015`：消息正文 `content` 与真实 provider-bound `api_content` 分离保存。`content` 仅用于
  用户可见历史；恢复 run 必须重放原始 `api_content` 字节，不能把 Memory、BM25 或 Watch 注入
  反写到可见正文。

## 异步运行与恢复

- `AGENT-020`：`assistant-rpc` 负责 API/read model；独立 `assistant-agent` worker 执行 run；
  Watch matcher 负责匹配与调度，不在请求进程同步调用模型。
- `AGENT-021`：MySQL 是 run、message、event、tool call 与 prompt 快照的权威库。worker 通过数据库
  lease queue 取任务；租约 60 秒，每 10 秒续租，崩溃或租约失效后从最后已提交 step 恢复。
- `AGENT-022`：客户端断线不取消 run。只有显式 Stop 或取消 API 才请求硬取消；用户前台 run 可
  抢占 Watch 与 memory-review，未发送 Watch 命中重新进入合并窗口。
- `AGENT-023`：SSE 事件先以单调 `seq` 写 MySQL，再尝试 Redis 通知。`Last-Event-ID` 后的事件必须
  补齐；Redis 不可用时以 MySQL 轮询降级，且不得丢失终止事件。
- `AGENT-024`：`GET /api/v2/assistant/runs/:id/events` 只允许 run 所属用户读取；持久事件类型为
  `run_started|token|tool_call|tool_result|confirm_required|source_card|memory_changed|done|error`。
- `AGENT-025`：run 只有一个终止状态。错误终止保留已经提交的部分文本、已完成副作用摘要和完整
  事件序列，不得先完成后报错或伪造成功。

## 授权、工具和副作用

- `AGENT-030`：用户必须显式授予 Agent capability consent；授权说明列出当前工具分组、数据边界、
  delete_post 逐次确认、Memory/Watch 和长任务预算。撤销后不接受新用户 run，活跃 run 安全停止。
- `AGENT-031`：用户 run 可获得授权版本覆盖的完整工具集；Watch run 只允许搜索、回源、推荐、读取
  MEMORY/USER、`search_history` 与 `present_sources`；memory-review 只允许 Memory 工具。
- `AGENT-032`：只有 `delete_post` 逐次确认。create/update、Memory 与 Watch 写仍须通过授权版本、
  schema、所有权、revision、幂等和审计校验，但不弹逐次确认。
- `AGENT-033`：工具副作用写入 command journal，以
  `(user_id, request_id, tool, canonical_args_digest)` 唯一。恢复或重复调用返回已提交结果，不能再次
  执行成功动作；参数摘要使用规范化 JSON 后计算。
- `AGENT-034`：删除确认一次性绑定 user/session/run/call/工具/规范化参数摘要/目标 revision；
  服务端用数据库 CAS 裁决。跨 run、参数变化、revision 变化、重复确认或过期确认一律无效。
- `AGENT-035`：工具输入和工具返回均是不可信数据，不得改变系统安全规则、可用工具、归属校验、
  确认或预算。平台不得提供账户 secret、验证码、普通私信、其它用户记忆或未发布内容工具。

## 模型传输与 Prompt

- `AGENT-040`：同时支持 Responses 与 Chat Completions。配置选择的协议必须实际支持工具调用；
  不支持时启动/readiness 失败，不能静默降级成无工具模型。
- `AGENT-041`：Prompt 顺序固定为：不可覆盖的平台安全规则 → 仓库版本化 `SOUL.md` → Agent/tool
  规则 → 冻结 MEMORY/USER → 当前会话历史。
- `AGENT-042`：`SOUL.md` 为 human-owned 仓库资产，用户不能编辑；默认人格为温暖伙伴。新 SOUL
  仅在新 session、冷启动或 compact 成功提交的新 prompt epoch 生效。
- `AGENT-043`：system prompt 在新 session 或无快照冷启动时构建并按字节保存；恢复未 compact
  session 必须复用原始快照，不受仓库、Memory 或工具表随后变化影响。

## Compact 与后台审查

- `AGENT-050`：估算上下文达到模型窗口 50% 时 compact；保留最近 20% token、未完成工具调用与确认，
  并加入压缩摘要。compact 成功提交后滚动 prompt epoch，重新加载 SOUL、MEMORY/USER 与工具快照。
- `AGENT-051`：compact 前原始消息不删除，继续保留一年并可被 `search_history` 召回；摘要不能覆盖
  权威消息，也不能把不可信历史提升为系统规则。
- `AGENT-052`：每 10 个成功且未中断的用户回合调度 memory-review run；最多 16 轮、累计输入最多
  600,000 token，仅允许 Memory 工具，不写主会话。新前台消息优先取消未完成的审查。
- `AGENT-053`：后台审查成功写入后增加不计未读的 `memory_changed` 系统行并提供撤销；审查失败不
  改判主会话成功状态。

## 历史 BM25

- `AGENT-060`：Elasticsearch 只建立 Assistant 历史的 CJK/BM25 派生索引，按 `userId` 强隔离；
  MySQL 始终权威。所有命中返回前回源校验 user、删除状态和 365 天保留期。
- `AGENT-061`：`search_history` 支持关键词发现、围绕锚点滚动、读取会话和浏览最近会话四种形态；
  默认 Top 3、最大 10。首结果包含锚点前后各 5 条及会话首尾摘要，低排名只含锚点。
- `AGENT-062`：默认只搜索 user/assistant 消息，排除 tool、memory-review 与当前 live context；允许
  已结束会话和 compact 掉的消息。普通用户之间私信永远不在索引或结果范围内。
- `AGENT-063`：索引通过 MySQL outbox 派生，支持 rebuild 与删除传播；ES 故障使 `search_history`
  工具明确失败，不影响 MySQL 中线程、消息和 run 的权威读写。

## 来源 Ledger

- `AGENT-070`：搜索、推荐和 web 工具把经服务端验证的结果登记为本 run source ledger，并只向模型
  返回不可伪造、绑定 run 的 source handle。handle 不能跨 run 使用。
- `AGENT-071`：`present_sources` 接受本 run 至多 10 个有效 handle，生成 `source_card` 结构化事件；
  Agent 自主选择是否调用。没有来源卡不影响普通回答成功。
- `AGENT-072`：服务端不得从模型正文解析帖子 ID、链接、引用或来源。客户端只信任 `source_card`；
  过期、越权、删除或 revision 不匹配的 source 在展示前重新校验并剔除或标记不可用。

## 资源预算与观测

- `AGENT-080`：每 run 硬上限为 500 模型轮、1,000 工具调用、30 分钟无活动、6 小时绝对时长、
  1,000,000 总生成 token；单次输出上限为 `min(provider_limit, 65,536)`。
- `AGENT-081`：warning 阈值为 5 分钟、30 轮或 100k 输出 token；critical 为 20 分钟、100 轮或
  500k 输出 token。每 run 每级每维只告警一次，仅进入 metrics、日志、Prometheus 和发给模型的
  不可见收敛提示，不向用户消息写预算文案。
- `AGENT-082`：真正触顶以 `AGENT_RESOURCE_LIMIT` 终止，并保留部分文本与已完成副作用摘要。
- `AGENT-083`：观测 run elapsed/idle、queue age、rounds、tool calls、input/output/cache tokens、cost、
  lease recovery、compact、BM25、Watch、memory-review 与 Redis 通知降级，且不得记录消息正文、
  prompt、secret 或普通私信。

## SLO 与验收

- `AGENT-090`：接收 p95 500ms、首持久事件 p95 2s、活跃心跳间隔不超过 30s；普通完成 p95 45s
  仅作观测目标，长任务不设统一完成 SLO；Watch 命中到主动私信 p95 5 分钟。
- `AGENT-A01`：覆盖 lease/recovery、一个前台 run、redirect/steer/FIFO、显式 Stop、断线续流、
  Redis 降级和终止唯一性。
- `AGENT-A02`：覆盖 command journal、删除确认 CAS、版本/参数变化、权限与工具分组。
- `AGENT-A03`：覆盖 50% compact、prompt 字节稳定、20% 保留、新 epoch 加载 SOUL/Memory、后台审查
  隔离和撤销。
- `AGENT-A04`：覆盖 BM25 用户隔离、四种历史调用、365 天回源、rebuild/delete 与私信永不入索引。
- `AGENT-A05`：覆盖 source handle run 绑定、`present_sources`、正文伪造来源无效与可选来源回答。
- `AGENT-A06`：覆盖两种 LLM transport 的工具调用，以及硬预算触顶与每维每级只告警一次。
