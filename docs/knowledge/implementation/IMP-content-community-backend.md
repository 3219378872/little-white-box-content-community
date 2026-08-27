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
  - app/assistant/rpc/internal/agent
  - app/assistant/rpc/internal/memory
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
verified_at: 2026-08-27
verified_commit: 237612ab4190b272870355cbd01af6f79e0fef39
---

# 小白盒内容社区后端实现映射

本页记录已批准规范到代码实现的映射与逐条状态（社区核心、发现、证据化
Assistant、Agent 模式、记忆、条件追踪、反馈可靠性）。
设计见 [DES-content-community-backend](../design/DES-content-community-backend.md)
与 [DES-assistant-agent-runtime](../design/DES-assistant-agent-runtime.md)；
源码、`.api`、`.proto`、SQL 与测试高于本页。

## 总体状态

`diverged`：2026-08-27 已把 Agent Runtime / 记忆 / Watch 映射进本页。代码可关闭项
仍保持对齐。仍偏离处：
- `CORE-032` 公开计数 30s 收敛缺少生产观测。
- `DISC-060`/`ASST-050`/`ASST-051` 人类冻结集未关闭；`DISC-063` 无学习模型、相对提升 0。
- `REL-033`/`REL-040`~`043`/`REL-A05` 缺少真实月度观测。
- `REL-A03` 未注入 `REL-054` 全部十行。
- Watch `discussion_spike` 与命中列表回源、下次对话注入仍未闭环。
- 未配置 `DB_ASSISTANT` 时记忆/Watch 列表返回空，写接口 503。

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

## SPEC-grounded-assistant 追踪

| 要求 | 状态 | 实现位置与偏离说明 |
| --- | --- | --- |
| ASST-001 仅认证用户、会话本人 | aligned | /assistant/chat 挂 jwt；会话按用户隔离 |
| ASST-002 只用已发布帖子或有效评论证据 | aligned | enhanced_search 工具只检索 published 帖子；Agent `search_posts`/`get_post_comments` 可附带父帖可见且 status=1 的评论，父帖未发布则拒绝 |
| ASST-003 仅元数据不构成证据 | aligned | 证据要求真实帖子正文或评论正文片段 |
| ASST-004 推荐候选需重读验证 | aligned | 推荐候选经 content 重读正文并验证 published 后才成为证据；评论按评论标识回源且父帖须可见 published |
| ASST-005 不提供资料工具 | aligned | 无用户资料工具 |
| ASST-006 内容指令不可信 | aligned | safety filter + 注入防护 |
| ASST-007 证据不足拒答 | aligned | 无已发布正文证据时返回固定拒答，不返回搜索/推荐元数据摘要 |
| ASST-010 段落必须含 [post:id] 或 [comment:id]、1~5 来源 | aligned | enhanced_search 事实回答强制至少一个 [post:id]；Agent 评论证据标注 [comment:id] 并携带父帖 revision；来源 1~5 上限；缺失引用时降级 |
| ASST-011 结构化来源含类型/id/标题/片段/revision | aligned | 来源含 type（post/comment/web）、id/标题/片段/revision；comment 携带父帖 revision；web 不作为社区证据 |
| ASST-012 仅服务端验证来源可返回 | aligned | 对外 SOURCE 只含回源验证过的帖子；user/tag 元数据不再提升为来源 |
| ASST-013 区分事实/观点/无法确认 | partial | 系统指令强制区分作者观点与平台事实；终验依赖 ASST-050/051 评测（来源有效率与事实支持率） |
| ASST-014 证据冲突呈现双方 | partial | 系统指令强制呈现冲突及各自来源；所有来源均提供给模型；终验依赖 ASST-050/051 评测 |
| ASST-015 来源不授额外权限 | aligned | 打开来源走正常权限 |
| ASST-020 输入≤2000/回答≤8000 | aligned | 输入 2000、回答 8000（LLM MaxOutputRunes） |
| ASST-021 限流 20/60s、会话 100 条/30 天 | aligned | Redis 原子限流 20/60s；会话 100 条 LTRIM；30 天 TTL；均有测试 |
| ASST-022 流事件结构 | aligned | token/source/done/error 事件 |
| ASST-023 不得先完成再失败 | aligned | 完成事件为终态 |
| ASST-024 截断不混入他人、一次性降级 | aligned | 会话按用户隔离 |
| ASST-030 来源变化标记 | aligned | 续接会话时按保存的 revision 重验来源，变化时输出 source-changed 警告 |
| ASST-031 来源不可用清理 | aligned | 续接会话时删除/取消发布来源标记 source-unavailable 并移除标题片段 |
| ASST-032 LLM 不可用返回证据摘要 | aligned | sendEvidenceDegraded 持久化并流式返回证据摘要 + 来源引用，以降级错误事件结束（LLM_UNAVAILABLE） |
| ASST-033 检索失败关闭 | aligned | 检索或 Content 回源失败返回错误，不降级成无证据自由生成 |
| ASST-034 安全策略拒绝、不泄露 | aligned | safety filter + 错误包装 |
| ASST-035 同请求重试不矛盾 | aligned | 同 request_id 的重复用户消息被去重，避免重复/矛盾回答 |
| ASST-040 /api/v2/assistant/chat 兼容 | aligned | 事件契约稳定 |
| ASST-041 证据边界不可变 | aligned | 社区证据仍为已发布帖子及其有效评论；记忆与 Watch 命中不能替代引用 |
| ASST-042 comment 来源已批准 | aligned | Agent 搜索可返回 `comment` 来源；`web` 为研究素材；旧客户端忽略未知来源（AGNT-A08） |
| ASST-043 enhanced_search 缺省 | aligned | 网关空/无法识别 mode 一律 `enhanced_search`；`TestEnhancedSearchIsDefaultMode` |
| ASST-050 人类评测集 | partial | 现有案例为 LLM 生成；SPEC 要求两名人类评审；本轮未重跑 live Gateway、未改 `eval/assistant_cases.json` |
| ASST-051 质量阈值 | partial | live（合成集）：注入 0、误拒 5.8% 达标；来源 77.3%、不足召回 8.3% 未达；本轮未重跑 live |

## SPEC-assistant-agent-mode 追踪

| 要求 | 状态 | 实现位置与偏离说明 |
| --- | --- | --- |
| AGNT-001 mode=agent/enhanced_search，缺省 enhanced_search | aligned | 网关空/无法识别值走 `ASSISTANT_MODE_ENHANCED_SEARCH`；`TestEnhancedSearchIsDefaultMode` |
| AGNT-002 未授权不得静默降级执行工具 | aligned | 网关在开流前查 consent，未授权只回 `AGENT_NOT_AUTHORIZED`；`agent_chat_gate_test.go` |
| AGNT-003 以当前用户身份、权限不超过本人 | aligned | 写帖 `AuthorId=session.UserID`；记忆/Watch 按 `user_id` 隔离；检索回源走当前用户可见性 |
| AGNT-004 授权可查询、先查再确认 | aligned | `GET /assistant/consent` → User `GetAgentCapabilityConsent`；默认未授权 |
| AGNT-005 授权说明披露工具清单 | aligned | 后端返回 `consentVersion`/`currentVersion`（当前披露版本常量 `2`）；说明文案由客户端展示，服务端不代替确认 |
| AGNT-006 可随时撤销、立即生效 | aligned | `SetAgentCapabilityConsent` 持久化 `revoked_at`；之后 agent 请求按 AGNT-002 拒绝 |
| AGNT-007 consent_version 与分组裁剪 | aligned | User 库 `consent_version`；v1 只保留 Write + `search_posts`/`web_search`；`TestRestrictToolsForConsentKeepsV1Set`、`TestRestrictHidesNewSearchToolsOnV1Consent` |
| AGNT-010 分组白名单 | partial | Search/Recommend/Memory/Watch/Write 已注册，配置可收缩；**UserState**（`get_my_favorites`/`get_my_likes`/`get_my_following`/`get_my_posts`）未实现 |
| AGNT-011 帖子/评论回源 | aligned | `search_posts`/`get_post`/`get_post_comments` 回源 Content；未发布父帖拒绝评论；`search_test.go` |
| AGNT-012 web_search 非社区证据 | aligned | 来源 `type=web`，文案禁止当帖子证据；无 key 时工具不可用失败关闭 |
| AGNT-013 写帖图片仅本会话附件 | aligned | `resolveAttachments` + `assertMediaOwnership`；会话外 mediaId 失败；`tools_test.go` |
| AGNT-014 v2 乐观锁 | aligned | update/delete 走 `expected_revision`；缺省先读再写，冲突原样反馈 |
| AGNT-015 幂等键由请求标识派生 | aligned | `agent:<action>:<requestID>:<callID>`；`TestCreatePostDerivesIdempotencyAndRestrictsImages` |
| AGNT-016 用户/标签搜索非证据、get_post 回源 | aligned | `TestSearchUsersAreNotEvidence`；`get_post` 调 Content |
| AGNT-017 UserState 只读本人列表 | partial | 四个 UserState 工具未注册 |
| AGNT-018 Recommend 真实 ID 并回源 | aligned | `recommend_posts` 调 RecommendRpc `scene=agent`，排除记忆负向 ID 后再 Content 回源；`recommend_test.go` |
| AGNT-019 Memory/Watch 受 consent 与失败反馈 | aligned | 出现在白名单；v1 consent 不可见；失败走 `errx`/`toolFeedback` |
| AGNT-020 删除逐次确认 | aligned | `delete_post` HighRisk；确认事件含帖子摘要；拒绝不执行 |
| AGNT-021 确认超时视为拒绝 | aligned | 默认 120s；`ErrConfirmExpired` 反馈模型不挂起；`confirm_test.go` |
| AGNT-022 确认凭据一次性 | aligned | Redis pending/decision 键，Wait 后删除；跨请求重放无效 |
| AGNT-023 Memory/Watch 不走删除确认 | aligned | 对应工具 `HighRisk=false` |
| AGNT-030 软硬步数预算 | aligned | 默认软 8 / 硬 12；超软限注入剩余步数；`TestRunnerInjectsSoftLimitNotice` |
| AGNT-031 硬限强制收尾 | aligned | 剥离工具再生成一次，失败 `AGENT_BUDGET_EXCEEDED`；`TestRunnerFinalizesWithoutToolsAtHardLimit` |
| AGNT-032 独立配额 | aligned | Agent Redis 配额默认 10/60s，与 enhanced_search 分开 |
| AGNT-033 单轮时长上限、已写入不回滚 | aligned | `TurnTimeoutMs` 默认 120s；取消/超时停止发送；已成功写保留 |
| AGNT-040 会话附件仅当次请求 | aligned | ChatReq.attachments；网关校验 mediaId/url 与 ≤9 |
| AGNT-041 图片 ≤9 | aligned | 附件与写帖均拒绝超出，不截断 |
| AGNT-042 context_post_id 可选提示 | aligned | 缺省忽略；不授予额外权限 |
| AGNT-050 安全过滤与不可信内容 | aligned | 输入/输出走 safety；Agent 系统提示把网络与社区内容当不可信数据 |
| AGNT-051 错误不泄密 | aligned | 对外 `errx` 包装；不回传提示词/secret/堆栈 |
| AGNT-052 工具审计条目 | partial | 指标 `agent_tool_calls_total`；`tool_call`/`agent_run` 表已建，**无写入路径** |
| AGNT-053 可查询运行记录 | partial | schema 已有 `agent_run`（禁止正文/secret）；Runtime 只打日志与指标，未落库 |
| AGNT-060 事件向后兼容 | aligned | 新增 CARD/ACTIONS/WATCH_HIT；网关未知类型忽略；`TestAssistantChatUnknownEventsAreIgnored` |
| AGNT-061 LLM 不可用不得自行写帖 | aligned | Agent LLM 失败走 `LLM_UNAVAILABLE` 降级；规则 Watch 匹配不在对话回合内 |
| AGNT-062 工具失败分类反馈 | aligned | 参数/越权/冲突/不可用/超限映射 `errx` 后回灌模型 |
| AGNT-063 卡片/动作标识须已验证 | partial | proto 与网关映射已有；Runner **尚未发出** CARD/ACTIONS |
| AGNT-064 推荐卡片 ID 来自真实结果 | partial | Recommend 工具只返回回源 ID；尚无推荐卡片事件 |

## SPEC-agent-memory 追踪

| 要求 | 状态 | 实现位置与偏离说明 |
| --- | --- | --- |
| MEM-001 结构化记录字段 | aligned | `memory.Item`：层/维度/值/分值/来源/置信度/更新时间；四层表在 `xbh_assistant` |
| MEM-002 来源仅 behavior/conversation/explicit | aligned | `Apply` 拒绝其它来源 |
| MEM-003 同键一条当前记录+历史 | aligned | 唯一键 + `history_json`；`TestMapStoreConflictKeepsOneCurrentRecord` |
| MEM-004 Interest 读取衰减 | aligned | `score * exp(-λ Δt)`，低于 0.05 不进 ContextBlock；`TestInterestDecayDropsBelowFloorInContext` |
| MEM-005 Episodic 默认不注入 | partial | 默认 List 不含 episodic；ContextBlock 仅在 recommend/community_opinion 跳过 episodic，其它 intent 仍可能带上 |
| MEM-006 “还有吗”续写 Task | partial | `ClassifyIntent` 把“还有吗/换一批”标 `continue_task` 并装载开放 Task；检索/推荐尚未强制套用 Task 已排除 ID |
| MEM-010 写入须校验合并 | partial | 显式句式 `Extract` + `Apply` 去重合并；无 LLM 候选 schema 路径 |
| MEM-011 显式偏好高于行为推断 | aligned | 对话显式句式 `conversation` 置信度 0.9；工具写入 `explicit` 置信度 1 |
| MEM-012 关闭个性化后 behavior 停用 | partial | 助手记忆路径未读取个性化关闭标记 |
| MEM-013 不得保存私密字段 | partial | 只拒绝空值；无手机号/验证码/私信/未发布内容丢弃规则 |
| MEM-020 可列出 Profile/Interest/Task | aligned | `GET /assistant/memory`；`confirmed` 区分高置信；episodic 按 layer 检索 |
| MEM-021 修改/删除/不要记住 | aligned | PATCH/DELETE；`suppressed=1` 后 SQL 冲突更新保持禁止 |
| MEM-022 只对所属用户开放 | aligned | JWT userId；store 按 user_id 过滤，他人记录 NotFound |
| MEM-023 回答不得引用已删/失效记忆 | partial | ContextBlock 跳过 suppressed 与衰减 Interest；无“回答引用已删记忆”专项测试 |
| MEM-030 Memory 工具只作用于当前用户 | aligned | `get/add/update/delete_memory`；add/update 走 `Apply`/`Update` |
| MEM-031 对外列表/修改/删除且幂等 | aligned | REST+RPC；无库时写 503、删不存在 404；列表无库返回空（见 MEM-040） |
| MEM-032 抽取不阻塞 DONE | aligned | 回合成功后 `go persistMemory`；失败只打日志 |
| MEM-040 存储不可用不得谎报成功 | partial | Memory 工具 store=nil → 503；**列表在无 `DB_ASSISTANT` 时返回空 items，不报暂时不可用**；写需要 DB |
| MEM-041 记忆不是社区证据 | aligned | Memory 工具不产出 post/comment Source |
| MEM-042 随后列表与回合可见刚提交修改 | aligned | 同进程 Apply 后 List 立即可见 |

## SPEC-agent-watch 追踪

| 要求 | 状态 | 实现位置与偏离说明 |
| --- | --- | --- |
| WCH-001 任务含稳定标识/类型/目标/启用 | aligned | `watch_task` + REST/工具 CRUD |
| WCH-002 四种规则条件 | aligned | `author_new_post`/`tag_new_post`/`keyword_new_post`/`post_revised`；`TestMatchRules` |
| WCH-003 discussion_spike 预筛选后才调模型 | partial | 允许创建该类型；`Match()` **未实现**预筛选或模型判定 |
| WCH-004 不可见/未发布不命中；目标不存在须创建失败 | partial | `Match` 要求 `Status==1`；创建路径**不校验**作者/标签是否存在 |
| WCH-005 同用户同条件同目标不重复 | aligned | 唯一键；重复 `IdempotencyConflict`；`TestCreateRejectsUnknownConditionAndDuplicates` |
| WCH-010 规则条件由事件驱动 | aligned | `app/assistant/mq` 订阅 `post-*`；`ApplyPostEvent` + `Match`；不轮询模型 |
| WCH-011 匹配执行记录与去重 | partial | `watch_execution(task_id,event_key)` 唯一；`RecordHit` INSERT IGNORE 后才写 hit；未给未命中任务写 skipped 执行行 |
| WCH-012 spike 未过阈值不调模型 | partial | 预筛选/模型否定路径未实现；matcher 不消费 `user-behavior-v2` |
| WCH-013 匹配失败不影响发帖主路径 | aligned | 独立消费者；发帖 outbox 不依赖 matcher；存储错误只重试本消费者 |
| WCH-020 命中只进助手收件箱 | partial | `watch_hit` + `GET /assistant/watch/hits`；不写私信/通知/推送；**下次 Agent 对话未注入未读摘要** |
| WCH-021 列表/已读且可见性过滤 | partial | 按 user_id 列出/标记已读；返回前**未回源**过滤不可见帖 |
| WCH-022 命中不是社区证据 | aligned | 命中不自动变成 Source；事实陈述仍须回源引用 |
| WCH-030 Watch 工具只作用于当前用户 | aligned | 四个工具 + RPC，均带 session/JWT userId |
| WCH-031 不走删除确认；删任务停后续命中 | aligned | 工具非 HighRisk；删除任务后 `Match` 不再看到该任务；已有 hit 保留 |
| WCH-032 越权拒绝 | aligned | REST jwt；SQL/MapStore 按 user_id |
| WCH-040 存储不可用 | partial | 无 `DB_ASSISTANT` 时创建/更新/删除 503；**列表任务/命中返回空**；无恢复后补扫 |
| WCH-041 未知条件类型拒绝 | aligned | `ValidateTask` → ParamError；`TestCreateRejectsUnknownConditionAndDuplicates` |
| WCH-042 WATCH_HIT 事件可忽略 | aligned | proto `CHAT_EVENT_TYPE_WATCH_HIT`；网关未知事件忽略 |

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
| REL-022 业务日志 30 天不泄密 | aligned | IgnoreContentMethods + Loki 30 天（镜像钉 `grafana/loki:3.7.6`，schema v13/tsdb，`compactor.delete_request_store`；禁止 `:latest`）；登录/注册/验证码日志不再写手机号 |
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
| AGNT-A01 未授权/授权/撤销网关行为 | aligned | `app/gateway/internal/logic/assistant/agent_chat_gate_test.go` |
| AGNT-A02 Write + search_posts/web_search 成功与失败 | aligned | `app/assistant/rpc/internal/agent/tools_test.go`、`app/assistant/rpc/internal/agent/search_test.go` |
| AGNT-A03 删除确认同意/拒绝/超时/重放 | aligned | `app/assistant/rpc/internal/agent/confirm_test.go`、`TestDeletePostRequiresConfirmation` |
| AGNT-A04 软限通知与硬限收尾 | aligned | `app/assistant/rpc/internal/agent/runner_openai_test.go` |
| AGNT-A05 注入不能触发白名单外操作 | partial | enhanced_search 注入单测仍绿；Agent 系统提示约束不可信内容，缺专门的白名单外工具注入用例 |
| AGNT-A06 consent_version=1 新分组不可见 | aligned | `TestRestrictToolsForConsentKeepsV1Set`、`TestRestrictHidesNewSearchToolsOnV1Consent` |
| AGNT-A07 Search/UserState/Recommend 成功与越权 | partial | Search/Recommend 有成功与不可见/回源路径；**UserState 未实现** |
| AGNT-A08 旧客户端忽略未知事件/来源 | aligned | `TestAssistantChatUnknownEventsAreIgnored`；web 来源 type=web |
| MEM-A01 显式写入/冲突合并 | aligned | `app/assistant/rpc/internal/memory/store_test.go`、`app/assistant/rpc/internal/agent/memory_tools_test.go` |
| MEM-A02 Interest 衰减与“还有吗”续写 | partial | 衰减已测；continue_task 仅 intent 分类，未测 Task 排除约束推荐 |
| MEM-A03 列表/修改/删除/不要记住 | partial | CRUD 与 suppressed 已实现；缺“删除后回答不得再引用”专项 |
| MEM-A04 越权拒绝与存储不可用 | partial | 他人记录 NotFound；工具无库 503；REST 列表无库返回空而非暂时不可用 |
| MEM-A05 记忆不能当社区证据/不能存私密资料 | partial | 工具不产出社区 Source；私密字段丢弃未测 |
| WCH-A01 四种规则命中与不可见不命中 | aligned | `TestMatchRules`、`TestApplyPostEvent*`；草稿 Status!=1 不命中；消费者 `TestConsumeWatchBatch_PublishedCreate_RecordsHit` |
| WCH-A02 重复任务/越权/未知类型 | aligned | `TestCreateRejectsUnknownConditionAndDuplicates`；store 按 user_id |
| WCH-A03 discussion_spike 预筛选 | partial | 类型可创建，匹配与预筛选未实现 |
| WCH-A04 命中仅本人收件箱、事件不重复未读 | partial | hit 按 user 隔离、同事件去重；下次 Agent 对话未注入未读摘要 |
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
- Assistant enhanced_search：`app/assistant/rpc/internal/logic/chat_logic.go`、
  `app/assistant/rpc/internal/tool/registry.go`、`app/assistant/rpc/internal/store/state.go`。
- Assistant Agent Runtime：`app/assistant/rpc/internal/agent/`（runtime、runner、tools、search、recommend、intent）；
  记忆 `app/assistant/rpc/internal/memory/store.go`；Watch `app/assistant/watch`（`Match`/`ApplyPostEvent`）；matcher 进程 `app/assistant/mq`。
- Assistant 权威库：`deploy/sql/xbh_assistant.sql`，存量补丁
  `deploy/sql/patches/20260827_assistant_runtime.sql`、
  `deploy/sql/patches/20260827_agent_consent_version.sql`；DSN 为可选 `DB_ASSISTANT`。
- 网关 Assistant REST：`app/gateway/internal/logic/assistant/`（chat/consent/memory/watch/feedback）；
  契约 `proto/assistant/assistant.proto`、`app/gateway/gateway.api`。
- 行为：`app/behavior/rpc/internal/logic/record_events_logic.go`、`pkg/event/behavior.go`。
- 搜索：`app/search/rpc/internal/logic/search_logic.go`。
- 共享回源：`app/content/visibility`（把 Content `GetPostsByIds` 适配为 `visibilityx.Fetcher`，Assistant/Feed/Recommend/Search/Gateway 统一复用）。
- 网关：`app/gateway/internal/logic/**` 与 `app/gateway/gateway.api`；
  写接口仅 `CreatePostV2/UpdatePostV2/DeletePostV2`（`internal/logic/posts/*_v2_logic.go`），
  `/api/v1` 帖子写路由已废弃移除。

## 证据

Agent Runtime 映射验证于 2026-08-27，见
[2026-08-27-content-community-agent-runtime.md](evidence/2026-08-27-content-community-agent-runtime.md)。
Watch matcher 接线见
[2026-08-27-content-community-watch-matcher.md](evidence/2026-08-27-content-community-watch-matcher.md)。
`verified_commit` 为本映射提交。历史全量套件证据仍以 `evidence/` 既有记录为准。

未覆盖边界：无 `DB_ASSISTANT` 时记忆/Watch 列表为空；ASST-050/051 人类冻结集
与 live Gateway 评测不在本轮；UserState 工具、`agent_run` 落库、discussion_spike
匹配与命中回源仍缺。
