# Phase 2: 互动功能

## 概述

### 阶段目标
实现互动、Feed流、消息通知功能，引入DTM分布式事务和RocketMQ异步消息处理。

### 预计周期
5 周

### 前置条件
- Phase 1 已完成
- Gateway/User/Content/Media 服务正常运行
- 中间件（etcd/MySQL/Redis/RocketMQ）正常运行

---

## 详细任务清单

### W1: 互动服务

#### 任务 1.1: 创建 Interaction RPC 服务
**涉及模块**: `app/interaction/rpc/`

**生成命令**:
```bash
cd app/interaction/rpc
goctl rpc protoc ../../proto/interaction/interaction.proto --go_out=. --go-grpc_out=. --zrpc_out=. --style go_zero
```

**interaction.proto 定义**:
```protobuf
syntax = "proto3";
package interaction;
option go_package = "./pb";

service InteractionService {
  rpc Like(LikeReq) returns (LikeResp);
  rpc Unlike(UnlikeReq) returns (UnlikeResp);
  rpc Favorite(FavoriteReq) returns (FavoriteResp);
  rpc Unfavorite(UnfavoriteReq) returns (UnfavoriteResp);
  rpc GetCounts(GetCountsReq) returns (GetCountsResp);
  rpc CheckUserAction(CheckUserActionReq) returns (CheckUserActionResp);
  rpc GetLikeList(GetLikeListReq) returns (GetLikeListResp);
  rpc GetFavoriteList(GetFavoriteListReq) returns (GetFavoriteListResp);
}

message LikeReq {
  int64 user_id = 1;
  int64 target_id = 2;
  int32 target_type = 3;  // 1=帖子, 2=评论
}

message GetCountsReq {
  int64 target_id = 1;
  int32 target_type = 2;
}

message GetCountsResp {
  int64 like_count = 1;
  int64 favorite_count = 2;
  int64 comment_count = 3;
  int64 share_count = 4;
}
```

**验收标准**:
- [ ] Interaction RPC 服务启动成功
- [ ] 服务注册到 etcd

---

#### 任务 1.2: 生成 Model
**涉及模块**: `app/interaction/rpc/internal/model/`

**生成命令**:
```bash
goctl model mysql ddl \
  -src deploy/sql/xbh_interaction.sql \
  -dir app/interaction/rpc/internal/model \
  -cache --style go_zero
```

**数据表设计**:
```sql
-- 点赞表
CREATE TABLE `like_record` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `user_id` bigint NOT NULL,
  `target_id` bigint NOT NULL,
  `target_type` tinyint NOT NULL COMMENT '1=帖子, 2=评论',
  `created_at` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_user_target` (`user_id`, `target_id`, `target_type`),
  KEY `idx_target` (`target_id`, `target_type`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 收藏表
CREATE TABLE `favorite_record` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `user_id` bigint NOT NULL,
  `target_id` bigint NOT NULL,
  `created_at` timestamp NOT NULL DEFAULT CURRENT_TIMESTAMP,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_user_target` (`user_id`, `target_id`),
  KEY `idx_target` (`target_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;

-- 计数表
CREATE TABLE `action_count` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `target_id` bigint NOT NULL,
  `target_type` tinyint NOT NULL,
  `like_count` bigint NOT NULL DEFAULT 0,
  `favorite_count` bigint NOT NULL DEFAULT 0,
  `comment_count` bigint NOT NULL DEFAULT 0,
  `share_count` bigint NOT NULL DEFAULT 0,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_target` (`target_id`, `target_type`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
```

**验收标准**:
- [ ] Model 代码生成成功
- [ ] 缓存配置正确

---

#### 任务 1.3: 实现点赞功能
**涉及模块**: `app/interaction/rpc/internal/logic/`

**技术要点**:
```go
func (l *LikeLogic) Like(in *interaction.LikeReq) (*interaction.LikeResp, error) {
    // 1. 检查是否已点赞
    exists, err := l.svcCtx.LikeRecordModel.FindOne(
        l.ctx, in.UserId, in.TargetId, in.TargetType)
    if err != nil && !errors.Is(err, model.ErrNotFound) {
        return nil, err
    }
    if exists != nil {
        return nil, status.Error(codes.AlreadyExists, "已经点赞过了")
    }

    // 2. 写入点赞记录
    err = l.svcCtx.LikeRecordModel.Insert(l.ctx, &model.LikeRecord{
        UserId:     in.UserId,
        TargetId:   in.TargetId,
        TargetType: in.TargetType,
    })
    if err != nil {
        return nil, err
    }

    // 3. 发送 MQ 消息异步更新计数
    err = l.svcCtx.MqProducer.Send(l.ctx, &mqx.Message{
        Topic: mqx.TopicUserAction,
        Tag:   "like",
        Body:  in,
    })

    return &interaction.LikeResp{Success: true}, nil
}
```

**验收标准**:
- [ ] 点赞功能正常
- [ ] 重复点赞返回错误
- [ ] MQ 消息发送成功

---

#### 任务 1.4: 实现收藏功能
**涉及模块**: `app/interaction/rpc/internal/logic/`

**功能列表**:
- 收藏帖子
- 取消收藏
- 收藏列表

**验收标准**:
- [ ] 收藏/取消收藏正常
- [ ] 收藏列表分页正确

---

#### 任务 1.5: 实现计数服务
**涉及模块**: `app/interaction/rpc/internal/logic/`

**技术要点**:
- 使用 Redis 缓存热点计数
- 定时同步 Redis → MySQL
- MQ 消费异步更新

**验收标准**:
- [ ] 计数实时性 < 1s
- [ ] 缓存命中率 > 90%

---

### W2: Feed 服务

#### 任务 2.1: 创建 Feed RPC 服务
**涉及模块**: `app/feed/rpc/`

**生成命令**:
```bash
cd app/feed/rpc
goctl rpc protoc ../../proto/feed/feed.proto --go_out=. --go-grpc_out=. --zrpc_out=. --style go_zero
```

**feed.proto 定义**:
```protobuf
syntax = "proto3";
package feed;
option go_package = "./pb";

service FeedService {
  rpc GetFollowFeed(GetFollowFeedReq) returns (GetFollowFeedResp);
  rpc GetRecommendFeed(GetRecommendFeedReq) returns (GetRecommendFeedResp);
  rpc PushToInbox(PushToInboxReq) returns (PushToInboxResp);
}

message GetFollowFeedReq {
  int64 user_id = 1;
  int64 last_id = 2;
  int32 page_size = 3;
}

message FeedItem {
  int64 post_id = 1;
  int64 author_id = 2;
  int64 created_at = 3;
  int32 feed_type = 4;  // 1=关注, 2=推荐
}

message GetFollowFeedResp {
  repeated FeedItem items = 1;
  bool has_more = 2;
}
```

**验收标准**:
- [ ] Feed RPC 服务启动成功

---

#### 任务 2.2: 实现关注流（推拉结合）
**涉及模块**: `app/feed/rpc/internal/logic/`

**推拉结合策略**:
```
用户发帖时:
  ├── 小V用户（粉丝 < 1万）: 写扩散，直接推送到粉丝收件箱
  └── 大V用户（粉丝 >= 1万）: 只写发件箱，粉丝拉取时实时查询

读取关注流时:
  ├── 小V用户: 直接从收件箱读取
  └── 大V用户: 合并收件箱 + 实时拉取大V发件箱
```

**技术实现**:
```go
func (l *GetFollowFeedLogic) GetFollowFeed(in *feed.GetFollowFeedReq) (*feed.GetFollowFeedResp, error) {
    // 1. 获取关注的大V列表
    bigFollowings, _ := l.svcCtx.UserRpc.GetBigFollowings(l.ctx, &user.GetBigFollowingsReq{
        UserId: in.UserId,
    })

    // 2. 从收件箱读取（小V的内容）
    inboxItems, _ := l.svcCtx.FeedModel.GetInbox(l.ctx, in.UserId, in.LastId, in.PageSize)

    // 3. 实时拉取大V发件箱
    var bigVItems []*FeedItem
    for _, bigV := range bigFollowings.Users {
        items, _ := l.svcCtx.FeedModel.GetOutbox(l.ctx, bigV.Id, in.LastId, 10)
        bigVItems = append(bigVItems, items...)
    }

    // 4. 合并排序
    allItems := mergeAndSort(inboxItems, bigVItems)

    return &feed.GetFollowFeedResp{
        Items:   allItems[:in.PageSize],
        HasMore: len(allItems) > int(in.PageSize),
    }, nil
}
```

**验收标准**:
- [ ] 关注流正常显示
- [ ] 推拉结合逻辑正确
- [ ] 大V用户优化生效

---

#### 任务 2.3: 实现推送到收件箱
**涉及模块**: `app/mq/feed-consumer/`

**MQ 消费者**:
```go
// 消费帖子发布事件，写扩散
func (c *FeedConsumer) Consume(msg *primitive.MessageExt) error {
    var event PostPublishEvent
    json.Unmarshal(msg.Body, &event)

    // 判断是否大V
    if event.FollowerCount < 10000 {
        // 写扩散：推送到所有粉丝收件箱
        followers, _ := c.userRpc.GetFollowers(ctx, &user.GetFollowersReq{
            UserId: event.AuthorId,
        })

        pipe := c.redis.Pipeline()
        for _, follower := range followers.Users {
            key := fmt.Sprintf("inbox:%d", follower.Id)
            pipe.ZAdd(ctx, key, &redis.Z{
                Score:  float64(event.CreatedAt),
                Member: event.PostId,
            })
            // 保留最近 1000 条
            pipe.ZRemRangeByRank(ctx, key, 0, -1001)
        }
        pipe.Exec(ctx)
    }

    // 写入发件箱
    c.feedModel.InsertOutbox(ctx, &model.FeedOutbox{
        AuthorId:  event.AuthorId,
        PostId:    event.PostId,
        CreatedAt: event.CreatedAt,
    })

    return nil
}
```

**验收标准**:
- [ ] 消费者正常启动
- [ ] 写扩散正常工作
- [ ] 收件箱数据正确

---

### W3: 消息服务

#### 任务 3.1: 创建 Message RPC 服务
**涉及模块**: `app/message/rpc/`

**生成命令**:
```bash
cd app/message/rpc
goctl rpc protoc ../../proto/message/message.proto --go_out=. --go-grpc_out=. --zrpc_out=. --style go_zero
```

**message.proto 定义**:
```protobuf
syntax = "proto3";
package message;
option go_package = "./pb";

service MessageService {
  rpc GetNotifications(GetNotificationsReq) returns (GetNotificationsResp);
  rpc MarkAsRead(MarkAsReadReq) returns (MarkAsReadResp);
  rpc GetUnreadCount(GetUnreadCountReq) returns (GetUnreadCountResp);
  rpc SendNotification(SendNotificationReq) returns (SendNotificationResp);
}

message Notification {
  int64 id = 1;
  int64 user_id = 2;
  int32 type = 3;  // 1=点赞, 2=评论, 3=关注, 4=系统
  int64 sender_id = 4;
  int64 target_id = 5;
  string content = 6;
  bool is_read = 7;
  int64 created_at = 8;
}

message GetNotificationsReq {
  int64 user_id = 1;
  int32 type = 2;  // 0=全部
  int64 last_id = 3;
  int32 page_size = 4;
}
```

**验收标准**:
- [ ] Message RPC 服务启动成功

---

#### 任务 3.2: 实现通知生成
**涉及模块**: `app/mq/message-consumer/`

**通知类型**:

| 类型 | 触发事件 | 模板 |
|------|---------|------|
| 点赞 | 用户点赞帖子 | "{username} 赞了你的帖子" |
| 评论 | 用户评论帖子 | "{username} 评论了你的帖子" |
| 关注 | 用户关注你 | "{username} 关注了你" |
| 系统 | 系统公告 | "{content}" |

**MQ 消费者**:
```go
func (c *MessageConsumer) Consume(msg *primitive.MessageExt) error {
    var event UserActionEvent
    json.Unmarshal(msg.Body, &event)

    notification := &model.Notification{
        UserId:   event.TargetUserId,
        Type:     event.ActionType,
        SenderId: event.UserId,
        TargetId: event.TargetId,
    }

    switch event.ActionType {
    case 1: // 点赞
        notification.Content = fmt.Sprintf("%s 赞了你的帖子", event.Username)
    case 2: // 评论
        notification.Content = fmt.Sprintf("%s 评论了你的帖子", event.Username)
    case 3: // 关注
        notification.Content = fmt.Sprintf("%s 关注了你", event.Username)
    }

    return c.notificationModel.Insert(ctx, notification)
}
```

**验收标准**:
- [ ] 通知正确生成
- [ ] 内容格式正确

---

#### 任务 3.3: 实现未读计数
**涉及模块**: `app/message/rpc/internal/logic/`

**技术要点**:
```go
func (l *GetUnreadCountLogic) GetUnreadCount(in *message.GetUnreadCountReq) (*message.GetUnreadCountResp, error) {
    // 1. 先从 Redis 读取
    key := fmt.Sprintf("unread:%d", in.UserId)
    count, err := l.svcCtx.Redis.Get(l.ctx, key).Int()
    if err == nil {
        return &message.GetUnreadCountResp{Count: int64(count)}, nil
    }

    // 2. Redis 没有，从数据库统计
    count, err = l.svcCtx.NotificationModel.CountUnread(l.ctx, in.UserId)
    if err != nil {
        return nil, err
    }

    // 3. 写入 Redis
    l.svcCtx.Redis.Set(l.ctx, key, count, time.Hour)

    return &message.GetUnreadCountResp{Count: int64(count)}, nil
}
```

**验收标准**:
- [ ] 未读计数准确
- [ ] Redis 缓存生效

---

### W4: DTM 分布式事务

#### 任务 4.1: 部署 DTM
**涉及模块**: `deploy/docker-compose.middleware.yml`

**DTM 配置**:
```yaml
dtm:
  image: yedf/dtm:latest
  ports:
    - "36789:36789"  # HTTP
    - "36790:36790"  # gRPC
  environment:
    STORE_DRIVER: mysql
    STORE_DSN: "root:123456@tcp(mysql:3306)/dtm?parseTime=true"
```

**验收标准**:
- [ ] DTM 服务正常启动
- [ ] DTM 控制台可访问

---

#### 任务 4.2: 实现帖子发布事务
**涉及模块**: `app/content/rpc/internal/logic/`

**事务场景**: 帖子发布 + 索引同步 + Feed 推送

**DTM 二阶段消息**:
```go
func (l *CreatePostLogic) CreatePost(in *content.CreatePostReq) (*content.CreatePostResp, error) {
    gid := uuid.New().String()

    // DTM 二阶段消息
    msg := dtmgrpc.NewMsgGrpc(l.svcCtx.Config.DtmServer, gid)

    // 添加分支操作：同步搜索索引
    msg.Add(
        l.svcCtx.Config.SearchRpcBusi+"/SyncPostIndex",
        &search.SyncPostReq{PostId: postId},
    )

    // 添加分支操作：Feed 推送
    msg.Add(
        l.svcCtx.Config.FeedRpcBusi+"/FanoutPost",
        &feed.FanoutReq{PostId: postId, AuthorId: in.AuthorId},
    )

    // 执行本地事务 + 提交消息
    err := msg.DoAndSubmitDB(l.svcCtx.Config.DtmServer, func(tx *sql.Tx) error {
        // 本地事务：写入帖子表
        return l.svcCtx.PostModel.InsertTx(tx, &model.Post{
            Title:    in.Title,
            Content:  in.Content,
            AuthorId: in.AuthorId,
        })
    })

    if err != nil {
        return nil, err
    }

    return &content.CreatePostResp{Id: postId}, nil
}
```

**验收标准**:
- [ ] 帖子发布事务成功
- [ ] 索引同步正常
- [ ] Feed 推送正常

---

#### 任务 4.3: 实现点赞事务
**涉及模块**: `app/interaction/rpc/internal/logic/`

**事务场景**: 点赞 + 计数更新

**DTM Saga 模式**:
```go
func (l *LikeLogic) Like(in *interaction.LikeReq) (*interaction.LikeResp, error) {
    gid := uuid.New().String()

    saga := dtmgrpc.NewSaga(l.svcCtx.Config.DtmServer, gid).
        Add(
            l.svcCtx.Config.InteractionRpcBusi+"/LikeAction",
            l.svcCtx.Config.InteractionRpcBusi+"/LikeActionRevert",
            &LikeActionReq{UserId: in.UserId, TargetId: in.TargetId},
        ).
        Add(
            l.svcCtx.Config.InteractionRpcBusi+"/IncrLikeCount",
            l.svcCtx.Config.InteractionRpcBusi+"/DecrLikeCount",
            &IncrCountReq{TargetId: in.TargetId},
        )

    err := saga.Submit()
    if err != nil {
        return nil, err
    }

    return &interaction.LikeResp{Success: true}, nil
}
```

**验收标准**:
- [ ] 点赞事务成功
- [ ] 计数更新正常
- [ ] 回滚逻辑正确

---

### W5: MQ 消费者集成

#### 任务 5.1: 创建 MQ 消费者目录
**涉及模块**: `app/mq/`

**目录结构**:
```
app/mq/
├── search-consumer/
│   └── main.go       # 搜索索引同步
├── recommend-consumer/
│   └── main.go       # 行为事件消费
├── feed-consumer/
│   └── main.go       # Feed 写扩散
├── message-consumer/
│   └── main.go       # 通知生成
└── media-consumer/
    └── main.go       # 媒体处理任务
```

**验收标准**:
- [ ] 所有消费者目录创建成功

---

#### 任务 5.2: 实现搜索索引同步消费者
**涉及模块**: `app/mq/search-consumer/`

**功能**:
- 消费帖子发布事件
- 同步到 Elasticsearch
- 同步向量到 Milvus

**验收标准**:
- [ ] 索引同步正常
- [ ] 错误重试机制

---

#### 任务 5.3: 实现行为事件消费者
**涉及模块**: `app/mq/recommend-consumer/`

**功能**:
- 消费点赞/收藏/浏览事件
- 更新用户画像
- 更新行为特征

**验收标准**:
- [ ] 用户画像更新正常
- [ ] 行为特征计算正确

---

#### 任务 5.4: 启动所有消费者
**涉及模块**: `deploy/docker-compose.yml`

**消费者配置**:
```yaml
search-consumer:
  build: ./app/mq/search-consumer
  depends_on:
    - rocketmq-namesrv
    - elasticsearch

feed-consumer:
  build: ./app/mq/feed-consumer
  depends_on:
    - rocketmq-namesrv
    - redis

message-consumer:
  build: ./app/mq/message-consumer
  depends_on:
    - rocketmq-namesrv
    - mysql
```

**验收标准**:
- [ ] 所有消费者启动成功
- [ ] 消息正常消费

---

## 技术要点

### RocketMQ Topic 定义

```go
// pkg/mqx/topics.go
const (
    TopicPostPublish   = "POST_PUBLISH"    // 帖子发布/更新/删除
    TopicUserAction    = "USER_ACTION"     // 点赞/收藏/浏览
    TopicUserFollow    = "USER_FOLLOW"     // 关注/取关
    TopicCommentCreate = "COMMENT_CREATE"  // 新评论
    TopicMediaProcess  = "MEDIA_PROCESS"   // 媒体处理完成
)

const (
    TagPostCreate = "create"
    TagPostUpdate = "update"
    TagPostDelete = "delete"
)
```

### context 超时控制

```go
func (l *GetFollowFeedLogic) GetFollowFeed(in *feed.GetFollowFeedReq) (*feed.GetFollowFeedResp, error) {
    // 设置总超时
    ctx, cancel := context.WithTimeout(l.ctx, 500*time.Millisecond)
    defer cancel()

    // 子操作独立超时
    inboxCtx, inboxCancel := context.WithTimeout(ctx, 200*time.Millisecond)
    defer inboxCancel()

    inboxItems, _ := l.svcCtx.FeedModel.GetInbox(inboxCtx, in.UserId, in.LastId, in.PageSize)

    return &feed.GetFollowFeedResp{Items: inboxItems}, nil
}
```

### 错误处理策略

**RPC 层错误**:
```go
// 返回 gRPC 标准错误
if errors.Is(err, model.ErrNotFound) {
    return nil, status.Error(codes.NotFound, "记录不存在")
}
return nil, status.Error(codes.Internal, "服务内部错误")
```

**Gateway 错误转换**:
```go
st, ok := status.FromError(err)
if ok && st.Code() == codes.NotFound {
    return nil, errx.ErrNotFound
}
return nil, err
```

---

## 依赖与风险

### 外部依赖
| 依赖 | 用途 |
|------|------|
| DTM | 分布式事务 |
| RocketMQ | 消息队列 |
| Redis | 缓存/计数 |

### 潜在风险

| 风险 | 等级 | 缓解措施 |
|------|------|---------|
| RocketMQ Go SDK 成熟度 | HIGH | DTM 二阶段消息替代事务消息 |
| Feed 写扩散性能 | MEDIUM | 大V用户使用拉模式 |
| 消息幂等性 | MEDIUM | 消息 ID 去重 |

---

## 验收标准

### 功能验收
- [ ] 点赞/收藏功能正常
- [ ] 关注流正常显示
- [ ] 通知生成正常
- [ ] 未读计数准确
- [ ] 分布式事务成功

### 性能验收
- [ ] Feed 响应时间 < 200ms
- [ ] 计数更新延迟 < 1s

### 测试验收
- [ ] 单元测试覆盖率 > 80%
- [ ] 集成测试通过

---

## 交付物清单

| 交付物 | 路径 |
|--------|------|
| Interaction RPC | `app/interaction/rpc/` |
| Feed RPC | `app/feed/rpc/` |
| Message RPC | `app/message/rpc/` |
| MQ 消费者 | `app/mq/` |
| DTM 事务配置 | 各服务 logic 层 |

---

## 下一步

Phase 2 完成后，进入 [Phase 3: 搜索系统](phase-3-search.md)。
