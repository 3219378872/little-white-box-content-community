# Interaction Service 设计文档

## 1. 概述

实现 `app/interaction` 的互动业务逻辑，覆盖点赞、取消点赞、收藏、取消收藏、互动计数查询、批量状态检查等功能。该服务是 Phase 2 的基础，Feed 和 Message 服务依赖其行为事件。

## 2. 架构概览

```
Client → Gateway (REST) → Interaction RPC (:?)
                              │
                              ├── MySQL (like_record / favorite / action_count)
                              ├── Redis (计数缓存)
                              └── RocketMQ (行为事件)
```

- Gateway 通过 zrpc 调用 Interaction RPC
- RPC 层通过 interceptor 将 `errx.BizError` 转换为 gRPC status
- 所有 Logic 通过 `svc.ServiceContext` 获取依赖

## 3. 数据模型

### 新增表：action_count

```sql
CREATE TABLE `action_count` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `target_id` bigint NOT NULL,
  `target_type` tinyint NOT NULL COMMENT '1=帖子, 2=评论',
  `like_count` bigint NOT NULL DEFAULT 0,
  `favorite_count` bigint NOT NULL DEFAULT 0,
  `comment_count` bigint NOT NULL DEFAULT 0,
  `share_count` bigint NOT NULL DEFAULT 0,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_target` (`target_id`, `target_type`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4;
```

### 现有表变更说明

- `like_record` 和 `favorite` 已有 `status` 字段（软删除），Unlike/Unfavorite 更新 `status=0`
- `favorite` 的收藏夹关联（`folder_id`）不在本期实现，只维护基础收藏关系

## 4. 业务逻辑层设计

### Like

1. 查询 `like_record` 是否已存在且 `status=1` —— 是则返回 `ErrAlreadyLiked`
2. 若存在但 `status=0`，更新为 `status=1`
3. 若不存在，插入新记录
4. Redis 中 `action_count:{target_id}:{target_type}` 的 `like_count` 自增
5. 发送 MQ `TopicLike` 消息

### Unlike

1. 查询 `like_record`，不存在或 `status=0` 则返回 `ErrNotLiked`
2. 更新 `status=0`
3. Redis `like_count` 自减
4. 发送 MQ `TopicUnlike` 消息

### Favorite / Unfavorite

与 Like / Unlike 对称，操作 `favorite` 表和 `favorite_count`。

### GetCounts

1. 先从 Redis 读 `action_count:{target_id}:{target_type}`
2. Redis miss 时通过 `singleflight` 保护查 DB
3. DB 无记录则缓存空值（TTL 30s）
4. DB 有记录则回填 Redis，TTL = 5min + rand(0, 60s)
5. `comment_count` 不维护，由 Content 服务提供，Gateway 层合并

### BatchCheckLiked / BatchCheckFavorited

`IN` 查询批量返回 `map[target_id]bool`。

## 5. 数据流与 MQ

```
Like/Unlike/Favorite/Unfavorite
    │
    ├──→ MySQL (记录写入)
    ├──→ Redis (计数自增/减)
    └──→ RocketMQ
            │
            ├──→ recommend-consumer (用户画像)
            ├──→ feed-consumer (Feed 生成)
            ├──→ message-consumer (通知生成)
            └──→ search-consumer (权重更新)
```

### 消息格式

```go
type UserActionEvent struct {
    UserId     int64  `json:"user_id"`
    TargetId   int64  `json:"target_id"`
    TargetType int32  `json:"target_type"`
    ActionType string `json:"action_type"` // like/unlike/favorite/unfavorite
    Timestamp  int64  `json:"timestamp"`
}
```

## 6. 缓存策略

### 计数缓存

- Key: `action_count:{target_id}:{target_type}`
- Value: hash `{like_count, favorite_count}`
- 主 TTL: 5min + 随机偏移
- 空值 TTL: 30s

### 缓存击穿防护

| 层级 | 机制 | 说明 |
|------|------|------|
| 1 | 随机 TTL | 避免大量 key 同时过期 |
| 2 | 空值缓存 | 防止不存在的 target_id 反复穿透 |
| 3 | singleflight | 同 target 并发只放行 1 个 DB 查询 |

## 7. 计数一致性

- Redis 计数与 MQ 发送为"尽力而为"，最终一致性
- 兜底：每 5min 定时任务比对 Redis 与 DB `action_count`，差异超阈值时以 DB 为准同步

## 8. 错误码

在 `pkg/errx/codes.go` 互动域（3000-3999）新增：

```go
ErrAlreadyLiked     = 3001 // 已点赞
ErrNotLiked         = 3002 // 未点赞
ErrAlreadyFavorited = 3003 // 已收藏
ErrNotFavorited     = 3004 // 未收藏
```

所有 Logic 返回 `errx.New(code, msg)`，禁止裸 `errors.New()`。

## 9. 依赖风险

### pkg/mqx Producer 未测试

`pkg/mqx` 包中的 `Producer` 封装（`producer.go`）目前**尚未经过单元测试或集成测试验证**。Interaction 服务的 Like / Favorite 等操作依赖该 Producer 发送 MQ 消息，若 Producer 存在连接、序列化或发送逻辑缺陷，会导致行为事件丢失。

**影响范围**：
- Like / Unlike / Favorite / Unfavorite 的事件通知链路
- 下游 Consumer（推荐、Feed、消息、搜索）无法收到事件

**缓解措施**：
1. 在 Interaction 集成测试中覆盖 MQ 发送路径（验证消息是否成功投递到 Topic）
2. 为 `pkg/mqx/producer.go` 补充单元测试（mock RocketMQ Producer 接口）
3. 若 Producer 在测试中发现阻塞或异常，Like/Favorite 的核心 DB 写入仍应成功，MQ 发送失败不影响主流程（降级为日志告警）

## 10. 测试策略

### 单元测试（目标 80%+）

| 模块 | 场景 |
|------|------|
| LikeLogic | 首次点赞、重复点赞、取消后重新点赞 |
| UnlikeLogic | 正常取消、未点赞取消 |
| FavoriteLogic | 对称 Like 测试 |
| GetCountsLogic | Redis hit、miss + singleflight、空值缓存 |
| BatchCheckLiked | 批量查询结果正确 |

### 集成测试

- testcontainers MySQL + Redis
- Like → Unlike → Like 完整链路
- 验证 action_count 与 Redis 计数一致性

## 10. 目录结构

```
app/interaction/
├── internal/
│   ├── logic/
│   │   ├── like_logic.go
│   │   ├── unlike_logic.go
│   │   ├── favorite_logic.go
│   │   ├── unfavorite_logic.go
│   │   ├── get_counts_logic.go
│   │   ├── check_liked_logic.go
│   │   ├── batch_check_liked_logic.go
│   │   ├── check_favorited_logic.go
│   │   ├── batch_check_favorited_logic.go
│   │   └── get_favorite_list_logic.go
│   ├── model/
│   │   └── action_count_model.go
│   └── svc/
│       └── service_context.go  # 新增 Redis + SingleFlight
```
