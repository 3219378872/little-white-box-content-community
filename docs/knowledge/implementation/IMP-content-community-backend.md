---
id: IMP-content-community-backend
layer: implementation
title: 小白盒内容社区后端实现映射
status: diverged
owner: agent
upstream:
  - DES-content-community-backend
tracks:
  - app/content/rpc/internal/logic
  - app/content/rpc/internal/model
  - app/interaction/rpc/internal/logic
  - app/user/rpc/internal/logic
  - app/message/rpc/internal/logic
  - app/media/rpc/internal/logic
  - app/recommend/rpc/internal/logic
  - app/recommend/mq/internal/store
  - app/assistant/rpc/internal/logic
  - app/assistant/internal/store
  - app/assistant/internal/retention
  - app/assistant/internal/runtime
  - app/assistant/internal/tool
  - app/assistant/internal/memory
  - app/assistant/worker
  - app/assistant/watch
  - app/assistant/mq
  - app/behavior/rpc/internal/logic
  - app/search/rpc/internal/logic
  - app/feed/rpc/internal/logic
  - app/content/mq/cleanup
  - app/gateway
  - app/gateway/internal/logic/assistant
  - pkg/visibilityx
  - app/content/visibility
  - proto/content/content.proto
  - proto/message/message.proto
  - proto/media/media.proto
  - proto/user/user.proto
  - proto/assistant/assistant.proto
  - proto/interaction/interaction.proto
  - proto/behavior/behavior.proto
  - proto/search/search.proto
  - deploy/sql/xbh_content.sql
  - deploy/sql/xbh_user.sql
  - deploy/sql/xbh_media.sql
  - deploy/sql/xbh_analytics.sql
  - deploy/sql/xbh_assistant.sql
  - deploy/sql/patches/20260827_assistant_runtime.sql
  - deploy/sql/patches/20260827_agent_consent_version.sql
  - deploy/sql/patches/20260830_assistant_message_change_id.sql
  - deploy/sql/patches/20260830_watch_bucket_not_before.sql
  - deploy/sql/patches/20260830_agent_provider_reliability.sql
  - deploy/sql/patches/20260831_assistant_retention_indexes.sql
  - deploy/sql/patches/20260831_watch_task_version.sql
  - deploy/loki/loki-config.yaml
  - deploy/docker-compose.middleware.yml
  - deploy/docker-compose.production.yml
  - deploy/nginx/nginx.conf
  - scripts/apply_production_sql_patches.sh
verified_at: 2026-08-30
verified_commit: 1992b06c955a812f25b0cad8ec096ca1a883f564
---

# 小白盒内容社区后端实现映射

本页记录已批准规范到代码实现的映射与逐条状态。`SPEC-grounded-assistant` 与
`SPEC-assistant-agent-mode` 已由 Hermes 规范退休，其历史 ASST/AGNT 条目不再代表当前产品约束。
持久异步 worker、虚拟线程和相关存储已落地，但 2026-08-30 复查证实部分早期 `aligned` 结论超前；
本页按当前源码、契约、SQL 和实际测试重新区分 `aligned` 与 `partial`。
设计见 [DES-content-community-backend](../design/DES-content-community-backend.md)
与 [DES-assistant-agent-runtime](../design/DES-assistant-agent-runtime.md)；
源码、`.api`、`.proto`、SQL 与测试高于本页。

## 总体状态

`diverged`：Hermes 异步 Agent runtime 已落地（RPC 不调模型、worker lease、自然语言 Memory、
Watch 内部 bucket）。仍偏离处：
- `CORE-032` 公开计数 30s 收敛缺少生产观测。
- `DISC-060` 人类冻结集未关闭；`DISC-063` 无学习模型、相对提升 0。
- `REL-033`/`REL-040`~`043`/`REL-A05` 缺少真实月度观测。
- `REL-A03` 未注入 `REL-054` 全部十行。
- `discussion_spike` 生产 matcher 未接 LLM 判定（过阈值且无模型记 failed，不写规则命中）。
- AGENT-A01~A09 / MEM-A04~A06 / WCH-A02 缺少外部 live LLM 与生产真实流量；确定性根栈验收另见本轮证据。

## 规格追踪

下表是实现层台账。`aligned` 表示当前代码与测试覆盖该条；`partial`/`n/a` 不得写成设计已完成。
人类未关闭的评测集、月度 SLO 禁止标 `aligned`。

## SPEC-community-core 追踪

| 要求 | 状态 | 实现位置与偏离说明 |
| --- | --- | --- |
| CORE-001 写操作验证调用者 | aligned | 所有写路由挂自有 `RequiredAuth`；logic 从强类型 JWT context 取 userId；内部 gRPC unary/stream 均安装同一 HMAC 边界，stream 不再绕过 unary interceptor |
| CORE-002 只能改自己内容 | aligned | update/delete 校验 author_id |
| CORE-003 会话参与者才可读私信 | aligned | GetMessages/MarkRead 按 user_id 归属校验 |
| CORE-004 注册登录资料维护 | aligned | user rpc；注销/申诉/后台不在范围 |
| CORE-005 一致的可见性结果 | aligned | GetPost/公开列表/标签/发现统一只暴露 published；列表回源后再滤；公开详情与评论列表对匿名开放 |
| CORE-010 状态机 | aligned | draft⇄published 双向 + 均→deleted 终态；Update 显式 status 支持取消发布 |
| CORE-011 创建返回 id/status/revision | aligned | CreatePostResp 返回 postId/status/revision=1 |
| CORE-012 草稿仅作者可读 | aligned | GetPost 作者可读草稿，非作者统一 404 |
| CORE-013 变更携带预期 revision | aligned | /api/v2/post 是唯一写路径，强制 expected_revision（缺失/0 → 400，冲突 → 409）；人类 2026-08-13 决定“直接废弃并迁移”，/api/v1 帖子写接口已移除（PROP-20260813-core-revision-contract 选项 B + 废弃决定） |
| CORE-014 变更后读取返回新状态/revision | aligned | 事务内写+outbox；Update 返回 status/revision；HTTP GetPost 与公开列表 PostItem 回传权威 status 与 revision |
| CORE-015 取消发布/删除不再出现 | aligned | 条目回源后只保留 published；`Total` 按本页回减（2026-08-15 SPEC 允许估计）；post 事件带 revision 且 outbox key=post_id，search/recommend 丢弃更旧快照 |
| CORE-016 匿名/非作者统一不存在 | aligned | 草稿/删除/非公开对非作者统一 404；已发布详情/评论/评论回复允许匿名读取；评论列表与楼中楼回复列表 SQL 过滤 status=1 后再内存二次过滤并回减 Total（纵深防御；回复列表另要求父评论可见，2026-08-22） |
| CORE-020 标题/正文边界 | aligned | 1~120/1~20000 Unicode 校验 |
| CORE-021 图片≤9 标签≤10、标签 1~32 | aligned | 数量与长度校验 |
| CORE-022 评论 1~2000 且只能附着已发布内容 | aligned | 上限校验 + 仅 published 可评论 |
| CORE-023 图片格式/10MiB/8192px/25MP | aligned | Handler 区分 MaxBytes 与非法 multipart；`DecodeConfig` 在完整解码前校验单边与像素预算；容量 2 semaphore 覆盖解码/缩放/编码，production media-rpc 为 512 MiB |
| CORE-024 媒体引用校验 | aligned | 帖子引用媒体 ID 时校验存在/归属/完成态；上传返回稳定 id |
| CORE-030 互动幂等 | aligned | Like/Unlike/Favorite/Follow 重复请求返回成功且不重复累计；命令层 no-op 不写 outbox |
| CORE-031 单一有效关系 | aligned | 唯一键 + 状态字段 |
| CORE-032 互动状态立即可查 | partial | 详情与列表经 Gateway viewerstate 回填；计数走 count-sync（独立消费组）；broker `JAVA_OPT_EXT` 含 `-XX:-UseContainerSupport`，避免 cgroup v2 上 StoreUtil 初始化失败导致 Pull 全挂；30s 收敛未经生产观测 |
| CORE-033 取消互动后查询无效 | aligned | Unlike/Unfavorite 置 inactive 并失效缓存 |
| CORE-034 赞藏仅已发布 | aligned | Interaction 写前调 Content `AssertInteractable`；帖子须 published，评论须有效且父帖 published；权威不可用失败关闭 |
| CORE-040 一对一私信能力 | aligned | 仅 text/image/video/audio（type 1-4），无群聊/撤回/删除 |
| CORE-041 消息正文/媒体消息 | aligned | 文本 1~1000；媒体消息必须引用本人已完成媒体并持久化 media_id |
| CORE-042 消息幂等键 | aligned | idempotency_key ≤128、同键同命令返回原 id、异命令（含不同 media_id）冲突 |
| CORE-043 标记已读仅影响自己 | aligned | MarkRead 只改 receiver==自己 的行 |
| CORE-044 私信成功不依赖通知 | aligned | SendMessage 以库提交为成功；无赞/评/关通知生产者 |
| CORE-050 创建帖子/评论/媒体幂等键 | aligned | 帖子/评论/媒体均实现幂等表，同键同命令返回原资源、异命令 409；媒体命令哈希含接收文件内容 sha256 指纹，幂等命中会补偿删除本次随机对象；评论命令哈希含回复目标评论与被回复用户（CORE-051 异命令冲突，2026-08-14） |
| CORE-051 可区分业务结果 | aligned | 版本冲突/幂等冲突 409 与业务码；网关透传 BizError；HTTPStatus 为唯一映射（密码错误 401、验证码错误/过期 400、空搜索 400、搜索超时 504，2026-08-14 补齐） |
| CORE-052 权威写入未确认不返回成功 | aligned | 事务+outbox 同事务；media 软删事件与权威行同事务，上传后落库/缩略图失败立即补偿，删除失败写 `upload_compensation` outbox 交现有消费者重试 |
| CORE-053 异步效果失败不改成功 | aligned | 互动/评论/帖子缓存失效失败只告警不改变已提交成功的响应 |
| CORE-054 不泄露内部信息 | aligned | FromHTTPError 未知错误走 SystemError 通用文案；解析失败不回传底层字符串 |
| CORE-060 单页内不重复 | aligned | 页式列表由 SQL 分页保证 |
| CORE-061 游标链约束 | aligned | 见 DISC-003/033 |
| CORE-062 /api/v1 读契约与 v2 写路径 | aligned | SPEC 已记录移除 v1 帖子写接口；其余 v1 读/互动与 `/api/v2/messages*` 兼容 |
| CORE-063 不依赖内部结构 | aligned | 契约层不暴露内部结构 |

## SPEC-content-discovery 追踪

| 要求 | 状态 | 实现位置与偏离说明 |
| --- | --- | --- |
| DISC-001 只返回可见已发布内容 | aligned | 条目经 Content 回源过滤；`Total` 按本页回减（SPEC 允许估计） |
| DISC-002 稳定内容标识 | aligned | post_id 稳定 |
| DISC-003 游标不重复、绑定上下文 | aligned | recommend 游标 HMAC+绑定；feed 游标按创建时间+id |
| DISC-004 未曝光不得解释为负反馈 | aligned | 负反馈只来自 hide/dislike 等明确动作 |
| DISC-010 关注流仅认证用户 | aligned | `/api/v2/feed/follow` 挂 jwt |
| DISC-011 关系变化按当前关系生成 | aligned | 关注流先分页拉取当前 following，inbox 按当前关系过滤，outbox 按作者分批回源 |
| DISC-012 空关注流返回空 | aligned | 不混入推荐 |
| DISC-020 搜索覆盖帖子/用户/标签 | aligned | Search RPC 综合搜索；帖子结果带 author_id/author_name/author_avatar（User 回填，失败时仍保留 author_id） |
| DISC-021 帖子结果来自可访问已发布内容 | aligned | 查询时回源 Content，标题/摘要/作者 ID 来自仍 published 的正文，不可见项丢弃 |
| DISC-022 无匹配空结果、索引不可用 503 | aligned | 搜索 RPC 区分空与不可用 |
| DISC-023 部分失败返回 degraded+unavailableTypes | aligned | 用户/标签搜索失败时降级并列出 unavailableTypes |
| DISC-030 认证用户个性化 | aligned | recommend 按身份特征召回 |
| DISC-031 匿名冷启动、不建立画像 | aligned | 匿名事件不写入推荐特征；匿名推荐只使用规则冷启动来源，不建立/不合并画像 |
| DISC-032 推荐响应含请求/位置/来源/版本/实验 | aligned | RecommendPost 字段齐全 |
| DISC-033 游标绑定+10 分钟有效期 | aligned | cursor codec HMAC + 600s TTL |
| DISC-034 作者配额 | aligned | ≥10 个作者时滑窗硬性保证任意 20 条内同一作者 ≤2 |
| DISC-035 负反馈 30 天/曝光 7 天 | aligned | 负反馈 30 天 TTL；曝光按 7 天窗口排除，候选不足时重入并标记原因 |
| DISC-036 个性化不可用规则降级 | aligned | recall/feature/inference 降级并标记 |
| DISC-040 无效页大小/游标/身份拒绝 | aligned | 参数校验 + 游标校验 |
| DISC-041 部分召回失败只返回验证结果 | aligned | enrichAndFilter 只留可见候选，不可验证则整体失败 |
| DISC-042 发现失败不影响写入/关系 | aligned | 独立服务 |
| DISC-050 /api/v2/feed/*、/search* 兼容 | aligned | 无破坏性变更 |
| DISC-051 分值/来源/版本语义稳定 | aligned | 字段语义与行为事件关联未变 |
| DISC-052 客户端不依赖固定排序 | aligned | 契约不承诺排序 |
| DISC-060 搜索人类冻结集 | partial | 现有 qrels 为 LLM 双评审合成集；SPEC 要求两名人类评审，不能关闭 |
| DISC-061 分能力评估 | aligned | 搜索/推荐/助手使用独立冻结文件与指标 |
| DISC-062 学习模型门槛 | aligned | 未达 1 万曝光/1 千身份，不宣称学习改善 |
| DISC-063 推荐相对提升 | partial | 规则基线相对提升 0，如实未达标 |

## SPEC-grounded-assistant 追踪

`SPEC-grounded-assistant` 已退休，由 `SPEC-assistant-agent` 接替。本节仅保留 heading 供知识迁移
完整性检查定位；旧 ASST 条款不得作为当前能力验收结论。

## SPEC-assistant-agent 追踪

已 retired 的 `SPEC-grounded-assistant` / `SPEC-assistant-agent-mode` 不再作为当前约束。
下列 AGENT 条款对照 `SPEC-assistant-agent` 与 `DES-assistant-agent-runtime`。

| 要求 | 状态 | 实现位置与偏离说明 |
| --- | --- | --- |
| AGENT-001 虚拟私信线程 | aligned | `assistant_thread`；不写普通 message 模型 |
| AGENT-002 无 mode 开关 | aligned | `/assistant/chat` 与 `mode` 已删除；`POST /assistant/messages` |
| AGENT-003 无 Intent Router | aligned | worker 直接把工具 schema 交给模型 |
| AGENT-004 身份边界 | aligned | user run 当前用户；Watch 只读；memory-review 仅 Memory 工具 |
| AGENT-010 线程/消息 API | aligned | `GetThread`；`ListMessages` 默认最新一页，`beforeId` 向前翻页，`afterId` 增量读取，两种游标互斥 |
| AGENT-011 异步 PostMessage | aligned | 事务写消息与 run，返回 disposition；`assistant_input_command` 使 started/redirected/steered/queued 接受结果按 requestId 原样重放 |
| AGENT-012 单前台 run + redirect/steer/FIFO32 | aligned | `input_version` 使 redirect 取消在途模型、丢弃旧响应并重跑；FIFO 按已读取最大 id 消费，不误删并发新输入或永久占满；`redirect_consent_test.go`、`accept_test.go` |
| AGENT-013 永久 session/冷拼接/清历史 | aligned | `splice.go` 30min 可见空闲后新建 run 滚 epoch；DeleteHistory 逻辑删消息；`POST /sessions` 已删除 |
| AGENT-014 未读 | aligned | Watch 成功事务增加未读；`memory_changed` 系统行 `unread=false` |
| AGENT-015 content/api_content 分离 | aligned | 可见正文不变；附件与 `contextPostId` 写入 provider-bound `api_content`/queued payload；MEMORY/USER 与 compact summary 进入独立不可信 user sidecar；Watch 命中写入隐藏 `watch_input` sidecar；恢复按原始 `api_content`/snapshot 字节重放 |
| AGENT-020 rpc 不调模型 | aligned | worker `app/assistant/worker` |
| AGENT-021 lease 60s/10s | aligned | 每进程唯一 owner；claim 递增 `lease_generation`；`RunStep` 对 owner/generation/expiry 加锁校验；续租失效立即 cancel Engine；`fencing_test.go`、`lease_test.go`、SQL integration |
| AGENT-022 断线不取消；用户抢占后台 | aligned | Subscribe 不写 cancel；PostMessage 取消 watch/review；Stop、撤权或 lease 丢失取消 work ctx，`cancel_requested` sticky（`loop_cancel_test.go`、`redirect_consent_test.go`） |
| AGENT-023 SSE MySQL+Redis 降级轮询 | aligned | MySQL replay + Redis/轮询；HTTP 取 `max(afterSeq, Last-Event-ID)`、输出 `id:`，每 25s comment heartbeat |
| AGENT-024 事件类型白名单 | aligned | 公开事件含 `response_reset`；内部 `provider_attempt` 只持久审计并由 `Subscribe` 过滤 |
| AGENT-025 唯一终止 | aligned | incomplete 以 reason + partial 写 error；final message/outbox/thread/Watch bucket+stat/run/terminal event 单事务；`terminal_run_id` 唯一索引；SQL failure integration 证明失败全回滚 |
| AGENT-026 stream writer fencing | aligned | `stream_writer.go` 以 run/lease/input/model-round/attempt 生成 `streamId`，首 delta 即提交、后续 200ms/2KiB 批量；retry、redirect 与 lease recovery 在旧流已有公开 token 时先写 `response_reset`；仅跟随 `present_sources` 的已流式正文不 reset，并作为可见 assistant 消息落库；SQL replay 与根 fixture 覆盖 |
| AGENT-030 consent | aligned | 接收事务冻结 `consent_version`；worker 持续复核当前授权；撤权取消所有开放 run，Watch 调度在共享锁事务内复核并 defer |
| AGENT-031 工具分组 | aligned | `tool.ForSource` |
| AGENT-032 仅 delete_post 确认 | aligned | HighRisk + Confirm CAS |
| AGENT-033 command journal | aligned | 副作用前 reserve pending；journal 绑定 lease generation；接管后以稳定下游幂等键恢复；Content update/delete 与 outbox 原子，Memory/Watch 重放可返回已提交结果；崩溃窗口测试验证副作用仅一次 |
| AGENT-034 确认 CAS | aligned | update/delete 省略 revision 时先回源冻结再算 digest；delete confirmation 绑定真实 target revision，批准后执行前复核；`confirmation_revision_test.go`、`revision_prepare_test.go` |
| AGENT-036 工具 metadata | aligned | `tool.Definition.Metadata` 统一派生 effect/source/consent/confirmation/availability/idempotency/output limit/poller；新 epoch 排除依赖不可用工具，恢复只取冻结定义且执行时继续按当前策略收紧 |
| AGENT-037 严格 schema/no-progress | aligned | `decodeStrictValue` + `validateSchemaValue` 拒绝未知字段、缺 required、类型/enum 错误与尾随 JSON；canonical 保留 `json.Number`；user/Watch/review 共用相同调用摘要 guard，第三次无进展以 `TOOL_NO_PROGRESS` 终止 |
| AGENT-040 双 WireAPI/canary | aligned | Chat Completions 与 Responses 均支持非流式/流式 tool call；启动 canary 强制无副作用工具并重放 tool result，所有启用 route 失败即 readiness false |
| AGENT-041 prompt 顺序 | aligned | system 仅含平台安全/SOUL/tool rules；MEMORY/USER 与 compact summary 为结构化不可信 user sidecar，标签经 JSON HTML 转义；输出在非流式与跨 chunk 流式路径 scrub |
| AGENT-043 快照复用 | aligned | session 冻结 system/sidecar/tool/provider capability；redirect/恢复不重写，新 fallback 或工具扩张不进入旧 epoch；compact 成功才滚动 |
| AGENT-044 typed retry/fallback | aligned | provider error 分类、最多三次有界抖动、上限内 Retry-After；fallback 仅选择同 boundary 且工具/流式/窗口/输出兼容的冻结 route；生产环境可显式启用一个默认关闭 fallback |
| AGENT-045 usage 规范化 | aligned | input/output/cache-read/cache-write/reasoning 分桶与独立成本；`agent_run` 持久化 cache write/reasoning/last prompt/usage estimated，缺 usage 明确置估算标志 |
| AGENT-050 compact 50%/keep 20% | aligned | 优先以 `last_prompt_tokens` 锚定增量；无 usage 时 CJK 按至少 1 字符/token；摘要按总预算选完整消息，强制保留 unmatched tool call/确认/Watch sidecar，无收益或仍超目标时拒绝提交 |
| AGENT-052/053 memory-review | aligned | 每 10 回合调度与 review 预算；成功 change 写结构化 `memory_changed(changeId)`，不计未读并复用 undo CAS |
| AGENT-054 辅助模型 | aligned | compact 可用 `LLM.AuxModel`、review 可用 `BackgroundReview.Model`，未配置时回退冻结主 route；辅助 client 继承 typed retry/usage 且不改前台 capability snapshot |
| AGENT-060/061/063 search_history | partial | user/assistant 可见消息同事务写 ES outbox，读取按 user/MySQL/365 天回源；到期物理删除与 delete outbox 同事务；四种 shape 的完整上下文与 live rebuild 尚未集成验证 |
| AGENT-070/071/072 source ledger | aligned | `app/assistant/internal/tool/sources_test.go`；`present_sources` 展示前对 post published/revision 和 web URL 重新回源，失效项剔除 |
| AGENT-080/081/082 预算 | aligned | `budget.go`；`budget_test.go` |
| AGENT-090 心跳/SLO | partial | HTTP SSE 25s comment heartbeat 与 cursor 单测通过；生产 p95/长连接观测未执行 |
| AGENT-A01~A09 验收 | partial | 单测/race/SQL integration 与确定性 provider/root fixture 覆盖 capability snapshot、canary、双 WireAPI stream、typed retry/fallback、cache usage、strict schema、no-progress、sidecar scrub、attempt reset/replay 与既有安全矩阵；仍无外部 live LLM、生产 profile/迁移和生产流量 |

## SPEC-agent-memory 追踪

| 要求 | 状态 | 实现位置与偏离说明 |
| --- | --- | --- |
| MEM-001 自然语言条目 | aligned | `core_memory_entry` target=memory\|user |
| MEM-002 容量 2200/1375 | aligned | `memory.store_test.go` |
| MEM-003 用户隔离 | aligned | SQL/MapStore 按 user_id |
| MEM-004 威胁扫描 | aligned | safety.Filter + 指令扫描 |
| MEM-010 add/replace/remove/batch | aligned | RPC + 工具共用 `internal/memory`；非匿名 request id 可重放已提交 replace/remove/batch，避免 journal 接管后再次变更 |
| MEM-011 version CAS | aligned | replace/remove 期望 version |
| MEM-012 规范化去重 | aligned | 同 target 规范化内容返回已有条目 |
| MEM-013 undo CAS | aligned | `memory_change` result_version |
| MEM-020/021 快照冻结 | aligned | MEMORY/USER 仅进入独立不可信 user sidecar；普通写不热更新，未 compact 恢复复用保存字节 |
| MEM-022/023 memory-review | aligned | 10 回合调度与预算；前台抢占；结构化 change IDs 生成 `memory_changed` 系统行，unread=false，可调用既有 undo API |
| MEM-025 sidecar 安全边界 | aligned | Go JSON HTML 转义阻止闭合标签注入；非流式与跨 chunk 流式 scrub 覆盖标签/平台 notice；MySQL 仍是唯一 Memory provider |
| MEM-030 API 字段 | aligned | List/Add/Replace/Remove/Batch/Undo |
| MEM-032 存储不可用失败 | aligned | store=nil → 503 |
| MEM-033 不能当 source card | aligned | Memory 工具不写 source ledger |

## SPEC-agent-watch 追踪

| 要求 | 状态 | 实现位置与偏离说明 |
| --- | --- | --- |
| WCH-001 任务 CRUD | aligned | watch_task + REST/工具；update/delete 必须携带 `expectedVersion`，SQL CAS 成功递增版本，冲突为 409 |
| WCH-002 规则条件 | aligned | 四种规则 + discussion_spike 预筛选 |
| WCH-003 不可见不命中 | aligned | matcher 回源 published |
| WCH-004 内部 hit 90 天 | aligned | `watch_hit` 非用户收件箱；worker 启动及每小时按索引小批物理删除过期 hit/execution |
| WCH-010 两分钟合并与限额 | aligned | 两分钟 bucket；独立 `not_before_ms` 延迟且不撞 merge unique key；小时/日计数仅在成功投递事务增加 |
| WCH-011 只读工具表 | aligned | `tool.WatchTools` |
| WCH-012 用户抢占/失败重排 | aligned | PostMessage 抢占、error/cancel finish 与 scheduler reconciliation 均把绑定 run 的未发送 bucket 重置 pending |
| WCH-013 成功写 assistant 消息+未读 | aligned | validated hit context 进入模型；token/message/outbox/unread/bucket sent/rate stats/done 在同一事务提交 |
| WCH-023 consent 撤销 | aligned | scheduler 在调度事务内复核 frozen/current consent；撤权取消已调度和活跃 run，未发送 bucket 退回 pending |
| WCH-020 删除 hits API | aligned | proto/gateway 已无 ListHits/MarkRead |
| WCH-021 任务不走 delete_post 确认 | aligned | Watch 工具非 HighRisk |
| WCH-A01~A05 | partial | 规则匹配与内部 RecordHit 单测保留；主动消息与限额依赖 worker |

## SPEC-feedback-reliability 追踪

| 要求 | 状态 | 实现位置与偏离说明 |
| --- | --- | --- |
| REL-001 客户端动作白名单 | aligned | 客户端仅可提交 exposure/click/dwell/view/play/share/hide/dislike |
| REL-002 事件 id ≤128 全局唯一、批 ≤100 | aligned | client_event_id ≤128、批量上限配置 |
| REL-003 事件关联身份、位置从 1 开始 | aligned | exposure 位置必须 ≥1 |
| REL-004 曝光定义与去重 | aligned | 视口/1s 由客户端判定；服务端强制 (requestId, postId) 去重 |
| REL-005 停留时长非负、未曝光不作负反馈 | aligned | duration 校验 + 负反馈来源 |
| REL-006 补报 30 天/超前 5 分钟 | aligned | MaxPastAgeHours/MaxFutureSkewSeconds |
| REL-007 批量逐项接受/拒绝 | aligned | RecordEvents 逐项结果 |
| REL-008 90 天去重 | aligned | 行为去重 TTL 7776000s=90 天 |
| REL-010 事件关联字段 | aligned | BehaviorEvent 携带请求/身份/位置/来源/版本/实验 |
| REL-011 拒绝/未去重不入特征 | aligned | 消费端去重后写特征 |
| REL-012 接受只表示进入消息边界 | aligned | 接口即发布 |
| REL-013 异步可观察 | aligned | 所有 MQ 消费者均有 outcome 计数与延迟直方图；outbox 积压/最长年龄指标 |
| REL-020 保留期限自动删除 | aligned | 原始行为 90 天、特征 30 天、去重 90 天、死信 7 天由 TTL/DDL 落地；Assistant 原始 message 365 天、Watch hit/execution 90 天由 worker 每小时有界批次物理删除，message 删除与 ES delete outbox 同事务；`daily_aggregates` 去标识聚合表 TTL 365 天 |
| REL-021 完整 IP 不入行为表 | aligned | 行为表不存完整 IP；访问日志 7 天 |
| REL-022 业务日志 30 天不泄密 | aligned | Gateway 关闭会 dump header/body 的框架 REST Log，RequiredAuth 不记录 token，SafeAccessLog 仅方法/路径/状态/耗时；Gateway/Assistant/Watch RPC 客户端关闭默认 request dump，改用 SafeDuration；Loki 30 天 |
| REL-023 关闭个性化 24h 删除特征 | aligned | 关闭接口与特征清理已落地；DB 权威 + Redis 快速标记；recommend-mq 新增定时主动清理（PurgeOptedOutFeatures，默认 1h 周期），不依赖用户后续行为事件；偏好读取失败 fail-closed 只走规则冷启动；单测覆盖清理脚本与错误路径 |
| REL-024 关闭前事件 90 天、不合并匿名 | aligned | 原始事件 TTL 90 天、死信 7 天；匿名身份哈希不合并 |
| REL-030 SLO 分母口径 | partial | 口径在 spec_evals.py；缺真实月度数据 |
| REL-031 降级计可用 | partial | 口径已实现；缺真实月度数据 |
| REL-032 延迟口径 | partial | 口径已实现；缺真实月度数据 |
| REL-033 月度目标表 | partial | 目标已编号；合成干跑只验证管线 |
| REL-040 outbox p95 30s/p99 5m | partial | delivery_latency_seconds 直方图已落地；真实月度观测待收集（合成干跑已验证报告管线） |
| REL-041 行为到特征 p95 60s/p99 5m | n/a | 需要真实观测数据（指标已埋点） |
| REL-042 内容到搜索 p95 30s/p99 2m | n/a | 需要真实观测数据（指标已埋点） |
| REL-043 RPO 0/RTO 30m | n/a | 运维目标 |
| REL-044 有界恢复不重试风暴 | aligned | relay 退避与租约 |
| REL-050 请求可观测 | aligned | metrics + 错误类别 + 降级标记 |
| REL-051 异步可观测 | aligned | MQ outcome/延迟与 outbox 积压 |
| REL-052 追踪标识 | aligned | trace_id 经 gRPC metadata 透传 |
| REL-053 存活与就绪 | aligned | /health 与 /health/ready；可选发现故障 degraded |
| REL-054 降级矩阵 | partial | 代码覆盖库/缓存/relay/可见性/搜索/LLM/状态存储；REL-A03 未逐行注入十类故障 |
| REL-060 行为契约带版本 | aligned | schema_version=2 |
| REL-061 语义保持 | aligned | 契约稳定 |

生产迁移由 `production-migration-backup`、`production-migrate`、`production-migration-check` 三个显式
阶段承载。破坏性 Assistant v3 重置的确认值只有在独立备份命令生成并验证两个 gzip dump 后才输出，
且绑定 manifest SHA-256；apply 会重新验证 target server UUID、patch checksum、文件 checksum 与内容
标记。`production-up` 只执行 check，不自动应用补丁。

## 验收标准追踪

原四份规范共 22 条验收标准（CORE-A01~A07、DISC/ASST/REL-A*），外加 AGNT-A01~A08、
MEM-A01~A05、WCH-A01~A05。代码行为类以 Go 测试落地，离线评测/观测类由
`scripts/spec_evals.py` 与冻结数据集承担（后者待人类输入，见
[IMP-todo-blocked-gates](IMP-todo-blocked-gates.md)）。

| 验收标准 | 状态 | 测试/证据位置 |
| --- | --- | --- |
| CORE-A01 状态机与 revision 冲突 | aligned | `app/content/rpc/internal/logic/post_logic_test.go`、`idempotency_revision_integration_test.go`、`post_integration_test.go` |
| CORE-A02 匿名读取/草稿删除不可见 | aligned | `app/gateway/rest_decision_table_test.go`（POST-LIST-ANON / POST-GET-ANON / COMMENT-LIST-ANON）、`app/content/rpc/internal/logic/post_logic_test.go` |
| CORE-A03 输入边界 | aligned | `app/content/rpc/internal/logic/post_logic_test.go`、`comment_logic_test.go`、`app/media/rpc/internal/mediautil` |
| CORE-A04 幂等收敛 | aligned | `app/content/rpc/internal/logic/idempotency_revision_integration_test.go`、`app/interaction/rpc/internal/logic/interaction_integration_test.go`、`app/message/rpc/internal/model/message_command_model_integration_test.go` |
| CORE-A05 会话参与者权限 | aligned | `app/message/rpc/internal/logic/message_logic_test.go` |
| CORE-A06 权威写与异步失败 | aligned | `app/content/rpc/internal/logic/post_integration_test.go`、`app/content/mq/cleanup/internal/store/count_sync_integration_test.go`、`pkg/outboxx/relay_test.go` |
| CORE-A07 详情状态与不可用点赞 | aligned | `app/gateway/internal/logic/posts/get_post_logic_test.go`；`like_logic_test.go` / `assert_interactable_logic_test.go`（草稿帖与评论父帖未发布均 404） |
| DISC-A01 三能力只返回可见内容 | aligned | `app/feed/rpc/internal/logic/get_follow_feed_logic_test.go`、`app/search/rpc/internal/logic/search_logic_test.go`、`app/recommend/rpc/internal/logic/recommend_logic_test.go` |
| DISC-A02 关注变化/空流/匿名冷启动 | aligned | `app/feed/rpc/internal/logic/get_follow_feed_logic_test.go`、`app/recommend/rpc/internal/logic/recommend_logic_test.go` |
| DISC-A03 搜索结果区分 | aligned | `app/search/rpc/internal/logic/search_logic_test.go` |
| DISC-A04 游标/配额/负反馈/降级 | aligned | `app/recommend/rpc/internal/logic/recommend_logic_test.go` |
| DISC-A05 曝光关联 | aligned | `app/behavior/rpc/internal/logic/record_events_logic_test.go`、`app/gateway/internal/logic/behavior/record_behavior_events_logic_test.go` |
| DISC-A06 冻结集复现门禁 | partial | 现有 live 结果基于 LLM 合成集，不满足人类双评审 |
| ASST-A01 证据/无结果/元数据/来源变化 | n/a | retired `SPEC-grounded-assistant` 验收项；当前来源验收见 AGENT-A05 |
| ASST-A02 候选重读与无资料工具 | n/a | retired `SPEC-grounded-assistant` 验收项；当前回源与工具边界见 AGENT-A02/A05 |
| ASST-A03 注入与伪造引用 | n/a | retired `SPEC-grounded-assistant` 验收项；当前注入边界见 AGENT-A02/A05 |
| ASST-A04 完成/取消/降级/越权 | n/a | retired `SPEC-grounded-assistant` 验收项；当前异步运行验收见 AGENT-A01/A06 |
| REL-A01 逐项接受/拒绝 | aligned | `app/behavior/rpc/internal/logic/record_events_logic_test.go`、`app/gateway/internal/logic/behavior/record_behavior_events_logic_test.go` |
| REL-A02 链路归因 | aligned | `integration/behavior_pipeline_integration_test.go`、`app/pipeline/behaviorlog` |
| REL-A03 故障降级矩阵 | partial | 仅覆盖网关 RPC-FAIL 与推荐推理注入，不是 REL-054 全部十行 |
| REL-A04 保留期与 24h 清理 | aligned | 24h 特征清理已测（`behavior_store_test.go`）；聚合 365 天 TTL 由 `daily_aggregates` 表 TTL 承担，`app/pipeline/behaviorlog/internal/store/clickhouse_store_integration_test.go` 的 `TestClickHouseStoreAggregateDailyDedupesAndIsIdempotent` 断言重复执行幂等与 365 天 TTL |
| REL-A05 月度 SLO 报告 | partial | `scripts/spec_evals.py slo`；月度观测数据待生产收集 |
| AGNT-A01 未授权/授权/撤销网关行为 | n/a | retired `SPEC-assistant-agent-mode` 验收项；当前授权验收见 AGENT-A01/A02 |
| AGNT-A02 Write + search_posts/web_search 成功与失败 | n/a | retired `SPEC-assistant-agent-mode` 验收项；当前工具验收见 AGENT-A02/A05 |
| AGNT-A03 删除确认同意/拒绝/超时/重放 | n/a | retired `SPEC-assistant-agent-mode` 验收项；当前确认验收见 AGENT-A02 |
| AGNT-A04 软限通知与硬限收尾 | n/a | retired `SPEC-assistant-agent-mode` 验收项；当前预算验收见 AGENT-A06 |
| AGNT-A05 注入不能触发白名单外操作 | n/a | retired `SPEC-assistant-agent-mode` 验收项 |
| AGNT-A06 consent_version=1 新分组不可见 | n/a | retired `SPEC-assistant-agent-mode` 验收项 |
| AGNT-A07 Search/UserState/Recommend 成功与越权 | n/a | retired `SPEC-assistant-agent-mode` 验收项 |
| AGNT-A08 旧客户端忽略未知事件/来源 | n/a | retired `SPEC-assistant-agent-mode` 验收项 |
| MEM-A01 双 target 操作/容量/冲突 | aligned | `app/assistant/internal/memory/store_test.go` |
| MEM-A02 扫描/隔离/undo CAS | aligned | Memory store 定向测试与 `app/assistant/internal/runtime/completion_test.go` |
| MEM-A03 prompt 冻结与 compact 刷新 | aligned | `app/assistant/internal/prompt/builder_test.go`、`app/assistant/internal/runtime/compact_test.go` |
| MEM-A04 review 隔离/抢占/memory_changed | aligned | `app/assistant/internal/runtime/completion_test.go`、`loop_cancel_test.go` |
| MEM-A05 存储失败与非来源 | partial | 工具不生成 source handle；真实存储故障未做集成注入 |
| MEM-A06 sidecar/旧快照 | aligned | `builder_test.go` 覆盖 Memory 不进 system、标签转义、sidecar 字节稳定、跨 chunk scrub；legacy prompt 保持旧格式直到新 epoch |
| WCH-A01 四种规则命中与不可见不命中 | aligned | `TestMatchRules`、`TestApplyPostEvent*`；草稿 Status!=1 不命中；消费者 `TestConsumeWatchBatch_PublishedCreate_RecordsHit` |
| WCH-A02 两分钟合并/小时与每日上限 | aligned | `app/assistant/internal/runtime/watch_test.go`；延迟窗口不改 merge unique key，计数仅成功增加 |
| WCH-A03 只读工具/用户抢占重排 | partial | 只读 registry 与失败/取消重排已实现；真实并发抢占未做 MySQL 集成测试 |
| WCH-A04 主动消息/未读/非普通私信 | aligned | `app/assistant/internal/runtime/watch_test.go` 覆盖 Assistant message、未读、outbox 与终态 |
| WCH-A05 CRUD/恢复可见性 | partial | CRUD 归属与停用已有测试；90 天后恢复及不可见内容补投仍缺集成验证 |


## 代码入口

- 内容生命周期与幂等：`app/content/rpc/internal/logic/{create,update,delete,get}_post_logic.go`、
  `comment_logic`、`app/content/rpc/internal/model/{post,comment}_command_model.go`、
  `pkg/idempotencyx/idempotency.go`（共享幂等，CORE-050）。
- 互动：`app/interaction/rpc/internal/logic/{like,unlike,favorite,unfavorite}_logic.go`、
  `published_target.go`；Content `assert_interactable_logic.go`（CORE-034）。
- 用户与隐私：`app/user/rpc/internal/logic/{get,set}_personalization_preference_logic.go`、
  `app/user/rpc/internal/model/personalization_preference_model.go`。
- 私信：`app/message/rpc/internal/logic/send_message_logic.go`、
  `app/message/rpc/internal/model/message_command_model.go`。
- 媒体：`app/media/rpc/internal/logic/upload_{image,video}_logic.go`、
  `app/media/rpc/internal/model/media_command_model.go`、
  `pkg/idempotencyx/idempotency.go`（共享幂等，CORE-050）。
- 推荐：`app/recommend/rpc/internal/logic/get_recommend_posts_logic.go`、`helpers.go`、
  `app/recommend/mq/internal/store/behavior_store.go`。
- Assistant runtime：`app/assistant/internal/{store,lease,memory,prompt,llm,tool,runtime,index}`；provider 重点入口
  `app/assistant/internal/llm/{stream,resilient,errors,canary}.go`，流归属
  `app/assistant/internal/runtime/stream_writer.go`，严格工具治理 `app/assistant/internal/tool/registry.go`；
  RPC 命令/读模型 `app/assistant/rpc`；worker `app/assistant/worker`；Watch `app/assistant/watch` + matcher `app/assistant/mq`。
- Assistant 权威库：`deploy/sql/xbh_assistant.sql` v3，破坏性 marker
  `deploy/sql/patches/20260829_assistant_runtime_v3.sql`；已有 v3 的幂等安全升级
  `deploy/sql/patches/20260830_assistant_run_fencing.sql`；provider usage/compact 锚点升级
  `deploy/sql/patches/20260830_agent_provider_reliability.sql`；DSN `DB_ASSISTANT`。
- 网关 Assistant REST：`app/gateway/internal/logic/assistant/`（messages/runs/consent/memory/watch）；
  契约 `proto/assistant/assistant.proto`、`app/gateway/gateway.api`。
- 行为：`app/behavior/rpc/internal/logic/record_events_logic.go`、`pkg/event/behavior.go`。
- 搜索：`app/search/rpc/internal/logic/search_logic.go`。
- 共享回源：`app/content/visibility`（把 Content `GetPostsByIds` 适配为 `visibilityx.Fetcher`，Assistant/Feed/Recommend/Search/Gateway 统一复用）。
- 网关：`app/gateway/internal/logic/**` 与 `app/gateway/gateway.api`；
  写接口仅 `CreatePostV2/UpdatePostV2/DeletePostV2`（`internal/logic/posts/*_v2_logic.go`），
  `/api/v1` 帖子写路由已废弃移除。

## 证据

Hermes 异步 Agent 硬切换见
[2026-08-29-assistant-agent-runtime.md](evidence/2026-08-29-assistant-agent-runtime.md)。
2026-08-30 runtime 完整性复查与定向修复见
[2026-08-30-assistant-runtime-completeness.md](evidence/2026-08-30-assistant-runtime-completeness.md)。
Assistant 并发、幂等、授权和 redirect 安全修复见
[2026-08-30-assistant-runtime-safety.md](evidence/2026-08-30-assistant-runtime-safety.md)。
永久前台 session 与 30 分钟冷拼接见
[2026-08-30-assistant-single-session.md](evidence/2026-08-30-assistant-single-session.md)。
Provider、capability、stream reset、严格工具与 sidecar 证据见
[2026-08-30-agent-provider-reliability.md](evidence/2026-08-30-agent-provider-reliability.md)。
历史 Agent Runtime 证据仍保留在 `evidence/`，不再作为当前契约完成证明。
