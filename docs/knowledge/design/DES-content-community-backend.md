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

本设计说明如何以现有 go-zero 工程结构满足四份已批准规范。
实现对齐状态以 [IMP-content-community-backend](../implementation/IMP-content-community-backend.md)
和源码、`.api`、`.proto`、SQL、测试为准；本文不覆盖代码事实。

## 组件边界

- Gateway（`app/gateway`）：只绑定参数、调用 RPC、返回响应；鉴权中间件解析 JWT。
- RPC 服务：user / content / interaction / media / message / feed / search / recommend /
  assistant / behavior，各自持有 Model 与事务。
- MQ 消费（`app/*/mq`）：search 索引、embedding、feed fanout、message 通知、行为链路、
  推荐特征、清理任务。
- 可靠写入：权威业务写入与 outbox 同事务提交，异步效果经 relay 投递；
  不再使用 DTM，Content 契约已删除 QueryPrepared。
- 行为闭环：客户端事件 → behavior-rpc（校验+去重）→ RocketMQ → behaviorlog pipeline
  （去重+ClickHouse 存储）→ recommend-mq 特征更新。
- 权威可见性：Content 是帖子状态的唯一权威。Feed、Search、Recommend 和 Assistant 对外返回前
  必须回源 `GetPostsByIds`/`GetPost`。特征库与搜索索引只提供候选；状态验证失败时发现和
  Assistant 失败关闭。

## 方案

### 可见性

Content 持有帖子状态机与正文。普通读取走 `FindPostById`/`FindByIds` 且不读缓存，避免
CORE-053 允许的失效失败把已取消发布内容继续当成 published。公开列表在 SQL `status=1`
之后再次丢弃非 published 行。发现与 Assistant 只把 ES、向量库、inbox/outbox 当候选，
返回前必须经 `app/content/visibility` 回源 `GetPostsByIds`，由 `pkg/visibilityx` 验证 published；验证失败则整次请求
失败关闭，不得降级成“空结果成功”。

### 关注流

`/api/v2/feed/follow` 只对认证用户开放。读路径先分页拉取当前 following（页大小 100），
空关注立即返回空列表，不混入推荐。inbox 只保留当前关注作者；outbox 按作者分批
`IN` 查询后归并。这样取关后的新请求不会再露出旧 inbox，也覆盖 BigV 只写 outbox 的路径。

### 搜索

ES 只索引 published，并在取消发布时尽力删文档，但这是异步效果。查询时再次回源 Content：
丢掉不可见 ID，标题与摘要改用权威正文，`Total` 减去本页被过滤的条数。用户/标签子搜索
失败可以降级，帖子可见性失败不能降级。

### 写入

权威写入与事务 outbox 同提交，relay 投递 MQ。不再使用 DTM。content/interaction/media
已接入 `scripts/generate.sh`；Content 契约不再包含 `QueryPrepared`。

## 失败模式

- 内容权威不可用：发现/Assistant/公开回源读取失败关闭。
- 关注关系不可用：关注流失败关闭，避免按过期关系越权。
- 搜索引擎不可用：帖子搜索返回暂时不可用，不用空列表冒充无匹配。
- 缓存/索引/通知失败：不改变已提交的权威写成功（CORE-053）。

## 取舍

- 关注流用当前关系拉 outbox，而不是信任 inbox。读放大换正确性。
- 搜索 `Total` 只按本页过滤值回减，不重扫 ES。全库精确计数需要索引与权威完全同步，
  目前做不到，因此 DISC-001/CORE-015 仍标 partial。
- `CORE-013` 与 `CORE-062` 冲突，v1 仍允许 `expectedRevision=0`。未获人类决定前不改契约。

## 实现追踪

逐条对齐状态以 [IMP-content-community-backend](../implementation/IMP-content-community-backend.md) 为准。
本文只说明如何满足规范，不把实现台账标成设计结论。

## 验收策略

按规范逐条补实现并在对应服务补失败路径测试（AGENTS.md：不为通过测试改测试）。
离线评测门禁（DISC-060~063、ASST-050~051、REL-A05）需要冻结评测集与脚本，属于独立
验证工程；代码行为类验收（CORE-A*、DISC-A*、ASST-A*、REL-A* 的接口部分）以 Go 测试
与集成测试落地。
