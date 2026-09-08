---
id: DES-content-community-backend
layer: design
title: 小白盒内容社区后端设计
status: active
owner: agent
updated_at: 2026-09-08
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

本设计说明如何以 go-zero 工程结构满足社区核心、发现、持久异步 Assistant Agent 与
反馈可靠性规范。Agent Runtime、记忆与 Watch 的细节以
[DES-assistant-agent-runtime](DES-assistant-agent-runtime.md) 为准。实现对齐状态以
[六域 IMP](../implementation/README.md) 和源码、`.api`、`.proto`、SQL、测试为准；本文不覆盖代码事实。

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

`Total` 只按本页过滤回减（CORE-015 / DISC-001）。全库精确计数需要索引与权威完全同步，
当前不做。

### 互动写路径

点赞/收藏在 Interaction 写入前经 Content `AssertInteractable` 确认目标可互动
（CORE-034）：帖子必须 published；评论必须有效且父帖 published。Content 不可用则失败关闭，
不写入关系。对不可用目标返回 `ContentNotFound`，与 CORE-016 一致。关注只校验用户身份。

公开计数：关系以 Interaction 为准；`post.like_count`/`favorite_count` 由 count-sync
异步收敛，目标 30 秒（CORE-032）。评论计数在 Content 事务内更新。

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

所有使用 sqlx 的服务入口在建立连接前调用 `sqlx.DisableLog()`，同时关闭正常、慢查询和错误路径的
参数展开 SQL 日志，覆盖私信及其他正文/认证数据。当前 go-zero 的同一 guard 也负责 SQL timing metric，
关闭后不声称该指标仍在；请求、RPC 与领域指标继续提供可观测性。AST wiring 回归与私信真实模型的
合成敏感值测试分别约束生产装配和日志行为，不依赖提高日志等级隐藏原文。

### 健康与观测

`/health` 存活，`/health/ready` 列出依赖；搜索/Assistant 可选，故障只标 `degraded`。
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
