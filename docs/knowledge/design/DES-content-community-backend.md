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

本设计说明如何以现有 go-zero 工程结构满足四份已批准规范，并逐条登记实现状态。
状态列含义：`aligned` 已实现且测试覆盖；`partial` 部分实现；`missing` 未实现；
`n/a` 非代码门禁（如离线评测）。实现状态以源码、`.api`、`.proto`、SQL 与测试为准，
本文档只记录映射与偏离原因，不覆盖代码事实。

## 组件边界

- Gateway（`app/gateway`）：只绑定参数、调用 RPC、返回响应；鉴权中间件解析 JWT。
- RPC 服务：user / content / interaction / media / message / feed / search / recommend /
  assistant / behavior，各自持有 Model 与事务。
- MQ 消费（`app/*/mq`）：search 索引、embedding、feed fanout、message 通知、行为链路、
  推荐特征、清理任务。
- 可靠写入：权威业务写入与 outbox 同事务提交，异步效果经 relay 投递；
  DTM 屏障表仅保留 QueryPrepared 兼容 RPC 契约。
- 行为闭环：客户端事件 → behavior-rpc（校验+去重）→ RocketMQ → behaviorlog pipeline
  （去重+ClickHouse 存储）→ recommend-mq 特征更新。

## SPEC-community-core 追踪

| 要求 | 状态 | 实现位置与偏离说明 |
| --- | --- | --- |
| CORE-001 写操作验证调用者 | aligned | 所有写路由挂 `jwt: Auth`；logic 从上下文取 userId |
| CORE-002 只能改自己内容 | aligned | update/delete 校验 author_id |
| CORE-003 会话参与者才可读私信 | aligned | GetMessages/MarkRead 按 user_id 归属校验 |
| CORE-004 注册登录资料维护 | aligned | user rpc；注销/申诉/后台不在范围 |
| CORE-005 一致的可见性结果 | aligned | GetPost/列表/发现统一只暴露 published |
| CORE-010 状态机 | aligned | draft⇄published 双向 + 均→deleted 终态；Update 显式 status 支持取消发布 |
| CORE-011 创建返回 id/status/revision | aligned | CreatePostResp 返回 postId/status/revision=1 |
| CORE-012 草稿仅作者可读 | aligned | GetPost 作者可读草稿，非作者统一 404 |
| CORE-013 变更携带预期 revision | aligned | Update/Delete 携带 expected_revision；冲突返回 409；迁移期 0 值跳过检查（CORE-062） |
| CORE-014 变更后读取返回新状态/revision | aligned | 事务内写+outbox；Update/Get 返回新 revision |
| CORE-015 取消发布/删除不再出现 | aligned | 列表/发现/评论列表只收录 published；搜索/向量/关注流按 status 移除或跳过取消发布内容 |
| CORE-016 匿名/非作者统一不存在 | aligned | 草稿/删除/非公开对非作者统一 404，含评论线程 |
| CORE-020 标题/正文边界 | aligned | 1~120/1~20000 Unicode 校验 |
| CORE-021 图片≤9 标签≤10、标签 1~32 | aligned | 数量与长度校验 |
| CORE-022 评论 1~2000 且只能附着已发布内容 | aligned | 上限校验 + 仅 published 可评论 |
| CORE-023 图片 JPEG/PNG/WebP ≤10MiB | aligned | mediautil 内容嗅探白名单 + 10MiB 上限 |
| CORE-024 媒体引用校验 | aligned | 帖子引用媒体 ID 时校验存在/归属/完成态；上传返回稳定 id |
| CORE-030 互动幂等 | aligned | Like/Unlike/Favorite/Follow 命令模型 no-op 返回同状态 |
| CORE-031 单一有效关系 | aligned | 唯一键 + 状态字段 |
| CORE-032 互动状态立即可查 | aligned | 写入同事务失效缓存；计数经 count-sync 消费者 30s 内收敛；列表/收藏页批量回填当前访问者 liked/favorited 状态 |
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
| CORE-062 /api/v1 与 /api/v2/messages 兼容 | aligned | 契约仅新增可选字段；expected_revision 为可选迁移期字段，旧客户端不带也成功 |
| CORE-063 不依赖内部结构 | aligned | 契约层不暴露内部结构 |

## SPEC-content-discovery 追踪

| 要求 | 状态 | 实现位置与偏离说明 |
| --- | --- | --- |
| DISC-001 只返回可见已发布内容 | aligned | feed/search/recommend 均做可见性校验 |
| DISC-002 稳定内容标识 | aligned | post_id 稳定 |
| DISC-003 游标不重复、绑定上下文 | aligned | recommend 游标 HMAC+绑定；feed 游标按创建时间+id |
| DISC-004 未曝光不得解释为负反馈 | aligned | 负反馈只来自 hide/dislike 等明确动作 |
| DISC-010 关注流仅认证用户 | aligned | `/api/v2/feed/follow` 挂 jwt |
| DISC-011 关系变化按当前关系生成 | aligned | feed 按 follower 关系查询 |
| DISC-012 空关注流返回空 | aligned | 不混入推荐 |
| DISC-020 搜索覆盖帖子/用户/标签 | aligned | Search RPC 综合搜索 |
| DISC-021 帖子结果来自可访问已发布内容 | aligned | ES 索引只收录 published |
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
| DISC-060~063 离线评测门禁 | partial | 评测脚本已就绪并强制 ≥200 条查询；双评标注冻结集待人类评审 |

## SPEC-grounded-assistant 追踪

| 要求 | 状态 | 实现位置与偏离说明 |
| --- | --- | --- |
| ASST-001 仅认证用户、会话本人 | aligned | /assistant/chat 挂 jwt；会话按用户隔离 |
| ASST-002 只用已发布帖子证据 | aligned | tool 只检索 published 帖子 |
| ASST-003 仅元数据不构成证据 | aligned | 证据要求真实正文片段 |
| ASST-004 推荐候选需重读验证 | aligned | 推荐候选经 content 重读正文并验证 published 后才成为证据 |
| ASST-005 不提供资料工具 | aligned | 无用户资料工具 |
| ASST-006 内容指令不可信 | aligned | safety filter + 注入防护 |
| ASST-007 证据不足拒答 | aligned | 无证据返回拒答/降级 |
| ASST-010 段落必须含 [post:id]、1~5 来源 | aligned | 事实回答强制至少一个 [post:id]；来源 1~5 上限；缺失引用时降级 |
| ASST-011 结构化来源含 id/标题/片段/revision | aligned | 来源含 id/标题/片段/revision（SSRC 事件与持久化） |
| ASST-012 仅服务端验证来源可返回 | aligned | 模型生成引用标记不提升为来源 |
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
| ASST-033 检索失败关闭 | aligned | 检索失败返回错误，不自由生成 |
| ASST-034 安全策略拒绝、不泄露 | aligned | safety filter + 错误包装 |
| ASST-035 同请求重试不矛盾 | aligned | 同 request_id 的重复用户消息被去重，避免重复/矛盾回答 |
| ASST-040 /api/v2/assistant/chat 兼容 | aligned | 事件契约稳定 |
| ASST-041 证据边界不可变 | aligned | 设计约束 |
| ASST-042 新来源需重新批准 | aligned | 仅 post 来源 |
| ASST-050~051 离线评测门禁 | partial | 评测脚本已就绪并强制 ≥200 案例；冻结集待人类评审 |

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
| REL-020 保留期限自动删除 | aligned | 原始行为 90 天、特征 30 天、去重 90 天、死信 7 天、Assistant 会话 30 天，均由 TTL/DDL 落地 |
| REL-021 完整 IP 不入行为表 | aligned | 行为表不存完整 IP；访问日志 7 天 |
| REL-022 业务日志 30 天不泄密 | aligned | 全部 RPC 服务抑制框架自动内容日志（IgnoreContentMethods）+ 30 天 Loki 保留；结构化业务日志不含正文/私信/全量输入 |
| REL-023 关闭个性化 24h 删除特征 | aligned | /api/v2/me/personalization 接口；关闭后停止新行为入特征、purge 在线特征；DB+Redis 标记 |
| REL-024 关闭前事件 90 天、不合并匿名 | aligned | 原始事件 TTL 90 天、死信 7 天；匿名身份哈希不合并 |
| REL-030~033 SLO 口径 | partial | 月度 SLO 口径已在 scripts/spec_evals.py slo 命令落地；生产观测数据待月度报告 |
| REL-040 outbox p95 30s/p99 5m | partial | delivery_latency_seconds 直方图已落地；p95/p99 达标需月度观测数据 |
| REL-041 行为到特征 p95 60s/p99 5m | n/a | 需要观测数据 |
| REL-042 内容到搜索 p95 30s/p99 2m | n/a | 需要观测数据 |
| REL-043 RPO 0/RTO 30m | n/a | 运维目标 |
| REL-044 有界恢复不重试风暴 | aligned | relay 退避与租约 |
| REL-050~053 可观测与健康检查 | aligned | metrics + /health 存活 + /health/ready 就绪（列出依赖；可选发现能力故障仅 degraded）；网关 trace_id 经 gRPC metadata 透传下游 |
| REL-060 行为契约带版本 | aligned | schema_version=2 |
| REL-061 语义保持 | aligned | 契约稳定 |

## 验收策略

按规范逐条补实现并在对应服务补失败路径测试（AGENTS.md：不为通过测试改测试）。
离线评测门禁（DISC-060~063、ASST-050~051、REL-A05）需要冻结评测集与脚本，属于独立
验证工程；代码行为类验收（CORE-A*、DISC-A*、ASST-A*、REL-A* 的接口部分）以 Go 测试
与集成测试落地。
