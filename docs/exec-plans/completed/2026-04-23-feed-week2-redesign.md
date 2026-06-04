# Feed Week2 Redesign Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 重做 Week2 Feed RPC 服务，落地关注流、收件箱/发件箱分发、小 V 推大 V 拉，以及最小可用推荐流。

**Architecture:** 先补齐 `user` 服务关注/粉丝 RPC 的真实数据能力，再重写 `proto/feed` 与 `deploy/sql/xbh_feed.sql`，用 goctl 生成 `app/feed` RPC 服务。Feed 通过 `User RPC` 判断作者粉丝数和关注关系，通过 `Content RPC` 校验帖子详情，通过 RocketMQ 消费 content 发帖事件并写入 outbox/inbox。为保证单元测试可 mock，Feed 的 `ServiceContext` 暴露最小接口字段，而不是直接依赖 goctl 生成的大接口或 `*mqx.Producer` 具体类型。

**Tech Stack:** Go 1.26.1, go-zero v1.10.1, gRPC/zrpc, MySQL 8.0, Redis 7, RocketMQ 5.1.0, testify/assert/mock/require, sqlmock, testcontainers-go

**Notes:** 不得手写 goctl 生成文件；修改 `.proto` 后必须重新生成。

---

## 文件变更总览

| 文件 | 责任 |
|------|------|
| `app/user/internal/model/user_follow_model.go` | 为关注/粉丝列表和计数提供自定义 SQL |
| `app/user/internal/svc/service_context.go` | 注入 `UserFollowModel` |
| `app/user/internal/logic/get_followers_logic.go` | 实现粉丝列表 RPC |
| `app/user/internal/logic/get_following_logic.go` | 实现关注列表 RPC |
| `app/user/internal/logic/get_followers_logic_test.go` | 粉丝列表单元测试 |
| `app/user/internal/logic/get_following_logic_test.go` | 关注列表单元测试 |
| `proto/feed/feed.proto` | Feed RPC 新契约 |
| `deploy/sql/xbh_feed.sql` | Feed outbox/inbox DDL |
| `app/feed/**` | goctl 生成的 Feed RPC 服务、pb、service、logic、model |
| `app/feed/internal/svc/service_context.go` | Feed 依赖注入和可测接口 |
| `app/feed/internal/model/feed_inbox_model.go` | inbox 批量写入与游标查询 |
| `app/feed/internal/model/feed_outbox_model.go` | outbox 幂等写入与多作者游标查询 |
| `app/feed/internal/logic/get_follow_feed_logic.go` | 关注流读取与 inbox/outbox 合并 |
| `app/feed/internal/logic/get_recommend_feed_logic.go` | 最小推荐流 |
| `app/feed/internal/logic/push_to_inbox_logic.go` | 内部批量推入 inbox RPC |
| `app/feed/internal/mqs/post_publish_consumer.go` | content 发帖事件消费与分发 |
| `app/content/internal/config/config.go` | content MQ producer 配置 |
| `app/content/internal/svc/service_context.go` | content MQ producer 接口注入 |
| `app/content/etc/content.yaml` | content MQ 配置占位 |
| `app/content/internal/logic/create_post_logic.go` | 发帖成功后发布事件 |
| `app/content/internal/logic/create_post_logic_test.go` | 发帖事件发布测试 |
| `go.work` | 加入新 `./app/feed` workspace 模块 |

## Task 1: 补齐 User 关注/粉丝 RPC

**Files:**
- Modify: `app/user/internal/model/user_follow_model.go`
- Modify: `app/user/internal/svc/service_context.go`
- Modify: `app/user/internal/logic/get_followers_logic.go`
- Modify: `app/user/internal/logic/get_following_logic.go`
- Create: `app/user/internal/logic/get_followers_logic_test.go`
- Create: `app/user/internal/logic/get_following_logic_test.go`

- [ ] **Step 1: 写 `GetFollowersLogic` 失败测试**

Create `app/user/internal/logic/get_followers_logic_test.go`:

```go
package logic

import (
    "context"
    "errors"
    "testing"
    "time"

    "errx"
    "user/internal/model"
    "user/internal/svc"
    "user/pb/xiaobaihe/user/pb"

    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/mock"
    "github.com/stretchr/testify/require"
)

type mockUserFollowModel struct{ mock.Mock }

func (m *mockUserFollowModel) FindFollowers(ctx context.Context, userID int64, offset, limit int64) ([]*model.UserProfile, error) {
    args := m.Called(ctx, userID, offset, limit)
    if v := args.Get(0); v != nil {
        return v.([]*model.UserProfile), args.Error(1)
    }
    return nil, args.Error(1)
}
func (m *mockUserFollowModel) FindFollowing(ctx context.Context, userID int64, offset, limit int64) ([]*model.UserProfile, error) {
    args := m.Called(ctx, userID, offset, limit)
    if v := args.Get(0); v != nil {
        return v.([]*model.UserProfile), args.Error(1)
    }
    return nil, args.Error(1)
}
func (m *mockUserFollowModel) CountFollowers(ctx context.Context, userID int64) (int64, error) {
    args := m.Called(ctx, userID)
    return args.Get(0).(int64), args.Error(1)
}
func (m *mockUserFollowModel) CountFollowing(ctx context.Context, userID int64) (int64, error) {
    args := m.Called(ctx, userID)
    return args.Get(0).(int64), args.Error(1)
}

func TestGetFollowersLogic_Success(t *testing.T) {
    now := time.Unix(1710000000, 0)
    followModel := new(mockUserFollowModel)
    followModel.On("FindFollowers", mock.Anything, int64(9), int64(0), int64(2)).Return([]*model.UserProfile{
        {Id: 101, Username: "u101", FollowerCount: 2, FollowingCount: 3, CreatedAt: now},
        {Id: 102, Username: "u102", FollowerCount: 4, FollowingCount: 5, CreatedAt: now},
    }, nil).Once()
    followModel.On("CountFollowers", mock.Anything, int64(9)).Return(int64(12), nil).Once()

    logic := NewGetFollowersLogic(context.Background(), &svc.ServiceContext{UserFollowModel: followModel})
    resp, err := logic.GetFollowers(&pb.GetFollowersReq{UserId: 9, Page: 1, PageSize: 2})

    require.NoError(t, err)
    require.Len(t, resp.Users, 2)
    assert.Equal(t, int64(12), resp.Total)
    assert.Equal(t, int64(101), resp.Users[0].Id)
    followModel.AssertExpectations(t)
}

func TestGetFollowersLogic_InvalidPage(t *testing.T) {
    logic := NewGetFollowersLogic(context.Background(), &svc.ServiceContext{})
    resp, err := logic.GetFollowers(&pb.GetFollowersReq{UserId: 9, Page: 0, PageSize: 2})
    require.Nil(t, resp)
    require.Error(t, err)
    assert.Equal(t, errx.ParamError, errx.CodeOf(err))
}

func TestGetFollowersLogic_ModelFailure(t *testing.T) {
    followModel := new(mockUserFollowModel)
    followModel.On("FindFollowers", mock.Anything, int64(9), int64(0), int64(2)).Return(nil, errors.New("db down")).Once()

    logic := NewGetFollowersLogic(context.Background(), &svc.ServiceContext{UserFollowModel: followModel})
    resp, err := logic.GetFollowers(&pb.GetFollowersReq{UserId: 9, Page: 1, PageSize: 2})

    require.Nil(t, resp)
    require.Error(t, err)
    assert.Equal(t, errx.SystemError, errx.CodeOf(err))
    followModel.AssertExpectations(t)
}
```

- [ ] **Step 2: 写 `GetFollowingLogic` 失败测试**

Create `app/user/internal/logic/get_following_logic_test.go`:

```go
package logic

import (
    "context"
    "errors"
    "testing"
    "time"

    "errx"
    "user/internal/model"
    "user/internal/svc"
    "user/pb/xiaobaihe/user/pb"

    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/mock"
    "github.com/stretchr/testify/require"
)

func TestGetFollowingLogic_Success(t *testing.T) {
    now := time.Unix(1710000000, 0)
    followModel := new(mockUserFollowModel)
    followModel.On("FindFollowing", mock.Anything, int64(9), int64(0), int64(2)).Return([]*model.UserProfile{
        {Id: 201, Username: "u201", FollowerCount: 10, FollowingCount: 11, CreatedAt: now},
        {Id: 202, Username: "u202", FollowerCount: 12, FollowingCount: 13, CreatedAt: now},
    }, nil).Once()
    followModel.On("CountFollowing", mock.Anything, int64(9)).Return(int64(22), nil).Once()

    logic := NewGetFollowingLogic(context.Background(), &svc.ServiceContext{UserFollowModel: followModel})
    resp, err := logic.GetFollowing(&pb.GetFollowingReq{UserId: 9, Page: 1, PageSize: 2})

    require.NoError(t, err)
    require.Len(t, resp.Users, 2)
    assert.Equal(t, int64(22), resp.Total)
    assert.Equal(t, int64(201), resp.Users[0].Id)
    followModel.AssertExpectations(t)
}

func TestGetFollowingLogic_InvalidPageSize(t *testing.T) {
    logic := NewGetFollowingLogic(context.Background(), &svc.ServiceContext{})
    resp, err := logic.GetFollowing(&pb.GetFollowingReq{UserId: 9, Page: 1, PageSize: 0})
    require.Nil(t, resp)
    require.Error(t, err)
    assert.Equal(t, errx.ParamError, errx.CodeOf(err))
}

func TestGetFollowingLogic_CountFailure(t *testing.T) {
    followModel := new(mockUserFollowModel)
    followModel.On("FindFollowing", mock.Anything, int64(9), int64(0), int64(2)).Return([]*model.UserProfile{}, nil).Once()
    followModel.On("CountFollowing", mock.Anything, int64(9)).Return(int64(0), errors.New("db down")).Once()

    logic := NewGetFollowingLogic(context.Background(), &svc.ServiceContext{UserFollowModel: followModel})
    resp, err := logic.GetFollowing(&pb.GetFollowingReq{UserId: 9, Page: 1, PageSize: 2})

    require.Nil(t, resp)
    require.Error(t, err)
    assert.Equal(t, errx.SystemError, errx.CodeOf(err))
    followModel.AssertExpectations(t)
}
```

- [ ] **Step 3: 运行测试确认失败**

Run:

```bash
go test -race ./app/user/internal/logic -run 'TestGetFollowersLogic|TestGetFollowingLogic'
```

Expected: build fails because `ServiceContext.UserFollowModel` and custom model methods are missing.

- [ ] **Step 4: 扩展 `UserFollowModel`**

Modify `app/user/internal/model/user_follow_model.go`:

```go
package model

import (
    "context"

    "github.com/zeromicro/go-zero/core/stores/sqlx"
)

var _ UserFollowModel = (*customUserFollowModel)(nil)

type (
    UserFollowModel interface {
        userFollowModel
        withSession(session sqlx.Session) UserFollowModel
        FindFollowers(ctx context.Context, userID int64, offset, limit int64) ([]*UserProfile, error)
        FindFollowing(ctx context.Context, userID int64, offset, limit int64) ([]*UserProfile, error)
        CountFollowers(ctx context.Context, userID int64) (int64, error)
        CountFollowing(ctx context.Context, userID int64) (int64, error)
    }

    customUserFollowModel struct {
        *defaultUserFollowModel
    }
)

func NewUserFollowModel(conn sqlx.SqlConn) UserFollowModel {
    return &customUserFollowModel{defaultUserFollowModel: newUserFollowModel(conn)}
}

func (m *customUserFollowModel) withSession(session sqlx.Session) UserFollowModel {
    return NewUserFollowModel(sqlx.NewSqlConnFromSession(session))
}

func (m *customUserFollowModel) FindFollowers(ctx context.Context, userID int64, offset, limit int64) ([]*UserProfile, error) {
    query := `SELECT p.id, p.username, p.password, p.phone, p.email, p.nickname, p.avatar_url, p.bio,
       p.gender, p.birthday, p.level, p.exp, p.follower_count, p.following_count,
       p.post_count, p.like_count, p.status, p.favorites_visibility, p.created_at, p.updated_at
FROM user_follow f
JOIN user_profile p ON p.id = f.user_id
WHERE f.target_user_id = ?
ORDER BY f.id DESC
LIMIT ? OFFSET ?`
    var profiles []*UserProfile
    if err := m.conn.QueryRowsCtx(ctx, &profiles, query, userID, limit, offset); err != nil {
        return nil, err
    }
    return profiles, nil
}

func (m *customUserFollowModel) FindFollowing(ctx context.Context, userID int64, offset, limit int64) ([]*UserProfile, error) {
    query := `SELECT p.id, p.username, p.password, p.phone, p.email, p.nickname, p.avatar_url, p.bio,
       p.gender, p.birthday, p.level, p.exp, p.follower_count, p.following_count,
       p.post_count, p.like_count, p.status, p.favorites_visibility, p.created_at, p.updated_at
FROM user_follow f
JOIN user_profile p ON p.id = f.target_user_id
WHERE f.user_id = ?
ORDER BY f.id DESC
LIMIT ? OFFSET ?`
    var profiles []*UserProfile
    if err := m.conn.QueryRowsCtx(ctx, &profiles, query, userID, limit, offset); err != nil {
        return nil, err
    }
    return profiles, nil
}

func (m *customUserFollowModel) CountFollowers(ctx context.Context, userID int64) (int64, error) {
    query := `SELECT COUNT(*) FROM user_follow WHERE target_user_id = ?`
    var total int64
    if err := m.conn.QueryRowCtx(ctx, &total, query, userID); err != nil {
        return 0, err
    }
    return total, nil
}

func (m *customUserFollowModel) CountFollowing(ctx context.Context, userID int64) (int64, error) {
    query := `SELECT COUNT(*) FROM user_follow WHERE user_id = ?`
    var total int64
    if err := m.conn.QueryRowCtx(ctx, &total, query, userID); err != nil {
        return 0, err
    }
    return total, nil
}
```

- [ ] **Step 5: 注入最小接口并实现 logic**

Modify `app/user/internal/svc/service_context.go` by adding a testable minimal interface plus the field and constructor value:

```go
type UserFollowStore interface {
    FindFollowers(ctx context.Context, userID int64, offset, limit int64) ([]*model.UserProfile, error)
    FindFollowing(ctx context.Context, userID int64, offset, limit int64) ([]*model.UserProfile, error)
    CountFollowers(ctx context.Context, userID int64) (int64, error)
    CountFollowing(ctx context.Context, userID int64) (int64, error)
}
```

```go
type ServiceContext struct {
    Config            config.Config
    DB                sqlx.SqlConn
    UserLoginLogModel model.UserLoginLogModel
    UserProfileModel  model.UserProfileModel
    UserFollowModel   UserFollowStore
    RedisClient       *redis.Redis
}
```

```go
return &ServiceContext{
    Config:            c,
    DB:                conn,
    UserLoginLogModel: model.NewUserLoginLogModel(conn),
    UserProfileModel:  model.NewUserProfileModel(conn),
    UserFollowModel:   model.NewUserFollowModel(conn),
    RedisClient:       newRedis,
}
```

Modify `app/user/internal/logic/get_followers_logic.go`:

```go
func (l *GetFollowersLogic) GetFollowers(in *pb.GetFollowersReq) (*pb.GetFollowersResp, error) {
    if in.UserId <= 0 || in.Page <= 0 || in.PageSize <= 0 {
        return nil, errx.NewWithCode(errx.ParamError)
    }
    offset := int64((in.Page - 1) * in.PageSize)
    limit := int64(in.PageSize)

    users, err := l.svcCtx.UserFollowModel.FindFollowers(l.ctx, in.UserId, offset, limit)
    if err != nil {
        l.Errorw("UserFollowModel.FindFollowers failed", logx.Field("err", err.Error()))
        return nil, errx.NewWithCode(errx.SystemError)
    }
    total, err := l.svcCtx.UserFollowModel.CountFollowers(l.ctx, in.UserId)
    if err != nil {
        l.Errorw("UserFollowModel.CountFollowers failed", logx.Field("err", err.Error()))
        return nil, errx.NewWithCode(errx.SystemError)
    }
    respUsers := make([]*pb.UserInfo, 0, len(users))
    for _, user := range users {
        respUsers = append(respUsers, UserProfileToUserInfo(user))
    }
    return &pb.GetFollowersResp{Users: respUsers, Total: total}, nil
}
```

Modify `app/user/internal/logic/get_following_logic.go`:

```go
func (l *GetFollowingLogic) GetFollowing(in *pb.GetFollowingReq) (*pb.GetFollowingResp, error) {
    if in.UserId <= 0 || in.Page <= 0 || in.PageSize <= 0 {
        return nil, errx.NewWithCode(errx.ParamError)
    }
    offset := int64((in.Page - 1) * in.PageSize)
    limit := int64(in.PageSize)

    users, err := l.svcCtx.UserFollowModel.FindFollowing(l.ctx, in.UserId, offset, limit)
    if err != nil {
        l.Errorw("UserFollowModel.FindFollowing failed", logx.Field("err", err.Error()))
        return nil, errx.NewWithCode(errx.SystemError)
    }
    total, err := l.svcCtx.UserFollowModel.CountFollowing(l.ctx, in.UserId)
    if err != nil {
        l.Errorw("UserFollowModel.CountFollowing failed", logx.Field("err", err.Error()))
        return nil, errx.NewWithCode(errx.SystemError)
    }
    respUsers := make([]*pb.UserInfo, 0, len(users))
    for _, user := range users {
        respUsers = append(respUsers, UserProfileToUserInfo(user))
    }
    return &pb.GetFollowingResp{Users: respUsers, Total: total}, nil
}
```

- [ ] **Step 6: 运行 User 测试确认通过**

Run:

```bash
go test -race ./app/user/internal/logic -run 'TestGetFollowersLogic|TestGetFollowingLogic'
```

Expected:

```text
ok   user/internal/logic
```

## Task 2: 生成 Feed RPC 与 Model 骨架

**Files:**
- Modify: `proto/feed/feed.proto`
- Modify: `deploy/sql/xbh_feed.sql`
- Create: `app/feed/**`
- Modify: `go.work`

- [ ] **Step 1: 重写 Feed proto**

Replace `proto/feed/feed.proto` with:

```protobuf
syntax = "proto3";

package feed;
option go_package = "xiaobaihe/feed/pb";

service FeedService {
  rpc GetFollowFeed(GetFollowFeedReq) returns (GetFollowFeedResp);
  rpc GetRecommendFeed(GetRecommendFeedReq) returns (GetRecommendFeedResp);
  rpc PushToInbox(PushToInboxReq) returns (PushToInboxResp);
}

message FeedItem {
  int64 post_id = 1;
  int64 author_id = 2;
  int64 created_at = 3;
  int32 feed_type = 4; // 1=follow 2=recommend
}

message GetFollowFeedReq {
  int64 user_id = 1;
  int64 cursor_created_at = 2;
  int64 cursor_post_id = 3;
  int32 page_size = 4;
}

message GetFollowFeedResp {
  repeated FeedItem items = 1;
  bool has_more = 2;
  int64 next_cursor_created_at = 3;
  int64 next_cursor_post_id = 4;
}

message GetRecommendFeedReq {
  int64 user_id = 1;
  int32 page = 2;
  int32 page_size = 3;
}

message GetRecommendFeedResp {
  repeated FeedItem items = 1;
  bool has_more = 2;
}

message PushToInboxReq {
  int64 author_id = 1;
  int64 post_id = 2;
  int64 created_at = 3;
  repeated int64 follower_ids = 4;
}

message PushToInboxResp {
  int64 pushed_count = 1;
}
```

- [ ] **Step 2: 重写 Feed SQL**

Replace `deploy/sql/xbh_feed.sql` with:

```sql
CREATE DATABASE IF NOT EXISTS `xbh_feed` DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
USE `xbh_feed`;

CREATE TABLE IF NOT EXISTS `feed_outbox` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `author_id` bigint NOT NULL,
  `post_id` bigint NOT NULL,
  `created_at` bigint NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_author_post` (`author_id`, `post_id`),
  KEY `idx_author_created_post` (`author_id`, `created_at`, `post_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;

CREATE TABLE IF NOT EXISTS `feed_inbox` (
  `id` bigint NOT NULL AUTO_INCREMENT,
  `user_id` bigint NOT NULL,
  `author_id` bigint NOT NULL,
  `post_id` bigint NOT NULL,
  `created_at` bigint NOT NULL,
  PRIMARY KEY (`id`),
  UNIQUE KEY `uk_user_post` (`user_id`, `post_id`),
  KEY `idx_user_created_post` (`user_id`, `created_at`, `post_id`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci;
```

- [ ] **Step 3: 生成 Feed RPC 骨架**

Run:

```bash
mkdir -p app/feed
goctl rpc protoc proto/feed/feed.proto --go_out=app/feed --go-grpc_out=app/feed --zrpc_out=app/feed --style go_zero
```

Expected generated files include:

```text
app/feed/feed.go
app/feed/etc/feed.yaml
app/feed/internal/logic/get_follow_feed_logic.go
app/feed/internal/logic/get_recommend_feed_logic.go
app/feed/internal/logic/push_to_inbox_logic.go
app/feed/feedservice/feed_service.go
app/feed/pb/xiaobaihe/feed/pb/feed.pb.go
```

- [ ] **Step 4: 生成 Feed Model 骨架**

Run:

```bash
goctl model mysql ddl -src deploy/sql/xbh_feed.sql -dir app/feed/internal/model --style go_zero
```

Expected generated files include:

```text
app/feed/internal/model/feed_inbox_model.go
app/feed/internal/model/feed_inbox_model_gen.go
app/feed/internal/model/feed_outbox_model.go
app/feed/internal/model/feed_outbox_model_gen.go
```

- [ ] **Step 5: 加入 workspace 并做空编译**

Add `./app/feed` to `go.work`:

```go
use (
    .
    ./app/feed
    ./app/gateway
    ./app/user
    ./pkg/cachex
    ./pkg/errx
    ./pkg/interceptor
    ./pkg/jwtx
    ./pkg/middleware
    ./pkg/mqx
    ./pkg/util
)
```

Run:

```bash
go test ./app/feed/... -run TestDoesNotExist
```

Expected: Feed packages compile or fail only on missing dependencies that Task 3 explicitly adds.

## Task 3: 初始化 Feed 配置与可测依赖接口

**Files:**
- Modify: `app/feed/internal/config/config.go`
- Modify: `app/feed/etc/feed.yaml`
- Modify: `app/feed/internal/svc/service_context.go`
- Create: `app/feed/internal/svc/service_context_test.go`

- [ ] **Step 1: 写 ServiceContext 字段编译测试**

Create `app/feed/internal/svc/service_context_test.go`:

```go
package svc

import "testing"

func TestServiceContext_TypeSurface(t *testing.T) {
    ctx := &ServiceContext{}
    _ = ctx.InboxModel
    _ = ctx.OutboxModel
    _ = ctx.UserService
    _ = ctx.ContentService
    _ = ctx.BigVThreshold
    _ = ctx.FanoutBatchSize
}
```

- [ ] **Step 2: 运行测试确认失败**

Run:

```bash
go test ./app/feed/internal/svc -run TestServiceContext_TypeSurface
```

Expected: build fails with missing `ServiceContext` fields.

- [ ] **Step 3: 扩展 Feed 配置**

Modify `app/feed/internal/config/config.go`:

```go
package config

import (
    "mqx"

    "github.com/zeromicro/go-zero/core/stores/redis"
    "github.com/zeromicro/go-zero/zrpc"
)

type Config struct {
    zrpc.RpcServerConf
    DataSource      string
    Redis           redis.RedisConf
    UserRpc         zrpc.RpcClientConf
    ContentRpc      zrpc.RpcClientConf
    MQ              mqx.ConsumerConfig
    BigVThreshold   int64
    FanoutBatchSize int64
}
```

Modify `app/feed/etc/feed.yaml`:

```yaml
Name: feed.rpc
ListenOn: 0.0.0.0:9091
Etcd:
  Hosts:
    - 127.0.0.1:2379
  Key: feed.rpc
DataSource: "${DB_FEED}"
Redis:
  Host: 127.0.0.1:6379
  Pass: "${REDIS_PASS}"
  Type: node
UserRpc:
  Etcd:
    Hosts:
      - 127.0.0.1:2379
    Key: user.rpc
ContentRpc:
  Etcd:
    Hosts:
      - 127.0.0.1:2379
    Key: content.rpc
MQ:
  NameServer: "${MQ_NAMESERVER}"
  GroupName: "feed-service-group"
BigVThreshold: 10000
FanoutBatchSize: 500
```

- [ ] **Step 4: 定义最小依赖接口并初始化**

Modify `app/feed/internal/svc/service_context.go`:

```go
package svc

import (
    "context"

    "esx/app/content/contentservice"
    "esx/app/feed/internal/config"
    "esx/app/feed/internal/model"
    "interceptor"
    "user/userservice"

    "github.com/zeromicro/go-zero/core/stores/redis"
    "github.com/zeromicro/go-zero/core/stores/sqlx"
    "github.com/zeromicro/go-zero/zrpc"
    "google.golang.org/grpc"
)

type InboxModel interface {
    BatchInsertIgnore(ctx context.Context, rows []*model.FeedInbox) (int64, error)
    FindByUserBefore(ctx context.Context, userID, cursorCreatedAt, cursorPostID int64, limit int64) ([]*model.FeedInbox, error)
}

type OutboxModel interface {
    InsertIgnore(ctx context.Context, row *model.FeedOutbox) error
    FindByAuthorsBefore(ctx context.Context, authorIDs []int64, cursorCreatedAt, cursorPostID int64, limit int64) ([]*model.FeedOutbox, error)
}

type UserService interface {
    GetUser(ctx context.Context, in *userservice.GetUserReq, opts ...grpc.CallOption) (*userservice.GetUserResp, error)
    GetFollowers(ctx context.Context, in *userservice.GetFollowersReq, opts ...grpc.CallOption) (*userservice.GetFollowersResp, error)
    GetFollowing(ctx context.Context, in *userservice.GetFollowingReq, opts ...grpc.CallOption) (*userservice.GetFollowingResp, error)
}

type ContentService interface {
    GetPostList(ctx context.Context, in *contentservice.GetPostListReq, opts ...grpc.CallOption) (*contentservice.GetPostListResp, error)
    GetPostsByIds(ctx context.Context, in *contentservice.GetPostsByIdsReq, opts ...grpc.CallOption) (*contentservice.GetPostsByIdsResp, error)
}

type ServiceContext struct {
    Config          config.Config
    Conn            sqlx.SqlConn
    Redis           *redis.Redis
    InboxModel      InboxModel
    OutboxModel     OutboxModel
    UserService     UserService
    ContentService  ContentService
    BigVThreshold   int64
    FanoutBatchSize int64
}

func NewServiceContext(c config.Config) *ServiceContext {
    conn := sqlx.NewMysql(c.DataSource)
    rds := redis.MustNewRedis(c.Redis)
    bizErrInterceptor := interceptor.BizErrorUnaryInterceptor()
    userClient := zrpc.MustNewClient(c.UserRpc, zrpc.WithUnaryClientInterceptor(bizErrInterceptor))
    contentClient := zrpc.MustNewClient(c.ContentRpc, zrpc.WithUnaryClientInterceptor(bizErrInterceptor))

    return &ServiceContext{
        Config:          c,
        Conn:            conn,
        Redis:           rds,
        InboxModel:      model.NewFeedInboxModel(conn),
        OutboxModel:     model.NewFeedOutboxModel(conn),
        UserService:     userservice.NewUserService(userClient),
        ContentService:  contentservice.NewContentService(contentClient),
        BigVThreshold:   c.BigVThreshold,
        FanoutBatchSize: c.FanoutBatchSize,
    }
}
```

- [ ] **Step 5: 运行 ServiceContext 测试确认通过**

Run:

```bash
go test ./app/feed/internal/svc -run TestServiceContext_TypeSurface
```

Expected:

```text
ok   esx/app/feed/internal/svc
```

## Task 4: 实现 Feed Model 自定义 SQL 与 PushToInbox

**Files:**
- Modify: `app/feed/internal/model/feed_inbox_model.go`
- Modify: `app/feed/internal/model/feed_outbox_model.go`
- Modify: `app/feed/internal/logic/push_to_inbox_logic.go`
- Create: `app/feed/internal/logic/push_to_inbox_logic_test.go`

- [ ] **Step 1: 写 PushToInbox 单元测试**

Create `app/feed/internal/logic/push_to_inbox_logic_test.go`:

```go
package logic

import (
    "context"
    "errors"
    "testing"

    "errx"
    "esx/app/feed/internal/model"
    "esx/app/feed/internal/svc"
    "esx/app/feed/pb/xiaobaihe/feed/pb"

    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/mock"
    "github.com/stretchr/testify/require"
)

type mockInboxModel struct{ mock.Mock }

func (m *mockInboxModel) BatchInsertIgnore(ctx context.Context, rows []*model.FeedInbox) (int64, error) {
    args := m.Called(ctx, rows)
    return args.Get(0).(int64), args.Error(1)
}
func (m *mockInboxModel) FindByUserBefore(ctx context.Context, userID, cursorCreatedAt, cursorPostID int64, limit int64) ([]*model.FeedInbox, error) {
    args := m.Called(ctx, userID, cursorCreatedAt, cursorPostID, limit)
    if v := args.Get(0); v != nil {
        return v.([]*model.FeedInbox), args.Error(1)
    }
    return nil, args.Error(1)
}

func TestPushToInboxLogic_Success(t *testing.T) {
    inboxModel := new(mockInboxModel)
    inboxModel.On("BatchInsertIgnore", mock.Anything, mock.MatchedBy(func(rows []*model.FeedInbox) bool {
        return len(rows) == 2 && rows[0].UserId == 1 && rows[1].UserId == 2 && rows[0].AuthorId == 9 && rows[0].PostId == 1001
    })).Return(int64(2), nil).Once()

    logic := NewPushToInboxLogic(context.Background(), &svc.ServiceContext{InboxModel: inboxModel})
    resp, err := logic.PushToInbox(&pb.PushToInboxReq{AuthorId: 9, PostId: 1001, CreatedAt: 1710000000000, FollowerIds: []int64{1, 0, 2}})

    require.NoError(t, err)
    assert.Equal(t, int64(2), resp.PushedCount)
    inboxModel.AssertExpectations(t)
}

func TestPushToInboxLogic_EmptyFollowers(t *testing.T) {
    logic := NewPushToInboxLogic(context.Background(), &svc.ServiceContext{})
    resp, err := logic.PushToInbox(&pb.PushToInboxReq{AuthorId: 9, PostId: 1001, CreatedAt: 1710000000000})
    require.NoError(t, err)
    assert.Equal(t, int64(0), resp.PushedCount)
}

func TestPushToInboxLogic_InvalidInput(t *testing.T) {
    logic := NewPushToInboxLogic(context.Background(), &svc.ServiceContext{})
    resp, err := logic.PushToInbox(&pb.PushToInboxReq{AuthorId: 0, PostId: 1001, CreatedAt: 1710000000000, FollowerIds: []int64{1}})
    require.Nil(t, resp)
    require.Error(t, err)
    assert.Equal(t, errx.ParamError, errx.CodeOf(err))
}

func TestPushToInboxLogic_ModelFailure(t *testing.T) {
    inboxModel := new(mockInboxModel)
    inboxModel.On("BatchInsertIgnore", mock.Anything, mock.Anything).Return(int64(0), errors.New("db down")).Once()
    logic := NewPushToInboxLogic(context.Background(), &svc.ServiceContext{InboxModel: inboxModel})
    resp, err := logic.PushToInbox(&pb.PushToInboxReq{AuthorId: 9, PostId: 1001, CreatedAt: 1710000000000, FollowerIds: []int64{1}})
    require.Nil(t, resp)
    require.Error(t, err)
    assert.Equal(t, errx.SystemError, errx.CodeOf(err))
    inboxModel.AssertExpectations(t)
}
```

- [ ] **Step 2: 运行测试确认失败**

Run:

```bash
go test -race ./app/feed/internal/logic -run TestPushToInboxLogic
```

Expected: fails because model custom methods and logic are not implemented.

- [ ] **Step 3: 实现 FeedInboxModel 自定义方法**

Modify `app/feed/internal/model/feed_inbox_model.go` so the custom section contains:

```go
package model

import (
    "context"
    "strings"

    "github.com/zeromicro/go-zero/core/stores/sqlx"
)

var _ FeedInboxModel = (*customFeedInboxModel)(nil)

type (
    FeedInboxModel interface {
        feedInboxModel
        withSession(session sqlx.Session) FeedInboxModel
        BatchInsertIgnore(ctx context.Context, rows []*FeedInbox) (int64, error)
        FindByUserBefore(ctx context.Context, userID, cursorCreatedAt, cursorPostID int64, limit int64) ([]*FeedInbox, error)
    }

    customFeedInboxModel struct {
        *defaultFeedInboxModel
    }
)

func NewFeedInboxModel(conn sqlx.SqlConn) FeedInboxModel {
    return &customFeedInboxModel{defaultFeedInboxModel: newFeedInboxModel(conn)}
}

func (m *customFeedInboxModel) withSession(session sqlx.Session) FeedInboxModel {
    return NewFeedInboxModel(sqlx.NewSqlConnFromSession(session))
}

func (m *customFeedInboxModel) BatchInsertIgnore(ctx context.Context, rows []*FeedInbox) (int64, error) {
    if len(rows) == 0 {
        return 0, nil
    }
    values := make([]string, 0, len(rows))
    args := make([]any, 0, len(rows)*4)
    for _, row := range rows {
        values = append(values, "(?, ?, ?, ?)")
        args = append(args, row.UserId, row.AuthorId, row.PostId, row.CreatedAt)
    }
    query := "INSERT IGNORE INTO feed_inbox (user_id, author_id, post_id, created_at) VALUES " + strings.Join(values, ",")
    ret, err := m.conn.ExecCtx(ctx, query, args...)
    if err != nil {
        return 0, err
    }
    return ret.RowsAffected()
}

func (m *customFeedInboxModel) FindByUserBefore(ctx context.Context, userID, cursorCreatedAt, cursorPostID int64, limit int64) ([]*FeedInbox, error) {
    query := `SELECT id, user_id, author_id, post_id, created_at
FROM feed_inbox
WHERE user_id = ? AND (created_at < ? OR (created_at = ? AND post_id < ?))
ORDER BY created_at DESC, post_id DESC
LIMIT ?`
    var rows []*FeedInbox
    if err := m.conn.QueryRowsCtx(ctx, &rows, query, userID, cursorCreatedAt, cursorCreatedAt, cursorPostID, limit); err != nil {
        return nil, err
    }
    return rows, nil
}
```

- [ ] **Step 4: 实现 FeedOutboxModel 自定义方法**

Modify `app/feed/internal/model/feed_outbox_model.go` so the custom section contains:

```go
package model

import (
    "context"
    "strings"

    "github.com/zeromicro/go-zero/core/stores/sqlx"
)

var _ FeedOutboxModel = (*customFeedOutboxModel)(nil)

type (
    FeedOutboxModel interface {
        feedOutboxModel
        withSession(session sqlx.Session) FeedOutboxModel
        InsertIgnore(ctx context.Context, row *FeedOutbox) error
        FindByAuthorsBefore(ctx context.Context, authorIDs []int64, cursorCreatedAt, cursorPostID int64, limit int64) ([]*FeedOutbox, error)
    }

    customFeedOutboxModel struct {
        *defaultFeedOutboxModel
    }
)

func NewFeedOutboxModel(conn sqlx.SqlConn) FeedOutboxModel {
    return &customFeedOutboxModel{defaultFeedOutboxModel: newFeedOutboxModel(conn)}
}

func (m *customFeedOutboxModel) withSession(session sqlx.Session) FeedOutboxModel {
    return NewFeedOutboxModel(sqlx.NewSqlConnFromSession(session))
}

func (m *customFeedOutboxModel) InsertIgnore(ctx context.Context, row *FeedOutbox) error {
    query := `INSERT IGNORE INTO feed_outbox (author_id, post_id, created_at) VALUES (?, ?, ?)`
    _, err := m.conn.ExecCtx(ctx, query, row.AuthorId, row.PostId, row.CreatedAt)
    return err
}

func (m *customFeedOutboxModel) FindByAuthorsBefore(ctx context.Context, authorIDs []int64, cursorCreatedAt, cursorPostID int64, limit int64) ([]*FeedOutbox, error) {
    if len(authorIDs) == 0 {
        return []*FeedOutbox{}, nil
    }
    placeholders := make([]string, 0, len(authorIDs))
    args := make([]any, 0, len(authorIDs)+4)
    for _, authorID := range authorIDs {
        placeholders = append(placeholders, "?")
        args = append(args, authorID)
    }
    args = append(args, cursorCreatedAt, cursorCreatedAt, cursorPostID, limit)
    query := `SELECT id, author_id, post_id, created_at
FROM feed_outbox
WHERE author_id IN (` + strings.Join(placeholders, ",") + `)
  AND (created_at < ? OR (created_at = ? AND post_id < ?))
ORDER BY created_at DESC, post_id DESC
LIMIT ?`
    var rows []*FeedOutbox
    if err := m.conn.QueryRowsCtx(ctx, &rows, query, args...); err != nil {
        return nil, err
    }
    return rows, nil
}
```

- [ ] **Step 5: 实现 PushToInbox logic**

Modify `app/feed/internal/logic/push_to_inbox_logic.go`:

```go
func (l *PushToInboxLogic) PushToInbox(in *pb.PushToInboxReq) (*pb.PushToInboxResp, error) {
    if in.AuthorId <= 0 || in.PostId <= 0 || in.CreatedAt <= 0 {
        return nil, errx.NewWithCode(errx.ParamError)
    }
    if len(in.FollowerIds) == 0 {
        return &pb.PushToInboxResp{PushedCount: 0}, nil
    }
    rows := make([]*model.FeedInbox, 0, len(in.FollowerIds))
    for _, followerID := range in.FollowerIds {
        if followerID <= 0 {
            continue
        }
        rows = append(rows, &model.FeedInbox{UserId: followerID, AuthorId: in.AuthorId, PostId: in.PostId, CreatedAt: in.CreatedAt})
    }
    if len(rows) == 0 {
        return &pb.PushToInboxResp{PushedCount: 0}, nil
    }
    affected, err := l.svcCtx.InboxModel.BatchInsertIgnore(l.ctx, rows)
    if err != nil {
        l.Errorw("InboxModel.BatchInsertIgnore failed", logx.Field("err", err.Error()))
        return nil, errx.NewWithCode(errx.SystemError)
    }
    return &pb.PushToInboxResp{PushedCount: affected}, nil
}
```

- [ ] **Step 6: 运行 PushToInbox 测试确认通过**

Run:

```bash
go test -race ./app/feed/internal/logic -run TestPushToInboxLogic
```

Expected:

```text
ok   esx/app/feed/internal/logic
```

## Task 5: 实现最小推荐流

**Files:**
- Modify: `app/feed/internal/logic/get_recommend_feed_logic.go`
- Create: `app/feed/internal/logic/get_recommend_feed_logic_test.go`

- [ ] **Step 1: 写推荐流单元测试**

Create `app/feed/internal/logic/get_recommend_feed_logic_test.go`:

```go
package logic

import (
    "context"
    "errors"
    "testing"

    "errx"
    "esx/app/content/contentservice"
    "esx/app/feed/internal/svc"
    "esx/app/feed/pb/xiaobaihe/feed/pb"

    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/mock"
    "github.com/stretchr/testify/require"
    "google.golang.org/grpc"
)

type mockContentService struct{ mock.Mock }

func (m *mockContentService) GetPostList(ctx context.Context, in *contentservice.GetPostListReq, opts ...grpc.CallOption) (*contentservice.GetPostListResp, error) {
    args := m.Called(ctx, in)
    if v := args.Get(0); v != nil {
        return v.(*contentservice.GetPostListResp), args.Error(1)
    }
    return nil, args.Error(1)
}
func (m *mockContentService) GetPostsByIds(ctx context.Context, in *contentservice.GetPostsByIdsReq, opts ...grpc.CallOption) (*contentservice.GetPostsByIdsResp, error) {
    args := m.Called(ctx, in)
    if v := args.Get(0); v != nil {
        return v.(*contentservice.GetPostsByIdsResp), args.Error(1)
    }
    return nil, args.Error(1)
}

func TestGetRecommendFeedLogic_Success(t *testing.T) {
    contentSvc := new(mockContentService)
    contentSvc.On("GetPostList", mock.Anything, &contentservice.GetPostListReq{Page: 1, PageSize: 2, SortBy: 3}).Return(&contentservice.GetPostListResp{
        Posts: []*contentservice.PostInfo{{Id: 11, AuthorId: 101, CreatedAt: 1001}, {Id: 12, AuthorId: 102, CreatedAt: 1000}},
        Total: 3,
    }, nil).Once()
    logic := NewGetRecommendFeedLogic(context.Background(), &svc.ServiceContext{ContentService: contentSvc})
    resp, err := logic.GetRecommendFeed(&pb.GetRecommendFeedReq{UserId: 1, Page: 1, PageSize: 2})
    require.NoError(t, err)
    require.Len(t, resp.Items, 2)
    assert.Equal(t, int32(2), resp.Items[0].FeedType)
    assert.True(t, resp.HasMore)
    contentSvc.AssertExpectations(t)
}

func TestGetRecommendFeedLogic_FallbackSort(t *testing.T) {
    contentSvc := new(mockContentService)
    contentSvc.On("GetPostList", mock.Anything, &contentservice.GetPostListReq{Page: 1, PageSize: 2, SortBy: 3}).Return(nil, errors.New("sort unsupported")).Once()
    contentSvc.On("GetPostList", mock.Anything, &contentservice.GetPostListReq{Page: 1, PageSize: 2, SortBy: 1}).Return(&contentservice.GetPostListResp{
        Posts: []*contentservice.PostInfo{{Id: 21, AuthorId: 201, CreatedAt: 2001}},
        Total: 1,
    }, nil).Once()
    logic := NewGetRecommendFeedLogic(context.Background(), &svc.ServiceContext{ContentService: contentSvc})
    resp, err := logic.GetRecommendFeed(&pb.GetRecommendFeedReq{UserId: 1, Page: 1, PageSize: 2})
    require.NoError(t, err)
    require.Len(t, resp.Items, 1)
    assert.False(t, resp.HasMore)
    contentSvc.AssertExpectations(t)
}

func TestGetRecommendFeedLogic_InvalidInput(t *testing.T) {
    logic := NewGetRecommendFeedLogic(context.Background(), &svc.ServiceContext{})
    resp, err := logic.GetRecommendFeed(&pb.GetRecommendFeedReq{UserId: 1, Page: 0, PageSize: 2})
    require.Nil(t, resp)
    require.Error(t, err)
    assert.Equal(t, errx.ParamError, errx.CodeOf(err))
}
```

- [ ] **Step 2: 运行测试确认失败**

Run:

```bash
go test -race ./app/feed/internal/logic -run TestGetRecommendFeedLogic
```

Expected: fails because `GetRecommendFeed` is not implemented.

- [ ] **Step 3: 实现推荐流 logic**

Modify `app/feed/internal/logic/get_recommend_feed_logic.go`:

```go
func (l *GetRecommendFeedLogic) GetRecommendFeed(in *pb.GetRecommendFeedReq) (*pb.GetRecommendFeedResp, error) {
    if in.Page <= 0 || in.PageSize <= 0 {
        return nil, errx.NewWithCode(errx.ParamError)
    }
    postResp, err := l.svcCtx.ContentService.GetPostList(l.ctx, &contentservice.GetPostListReq{Page: in.Page, PageSize: in.PageSize, SortBy: 3})
    if err != nil {
        postResp, err = l.svcCtx.ContentService.GetPostList(l.ctx, &contentservice.GetPostListReq{Page: in.Page, PageSize: in.PageSize, SortBy: 1})
        if err != nil {
            l.Errorw("ContentService.GetPostList failed", logx.Field("err", err.Error()))
            return nil, errx.NewWithCode(errx.SystemError)
        }
    }
    items := make([]*pb.FeedItem, 0, len(postResp.Posts))
    for _, post := range postResp.Posts {
        items = append(items, &pb.FeedItem{PostId: post.Id, AuthorId: post.AuthorId, CreatedAt: post.CreatedAt, FeedType: 2})
    }
    return &pb.GetRecommendFeedResp{Items: items, HasMore: int64(in.Page*in.PageSize) < postResp.Total}, nil
}
```

- [ ] **Step 4: 运行推荐流测试确认通过**

Run:

```bash
go test -race ./app/feed/internal/logic -run TestGetRecommendFeedLogic
```

Expected:

```text
ok   esx/app/feed/internal/logic
```

## Task 6: 实现关注流读取与合并

**Files:**
- Modify: `app/feed/internal/logic/get_follow_feed_logic.go`
- Create: `app/feed/internal/logic/get_follow_feed_logic_test.go`

- [ ] **Step 1: 写关注流单元测试**

Create `app/feed/internal/logic/get_follow_feed_logic_test.go`:

```go
package logic

import (
    "context"
    "errors"
    "testing"

    "errx"
    "esx/app/content/contentservice"
    "esx/app/feed/internal/model"
    "esx/app/feed/internal/svc"
    "esx/app/feed/pb/xiaobaihe/feed/pb"
    "user/userservice"

    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/mock"
    "github.com/stretchr/testify/require"
    "google.golang.org/grpc"
)

type mockOutboxModel struct{ mock.Mock }
func (m *mockOutboxModel) InsertIgnore(ctx context.Context, row *model.FeedOutbox) error { return m.Called(ctx, row).Error(0) }
func (m *mockOutboxModel) FindByAuthorsBefore(ctx context.Context, authorIDs []int64, cursorCreatedAt, cursorPostID int64, limit int64) ([]*model.FeedOutbox, error) {
    args := m.Called(ctx, authorIDs, cursorCreatedAt, cursorPostID, limit)
    if v := args.Get(0); v != nil { return v.([]*model.FeedOutbox), args.Error(1) }
    return nil, args.Error(1)
}

type mockUserService struct{ mock.Mock }
func (m *mockUserService) GetUser(ctx context.Context, in *userservice.GetUserReq, opts ...grpc.CallOption) (*userservice.GetUserResp, error) {
    args := m.Called(ctx, in)
    if v := args.Get(0); v != nil { return v.(*userservice.GetUserResp), args.Error(1) }
    return nil, args.Error(1)
}
func (m *mockUserService) GetFollowers(ctx context.Context, in *userservice.GetFollowersReq, opts ...grpc.CallOption) (*userservice.GetFollowersResp, error) {
    args := m.Called(ctx, in)
    if v := args.Get(0); v != nil { return v.(*userservice.GetFollowersResp), args.Error(1) }
    return nil, args.Error(1)
}
func (m *mockUserService) GetFollowing(ctx context.Context, in *userservice.GetFollowingReq, opts ...grpc.CallOption) (*userservice.GetFollowingResp, error) {
    args := m.Called(ctx, in)
    if v := args.Get(0); v != nil { return v.(*userservice.GetFollowingResp), args.Error(1) }
    return nil, args.Error(1)
}

func TestGetFollowFeedLogic_InboxAndBigVOutboxMerge(t *testing.T) {
    inbox := new(mockInboxModel)
    outbox := new(mockOutboxModel)
    userSvc := new(mockUserService)
    contentSvc := new(mockContentService)
    userSvc.On("GetFollowing", mock.Anything, &userservice.GetFollowingReq{UserId: 1, Page: 1, PageSize: 500}).Return(&userservice.GetFollowingResp{Users: []*userservice.UserInfo{{Id: 99, FollowerCount: 10000}}}, nil).Once()
    inbox.On("FindByUserBefore", mock.Anything, int64(1), int64(math.MaxInt64), int64(math.MaxInt64), int64(4)).Return([]*model.FeedInbox{{PostId: 101, AuthorId: 11, CreatedAt: 1100}, {PostId: 102, AuthorId: 12, CreatedAt: 1050}}, nil).Once()
    outbox.On("FindByAuthorsBefore", mock.Anything, []int64{99}, int64(math.MaxInt64), int64(math.MaxInt64), int64(4)).Return([]*model.FeedOutbox{{PostId: 201, AuthorId: 99, CreatedAt: 1200}, {PostId: 202, AuthorId: 99, CreatedAt: 1000}}, nil).Once()
    contentSvc.On("GetPostsByIds", mock.Anything, &contentservice.GetPostsByIdsReq{PostIds: []int64{201, 101, 102}}).Return(&contentservice.GetPostsByIdsResp{}, nil).Once()
    logic := NewGetFollowFeedLogic(context.Background(), &svc.ServiceContext{InboxModel: inbox, OutboxModel: outbox, UserService: userSvc, ContentService: contentSvc, BigVThreshold: 10000, FanoutBatchSize: 500})
    resp, err := logic.GetFollowFeed(&pb.GetFollowFeedReq{UserId: 1, PageSize: 3})
    require.NoError(t, err)
    assert.Equal(t, []int64{201, 101, 102}, []int64{resp.Items[0].PostId, resp.Items[1].PostId, resp.Items[2].PostId})
    assert.True(t, resp.HasMore)
    assert.Equal(t, int64(102), resp.NextCursorPostId)
}

func TestGetFollowFeedLogic_EmptyFeed(t *testing.T) {
    inbox := new(mockInboxModel)
    outbox := new(mockOutboxModel)
    userSvc := new(mockUserService)
    userSvc.On("GetFollowing", mock.Anything, &userservice.GetFollowingReq{UserId: 1, Page: 1, PageSize: 500}).Return(&userservice.GetFollowingResp{}, nil).Once()
    inbox.On("FindByUserBefore", mock.Anything, int64(1), int64(math.MaxInt64), int64(math.MaxInt64), int64(3)).Return([]*model.FeedInbox{}, nil).Once()
    outbox.On("FindByAuthorsBefore", mock.Anything, []int64{}, int64(math.MaxInt64), int64(math.MaxInt64), int64(3)).Return([]*model.FeedOutbox{}, nil).Once()
    logic := NewGetFollowFeedLogic(context.Background(), &svc.ServiceContext{InboxModel: inbox, OutboxModel: outbox, UserService: userSvc, BigVThreshold: 10000, FanoutBatchSize: 500})
    resp, err := logic.GetFollowFeed(&pb.GetFollowFeedReq{UserId: 1, PageSize: 2})
    require.NoError(t, err)
    assert.Empty(t, resp.Items)
    assert.False(t, resp.HasMore)
}

func TestGetFollowFeedLogic_UserRpcFailure(t *testing.T) {
    userSvc := new(mockUserService)
    userSvc.On("GetFollowing", mock.Anything, mock.Anything).Return(nil, errors.New("rpc down")).Once()
    logic := NewGetFollowFeedLogic(context.Background(), &svc.ServiceContext{UserService: userSvc, FanoutBatchSize: 500})
    resp, err := logic.GetFollowFeed(&pb.GetFollowFeedReq{UserId: 1, PageSize: 2})
    require.Nil(t, resp)
    require.Error(t, err)
    assert.Equal(t, errx.SystemError, errx.CodeOf(err))
}

func TestGetFollowFeedLogic_InvalidInput(t *testing.T) {
    logic := NewGetFollowFeedLogic(context.Background(), &svc.ServiceContext{})
    resp, err := logic.GetFollowFeed(&pb.GetFollowFeedReq{UserId: 0, PageSize: 2})
    require.Nil(t, resp)
    require.Error(t, err)
    assert.Equal(t, errx.ParamError, errx.CodeOf(err))
}
```

- [ ] **Step 2: 运行测试确认失败**

Run:

```bash
go test -race ./app/feed/internal/logic -run TestGetFollowFeedLogic
```

Expected: fails because `GetFollowFeed` and merge helper are not implemented.

- [ ] **Step 3: 实现关注流 logic**

Modify `app/feed/internal/logic/get_follow_feed_logic.go`:

```go
func (l *GetFollowFeedLogic) GetFollowFeed(in *pb.GetFollowFeedReq) (*pb.GetFollowFeedResp, error) {
    if in.UserId <= 0 || in.PageSize <= 0 {
        return nil, errx.NewWithCode(errx.ParamError)
    }
    cursorCreatedAt := in.CursorCreatedAt
    if cursorCreatedAt == 0 { cursorCreatedAt = math.MaxInt64 }
    cursorPostID := in.CursorPostId
    if cursorPostID == 0 { cursorPostID = math.MaxInt64 }
    fanoutPageSize := int32(l.svcCtx.FanoutBatchSize)
    if fanoutPageSize <= 0 { fanoutPageSize = 500 }

    followingResp, err := l.svcCtx.UserService.GetFollowing(l.ctx, &userservice.GetFollowingReq{UserId: in.UserId, Page: 1, PageSize: fanoutPageSize})
    if err != nil {
        l.Errorw("UserService.GetFollowing failed", logx.Field("err", err.Error()))
        return nil, errx.NewWithCode(errx.SystemError)
    }
    bigVAuthorIDs := make([]int64, 0)
    for _, user := range followingResp.Users {
        if user.FollowerCount >= l.svcCtx.BigVThreshold {
            bigVAuthorIDs = append(bigVAuthorIDs, user.Id)
        }
    }
    queryLimit := int64(in.PageSize) + 1
    inboxRows, err := l.svcCtx.InboxModel.FindByUserBefore(l.ctx, in.UserId, cursorCreatedAt, cursorPostID, queryLimit)
    if err != nil { return nil, errx.NewWithCode(errx.SystemError) }
    outboxRows, err := l.svcCtx.OutboxModel.FindByAuthorsBefore(l.ctx, bigVAuthorIDs, cursorCreatedAt, cursorPostID, queryLimit)
    if err != nil { return nil, errx.NewWithCode(errx.SystemError) }
    items := mergeFeedRows(inboxRows, outboxRows, int(queryLimit))
    hasMore := len(items) > int(in.PageSize)
    if hasMore { items = items[:in.PageSize] }
    if len(items) == 0 {
        return &pb.GetFollowFeedResp{Items: []*pb.FeedItem{}, HasMore: false}, nil
    }
    postIDs := make([]int64, 0, len(items))
    for _, item := range items { postIDs = append(postIDs, item.PostId) }
    if _, err := l.svcCtx.ContentService.GetPostsByIds(l.ctx, &contentservice.GetPostsByIdsReq{PostIds: postIDs}); err != nil {
        l.Errorw("ContentService.GetPostsByIds failed", logx.Field("err", err.Error()))
        return nil, errx.NewWithCode(errx.SystemError)
    }
    last := items[len(items)-1]
    return &pb.GetFollowFeedResp{Items: items, HasMore: hasMore, NextCursorCreatedAt: last.CreatedAt, NextCursorPostId: last.PostId}, nil
}

func mergeFeedRows(inboxRows []*model.FeedInbox, outboxRows []*model.FeedOutbox, limit int) []*pb.FeedItem {
    items := make([]*pb.FeedItem, 0, len(inboxRows)+len(outboxRows))
    seen := make(map[int64]struct{}, len(inboxRows)+len(outboxRows))
    for _, row := range inboxRows {
        if _, ok := seen[row.PostId]; ok { continue }
        seen[row.PostId] = struct{}{}
        items = append(items, &pb.FeedItem{PostId: row.PostId, AuthorId: row.AuthorId, CreatedAt: row.CreatedAt, FeedType: 1})
    }
    for _, row := range outboxRows {
        if _, ok := seen[row.PostId]; ok { continue }
        seen[row.PostId] = struct{}{}
        items = append(items, &pb.FeedItem{PostId: row.PostId, AuthorId: row.AuthorId, CreatedAt: row.CreatedAt, FeedType: 1})
    }
    sort.Slice(items, func(i, j int) bool {
        if items[i].CreatedAt == items[j].CreatedAt { return items[i].PostId > items[j].PostId }
        return items[i].CreatedAt > items[j].CreatedAt
    })
    if len(items) > limit { return items[:limit] }
    return items
}
```

- [ ] **Step 4: 运行关注流测试确认通过**

Run:

```bash
go test -race ./app/feed/internal/logic -run TestGetFollowFeedLogic
```

Expected:

```text
ok   esx/app/feed/internal/logic
```

## Task 7: 接入 Content 发帖事件与 Feed MQ Consumer

**Files:**
- Modify: `app/content/internal/config/config.go`
- Modify: `app/content/internal/svc/service_context.go`
- Modify: `app/content/etc/content.yaml`
- Modify: `app/content/internal/logic/create_post_logic.go`
- Modify: `app/content/internal/logic/create_post_logic_test.go`
- Create: `app/feed/internal/mqs/post_publish_consumer.go`
- Create: `app/feed/internal/mqs/post_publish_consumer_test.go`
- Modify: `app/feed/feed.go`

- [ ] **Step 1: 抽象 content MQ producer 并写测试**

Modify `app/content/internal/svc/service_context.go` to add an interface and field:

```go
type MQProducer interface {
    SendSyncWithTag(ctx context.Context, topic, tag string, body []byte) (*primitive.SendResult, error)
}

type ServiceContext struct {
    Config       config.Config
    Conn         sqlx.SqlConn
    PostModel    model.PostModel
    CommentModel model.CommentModel
    TagModel     model.TagModel
    PostTagModel model.PostTagModel
    MQProducer   MQProducer
}
```

In `NewServiceContext`, keep `mqx.NewProducer(c.MQ)` guarded by `if c.MQ.NameServer != ""` and assign it to `MQProducer`.

Add to `app/content/internal/logic/create_post_logic_test.go`:

```go
type fakeMQProducer struct {
    topic string
    tag   string
    body  []byte
}

func (f *fakeMQProducer) SendSyncWithTag(ctx context.Context, topic, tag string, body []byte) (*primitive.SendResult, error) {
    f.topic = topic
    f.tag = tag
    f.body = append([]byte(nil), body...)
    return &primitive.SendResult{}, nil
}

func TestCreatePostLogic_PublishPostCreatedEvent(t *testing.T) {
    mq := &fakeMQProducer{}
    svcCtx := newCreatePostTestServiceContext(t)
    svcCtx.MQProducer = mq
    logic := NewCreatePostLogic(context.Background(), svcCtx)
    resp, err := logic.CreatePost(&pb.CreatePostReq{AuthorId: 9, Title: "t", Content: "content"})
    require.NoError(t, err)
    require.NotZero(t, resp.PostId)
    assert.Equal(t, mqx.TopicPostCreate, mq.topic)
    assert.Equal(t, mqx.TagDefault, mq.tag)
    var got map[string]int64
    require.NoError(t, json.Unmarshal(mq.body, &got))
    assert.Equal(t, int64(9), got["author_id"])
    assert.Equal(t, resp.PostId, got["post_id"])
    assert.NotZero(t, got["created_at"])
}
```

- [ ] **Step 2: 运行 content 事件测试确认失败**

Run:

```bash
go test -race ./app/content/internal/logic -run TestCreatePostLogic_PublishPostCreatedEvent
```

Expected: fails because event publish is not implemented.

- [ ] **Step 3: 实现 content MQ 配置与发帖事件**

Modify `app/content/internal/config/config.go`:

```go
package config

import (
    "mqx"
    "github.com/zeromicro/go-zero/zrpc"
)

type Config struct {
    zrpc.RpcServerConf
    DataSource string
    MQ         mqx.ProducerConfig
}
```

Append to `app/content/etc/content.yaml`:

```yaml
MQ:
  NameServer: "${MQ_NAMESERVER}"
  GroupName: "content-producer-group"
  Retry: 3
  SendTimeout: 3000
```

Modify `app/content/internal/logic/create_post_logic.go` after successful tag insertion and before return:

```go
if l.svcCtx.MQProducer != nil {
    body, marshalErr := json.Marshal(map[string]int64{"post_id": id, "author_id": in.AuthorId, "created_at": time.Now().UnixMilli()})
    if marshalErr != nil {
        l.Errorw("marshal post created event failed", logx.Field("err", marshalErr.Error()))
        return nil, errx.NewWithCode(errx.SystemError)
    }
    if _, sendErr := l.svcCtx.MQProducer.SendSyncWithTag(l.ctx, mqx.TopicPostCreate, mqx.TagDefault, body); sendErr != nil {
        l.Errorw("publish post created event failed", logx.Field("err", sendErr.Error()))
        return nil, errx.NewWithCode(errx.SystemError)
    }
}
```

- [ ] **Step 4: 写 consumer handler 单元测试**

Create `app/feed/internal/mqs/post_publish_consumer_test.go`:

```go
package mqs

import (
    "context"
    "testing"

    "esx/app/feed/internal/model"
    "esx/app/feed/internal/svc"
    "user/userservice"

    "github.com/stretchr/testify/mock"
    "github.com/stretchr/testify/require"
)

func TestHandlePostPublished_SmallVFanout(t *testing.T) {
    inbox := new(mockInboxModel)
    outbox := new(mockOutboxModel)
    userSvc := new(mockUserService)
    event := postPublishedMessage{PostId: 1001, AuthorId: 9, CreatedAt: 1710000000000}
    userSvc.On("GetUser", mock.Anything, &userservice.GetUserReq{UserId: 9}).Return(&userservice.GetUserResp{User: &userservice.UserInfo{Id: 9, FollowerCount: 99}}, nil).Once()
    outbox.On("InsertIgnore", mock.Anything, &model.FeedOutbox{AuthorId: 9, PostId: 1001, CreatedAt: 1710000000000}).Return(nil).Once()
    userSvc.On("GetFollowers", mock.Anything, &userservice.GetFollowersReq{UserId: 9, Page: 1, PageSize: 500}).Return(&userservice.GetFollowersResp{Users: []*userservice.UserInfo{{Id: 1}, {Id: 2}, {Id: 3}}}, nil).Once()
    inbox.On("BatchInsertIgnore", mock.Anything, mock.MatchedBy(func(rows []*model.FeedInbox) bool { return len(rows) == 3 && rows[0].UserId == 1 })).Return(int64(3), nil).Once()
    err := handlePostPublished(context.Background(), &svc.ServiceContext{InboxModel: inbox, OutboxModel: outbox, UserService: userSvc, BigVThreshold: 10000, FanoutBatchSize: 500}, event)
    require.NoError(t, err)
    inbox.AssertExpectations(t)
    outbox.AssertExpectations(t)
    userSvc.AssertExpectations(t)
}

func TestHandlePostPublished_BigVOutboxOnly(t *testing.T) {
    inbox := new(mockInboxModel)
    outbox := new(mockOutboxModel)
    userSvc := new(mockUserService)
    event := postPublishedMessage{PostId: 1001, AuthorId: 9, CreatedAt: 1710000000000}
    userSvc.On("GetUser", mock.Anything, &userservice.GetUserReq{UserId: 9}).Return(&userservice.GetUserResp{User: &userservice.UserInfo{Id: 9, FollowerCount: 10000}}, nil).Once()
    outbox.On("InsertIgnore", mock.Anything, &model.FeedOutbox{AuthorId: 9, PostId: 1001, CreatedAt: 1710000000000}).Return(nil).Once()
    err := handlePostPublished(context.Background(), &svc.ServiceContext{InboxModel: inbox, OutboxModel: outbox, UserService: userSvc, BigVThreshold: 10000, FanoutBatchSize: 500}, event)
    require.NoError(t, err)
    inbox.AssertNotCalled(t, "BatchInsertIgnore", mock.Anything, mock.Anything)
    outbox.AssertExpectations(t)
    userSvc.AssertExpectations(t)
}
```

- [ ] **Step 5: 实现 Feed consumer**

Create `app/feed/internal/mqs/post_publish_consumer.go`:

```go
package mqs

import (
    "context"
    "encoding/json"
    "fmt"

    "esx/app/feed/internal/model"
    "esx/app/feed/internal/svc"
    "mqx"
    "user/userservice"

    "github.com/apache/rocketmq-client-go/v2/consumer"
    "github.com/apache/rocketmq-client-go/v2/primitive"
    "github.com/zeromicro/go-zero/core/logx"
)

type postPublishedMessage struct {
    PostId    int64 `json:"post_id"`
    AuthorId  int64 `json:"author_id"`
    CreatedAt int64 `json:"created_at"`
}

func NewPostPublishConsumer(svcCtx *svc.ServiceContext) (*mqx.Consumer, error) {
    c, err := mqx.NewConsumer(svcCtx.Config.MQ)
    if err != nil { return nil, fmt.Errorf("create post publish consumer: %w", err) }
    handler := func(ctx context.Context, msgs ...*primitive.MessageExt) (consumer.ConsumeResult, error) {
        for _, msg := range msgs {
            var event postPublishedMessage
            if err := json.Unmarshal(msg.Body, &event); err != nil {
                logx.WithContext(ctx).Errorw("unmarshal post created event failed", logx.Field("msg_id", msg.MsgId), logx.Field("err", err.Error()))
                continue
            }
            if err := handlePostPublished(ctx, svcCtx, event); err != nil {
                logx.WithContext(ctx).Errorw("handle post created event failed", logx.Field("post_id", event.PostId), logx.Field("err", err.Error()))
                return consumer.ConsumeRetryLater, nil
            }
        }
        return consumer.ConsumeSuccess, nil
    }
    if err := c.SubscribeWithTopic(mqx.TopicPostCreate, mqx.TagDefault, handler); err != nil {
        return nil, fmt.Errorf("subscribe post create topic: %w", err)
    }
    return c, nil
}

func handlePostPublished(ctx context.Context, svcCtx *svc.ServiceContext, event postPublishedMessage) error {
    userResp, err := svcCtx.UserService.GetUser(ctx, &userservice.GetUserReq{UserId: event.AuthorId})
    if err != nil { return err }
    if err := svcCtx.OutboxModel.InsertIgnore(ctx, &model.FeedOutbox{AuthorId: event.AuthorId, PostId: event.PostId, CreatedAt: event.CreatedAt}); err != nil { return err }
    if userResp.User == nil || userResp.User.FollowerCount >= svcCtx.BigVThreshold { return nil }
    pageSize := int32(svcCtx.FanoutBatchSize)
    if pageSize <= 0 { pageSize = 500 }
    followersResp, err := svcCtx.UserService.GetFollowers(ctx, &userservice.GetFollowersReq{UserId: event.AuthorId, Page: 1, PageSize: pageSize})
    if err != nil { return err }
    rows := make([]*model.FeedInbox, 0, len(followersResp.Users))
    for _, user := range followersResp.Users {
        if user.Id <= 0 { continue }
        rows = append(rows, &model.FeedInbox{UserId: user.Id, AuthorId: event.AuthorId, PostId: event.PostId, CreatedAt: event.CreatedAt})
    }
    _, err = svcCtx.InboxModel.BatchInsertIgnore(ctx, rows)
    return err
}
```

- [ ] **Step 6: 接入 Feed 服务入口**

Modify `app/feed/feed.go` after `svcCtx := svc.NewServiceContext(c)`:

```go
var postConsumer *mqx.Consumer
if c.MQ.NameServer != "" {
    var err error
    postConsumer, err = mqs.NewPostPublishConsumer(svcCtx)
    if err != nil {
        logx.Must(err)
    }
    if err := postConsumer.Start(); err != nil {
        logx.Must(err)
    }
    defer postConsumer.Shutdown()
}
```

- [ ] **Step 7: 运行 MQ 测试确认通过**

Run:

```bash
go test -race ./app/feed/internal/mqs ./app/content/internal/logic -run 'TestHandlePostPublished|TestCreatePostLogic_PublishPostCreatedEvent'
```

Expected:

```text
ok   esx/app/feed/internal/mqs
ok   esx/app/content/internal/logic
```

## Task 8: 集成测试与最终校验

**Files:**
- Create: `app/feed/internal/model/feed_model_integration_test.go`
- Create: `app/feed/internal/logic/feed_integration_test.go`

- [ ] **Step 1: 写 model 集成测试**

Create `app/feed/internal/model/feed_model_integration_test.go` with real MySQL testcontainers setup and these tests:

```go
func TestFeedInboxModel_BatchInsertIgnore_Deduplicates(t *testing.T) {
    conn, cleanup := newFeedTestDB(t)
    defer cleanup()
    model := NewFeedInboxModel(conn)
    rows := []*FeedInbox{{UserId: 1, AuthorId: 9, PostId: 1001, CreatedAt: 1000}, {UserId: 1, AuthorId: 9, PostId: 1001, CreatedAt: 1000}}
    affected, err := model.BatchInsertIgnore(context.Background(), rows)
    require.NoError(t, err)
    assert.Equal(t, int64(1), affected)
}

func TestFeedInboxModel_FindByUserBefore_OrderStable(t *testing.T) {
    conn, cleanup := newFeedTestDB(t)
    defer cleanup()
    model := NewFeedInboxModel(conn)
    _, err := model.BatchInsertIgnore(context.Background(), []*FeedInbox{{UserId: 1, AuthorId: 9, PostId: 1001, CreatedAt: 1000}, {UserId: 1, AuthorId: 9, PostId: 1002, CreatedAt: 1000}, {UserId: 1, AuthorId: 9, PostId: 1003, CreatedAt: 999}})
    require.NoError(t, err)
    rows, err := model.FindByUserBefore(context.Background(), 1, math.MaxInt64, math.MaxInt64, 3)
    require.NoError(t, err)
    assert.Equal(t, []int64{1002, 1001, 1003}, []int64{rows[0].PostId, rows[1].PostId, rows[2].PostId})
}
```

The helper `newFeedTestDB(t)` must start MySQL 8 via testcontainers, execute `deploy/sql/xbh_feed.sql`, return `sqlx.NewMysql(dsn)`, and terminate the container in cleanup.

- [ ] **Step 2: 写 fanout 集成测试**

Create `app/feed/internal/logic/feed_integration_test.go` with the same `newFeedTestDB(t)` helper and these tests:

```go
package logic

import (
    "context"
    "testing"

    "esx/app/feed/internal/model"
    "esx/app/feed/internal/mqs"
    "esx/app/feed/internal/svc"
    "user/userservice"

    "github.com/stretchr/testify/assert"
    "github.com/stretchr/testify/mock"
    "github.com/stretchr/testify/require"
)

func TestFeedFanout_SmallV_WritesInboxAndOutbox(t *testing.T) {
    conn, cleanup := newFeedTestDB(t)
    defer cleanup()
    userSvc := new(mockUserService)
    userSvc.On("GetUser", mock.Anything, &userservice.GetUserReq{UserId: 9}).Return(&userservice.GetUserResp{User: &userservice.UserInfo{Id: 9, FollowerCount: 99}}, nil).Once()
    userSvc.On("GetFollowers", mock.Anything, &userservice.GetFollowersReq{UserId: 9, Page: 1, PageSize: 500}).Return(&userservice.GetFollowersResp{Users: []*userservice.UserInfo{{Id: 1}, {Id: 2}, {Id: 3}}}, nil).Once()
    svcCtx := &svc.ServiceContext{
        InboxModel:      model.NewFeedInboxModel(conn),
        OutboxModel:     model.NewFeedOutboxModel(conn),
        UserService:     userSvc,
        BigVThreshold:   10000,
        FanoutBatchSize: 500,
    }

    err := mqs.HandlePostPublishedForTest(context.Background(), svcCtx, 1001, 9, 1710000000000)

    require.NoError(t, err)
    var outboxCount int64
    require.NoError(t, conn.QueryRowCtx(context.Background(), &outboxCount, "SELECT COUNT(*) FROM feed_outbox"))
    assert.Equal(t, int64(1), outboxCount)
    var inboxCount int64
    require.NoError(t, conn.QueryRowCtx(context.Background(), &inboxCount, "SELECT COUNT(*) FROM feed_inbox"))
    assert.Equal(t, int64(3), inboxCount)
    userSvc.AssertExpectations(t)
}

func TestFeedFanout_BigV_WritesOutboxOnly(t *testing.T) {
    conn, cleanup := newFeedTestDB(t)
    defer cleanup()
    userSvc := new(mockUserService)
    userSvc.On("GetUser", mock.Anything, &userservice.GetUserReq{UserId: 9}).Return(&userservice.GetUserResp{User: &userservice.UserInfo{Id: 9, FollowerCount: 10000}}, nil).Once()
    svcCtx := &svc.ServiceContext{
        InboxModel:      model.NewFeedInboxModel(conn),
        OutboxModel:     model.NewFeedOutboxModel(conn),
        UserService:     userSvc,
        BigVThreshold:   10000,
        FanoutBatchSize: 500,
    }

    err := mqs.HandlePostPublishedForTest(context.Background(), svcCtx, 1001, 9, 1710000000000)

    require.NoError(t, err)
    var outboxCount int64
    require.NoError(t, conn.QueryRowCtx(context.Background(), &outboxCount, "SELECT COUNT(*) FROM feed_outbox"))
    assert.Equal(t, int64(1), outboxCount)
    var inboxCount int64
    require.NoError(t, conn.QueryRowCtx(context.Background(), &inboxCount, "SELECT COUNT(*) FROM feed_inbox"))
    assert.Equal(t, int64(0), inboxCount)
    userSvc.AssertExpectations(t)
}
```

Add this test-only exported wrapper in `app/feed/internal/mqs/post_publish_consumer_test.go` so the integration test can call the unexported handler without exporting production API:

```go
func HandlePostPublishedForTest(ctx context.Context, svcCtx *svc.ServiceContext, postID, authorID, createdAt int64) error {
    return handlePostPublished(ctx, svcCtx, postPublishedMessage{PostId: postID, AuthorId: authorID, CreatedAt: createdAt})
}
```

- [ ] **Step 3: 运行目标测试**

Run:

```bash
go test -race ./app/user/internal/logic ./app/feed/internal/svc ./app/feed/internal/logic ./app/feed/internal/mqs ./app/content/internal/logic
```

Expected:

```text
ok   user/internal/logic
ok   esx/app/feed/internal/svc
ok   esx/app/feed/internal/logic
ok   esx/app/feed/internal/mqs
ok   esx/app/content/internal/logic
```

- [ ] **Step 4: 运行仓库级校验**

Run:

```bash
go test ./... -race -cover
goctl check
go vet ./...
golangci-lint run
```

Expected: all commands pass. If `goctl` or `golangci-lint` is not installed, record the exact missing-tool error and do not claim full completion.

## 自检结果

- **Spec coverage:** 关注/粉丝 RPC、Feed proto/schema、PushToInbox、推荐流、关注流、小 V 推/大 V 拉、MQ 事件、集成测试和最终校验均有对应任务。
- **Placeholder scan:** 原计划中的空测试函数、对称实现、补两个、后续调整等占位已替换为具体代码或明确失败/通过命令。
- **Type consistency:** Feed 单元测试统一依赖 `svc.InboxModel`/`svc.OutboxModel`/`svc.UserService`/`svc.ContentService` 最小接口，避免 mock goctl 全接口；content MQ 使用接口字段避免 mock `*mqx.Producer` 具体类型。
- **Known risk:** Task 8 的 testcontainers helper 需要按项目现有集成测试风格落地；若环境无法拉取 MySQL 镜像，应记录阻塞并保留单元测试通过证据。

Plan complete and saved to `docs/superpowers/plans/2026-04-23-feed-week2-redesign.md`. Two execution options:

1. **Subagent-Driven (recommended)** - dispatch a fresh worker per task, review between tasks, fast iteration.
2. **Inline Execution** - execute tasks in this session using executing-plans with checkpoints.

Which approach?
