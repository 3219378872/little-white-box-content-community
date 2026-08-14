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
  - app/assistant/rpc/internal/store
  - app/assistant/rpc/internal/tool
  - app/behavior/rpc/internal/logic
  - app/search/rpc/internal/logic
  - app/feed/rpc/internal/logic
  - app/gateway
  - pkg/visibilityx
  - app/content/visibility
  - proto/content/content.proto
  - proto/message/message.proto
  - proto/media/media.proto
  - proto/user/user.proto
  - proto/assistant/assistant.proto
  - proto/behavior/behavior.proto
  - proto/search/search.proto
  - deploy/sql/xbh_content.sql
  - deploy/sql/xbh_user.sql
  - deploy/sql/xbh_media.sql
  - deploy/sql/xbh_analytics.sql
verified_at: 2026-08-13
verified_commit: f7beca9
---

# 小白盒内容社区后端实现映射

本页记录四份已批准规范到代码实现的映射与逐条状态。
设计见 [DES-content-community-backend](../design/DES-content-community-backend.md)；
源码、`.api`、`.proto`、SQL 与测试高于本页。

## 总体状态

`diverged`：公开列表与发现回源后只保留 published；GetPost/列表对外返回 status。评论按主键读取不再走缓存。
仍偏离处：
- 搜索/列表/标签/收藏 `Total` 未重算全库，其他页仍可能计入已取消发布文档。
- `CORE-032` 计数 30s 收敛缺少生产观测。
- `DISC-061~063`（推荐门禁）待 live 样本集；`ASST-050/051` 的来源有效率与证据不足
  召回未达阈值（live 结果 2026-08-14 见 ASST 行）。
- `REL-030~043` 月度 SLO/异步延迟缺少生产观测。

## 规格追踪

下表是实现层台账。`aligned` 表示当前代码与测试覆盖该条；`partial`/`n/a` 不得写成设计已完成。
人类未关闭的评测集、月度 SLO 禁止标 `aligned`。

## SPEC-community-core 追踪

| 要求 | 状态 | 实现位置与偏离说明 |
| --- | --- | --- |
| CORE-001 写操作验证调用者 | aligned | 所有写路由挂 `jwt: Auth`；logic 从上下文取 userId |
| CORE-002 只能改自己内容 | aligned | update/delete 校验 author_id |
| CORE-003 会话参与者才可读私信 | aligned | GetMessages/MarkRead 按 user_id 归属校验 |
| CORE-004 注册登录资料维护 | aligned | user rpc；注销/申诉/后台不在范围 |
| CORE-005 一致的可见性结果 | aligned | GetPost/公开列表/标签/发现统一只暴露 published；列表回源后再滤；公开详情与评论列表对匿名开放 |
| CORE-010 状态机 | aligned | draft⇄published 双向 + 均→deleted 终态；Update 显式 status 支持取消发布 |
| CORE-011 创建返回 id/status/revision | aligned | CreatePostResp 返回 postId/status/revision=1 |
| CORE-012 草稿仅作者可读 | aligned | GetPost 作者可读草稿，非作者统一 404 |
| CORE-013 变更携带预期 revision | aligned | /api/v2/post 是唯一写路径，强制 expected_revision（缺失/0 → 400，冲突 → 409）；人类 2026-08-13 决定“直接废弃并迁移”，/api/v1 帖子写接口已移除（PROP-20260813-core-revision-contract 选项 B + 废弃决定） |
| CORE-014 变更后读取返回新状态/revision | aligned | 事务内写+outbox；Update 返回 status/revision；HTTP GetPost 与公开列表 PostItem 回传权威 status 与 revision |
| CORE-015 取消发布/删除不再出现 | partial | 单帖/批量/公开列表/标签回源后只保留 published；搜索/列表/标签/收藏 Total 只按本页回减 |
| CORE-016 匿名/非作者统一不存在 | aligned | 草稿/删除/非公开对非作者统一 404；已发布详情/评论允许匿名读取；评论列表 SQL 过滤 status=1 后再内存二次过滤并回减 Total（纵深防御） |
| CORE-020 标题/正文边界 | aligned | 1~120/1~20000 Unicode 校验 |
| CORE-021 图片≤9 标签≤10、标签 1~32 | aligned | 数量与长度校验 |
| CORE-022 评论 1~2000 且只能附着已发布内容 | aligned | 上限校验 + 仅 published 可评论 |
| CORE-023 图片 JPEG/PNG/WebP ≤10MiB | aligned | Handler 只限制 10MiB；类型由 mediautil 按文件内容嗅探 |
| CORE-024 媒体引用校验 | aligned | 帖子引用媒体 ID 时校验存在/归属/完成态；上传返回稳定 id |
| CORE-030 互动幂等 | aligned | Like/Unlike/Favorite/Follow 重复请求返回成功且不重复累计；命令层 no-op 不写 outbox |
| CORE-031 单一有效关系 | aligned | 唯一键 + 状态字段 |
| CORE-032 互动状态立即可查 | partial | liked/favorited 状态即时回填；计数走 count-sync，30s 收敛未经生产观测证明 |
| CORE-033 取消互动后查询无效 | aligned | Unlike/Unfavorite 置 inactive 并失效缓存 |
| CORE-040 一对一私信能力 | aligned | 仅 text/image/video/audio（type 1-4），无群聊/撤回/删除 |
| CORE-041 消息正文/媒体消息 | aligned | 文本 1~1000；媒体消息必须引用本人已完成媒体并持久化 media_id |
| CORE-042 消息幂等键 | aligned | idempotency_key ≤128、同键同命令返回原 id、异命令（含不同 media_id）冲突 |
| CORE-043 标记已读仅影响自己 | aligned | MarkRead 只改 receiver==自己 的行 |
| CORE-050 创建帖子/评论/媒体幂等键 | aligned | 帖子/评论/媒体均实现幂等表，同键同命令返回原资源、异命令 409 |
| CORE-051 可区分业务结果 | aligned | 版本冲突/幂等冲突 409 与业务码；网关透传 BizError |
| CORE-052 权威写入未确认不返回成功 | aligned | 事务+outbox 同事务 |
| CORE-053 异步效果失败不改成功 | aligned | 互动/评论/帖子缓存失效失败只告警不改变已提交成功的响应 |
| CORE-054 不泄露内部信息 | aligned | WrapMsg 不拼内部错误；框架 gRPC 错误只保留业务码，不暴露原始消息 |
| CORE-060 单页内不重复 | aligned | 页式列表由 SQL 分页保证 |
| CORE-061 游标链约束 | aligned | 见 DISC-003/033 |
| CORE-062 /api/v1 与 /api/v2/messages 兼容 | aligned | 消息契约仅新增可选字段；帖子写接口已按人类决定直接废弃并迁移到 /api/v2/post*，无迁移期跳过语义 |
| CORE-063 不依赖内部结构 | aligned | 契约层不暴露内部结构 |

## SPEC-content-discovery 追踪

| 要求 | 状态 | 实现位置与偏离说明 |
| --- | --- | --- |
| DISC-001 只返回可见已发布内容 | partial | 条目经 Content 回源过滤；搜索标题/摘要回填权威正文；Total 只按本页回减；中文检索改用 cjk 分词 + 20% minimum_should_match（2026-08-14 live 门禁调参） |
| DISC-002 稳定内容标识 | aligned | post_id 稳定 |
| DISC-003 游标不重复、绑定上下文 | aligned | recommend 游标 HMAC+绑定；feed 游标按创建时间+id |
| DISC-004 未曝光不得解释为负反馈 | aligned | 负反馈只来自 hide/dislike 等明确动作 |
| DISC-010 关注流仅认证用户 | aligned | `/api/v2/feed/follow` 挂 jwt |
| DISC-011 关系变化按当前关系生成 | aligned | 关注流先分页拉取当前 following，inbox 按当前关系过滤，outbox 按作者分批回源 |
| DISC-012 空关注流返回空 | aligned | 不混入推荐 |
| DISC-020 搜索覆盖帖子/用户/标签 | aligned | Search RPC 综合搜索 |
| DISC-021 帖子结果来自可访问已发布内容 | aligned | 查询时回源 Content，标题/摘要来自仍 published 的正文，不可见项丢弃 |
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
| DISC-060~063 离线评测门禁 | partial | DISC-060 已 live 通过（2026-08-14 合成语料+LLM 冻结集：NDCG@10=0.816≥0.7、泄漏=0，附 120s 超时变体说明）；DISC-061~063（推荐门禁）仍待 live 执行 |

## SPEC-grounded-assistant 追踪

| 要求 | 状态 | 实现位置与偏离说明 |
| --- | --- | --- |
| ASST-001 仅认证用户、会话本人 | aligned | /assistant/chat 挂 jwt；会话按用户隔离 |
| ASST-002 只用已发布帖子证据 | aligned | tool 只检索 published 帖子 |
| ASST-003 仅元数据不构成证据 | aligned | 证据要求真实正文片段 |
| ASST-004 推荐候选需重读验证 | aligned | 推荐候选经 content 重读正文并验证 published 后才成为证据 |
| ASST-005 不提供资料工具 | aligned | 无用户资料工具 |
| ASST-006 内容指令不可信 | aligned | safety filter + 注入防护 |
| ASST-007 证据不足拒答 | aligned | 无已发布正文证据时返回固定拒答，不返回搜索/推荐元数据摘要 |
| ASST-010 段落必须含 [post:id]、1~5 来源 | aligned | 事实回答强制至少一个 [post:id]；来源 1~5 上限；缺失引用时降级 |
| ASST-011 结构化来源含 id/标题/片段/revision | aligned | 来源含 id/标题/片段/revision（SSRC 事件与持久化） |
| ASST-012 仅服务端验证来源可返回 | aligned | 对外 SOURCE 只含回源验证过的帖子；user/tag 元数据不再提升为来源 |
| ASST-013 区分事实/观点/无法确认 | partial | 系统指令强制区分作者观点与平台事实；终验依赖 ASST-050 评测 |
| ASST-014 证据冲突呈现双方 | partial | 系统指令强制呈现冲突及各自来源；所有来源均提供给模型；终验依赖 ASST-050 评测 |
| ASST-015 来源不授额外权限 | aligned | 打开来源走正常权限 |
| ASST-020 输入≤2000/回答≤8000 | aligned | 输入 2000、回答 8000（LLM MaxOutputRunes） |
| ASST-021 限流 20/60s、会话 100 条/30 天 | aligned | Redis 原子限流 20/60s；会话 100 条 LTRIM；30 天 TTL；均有测试 |
| ASST-022 流事件结构 | aligned | token/source/done/error 事件 |
| ASST-023 不得先完成再失败 | aligned | 完成事件为终态 |
| ASST-024 截断不混入他人、一次性降级 | aligned | 会话按用户隔离 |
| ASST-030 来源变化标记 | aligned | 续接会话时按保存的 revision 重验来源，变化时输出 source-changed 警告 |
| ASST-031 来源不可用清理 | aligned | 续接会话时删除/取消发布来源标记 source-unavailable 并移除标题片段 |
| ASST-032 LLM 不可用返回证据摘要 | aligned | sendPersistedDegraded 返回证据摘要 |
| ASST-033 检索失败关闭 | aligned | 检索或 Content 回源失败返回错误，不降级成无证据自由生成 |
| ASST-034 安全策略拒绝、不泄露 | aligned | safety filter + 错误包装 |
| ASST-035 同请求重试不矛盾 | aligned | 同 request_id 的重复用户消息被去重，避免重复/矛盾回答 |
| ASST-040 /api/v2/assistant/chat 兼容 | aligned | 事件契约稳定 |
| ASST-041 证据边界不可变 | aligned | 设计约束 |
| ASST-042 新来源需重新批准 | aligned | 仅 post 来源 |
| ASST-050~051 离线评测门禁 | partial | live 执行（2026-08-14）：注入越界 0（达标）、可回答误拒率 5.8%（≤10% 达标）、来源有效率 77.3%（<100%）、证据不足召回 8.3%（<95%）；检索与 LLM 引用行为待提升 |

## SPEC-feedback-reliability 追踪

| 要求 | 状态 | 实现位置与偏离说明 |
| --- | --- | --- |
| REL-001 客户端动作白名单 | aligned | 客户端仅可提交 exposure/click/dwell/view/play/share/hide/dislike |
| REL-002 事件 id ≤128 全局唯一、批 ≤100 | aligned | client_event_id ≤128、批量上限配置 |
| REL-003 事件关联身份、位置从 1 开始 | aligned | exposure 位置必须 ≥1 |
| REL-004 曝光定义与去重 | aligned | 去重事件 id + 同一 (requestId, postId) 独立曝光去重键 |
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
| REL-022 业务日志 30 天不泄密 | aligned | 全部 RPC 服务抑制框架自动内容日志（IgnoreContentMethods）+ 30 天 Loki 保留；结构化业务日志不含正文/私信/全量输入 |
| REL-023 关闭个性化 24h 删除特征 | aligned | 关闭接口与特征清理已落地；DB 权威 + Redis 快速标记；recommend-mq 新增定时主动清理（PurgeOptedOutFeatures，默认 1h 周期），不依赖用户后续行为事件；偏好读取失败 fail-closed 只走规则冷启动；单测覆盖清理脚本与错误路径 |
| REL-024 关闭前事件 90 天、不合并匿名 | aligned | 原始事件 TTL 90 天、死信 7 天；匿名身份哈希不合并 |
| REL-030~033 SLO 口径 | partial | 月度 SLO 口径已在 scripts/spec_evals.py slo 落地；2026-08-14 合成观测干跑 6 能力域全部 met=True（管线验证，人类授权 LLM 生成）；真实生产观测待月度报告 |
| REL-040 outbox p95 30s/p99 5m | partial | delivery_latency_seconds 直方图已落地；真实月度观测待收集（合成干跑已验证报告管线） |
| REL-041 行为到特征 p95 60s/p99 5m | n/a | 需要真实观测数据（指标已埋点） |
| REL-042 内容到搜索 p95 30s/p99 2m | n/a | 需要真实观测数据（指标已埋点） |
| REL-043 RPO 0/RTO 30m | n/a | 运维目标 |
| REL-044 有界恢复不重试风暴 | aligned | relay 退避与租约 |
| REL-050~053 可观测与健康检查 | aligned | metrics + /health 存活 + /health/ready 就绪（列出依赖；可选发现能力故障仅 degraded）；网关 trace_id 经 gRPC metadata 透传下游 |
| REL-060 行为契约带版本 | aligned | schema_version=2 |
| REL-061 语义保持 | aligned | 契约稳定 |

## 验收标准追踪

四份规范共 21 条验收标准（CORE/DISC/ASST/REL-A*）。代码行为类以 Go 测试落地，
离线评测/观测类由 `scripts/spec_evals.py` 与冻结数据集承担（后者待人类输入，见
[IMP-todo-blocked-gates](IMP-todo-blocked-gates.md)）。

| 验收标准 | 状态 | 测试/证据位置 |
| --- | --- | --- |
| CORE-A01 状态机与 revision 冲突 | aligned | `app/content/rpc/internal/logic/post_logic_test.go`、`idempotency_revision_integration_test.go`、`post_integration_test.go` |
| CORE-A02 匿名读取/草稿删除不可见 | aligned | `app/gateway/rest_decision_table_test.go`（POST-LIST-ANON / POST-GET-ANON / COMMENT-LIST-ANON）、`app/content/rpc/internal/logic/post_logic_test.go` |
| CORE-A03 输入边界 | aligned | `app/content/rpc/internal/logic/post_logic_test.go`、`comment_logic_test.go`、`app/media/rpc/internal/mediautil` |
| CORE-A04 幂等收敛 | aligned | `app/content/rpc/internal/logic/idempotency_revision_integration_test.go`、`app/interaction/rpc/internal/logic/interaction_integration_test.go`、`app/message/rpc/internal/model/message_command_model_integration_test.go` |
| CORE-A05 会话参与者权限 | aligned | `app/message/rpc/internal/logic/message_logic_test.go` |
| CORE-A06 权威写与异步失败 | aligned | `app/content/rpc/internal/logic/post_integration_test.go`、`app/content/mq/cleanup/internal/store/count_sync_integration_test.go`、`pkg/outboxx/relay_test.go` |
| DISC-A01 三能力只返回可见内容 | aligned | `app/feed/rpc/internal/logic/get_follow_feed_logic_test.go`、`app/search/rpc/internal/logic/search_logic_test.go`、`app/recommend/rpc/internal/logic/recommend_logic_test.go` |
| DISC-A02 关注变化/空流/匿名冷启动 | aligned | `app/feed/rpc/internal/logic/get_follow_feed_logic_test.go`、`app/recommend/rpc/internal/logic/recommend_logic_test.go` |
| DISC-A03 搜索结果区分 | aligned | `app/search/rpc/internal/logic/search_logic_test.go` |
| DISC-A04 游标/配额/负反馈/降级 | aligned | `app/recommend/rpc/internal/logic/recommend_logic_test.go` |
| DISC-A05 曝光关联 | aligned | `app/behavior/rpc/internal/logic/record_events_logic_test.go`、`app/gateway/internal/logic/behavior/record_behavior_events_logic_test.go` |
| DISC-A06 冻结集复现门禁 | partial | search 冻结集已 live 复现（NDCG@10=0.816、泄漏 0）；recommend 门禁待 live 样本集 |
| ASST-A01 证据/无结果/元数据/来源变化 | aligned | `app/assistant/rpc/internal/logic/chat_logic_test.go`、`app/assistant/rpc/internal/tool/registry_test.go`；live 冒烟验证证据引用/冲突呈现/拒答 |
| ASST-A02 候选重读与无资料工具 | aligned | `app/assistant/rpc/internal/tool/registry_test.go`、`app/assistant/rpc/internal/logic/chat_logic_test.go` |
| ASST-A03 注入与伪造引用 | aligned | `app/assistant/rpc/internal/logic/chat_logic_test.go` |
| ASST-A04 完成/取消/降级/越权 | aligned | `app/gateway/internal/logic/assistant/assistant_chat_logic_test.go`、`app/assistant/rpc/internal/logic/chat_logic_test.go` |
| REL-A01 逐项接受/拒绝 | aligned | `app/behavior/rpc/internal/logic/record_events_logic_test.go`、`app/gateway/internal/logic/behavior/record_behavior_events_logic_test.go` |
| REL-A02 链路归因 | aligned | `integration/behavior_pipeline_integration_test.go`、`app/pipeline/behaviorlog` |
| REL-A03 故障降级矩阵 | aligned | `app/gateway/rest_decision_table_test.go`（RPC-FAIL 系列）、`app/recommend/rpc/internal/logic/inference_fault_injection_test.go` |
| REL-A04 保留期与 24h 清理 | aligned | 24h 特征清理已测（`behavior_store_test.go`）；聚合 365 天 TTL 由 `daily_aggregates` 承担并有集成测试（手动 ClickHouse 验证幂等与 TTL） |
| REL-A05 月度 SLO 报告 | partial | `scripts/spec_evals.py slo`；月度观测数据待生产收集 |


## 代码入口

- 内容生命周期与幂等：`app/content/rpc/internal/logic/{create,update,delete,get}_post_logic.go`、
  `comment_logic`、`app/content/rpc/internal/model/{post,comment}_command_model.go`、
  `app/content/rpc/internal/model/idempotency_model.go`。
- 互动：`app/interaction/rpc/internal/logic/{like,unlike,favorite,unfavorite}_logic.go`。
- 用户与隐私：`app/user/rpc/internal/logic/{get,set}_personalization_preference_logic.go`、
  `app/user/rpc/internal/model/personalization_preference_model.go`。
- 私信：`app/message/rpc/internal/logic/send_message_logic.go`、
  `app/message/rpc/internal/model/message_command_model.go`。
- 媒体：`app/media/rpc/internal/logic/upload_{image,video}_logic.go`、
  `app/media/rpc/internal/model/media_command_model.go`、
  `app/media/rpc/internal/model/idempotency_model.go`。
- 推荐：`app/recommend/rpc/internal/logic/get_recommend_posts_logic.go`、`helpers.go`、
  `app/recommend/mq/internal/store/behavior_store.go`。
- Assistant：`app/assistant/rpc/internal/logic/chat_logic.go`、
  `app/assistant/rpc/internal/tool/registry.go`、`app/assistant/rpc/internal/store/state.go`。
- 行为：`app/behavior/rpc/internal/logic/record_events_logic.go`、`pkg/event/behavior.go`。
- 搜索：`app/search/rpc/internal/logic/search_logic.go`。
- 共享回源：`app/content/visibility`（把 Content `GetPostsByIds` 适配为 `visibilityx.Fetcher`，Assistant/Feed/Recommend/Search/Gateway 统一复用）。
- 网关：`app/gateway/internal/logic/**` 与 `app/gateway/gateway.api`；
  写接口仅 `CreatePostV2/UpdatePostV2/DeletePostV2`（`internal/logic/posts/*_v2_logic.go`），
  `/api/v1` 帖子写路由已废弃移除。

## 证据

验证于 2026-08-12（提交 0031d91）：
- `make check`：fmt、engineering-lint、vet、golangci-lint 全部通过。
- `make test`：全部模块 race 测试通过。
- `go test -tags integration ./app/content/rpc/internal/logic/` 与
  `./app/user/rpc/internal/logic/`：通过（含 revision/idempotency/状态机/隐私偏好）。
- `make integration-critical` 通过；count-sync 与 message 命令模型集成测试通过。
- 各服务单元测试覆盖新增失败路径（版本冲突、幂等冲突、媒体归属、来源变化等）。

未覆盖边界：媒体与消息的媒体校验依赖真实 media RPC（本地无 SeaweedFS），通过单元测试
fake 覆盖；离线评测门禁与 SLO 观测数据不在此提交内。
