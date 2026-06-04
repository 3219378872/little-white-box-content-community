# Feed Week2 重做设计文档

**日期**: 2026-04-23
**范围**: `proto/feed`、`app/feed`、`deploy/sql/xbh_feed.sql`
**关联文档**: `doc/phases/phase-2-interaction.md` Week2

---

## 1. 概述

按 `doc/phases/phase-2-interaction.md` 的 Week2 设计重做 Feed 服务，统一收敛到“关注流分发系统”语义，不再延续仓库里现有的“活动流 / timeline / hot_post”旧设计。

本次重做的目标是交付一个可运行、可测试、符合 go-zero 三层架构的 Feed RPC 服务，支持：

- 关注流读取
- 推送到收件箱
- 小V写扩散 / 大V拉模型
- 推荐流最小可用实现

本次不包含：

- 复杂推荐算法
- Feed 网关接入
- DTM 联动改造
- 旧 Feed 接口兼容层

## 2. 设计目标

### 2.1 业务目标

- 关注流基于用户关注关系返回帖子列表
- 小V作者发帖时直接写扩散到粉丝收件箱
- 大V作者发帖时只写发件箱，读关注流时实时合并
- 推荐流提供最小可用返回路径，为后续增强预留接口

### 2.2 工程目标

- 与 `phase-2-interaction.md` 的接口和模型语义一致
- 遵守仓库约定：Handler/Logic/Model 分层、ctx 透传、`errx` 统一错误
- 避免沿用旧 Feed 结构造成语义混乱
- 为 Week3 消息服务和 Week4 DTM 集成保留稳定边界

## 3. 当前问题

仓库当前 Feed 设计与 Week2 文档存在根本不一致：

- `proto/feed/feed.proto` 当前定义的是 `GetFollowingFeed`、`GetUserFeed`、`PushFeed`
- `deploy/sql/xbh_feed.sql` 当前定义的是 `feed`、`timeline`、`hot_post`
- 当前设计更偏“动态事件流 / 活动流”，而非“关注流分发系统”
- 当前表结构未将“小V推、大V拉”建模为一等能力

如果继续在现有结构上增补功能，会导致：

- 服务语义继续混杂
- 后续 MQ 写扩散实现不自然
- 大V优化只能通过额外补丁拼接，难以维护

因此本次采用“同路径重写、语义完全收敛到文档方案”的方式。

## 4. 总体架构

### 4.1 服务边界

`FeedService` 仅负责“关注流分发与读取”，不负责：

- 点赞/评论/关注等活动事件流展示
- 独立热榜存储
- 复杂推荐排序模型

### 4.2 依赖关系

```
Post Published Event
        │
        ▼
  Feed Consumer
        │
        ├── User RPC     (获取作者粉丝数 / 粉丝列表 / 关注列表)
        ├── MySQL        (feed_outbox / feed_inbox)
        └── Redis        (可选热点收件箱优化)

Client / Gateway
        │
        ▼
     Feed RPC
        │
        ├── User RPC     (批量用户信息、关注列表)
        ├── Content RPC  (批量帖子详情 / 推荐源)
        ├── MySQL        (收件箱 / 发件箱查询)
        └── Redis        (热点游标、去重、短期缓存)
```

### 4.3 核心读写路径

写路径：

1. 作者发帖成功后发布 `post_published` 事件
2. Feed consumer 根据作者粉丝数判断小V / 大V
3. 无论大小V，统一写作者 `feed_outbox`
4. 若作者为小V，额外批量写入粉丝 `feed_inbox`

读路径：

1. `GetFollowFeed` 先读取当前用户 `feed_inbox`
2. 再查询当前用户已关注的大V作者集合
3. 从这些作者的 `feed_outbox` 拉取候选帖子
4. 服务内合并排序并截断分页
5. 通过 `Content.GetPostsByIds` 和 `User.BatchGetUsers` 回填展示字段

## 5. RPC 设计

`proto/feed/feed.proto` 按文档语义重写为以下接口：

```protobuf
service FeedService {
  rpc GetFollowFeed(GetFollowFeedReq) returns (GetFollowFeedResp);
  rpc GetRecommendFeed(GetRecommendFeedReq) returns (GetRecommendFeedResp);
  rpc PushToInbox(PushToInboxReq) returns (PushToInboxResp);
}
```

### 5.1 GetFollowFeed

职责：

- 返回当前用户的关注流
- 聚合“小V收件箱内容”和“大V发件箱实时拉取内容”
- 不直接承载帖子全文和作者完整资料，由下游 RPC 回填

建议请求：

```protobuf
message GetFollowFeedReq {
  int64 user_id = 1;
  int64 cursor_created_at = 2;
  int64 cursor_post_id = 3;
  int32 page_size = 4;
}
```

建议响应：

```protobuf
message FeedItem {
  int64 post_id = 1;
  int64 author_id = 2;
  int64 created_at = 3;
  int32 feed_type = 4; // 1=follow, 2=recommend
}

message GetFollowFeedResp {
  repeated FeedItem items = 1;
  bool has_more = 2;
  int64 next_cursor_created_at = 3;
  int64 next_cursor_post_id = 4;
}
```

说明：

- 不继续沿用旧 `last_id` 单游标，改为稳定的复合游标，避免跨源合并时漏读或重复
- 若实施阶段决定必须保持文档中的 `last_id`，则需要在计划中明确降级风险

### 5.2 GetRecommendFeed

职责：

- 提供最小可用推荐流入口
- 先通过 Content 服务复用现有推荐 / 最新帖子能力
- 不在本次引入独立推荐存储或热度衰减模型

建议请求：

```protobuf
message GetRecommendFeedReq {
  int64 user_id = 1;
  int32 page = 2;
  int32 page_size = 3;
}
```

建议响应：

```protobuf
message GetRecommendFeedResp {
  repeated FeedItem items = 1;
  bool has_more = 2;
}
```

### 5.3 PushToInbox

职责：

- 供内部 consumer / 补偿任务写入粉丝收件箱
- 不作为网关公开业务接口使用

建议请求：

```protobuf
message PushToInboxReq {
  int64 author_id = 1;
  int64 post_id = 2;
  int64 created_at = 3;
  repeated int64 follower_ids = 4;
}
```

建议响应：

```protobuf
message PushToInboxResp {
  int64 pushed_count = 1;
}
```

## 6. 数据模型设计

`deploy/sql/xbh_feed.sql` 重写为两张核心表。

### 6.1 发件箱表 `feed_outbox`

```sql
CREATE TABLE `feed_outbox` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `author_id` bigint NOT NULL,
  `post_id` bigint NOT NULL,
  `created_at` bigint NOT NULL COMMENT 'Unix毫秒时间戳',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_author_post` (`author_id`, `post_id`),
  KEY `idx_author_created` (`author_id`, `created_at`, `id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
```

用途：

- 存储作者自己的发件记录
- 所有作者统一写入
- 大V关注流拉取时直接从此表查询

### 6.2 收件箱表 `feed_inbox`

```sql
CREATE TABLE `feed_inbox` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `user_id` bigint NOT NULL,
  `author_id` bigint NOT NULL,
  `post_id` bigint NOT NULL,
  `created_at` bigint NOT NULL COMMENT 'Unix毫秒时间戳',
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_user_post` (`user_id`, `post_id`),
  KEY `idx_user_created` (`user_id`, `created_at`, `id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
```

用途：

- 存储粉丝收件箱
- 小V发帖时批量写入
- `uk_user_post` 保证幂等和重复投递防重

### 6.3 删除旧模型

旧 `feed`、`timeline`、`hot_post` 模型整体移除，不保留语义兼容层。

原因：

- 与新方案目标不一致
- 会干扰实现边界
- 当前仓库中未看到已落地的新功能依赖旧 Feed 读写

## 7. 业务规则

### 7.1 小V / 大V判定

- 通过 `User.GetUser` 获取作者 `follower_count`
- 默认阈值 `10000`
- 阈值必须来自 Feed 配置，不允许硬编码在逻辑层

规则：

- `follower_count < threshold` 视为小V
- `follower_count >= threshold` 视为大V

### 7.2 发帖分发规则

- 小V：写 `feed_outbox` + 写所有粉丝 `feed_inbox`
- 大V：只写 `feed_outbox`

### 7.3 关注流读取规则

- 普通已关注作者内容优先从 `feed_inbox` 获取
- 已关注大V内容从 `feed_outbox` 实时拉取
- 合并后统一按 `(created_at desc, post_id desc)` 排序

### 7.4 推荐流规则

- 优先复用 `Content.GetPostList(sort_by=3)`
- 若推荐能力未就绪，回退到最新帖子
- Feed 服务不负责复杂推荐策略

## 8. 逻辑层设计

### 8.1 GetFollowFeedLogic

主要步骤：

1. 校验 `user_id` 和 `page_size`
2. 查询用户关注列表
3. 根据关注用户资料识别大V集合
4. 查询当前用户 `feed_inbox`
5. 查询大V作者 `feed_outbox`
6. 合并、排序、截断
7. 调用 `Content.GetPostsByIds`
8. 调用 `User.BatchGetUsers`
9. 组装响应

注意点：

- 下游 RPC 失败要返回 `errx` 业务错误
- 合并排序必须稳定，避免相同时间戳下顺序抖动
- 读取逻辑不能依赖 `context.Background()`

### 8.2 GetRecommendFeedLogic

主要步骤：

1. 校验分页参数
2. 调用 Content 推荐源
3. 将结果映射为 `feed_type=2`
4. 返回最小可用推荐流

### 8.3 PushToInboxLogic

主要步骤：

1. 校验 `author_id`、`post_id`、`created_at`
2. 批量写入 `feed_inbox`
3. 利用唯一键保证幂等
4. 返回成功写入数量

注意点：

- 批量写入应尽量使用单条多值插入或分批插入，避免 N+1
- 对重复投递不返回业务错误

## 9. Consumer 设计

新增 Feed consumer，消费帖子发布事件。

### 9.1 输入事件

事件最小字段：

```go
type PostPublishedEvent struct {
    PostId         int64 `json:"post_id"`
    AuthorId       int64 `json:"author_id"`
    CreatedAt      int64 `json:"created_at"`
}
```

如事件中已携带粉丝数，可减少一次用户 RPC；否则由 consumer 查询用户信息。

### 9.2 消费流程

1. 反序列化事件
2. 查询作者资料
3. 写 `feed_outbox`
4. 若为小V，查询粉丝列表
5. 调用 `PushToInbox` 或直接批量写 `feed_inbox`
6. 记录带 `ctx` 的结构化日志

### 9.3 幂等性

- `feed_outbox` 依赖 `uk_author_post`
- `feed_inbox` 依赖 `uk_user_post`
- 重复消费不应制造重复数据

### 9.4 失败策略

- 反序列化失败：记录日志并丢弃
- 用户服务 / 数据库暂时失败：返回错误，由消费框架重试
- 部分批量插入重复：忽略重复项，保留成功项

## 10. 配置设计

Feed 配置新增：

```yaml
Name: feed.rpc
ListenOn: 0.0.0.0:9091

Etcd:
  Hosts:
    - ${ETCD_HOST}
  Key: feed.rpc

DataSource: ${FEED_MYSQL_DSN}

Redis:
  Host: ${REDIS_HOST}
  Type: node

UserRpc:
  Etcd:
    Hosts:
      - ${ETCD_HOST}
    Key: user.rpc

ContentRpc:
  Etcd:
    Hosts:
      - ${ETCD_HOST}
    Key: content.rpc

BigVThreshold: 10000
InboxFanoutBatchSize: 500
```

要求：

- 敏感值使用环境变量占位
- 逻辑层从配置读取阈值和批量大小

## 11. 错误处理

按仓库统一规范，Logic 层必须返回 `errx.New(...)` 或仓库已有等价封装，不使用裸字符串错误。

建议新增或复用以下错误语义：

- 参数非法
- 用户不存在
- 下游内容服务失败
- 下游用户服务失败
- 分发写入失败

处理原则：

- Handler / gRPC server 不自行拼 HTTP 或 status 码
- 统一交给 `errx` 和拦截器映射

## 12. 目录与文件边界

计划中的主要文件边界：

- `proto/feed/feed.proto`
  - Feed RPC 契约
- `deploy/sql/xbh_feed.sql`
  - Feed 表结构
- `app/feed/...`
  - 新 Feed RPC 服务代码
- `app/feed/internal/model/`
  - `feed_outbox` / `feed_inbox` model 及自定义查询
- `app/feed/internal/logic/`
  - `get_follow_feed_logic.go`
  - `get_recommend_feed_logic.go`
  - `push_to_inbox_logic.go`
- `app/feed/internal/svc/service_context.go`
  - DB、Redis、UserRpc、ContentRpc 注入

说明：

- 生成代码由 `goctl` 负责，禁止直接手改生成产物
- 自定义 SQL 放在非 `*_gen.go` 文件

## 13. 测试策略

严格按 TDD 执行，先写失败测试，再写最小实现。

### 13.1 单元测试

`GetFollowFeedLogic`：

- 仅收件箱数据时返回正确
- 同时存在收件箱和大V发件箱时合并排序正确
- 分页截断与 `has_more` 正确
- 用户 RPC 失败时返回业务错误
- 内容 RPC 失败时返回业务错误

`GetRecommendFeedLogic`：

- 推荐源成功时返回 `feed_type=2`
- 推荐源失败且存在回退路径时正确回退

`PushToInboxLogic`：

- 正常批量写入成功
- 重复写入幂等
- 非法参数返回错误

### 13.2 集成测试

- `feed_outbox` 插入与按作者分页查询
- `feed_inbox` 批量插入与按用户分页查询
- 小V写扩散：outbox + inbox 都落库
- 大V写扩散：仅 outbox 落库

### 13.3 验收标准

- Feed RPC 服务能够启动并注册 etcd
- `GetFollowFeed` 能正确返回关注流
- 小V / 大V分发策略符合文档
- `PushToInbox` 幂等
- `GetRecommendFeed` 有最小可用结果

## 14. 风险与取舍

### 14.1 复合游标与文档 `last_id` 不一致

Week2 文档示例使用 `last_id`，但跨源合并场景下单游标稳定性不足。本设计优先选择复合游标，原因是：

- 更适合收件箱 + 发件箱合并
- 可避免同时间戳下翻页重复 / 漏读
- 后续扩展推荐流也更稳定

如果实施时必须保留 `last_id`，应在计划中显式记录这一降级及补救策略。

### 14.2 推荐流仅做最小实现

本期目标是完成 Week2 Feed 基础能力，不引入：

- 独立热榜表
- 热度衰减
- 个性化召回

这是有意缩小范围，避免阻塞主链路交付。

### 14.3 旧 Feed 设计直接下线

本次不做兼容层。若后续发现仓库某处仍依赖旧 Feed 接口，需要在实施阶段补做迁移或同步修改。

## 15. 结论

本设计以文档 Week2 为唯一基准，重做 Feed 为“关注流分发系统”：

- RPC 契约重写
- 表结构重写为 `feed_outbox` + `feed_inbox`
- 小V写扩散 / 大V拉模型落地
- 推荐流保持最小可用
- 不保留旧活动流语义和兼容层

该方案边界清晰、实现路径明确，并能为后续消息通知、DTM 联动和推荐增强提供稳定基础。
