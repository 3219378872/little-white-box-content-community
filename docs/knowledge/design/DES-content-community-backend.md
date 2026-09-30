---
id: DES-content-community-backend
layer: design
title: 小白盒内容社区后端设计
status: active
owner: agent
updated_at: '2026-09-28'
tracks:
- CORE-001
- CORE-002
- CORE-003
- CORE-004
- CORE-005
- CORE-010
- CORE-011
- CORE-012
- CORE-013
- CORE-014
- CORE-015
- CORE-016
- CORE-020
- CORE-021
- CORE-022
- CORE-023
- CORE-024
- CORE-030
- CORE-031
- CORE-032
- CORE-033
- CORE-034
- CORE-040
- CORE-041
- CORE-042
- CORE-043
- CORE-044
- CORE-050
- CORE-051
- CORE-052
- CORE-053
- CORE-054
- CORE-060
- CORE-061
- CORE-062
- CORE-063
- CORE-A01
- CORE-A02
- CORE-A03
- CORE-A04
- CORE-A05
- CORE-A06
- CORE-A07
- DISC-001
- DISC-002
- DISC-003
- DISC-004
- DISC-010
- DISC-011
- DISC-012
- DISC-020
- DISC-021
- DISC-022
- DISC-023
- DISC-030
- DISC-031
- DISC-032
- DISC-033
- DISC-034
- DISC-035
- DISC-036
- DISC-040
- DISC-041
- DISC-042
- DISC-050
- DISC-051
- DISC-052
- DISC-060
- DISC-061
- DISC-062
- DISC-063
- DISC-A01
- DISC-A02
- DISC-A03
- DISC-A04
- DISC-A05
- DISC-A06
- REL-001
- REL-002
- REL-003
- REL-004
- REL-005
- REL-006
- REL-007
- REL-008
- REL-010
- REL-011
- REL-012
- REL-013
- REL-020
- REL-021
- REL-022
- REL-023
- REL-024
- REL-030
- REL-031
- REL-032
- REL-033
- REL-040
- REL-041
- REL-042
- REL-043
- REL-044
- REL-045
- REL-050
- REL-051
- REL-052
- REL-053
- REL-054
- REL-054-01
- REL-054-02
- REL-054-03
- REL-054-04
- REL-054-05
- REL-054-06
- REL-054-07
- REL-054-08
- REL-054-09
- REL-054-10
- REL-054-11
- REL-054-12
- REL-060
- REL-061
- REL-A01
- REL-A02
- REL-A03
- REL-A04
- REL-A05
---

# 小白盒内容社区后端设计

本设计说明如何以 Kitex / Hertz 工程结构满足社区核心、发现、持久异步 Assistant Agent 与
反馈可靠性规范。Agent Runtime、记忆与 Watch 的细节以
[DES-assistant-agent-runtime](DES-assistant-agent-runtime.md) 为准。实现对齐状态以
[六域 IMP](../implementation/README.md) 和源码、`openapi.yaml`、`.proto`、SQL、测试为准；本文不覆盖代码事实。

> 2026-09-05：社区优先、问答、互联网补充与逐项引用由
> [社区研究设计](DES-agent-community-research.md)承接。下文旧协议的可选来源描述不能豁免新流程的
> 引用义务；既有基础机制继续适用，实际实现与验证状态见实现层。
> 本次仅登记边界，后续工作见 [迁移登记](../TRANSITION.md)。

## 组件边界

- Gateway（`app/gateway`）：HTTP 绑定、鉴权、SSE、BFF 组合。详情和列表的访问者点赞/收藏
  状态在 Gateway 回填 Interaction，不把 Interaction 客户端放进 Content。收藏隐私、回源过滤
  也在 Gateway 组合，因为没有独立 favorites 读模型跨库。
- RPC：user / content / interaction / media / message / feed / search / recommend /
  assistant / behavior，各自持有权威库与事务；独立 assistant-agent worker 只拥有运行执行职责。
- MQ：search 索引、embedding、feed fanout、行为管道、推荐特征、媒体清理、内容计数同步。
- 算法旁路（`algorithm/`）：可选在线推理与离线训练。推荐在超时或不可用时规则降级
  （DISC-036）；算法不拥有可见性。
- 可靠写入：权威业务与 outbox 同事务，relay 投递 RocketMQ。不再使用 DTM。
- 行为闭环：客户端事件 → behavior-rpc（校验+去重）→ RocketMQ → behaviorlog
  （ClickHouse）→ recommend-mq 特征。
- 权威可见性：Content 是帖子状态唯一权威。Feed、Search、Recommend、Assistant 和
  收藏列表把索引/inbox/召回当候选，返回前经 `app/content/visibility` + `pkg/visibilityx`
  回源；验证失败整次请求失败关闭。

## 方案

### 可见性与详情状态

Content 持有状态机与正文。`FindPostById`/`FindByIds` 不读缓存，避免 CORE-053 允许的
失效失败把已取消发布内容继续当成 published。公开列表 SQL `status=1` 后再丢弃非
published 行。

已认证 GetPost/列表的 `isLiked`/`isFavorited` 由 Gateway 调 Interaction
`BatchCheckLiked`/`BatchCheckFavorited` 回填（CORE-032）。Content proto 可保留这两个
字段但 Content Logic 不再填写；对外以 Gateway 为准。

用户资料 `GetUserResp.isFollowing` 按当前访问者读取 User 的权威关注表。Gateway 只从 OptionalAuth
验证过的上下文传递 `GetUserReq.viewer_id`，不接受客户端自报访问者；匿名/本人返回 false，关系查询
失败返回业务错误。该字段不写入公共 UserInfo 或用户缓存，关注与取消后的下一次读取直接反映关系。

`Total` 只按本页过滤回减（CORE-015 / DISC-001）。全库精确计数需要索引与权威完全同步，
当前不做。

### 互动写路径

点赞/收藏在 Interaction 写入前经 Content `AssertInteractable` 确认目标可互动
（CORE-034）：帖子必须 published；评论必须有效且父帖 published。Content 不可用则失败关闭，
不写入关系。对不可用目标返回 `ContentNotFound`，与 CORE-016 一致。关注只校验用户身份。

取消点赞/收藏的单关系读取直接查询 MySQL，禁止使用待失效的缓存来判定取消为无操作。关系变更、
计数和 outbox 仍由已有事务及条件更新原子提交，重复取消不重复扣减或投递。

公开计数：关系以 Interaction 为准；`post.like_count`/`favorite_count` 由 count-sync
异步收敛，目标 30 秒（CORE-032）。Interaction 的 action_count 锁事务生成单调 revision 与完整
计数快照；Content 在同事务提交事件收据、目标版本门槛、计数替换与派生 outbox，Redis 不承担
持久去重。乱序旧快照不能覆盖新状态；零到零无需 RowsAffected=1。存量卷必须暂停写入并完成
[权威基线迁移](../guides/count-projection-migration.md) 后才能确认历史无版本事件。
评论数量在 Content 事务内更新，评论点赞走同一版本快照协议。Content 的 `post.stats_seq`
由互动、评论和帖子 lifecycle 事务在同一 post 行锁下单调分配；lifecycle 计数载荷取自锁内当前行。
Search 独立比较正文 revision 与 stats_seq，保留删除墓碑，不再用 ES 内部 `_version` 代替业务版本。
存量统计序列须先覆盖现有 SQL/Search 与全部可重放旧事件水位；未初始化序列失败关闭。

### 认证凭据消费

手机号注册与验证码登录共用 Redis Lua：校验、共享失败次数与成功消费在一次操作内完成。成功码只
允许一个请求消费；错误达到上限即作废，Redis 执行失败不继续注册或签发登录凭据。
刷新轮换先生成后继令牌，再用 Lua 校验旧 jti 的归属、以 TTL/NX 登记后继并删除旧 jti。Redis Lua
不回滚运行错误，因此登记必须先于删除；登记失败保留旧凭据供恢复后重试。成功轮换仍拒绝旧凭据重放，
提交后响应丢失不提供旧令牌结果重放。

### 关注流

`/api/v2/feed/follow` 仅认证用户。先分页拉取当前 following（页大小 100），空关注立即
返回空列表，不混入推荐。inbox 只保留当前关注作者；outbox 按作者分批 `IN` 后归并。
读路径回源 Content；inbox 不在取消发布时主动撤回，新请求靠回源排除（DISC-011）。

### 搜索

ES 只索引 published，取消发布时尽力删文档。`post-update` 按 `post_id` 投递，载荷带
`revision`；索引写入用 external version，旧快照 409 丢弃。查询再回源 Content：丢掉
不可见 ID，标题与摘要改用权威正文，`Total` 按本页回减。用户/标签失败可降级并列出
`unavailableTypes`；帖子可见性或索引不可用不能降级成空成功。

### 推荐

候选来自规则召回，可选 OnlineInfer。匿名或关闭个性化只走规则冷启动（DISC-031）。
游标 HMAC 绑定身份/请求/场景/会话/实验/页大小，TTL 600s。作者配额、负反馈 30 天、
曝光 7 天按 DISC-034/035。返回前 `visibilityx` 过滤；可见性失败关闭，推理失败规则降级。
推荐可直连 ES/Milvus 作召回源，但仍必须回源 Content。候选特征按 `revision` 单调覆盖，
旧快照不回写。

推荐快照翻页以原始候选序列的实际消费位置推进，不能用过滤后条目数替代原始偏移。Recommend 的合法
空结果原样返回；参数错误、上下文不匹配和已有普通游标的失败不得重启为降级首页。仅首屏明确的依赖
不可用可进入 Feed 规则降级，回源可见性或已认证用户的负反馈读取不可用时仍失败关闭。

Feed 降级的 `feedv3` 签名游标绑定同样的身份、请求、场景、会话、实验和页大小，并指向 Redis 中的
不可变继续状态。状态保存每个来源的未消费候选、上游游标、耗尽标记、交织位置与已展示 ID；整个链
以首次请求的 600 秒到期点为准，不续期。每页重验负反馈、内容及关注关系，来源耗尽后不重新请求首页。
Redis 无法保存分页状态时不得返回无法继续的成功页；旧 `feedv*`、过期和状态缺失返回参数错误。
Feed 的 `FeatureVersion` 与 Recommend 生产者、读取端统一为 `v2`；配置回归比较三个实际 YAML，隔离
Redis 接线回归验证通过真实 ServiceContext 读取的负反馈命名空间，而不只验证替身中的 key。

### Assistant Agent

消息页虚拟线程由 assistant RPC 提供独立 read model，不创建 message 用户。所有用户输入先写
`xbh_assistant`，独立 worker 通过 MySQL lease 执行；断线不取消，事件从 MySQL 按序重放，Redis 仅
作通知。模型可直接对话并自主调用受授权工具，不再有 enhanced_search、模式字段或 Intent Router。
检索结果必须回源；`present_sources` 只把经复核的 run-local handle 发布为结构化来源卡。研究回答仍须
让实质信息就近关联实际取得且支持表述的帖子/网页 URL，不能以未调用 `present_sources`、只有正文链接
或卡片存在为由豁免；普通闲聊和澄清按 SPEC 明确例外。
完整运行、Memory、Watch、历史 BM25、compact 与预算设计见
[DES-assistant-agent-runtime](DES-assistant-agent-runtime.md)。

### 写入与私信

帖子写路径只有 `/api/v2/post*`，强制 `expectedRevision`（CORE-013/062）。帖子/评论/
媒体/互动/关注走事务 outbox。私信权威写入以 message 库提交为成功（CORE-044）；不实现
赞/评/关通知生产者。`message-push` 消费者不是当前产品路径，部署可不启动；主题保留不
构成对外能力。

帖子新增媒体只接受 Media RPC 验证为本人且上传完成的 `mediaIds`，`images` 若提供必须与解析的 URL
一致，不能覆盖校验结果。编辑允许保留或移除同帖原有 URL、加入新验证的媒体；旧 ID 失效时拒绝部分
保留，不把已知失效媒体改作 legacy URL 接受，用户仍可完整清空或以已验证 ID 整体替换。纯历史 URL
只能在同一作者的同一帖子内保留，不能在新建或其他帖子上作为新增引用使用。
已有图片统一解码当前 JSON 数组和历史逗号格式，创建后的局部保留、删减及重排沿用同一媒体归属校验。

公开更新请求的 `images` / `mediaIds` 缺省表示保留，显式空数组表示清空。Gateway 将 slice presence
写入内部 RPC 的 `images_provided` / `media_ids_provided`，避免 protobuf repeated 丢失空集合的存在性；
内容事务同时更新图片与媒体 ID，幂等摘要包含 presence。公开 Gateway API 字段不变。
幂等摘要仅为显式空集合追加 presence，缺省与既有非空集合保留原摘要格式，避免升级后旧命令重试
变成冲突；显式清空仍不能复用缺省命令的成功结果。

媒体图片在读取完整像素前用标准图片头解析校验 `8192 px / 25 MP`，每个 media-rpc 实例用容量 2 的
信号量覆盖完整解码、缩放和编码生命周期，生产容器限制为 512 MiB。对象上传先于权威行提交时，任一
后续步骤失败或幂等命中都删除本次随机对象；立即删除失败则写现有 media-delete outbox，由清理消费者
幂等重试，避免无数据库引用的对象静默累积。

### 行为与隐私

Gateway 接收白名单动作。曝光的 50%/1s 由客户端判定，服务端只强制 `(requestId, postId)`
去重（REL-004）。完整 IP 在写入 ClickHouse 前哈希。业务日志不写手机号、验证码、正文或
私信。关闭个性化后 recommend-mq 定时清特征，24 小时内完成（REL-023）。

SQL 通过 `pkg/sqlstore` 的 database/sql + sqlx 映射执行，代码路径不输出 SQL 或参数。
项目 `slog` 边界记录上下文；Kitex/Hertz 框架日志只记录级别、组件及调用位置，避免框架参数携带
请求、响应或内部错误。业务 RPC 中间件只记录方法、方向和耗时；生产构造入口由 AST 回归约束，
真实本地 RPC 与数据库失败路径验证敏感正文不进入日志。框架诊断不包含错误原文，排障应使用
方法、trace、业务错误码与领域指标关联。

### 健康与观测

`/health` 存活，`/health/ready` 列出依赖；搜索/Assistant 可选，故障只标 `degraded`。
Gateway 在独立诊断端口 9180 导出 `esx_gateway_requests_total` 与 `esx_gateway_duration_seconds`；
RPC 导出 `esx_rpc_requests_total` 与 `esx_rpc_duration_seconds`，Kitex tracer 到流关闭才结束计时。
Prometheus 开发/生产目标均指向诊断端口，SSE 与普通 HTTP 的业务端口不暴露 metrics。
MQ 消费者与 outbox relay 暴露 outcome 与延迟。SLO 报告由 `scripts/spec_evals.py slo`
按 REL-030~033 口径计算；正式关闭依赖真实月度数据。

## 失败模式

对应 `REL-054`：

- 权威库不可用：写/读返回 503。
- Redis 不可用：回源；已提交写仍成功。
- outbox relay 不可用：业务成功，异步延迟并告警。
- 行为 Broker 不可用：行为接收 503。
- 部分推荐来源不可用：规则降级并标记。
- 可见性不可用：发现与 Assistant 失败关闭。
- 帖子搜索索引不可用：搜索 503。
- LLM 不可用：run 明确错误终止，保留已提交文本、来源卡与副作用摘要。
- Agent 触达任一硬预算：`AGENT_RESOURCE_LIMIT`，已成功副作用保留且可审计。
- Agent 确认等待超时：数据库 CAS 拒绝，run 可继续收尾或明确终止。
- Assistant MySQL 不可用：拒绝接受新 run，不同步执行；Redis 不可用则 SSE 轮询 MySQL。
- 指标后端不可用：业务继续。

## 取舍

- `Total` 不重扫全库，换可实现的可见性保证。
- 详情互动状态放在 Gateway 而不是 Content，避免 Content 依赖 Interaction。
- 点赞前同步问 Content，换 CORE-034/016，增加 Interaction→Content 边。
- 不把赞/评/关通知做成产品，避免死消费者冒充完整通知系统。
- 曝光视口不在服务端复测，换可实现的去重边界。
- 算法旁路可选；未达 DISC-062 门槛不宣称学习改善。

## 验收策略

代码行为类（CORE-A*、DISC-A01~A05、AGENT-A01~A06、REL-A01~A04 的接口部分）用 Go 测试
落地，每个改动的 Logic 至少一条失败路径。DISC-A06 需要人类冻结集，REL-A05 需要真实观测；
两者由 [开放门禁](../status/open-gates.md) 汇总，权威状态留在对应 domain IMP，未取得合格 EVD 时禁止
标 `aligned`。Hermes Agent 的异步恢复、compact、历史召回和来源 ledger 以精确 AGENT 验收条款核对。

## 生产部署与迁移

开发中间件宿主端口只绑定回环地址。production overlay 清空中间件和内部服务继承的 host ports，只有
Nginx 发布 HTTP/HTTPS；边缘统一设置 CSP、HSTS、nosniff、Referrer-Policy 和 frame protection，入口
HTML/启动脚本禁缓存，带内容指纹的静态资产长期 immutable。

生产 SQL 严格分成 `production-migrate` 与 `production-up` 两阶段。迁移命令绑定预期 MySQL
`server_uuid`，以补丁名和 SHA-256 写独立 ledger，已登记补丁内容变化立即失败。首次缺少
`assistant_runtime_v3` marker 的破坏性重置必须先用独立命令分别备份并校验 `xbh_assistant` 与 Agent
consent；准备命令只写备份、不执行 SQL，并输出绑定 target UUID、patch checksum 与备份 manifest
SHA-256 的精确确认值。后续 `production-migrate` 重新验证文件、内容标记和 checksum 后才接受该值；
`production-up` 只检查 pending/checksum，存在待迁移项时失败，不代替操作员迁移。

MySQL 空卷只白名单挂载 MySQL schema，ClickHouse schema 不进入 MySQL initdb；健康检查必须带配置的
root 凭据完成真实认证，不能把 Access denied 当作健康。

## 2026-09-25 审查修复

密码失败锁定以查询得到的稳定用户 ID 为键；检查和累加均用 Redis Lua 原子维护固定窗口，
同时修复已存在但没有 TTL 的计数。用户名大小写或重音等价形式不能拆分同一账户的尝试次数。
部署切换时旧的用户名锁键不再沿用；Redis 故障仍遵守既有 fail-open 登录策略。

关闭个性化的持久偏好归 User RPC 所有。Redis 关闭标记仅用于快速拒绝；标记缺失、过期或读取
失败时，推荐与消费者回查 User RPC，权威状态未知时不建立画像。消费者的作者发帖分发也检查
接收者偏好。定时清理以 SCAN 枚举现存用户特征与召回键，回查偏好，清除场景召回及关注分发成员，
不依赖关闭标记仍存在。新增权威 RPC 调用尚无容量测试证据。

推荐游标冻结排序但不冻结用户的明确隐藏反馈：每次续页重读负反馈并按原快照推进偏移。
关闭个性化后旧个性化快照失效；关闭后新建的规则快照可继续分页。依赖读取失败不返回未经核验的
个性化或隐藏条目。

## 私信视频和音频上传（2026-09-27）

承接 CORE-023/024/040/041/042/050/051。Gateway 的 `/api/v1/media/video|audio` 是认证 multipart
端点，使用 `file` 与必填 `idempotencyKey`（最长 128 字符）；图片旧入口继续兼容可选键。响应包含
mediaId、url、fileType、mimeType、fileSize；不伪造缩略图、时长，也不承诺转码或客户端编码兼容。

上传处理把超过 1 MiB 的文件暂存磁盘，总请求预算比文件预算多 1 MiB，解析完仍按文件实际字节校验，
所有路径移除临时文件。Hertz 开启流式 body 且禁用 multipart 预解析；路由校验 Content-Length，
未知长度请求也在读取时执行有界限制，超限返回统一业务 JSON。Gateway 图片路由 120 秒，视频/音频
300 秒；普通路由保持原限制。SSE 不继承普通请求的总时长限制。
视频复用 UploadVideo；音频新增 UploadAudio，复用带限额的 TempSink、内容哈希、事务媒体写入和
对象补偿/outbox。原图片/视频幂等指纹不变，音频使用独立 media:upload:audio scope，避免跨类型命中。

ISO 容器递归读取 moov/trak/mdia/hdlr，视频必须含 vide 轨，音频必须含 soun 且不含 vide；
WebM/Matroska 读取 TrackType，音频仅接受 MP3/WAV/M4A。解析边界、扩展长度与层数受控，
不把文件名、请求 MIME 或品牌头当作轨道证明。该检查识别类型，不证明完整解码或可播放性。

Message RPC 在权威写入前检查本人、完成状态和 msgType/fileType 对应关系，持久化媒体记录 URL，
不使用客户端提供的媒体 URL。发送重试继续遵循现有消息幂等键；客户端上传键与消息键相互独立。

## 框架与公开契约迁移（2026-09-28）

内部十个业务服务使用 Kitex v0.16.2，保持 Protobuf wire package、方法和字段编号；生成代码在
`kitex_gen`。客户端 façade 与服务端分发由 IDL 同步生成，Python embedding/inference 继续使用
标准 gRPC。etcd 注册前缀为 `/little/kitex`，逻辑服务名不变；整栈切换，不能把旧实例混入新发现域。
内部 HMAC 与业务错误 details 经过显式 Kitex/标准 gRPC 转换，trace 与取消沿上下文传播。

Gateway 使用 Hertz v0.10.6。`app/gateway/openapi.yaml` 是唯一公开契约源，描述参数位置、必填、
默认值、认证、multipart 和 SSE；项目生成器输出 DTO、原生路由，前端生成器输出兼容 Dart DTO/API。
应用自有传输继续负责无损 ID、刷新重试、媒体流和 SSE 游标。普通 RPC 超时不限制流的整体生命周期。
readiness 主动检查标准 gRPC Health，并保持必需/可选依赖的 ready/degraded/unavailable 区分。

数据库结构与业务幂等键不变。模型缓存改用 `cache:v2:` 前缀；二级索引保存主键引用，主键失效后
通过索引访问会重新查库。正缓存 7 天、负缓存 60 秒；缓存不可用可回源，已提交写入不因失效失败
改报失败。入口退出先停止服务器/消费者、排空 outbox，再释放 RPC、SQL、Redis 与诊断资源。

迁移过程证据绑定实际提交；历史 EVD 不证明迁移后的运行。生产容量、月度 SLO 与设备门禁仍独立。
