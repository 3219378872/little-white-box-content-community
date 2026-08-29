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
  - deploy/loki/loki-config.yaml
  - deploy/docker-compose.middleware.yml
verified_at: 2026-08-28
verified_commit: 648360f48c25283bf283406744ec80661c4db39c
---

# 小白盒内容社区后端实现映射

本页记录已批准规范到代码实现的映射与逐条状态。现有 Assistant/Agent、Memory 和 Watch
表格是 2026-08-28 之前同步实现的历史快照；其中 `SPEC-grounded-assistant`、
`SPEC-assistant-agent-mode` 及其 ASST/AGNT/MEM/WCH 条款已由 Hermes 规范退休，不再是当前
产品约束。`SPEC-assistant-agent` 的持久异步 worker、虚拟线程、compact、BM25、source
ledger 和主动 Watch 消息尚未实现或验证，故本页继续保持 `diverged`，不得把下方旧表格当作
新契约的完成证据。
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
- AGENT-A01~A06 / MEM-A04 / WCH-A02 缺少 live LLM 与根真实栈。

## 规格追踪

下表是实现层台账。`aligned` 表示当前代码与测试覆盖该条；`partial`/`n/a` 不得写成设计已完成。
人类未关闭的评测集、月度 SLO 禁止标 `aligned`。

## SPEC-community-core 追踪

| 要求 | 状态 | 实现位置与偏离说明 |
| --- | --- | --- |
| CORE-001 写操作验证调用者 | aligned | 所有写路由挂自有 `RequiredAuth`（避免框架鉴权失败 dump 完整请求）；logic 从强类型 JWT context 取 userId |
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
| CORE-023 图片 JPEG/PNG/WebP ≤10MiB | aligned | Handler 区分 MaxBytes 与非法 multipart；类型由 mediautil 按内容嗅探 |
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
| CORE-050 创建帖子/评论/媒体幂等键 | aligned | 帖子/评论/媒体均实现幂等表，同键同命令返回原资源、异命令 409；媒体命令哈希含接收文件内容 sha256 指纹；评论命令哈希含回复目标评论与被回复用户（CORE-051 异命令冲突，2026-08-14） |
| CORE-051 可区分业务结果 | aligned | 版本冲突/幂等冲突 409 与业务码；网关透传 BizError；HTTPStatus 为唯一映射（密码错误 401、验证码错误/过期 400、空搜索 400、搜索超时 504，2026-08-14 补齐） |
| CORE-052 权威写入未确认不返回成功 | aligned | 事务+outbox 同事务（帖子/评论；media 软删已接入：media-deleted 事件与软删同事务，relay 投递，避免提交后崩溃丢事件产生 S3 孤儿对象） |
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

## SPEC-assistant-agent 追踪

已 retired 的 `SPEC-grounded-assistant` / `SPEC-assistant-agent-mode` 不再作为当前约束。
下列 AGENT 条款对照 `SPEC-assistant-agent` 与 `DES-assistant-agent-runtime`。

| 要求 | 状态 | 实现位置与偏离说明 |
| --- | --- | --- |
| AGENT-001 虚拟私信线程 | aligned | `assistant_thread`；不写普通 message 模型 |
| AGENT-002 无 mode 开关 | aligned | `/assistant/chat` 与 `mode` 已删除；`POST /assistant/messages` |
| AGENT-003 无 Intent Router | aligned | worker 直接把工具 schema 交给模型 |
| AGENT-004 身份边界 | aligned | user run 当前用户；Watch 只读；memory-review 仅 Memory 工具 |
| AGENT-010 线程/消息 API | aligned | `GetThread`/`ListMessages` |
| AGENT-011 异步 PostMessage | aligned | 事务写消息与 run，返回 disposition |
| AGENT-012 单前台 run + redirect/steer/FIFO32 | aligned | `runtime.DecideDisposition`；`accept_test.go` |
| AGENT-013 新会话/清历史 | aligned | CreateSession 滚 epoch；DeleteHistory 逻辑删消息 |
| AGENT-014 未读 | aligned | Watch 计入；`memory_changed` unread=false |
| AGENT-015 content/api_content 分离 | aligned | `assistant_message`；工具轮隐藏 `kind=tool` sidecar，下一轮按 `api_content` 重放 |
| AGENT-020 rpc 不调模型 | aligned | worker `app/assistant/worker` |
| AGENT-021 lease 60s/10s | aligned | `internal/lease` SKIP LOCKED |
| AGENT-022 断线不取消；用户抢占后台 | aligned | Subscribe 不写 cancel；PostMessage 取消 watch/review；Stop 取消 work ctx 并 sticky `cancel_requested`（`loop_cancel_test.go`） |
| AGENT-023 SSE MySQL+Redis 降级轮询 | aligned | `runtime.Subscribe` |
| AGENT-024 事件类型白名单 | aligned | `store.Event*` |
| AGENT-025 唯一终止 | aligned | finish 写 done/error 后清 active_run_id |
| AGENT-030 consent | aligned | PostMessage 查 UserService.GetAgentCapabilityConsent |
| AGENT-031 工具分组 | aligned | `tool.ForSource` |
| AGENT-032 仅 delete_post 确认 | aligned | HighRisk + Confirm CAS |
| AGENT-033 command journal | aligned | UNIQUE(user,request,tool,digest) |
| AGENT-034 确认 CAS | aligned | `runtime.Confirm`；`confirm_test.go` |
| AGENT-040 双 WireAPI 工具调用 | aligned | Chat Completions `tool_calls`/`role=tool`；Responses `function_call`/`function_call_output` |
| AGENT-041 prompt 顺序 | aligned | `internal/prompt` |
| AGENT-043 快照复用 | aligned | `builder_test.go` |
| AGENT-050 compact 50%/keep 20% | aligned | 只标记未保留消息 compacted；未完成 tool 强制保留 |
| AGENT-052 memory-review 每 10 回合 | aligned | `maybeScheduleReview` |
| AGENT-060/061 search_history | aligned | ES `assistant-history-v1` + MySQL 回源 |
| AGENT-070/071 source ledger + present_sources | aligned | `tool/sources_test.go` |
| AGENT-080/081/082 预算 | aligned | `budget.go`；`budget_test.go` |
| AGENT-A01~A06 验收 | partial | 单测覆盖 disposition/confirm/budget/source/prompt/llm/tool-round replay；无 live LLM、无根真实栈 |

## SPEC-agent-memory 追踪

| 要求 | 状态 | 实现位置与偏离说明 |
| --- | --- | --- |
| MEM-001 自然语言条目 | aligned | `core_memory_entry` target=memory\|user |
| MEM-002 容量 2200/1375 | aligned | `memory.store_test.go` |
| MEM-003 用户隔离 | aligned | SQL/MapStore 按 user_id |
| MEM-004 威胁扫描 | aligned | safety.Filter + 指令扫描 |
| MEM-010 add/replace/remove/batch | aligned | RPC + 工具共用 `internal/memory` |
| MEM-011 version CAS | aligned | replace/remove 期望 version |
| MEM-012 规范化去重 | aligned | 同 target 规范化内容返回已有条目 |
| MEM-013 undo CAS | aligned | `memory_change` result_version |
| MEM-020/021 快照冻结 | aligned | 普通写不热更新 prompt snapshot |
| MEM-022/023 memory-review | aligned | 10 回合调度；前台抢占；memory_changed unread=false |
| MEM-030 API 字段 | aligned | List/Add/Replace/Remove/Batch/Undo |
| MEM-032 存储不可用失败 | aligned | store=nil → 503 |
| MEM-033 不能当 source card | aligned | Memory 工具不写 source ledger |

## SPEC-agent-watch 追踪

| 要求 | 状态 | 实现位置与偏离说明 |
| --- | --- | --- |
| WCH-001 任务 CRUD | aligned | watch_task + REST/工具 |
| WCH-002 规则条件 | aligned | 四种规则 + discussion_spike 预筛选 |
| WCH-003 不可见不命中 | aligned | matcher 回源 published |
| WCH-004 内部 hit 90 天 | aligned | `watch_hit` 非用户收件箱 |
| WCH-010 两分钟合并与限额 | aligned | `watch_delivery_bucket` + `watch_send_stat` |
| WCH-011 只读工具表 | aligned | `tool.WatchTools` |
| WCH-012 用户抢占重置 bucket | aligned | PostMessage `ResetUnsentBuckets` |
| WCH-013 成功写 assistant 消息+未读 | aligned | worker Watch run 同事务 |
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
| REL-020 保留期限自动删除 | aligned | 原始行为 90 天、特征 30 天、去重 90 天、死信 7 天、Assistant 会话 30 天，均由 TTL/DDL 落地；新增 `daily_aggregates` 去标识聚合表（TTL 365 天，ReplacingMergeTree 幂等）与 behavior-log 定时聚合任务（`AggregateIntervalSeconds`/`AggregateBackfillDays`）；修复既有 schema 在 DateTime64 列上的 TTL 建表错误（BAD_TTL_EXPRESSION），ClickHouse 集成测试现可初始化 |
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
| ASST-A01 证据/无结果/元数据/来源变化 | aligned | `app/assistant/rpc/internal/logic/chat_logic_test.go`、`app/assistant/rpc/internal/tool/registry_test.go`；live 冒烟验证证据引用/冲突呈现/拒答 |
| ASST-A02 候选重读与无资料工具 | aligned | `registry_test.go`、`chat_logic_test.go`；Agent 评论：父帖未发布拒绝、无效评论不附带（`search_test.go`） |
| ASST-A03 注入与伪造引用 | aligned | `app/assistant/rpc/internal/logic/chat_logic_test.go` |
| ASST-A04 完成/取消/降级/越权 | aligned | `app/gateway/internal/logic/assistant/assistant_chat_logic_test.go`、`app/assistant/rpc/internal/logic/chat_logic_test.go` |
| REL-A01 逐项接受/拒绝 | aligned | `app/behavior/rpc/internal/logic/record_events_logic_test.go`、`app/gateway/internal/logic/behavior/record_behavior_events_logic_test.go` |
| REL-A02 链路归因 | aligned | `integration/behavior_pipeline_integration_test.go`、`app/pipeline/behaviorlog` |
| REL-A03 故障降级矩阵 | partial | 仅覆盖网关 RPC-FAIL 与推荐推理注入，不是 REL-054 全部十行 |
| REL-A04 保留期与 24h 清理 | aligned | 24h 特征清理已测（`behavior_store_test.go`）；聚合 365 天 TTL 由 `daily_aggregates` 表 TTL 承担，`app/pipeline/behaviorlog/internal/store/clickhouse_store_integration_test.go` 的 `TestClickHouseStoreAggregateDailyDedupesAndIsIdempotent` 断言重复执行幂等与 365 天 TTL |
| REL-A05 月度 SLO 报告 | partial | `scripts/spec_evals.py slo`；月度观测数据待生产收集 |
| AGNT-A01 未授权/授权/撤销网关行为 | aligned | 网关 `agent_chat_gate_test.go`；RPC `TestAgentChatRejectsUnauthorizedBeforeSideEffects` |
| AGNT-A02 Write + search_posts/web_search 成功与失败 | aligned | `app/assistant/rpc/internal/agent/tools_test.go`、`app/assistant/rpc/internal/agent/search_test.go` |
| AGNT-A03 删除确认同意/拒绝/超时/重放 | aligned | `app/assistant/rpc/internal/agent/confirm_test.go`、`TestDeletePostRequiresConfirmation` |
| AGNT-A04 软限通知与硬限收尾 | aligned | `app/assistant/rpc/internal/agent/runner_openai_test.go` |
| AGNT-A05 注入不能触发白名单外操作 | partial | enhanced_search 注入单测仍绿；Agent 系统提示约束不可信内容，缺专门的白名单外工具注入用例 |
| AGNT-A06 consent_version=1 新分组不可见 | aligned | `TestRestrictToolsForConsentKeepsV1Set`、`TestRestrictHidesNewSearchToolsOnV1Consent` |
| AGNT-A07 Search/UserState/Recommend 成功与越权 | aligned | UserState 成功回源与忽略外来 `user_id`；Search/Recommend 既有路径；`userstate_test.go`、`recommend_test.go` |
| AGNT-A08 旧客户端忽略未知事件/来源 | aligned | `TestAssistantChatUnknownEventsAreIgnored`；web 来源 type=web |
| MEM-A01 显式写入/冲突合并 | aligned | `app/assistant/rpc/internal/memory/store_test.go`、`app/assistant/rpc/internal/agent/memory_tools_test.go` |
| MEM-A02 Interest 衰减与“还有吗”续写 | aligned | 衰减已测；历史注入 + recommend Task upsert；开放 Task 排除 ID 约束推荐 |
| MEM-A03 列表/修改/删除/不要记住 | partial | CRUD 与 suppressed 已实现；缺“删除后回答不得再引用”专项 |
| MEM-A04 越权拒绝与存储不可用 | aligned | 他人记录 NotFound；工具与列表无库 503 |
| MEM-A05 记忆不能当社区证据/不能存私密资料 | aligned | 工具不产出社区 Source；Extract/Apply 丢弃手机号/验证码/私信样值 |
| WCH-A01 四种规则命中与不可见不命中 | aligned | `TestMatchRules`、`TestApplyPostEvent*`；草稿 Status!=1 不命中；消费者 `TestConsumeWatchBatch_PublishedCreate_RecordsHit` |
| WCH-A02 重复任务/越权/未知类型 | aligned | `TestCreateRejectsUnknownConditionAndDuplicates`；store 按 user_id |
| WCH-A03 discussion_spike 预筛选 | aligned | 未过阈值不调模型、不命中；过阈值无模型/模型否定不命中；`apply_test.go` |
| WCH-A04 命中仅本人收件箱、事件不重复未读 | aligned | hit 按 user 隔离、同事件去重；下次对话注入未读摘要 |
| WCH-A05 停用/删除后不再命中且不影响发帖 | aligned | disabled 任务不进入 `ListEnabled`；发帖主路径独立 |


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
- Assistant runtime：`app/assistant/internal/{store,lease,memory,prompt,llm,tool,runtime,index}`；
  RPC 命令/读模型 `app/assistant/rpc`；worker `app/assistant/worker`；Watch `app/assistant/watch` + matcher `app/assistant/mq`。
- Assistant 权威库：`deploy/sql/xbh_assistant.sql` v3，破坏性 marker
  `deploy/sql/patches/20260829_assistant_runtime_v3.sql`；DSN `DB_ASSISTANT`。
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
历史 Agent Runtime 证据仍保留在 `evidence/`，不再作为当前契约完成证明。
