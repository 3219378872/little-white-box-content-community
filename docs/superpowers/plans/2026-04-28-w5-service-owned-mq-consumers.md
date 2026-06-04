# W5 服务归属 MQ Consumer 集成 实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 按 W5 语义新增 5 个服务归属式独立 MQ consumer 入口（Feed/Message/Media/Search/Recommend），将现有 Feed/Message/Media 的消费职责从 RPC 进程迁移到 `app/*/mq`，Search/Recommend 交付可启动可测试的骨架。

**Architecture:** 每个 consumer 拥有独立 `config/svc/mqs/logic/model`（或 `storage`/`indexer`/`store`），通过 RocketMQ push consumer 订阅 topic，consumer callback 调用独立 logic，logic 通过 svc 注入的依赖访问 DB/Redis/RPC/S3。禁止复用 RPC `internal/` 包。

**Tech Stack:** go-zero v1.10.1 · RocketMQ 5.1.0 (rocketmq-client-go v2) · MySQL 8.0 (go-zero sqlx) · Redis 7 · MinIO (minio-go v7) · testify v1.11.1

---

## 文件结构总览

```
app/feed/mq/                        app/message/mq/
├── etc/feed-consumer.yaml          ├── etc/message-consumer.yaml
├── main.go                         ├── main.go
└── internal/                       └── internal/
    ├── config/config.go                ├── config/config.go
    ├── svc/service_context.go          ├── svc/
    ├── mqs/                            │   ├── service_context.go
    │   ├── post_publish_consumer.go    │   └── redis_store.go
    │   └── post_publish_consumer_test.go   ├── mqs/
    ├── logic/                          │   ├── message_consumer.go
    │   ├── fanout.go                   │   └── message_consumer_test.go
    │   └── fanout_test.go             └── model/
    └── model/                              ├── notification_model.go
        ├── feed_outbox_model.go            └── vars.go
        ├── feed_inbox_model.go
        └── vars.go

app/media/mq/                       app/search/mq/
├── etc/media-consumer.yaml         ├── etc/search-consumer.yaml
├── main.go                         ├── main.go
└── internal/                       └── internal/
    ├── config/config.go                ├── config/config.go
    ├── svc/service_context.go          ├── svc/service_context.go
    ├── mqs/                            ├── mqs/
    │   ├── media_cleanup_consumer.go   │   ├── search_consumer.go
    │   └── media_cleanup_consumer_test.go  │   └── search_consumer_test.go
    └── storage/                        └── indexer/
        └── s3.go                           └── indexer.go

app/recommend/mq/
├── etc/recommend-consumer.yaml
├── main.go
└── internal/
    ├── config/config.go
    ├── svc/service_context.go
    ├── mqs/
    │   ├── recommend_consumer.go
    │   └── recommend_consumer_test.go
    └── store/
        └── behavior_store.go
```

---

### Task 1: Feed MQ — Model 层

**Files:**
- Create: `app/feed/mq/internal/model/vars.go`
- Create: `app/feed/mq/internal/model/feed_outbox_model.go`
- Create: `app/feed/mq/internal/model/feed_inbox_model.go`

- [ ] **Step 1: 创建 model/vars.go**

```go
package model

import "github.com/zeromicro/go-zero/core/stores/sqlx"

var ErrNotFound = sqlx.ErrNotFound
```

- [ ] **Step 2: 创建 FeedOutbox model（含 InsertIgnore）**

`app/feed/mq/internal/model/feed_outbox_model.go`:

```go
package model

import (
	"context"

	"github.com/zeromicro/go-zero/core/stores/sqlx"
)

type FeedOutbox struct {
	Id        int64 `db:"id"`
	AuthorId  int64 `db:"author_id"`
	PostId    int64 `db:"post_id"`
	CreatedAt int64 `db:"created_at"`
}

type FeedOutboxModel interface {
	InsertIgnore(ctx context.Context, row *FeedOutbox) error
}

type feedOutboxModel struct {
	conn  sqlx.SqlConn
	table string
}

func NewFeedOutboxModel(conn sqlx.SqlConn) FeedOutboxModel {
	return &feedOutboxModel{conn: conn, table: "feed_outbox"}
}

func (m *feedOutboxModel) InsertIgnore(ctx context.Context, row *FeedOutbox) error {
	query := "INSERT IGNORE INTO feed_outbox (author_id, post_id, created_at) VALUES (?, ?, ?)"
	_, err := m.conn.ExecCtx(ctx, query, row.AuthorId, row.PostId, row.CreatedAt)
	return err
}
```

- [ ] **Step 3: 创建 FeedInbox model（含 BatchInsertIgnore）**

`app/feed/mq/internal/model/feed_inbox_model.go`:

```go
package model

import (
	"context"
	"strings"

	"github.com/zeromicro/go-zero/core/stores/sqlx"
)

type FeedInbox struct {
	Id        int64 `db:"id"`
	UserId    int64 `db:"user_id"`
	AuthorId  int64 `db:"author_id"`
	PostId    int64 `db:"post_id"`
	CreatedAt int64 `db:"created_at"`
}

type FeedInboxModel interface {
	BatchInsertIgnore(ctx context.Context, rows []*FeedInbox) (int64, error)
}

type feedInboxModel struct {
	conn  sqlx.SqlConn
	table string
}

func NewFeedInboxModel(conn sqlx.SqlConn) FeedInboxModel {
	return &feedInboxModel{conn: conn, table: "feed_inbox"}
}

func (m *feedInboxModel) BatchInsertIgnore(ctx context.Context, rows []*FeedInbox) (int64, error) {
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
```

- [ ] **Step 4: Commit**

```bash
git -C /home/bt/projects/backend/little-white-box-content-community add app/feed/mq/internal/model/vars.go app/feed/mq/internal/model/feed_outbox_model.go app/feed/mq/internal/model/feed_inbox_model.go
git -C /home/bt/projects/backend/little-white-box-content-community commit -m "feat(feed-mq): add minimal FeedOutbox and FeedInbox models"
```

---

### Task 2: Feed MQ — Config 和 ServiceContext

**Files:**
- Create: `app/feed/mq/internal/config/config.go`
- Create: `app/feed/mq/internal/svc/service_context.go`

- [ ] **Step 1: 创建 config.go**

`app/feed/mq/internal/config/config.go`:

```go
package config

import (
	"mqx"
)

type Config struct {
	DataSource      string
	MQ              mqx.ConsumerConfig
	UserRpc         string `json:"UserRpc"` // etcd discovery key, e.g. "user.rpc"
	BigVThreshold   int64
	FanoutBatchSize int64
}
```

说明：`UserRpc` 是简单 string（etcd key），我们在 ServiceContext 里手动构建 zrpc client，不依赖 `zrpc.RpcClientConf` 的自动解析。这会简化 yaml 结构且避免引入不需要的 RpcServerConf 嵌入。

- [ ] **Step 2: 创建 service_context.go**

`app/feed/mq/internal/svc/service_context.go`:

```go
package svc

import (
	"context"
	"esx/app/feed/mq/internal/config"
	"esx/app/feed/mq/internal/model"
	"fmt"
	"user/userservice"

	"github.com/zeromicro/go-zero/core/stores/sqlx"
	"github.com/zeromicro/go-zero/zrpc"
	"google.golang.org/grpc"
)

type UserService interface {
	GetUser(ctx context.Context, in *userservice.GetUserReq, opts ...grpc.CallOption) (*userservice.GetUserResp, error)
	GetFollowers(ctx context.Context, in *userservice.GetFollowersReq, opts ...grpc.CallOption) (*userservice.GetFollowersResp, error)
}

type ServiceContext struct {
	Config          config.Config
	Conn            sqlx.SqlConn
	OutboxModel     model.FeedOutboxModel
	InboxModel      model.FeedInboxModel
	UserService     UserService
	BigVThreshold   int64
	FanoutBatchSize int64
}

func NewServiceContext(c config.Config) *ServiceContext {
	conn := sqlx.NewMysql(c.DataSource)
	userRpcClient := zrpc.MustNewClient(zrpc.RpcClientConf{
		Etcd: zrpc.EtcdConf{
			Hosts: []string{"127.0.0.1:2379"},
			Key:   c.UserRpc,
		},
	})
	return &ServiceContext{
		Config:          c,
		Conn:            conn,
		OutboxModel:     model.NewFeedOutboxModel(conn),
		InboxModel:      model.NewFeedInboxModel(conn),
		UserService:     userservice.NewUserServiceClient(userRpcClient.Conn()),
		BigVThreshold:   c.BigVThreshold,
		FanoutBatchSize: c.FanoutBatchSize,
	}
}

func MustServiceContext(c config.Config) *ServiceContext {
	ctx := NewServiceContext(c)
	if ctx.Conn == nil {
		panic(fmt.Sprintf("feed-consumer: mysql connection failed"))
	}
	return ctx
}
```

- [ ] **Step 3: Commit**

```bash
git -C /home/bt/projects/backend/little-white-box-content-community add app/feed/mq/internal/config/config.go app/feed/mq/internal/svc/service_context.go
git -C /home/bt/projects/backend/little-white-box-content-community commit -m "feat(feed-mq): add config and ServiceContext with User RPC client"
```

---

### Task 3: Feed MQ — Fanout Logic (TDD)

**Files:**
- Create: `app/feed/mq/internal/logic/fanout_test.go`
- Create: `app/feed/mq/internal/logic/fanout.go`

- [ ] **Step 1: 写失败测试 — fanout_test.go**

```go
package logic

import (
	"context"
	"errors"
	"testing"

	"esx/app/feed/mq/internal/model"
	"user/userservice"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

// --- mocks ---

type mockOutboxModel struct{ mock.Mock }

func (m *mockOutboxModel) InsertIgnore(ctx context.Context, row *model.FeedOutbox) error {
	return m.Called(ctx, row).Error(0)
}

type mockInboxModel struct{ mock.Mock }

func (m *mockInboxModel) BatchInsertIgnore(ctx context.Context, rows []*model.FeedInbox) (int64, error) {
	args := m.Called(ctx, rows)
	return args.Get(0).(int64), args.Error(1)
}

type mockUserService struct{ mock.Mock }

func (m *mockUserService) GetUser(ctx context.Context, in *userservice.GetUserReq, opts ...grpc.CallOption) (*userservice.GetUserResp, error) {
	args := m.Called(ctx, in)
	if v := args.Get(0); v != nil {
		return v.(*userservice.GetUserResp), args.Error(1)
	}
	return nil, args.Error(1)
}

func (m *mockUserService) GetFollowers(ctx context.Context, in *userservice.GetFollowersReq, opts ...grpc.CallOption) (*userservice.GetFollowersResp, error) {
	args := m.Called(ctx, in)
	if v := args.Get(0); v != nil {
		return v.(*userservice.GetFollowersResp), args.Error(1)
	}
	return nil, args.Error(1)
}

// --- tests ---

func TestFanout_SmallV_WritesInboxAndOutbox(t *testing.T) {
	outbox := new(mockOutboxModel)
	inbox := new(mockInboxModel)
	userSvc := new(mockUserService)
	event := PostPublished{PostId: 1001, AuthorId: 9, CreatedAt: 1710000000000}

	userSvc.On("GetUser", mock.Anything, &userservice.GetUserReq{UserId: 9}).
		Return(&userservice.GetUserResp{User: &userservice.UserInfo{Id: 9, FollowerCount: 99}}, nil).Once()
	outbox.On("InsertIgnore", mock.Anything, &model.FeedOutbox{AuthorId: 9, PostId: 1001, CreatedAt: 1710000000000}).
		Return(nil).Once()
	userSvc.On("GetFollowers", mock.Anything, &userservice.GetFollowersReq{UserId: 9, Page: 1, PageSize: 500}).
		Return(&userservice.GetFollowersResp{Users: []*userservice.UserInfo{{Id: 1}, {Id: 2}, {Id: 3}}}, nil).Once()
	inbox.On("BatchInsertIgnore", mock.Anything, mock.MatchedBy(func(rows []*model.FeedInbox) bool {
		return len(rows) == 3 && rows[0].UserId == 1 && rows[1].UserId == 2 && rows[2].UserId == 3
	})).Return(int64(3), nil).Once()

	pushed, err := HandlePostPublished(context.Background(), outbox, inbox, userSvc, 10000, 500, event)

	require.NoError(t, err)
	require.Equal(t, int64(3), pushed)
}

func TestFanout_BigV_OutboxOnly(t *testing.T) {
	outbox := new(mockOutboxModel)
	inbox := new(mockInboxModel)
	userSvc := new(mockUserService)
	event := PostPublished{PostId: 1001, AuthorId: 9, CreatedAt: 1710000000000}

	userSvc.On("GetUser", mock.Anything, &userservice.GetUserReq{UserId: 9}).
		Return(&userservice.GetUserResp{User: &userservice.UserInfo{Id: 9, FollowerCount: 10000}}, nil).Once()
	outbox.On("InsertIgnore", mock.Anything, &model.FeedOutbox{AuthorId: 9, PostId: 1001, CreatedAt: 1710000000000}).
		Return(nil).Once()

	pushed, err := HandlePostPublished(context.Background(), outbox, inbox, userSvc, 10000, 500, event)

	require.NoError(t, err)
	require.Zero(t, pushed)
	inbox.AssertNotCalled(t, "BatchInsertIgnore", mock.Anything, mock.Anything)
}

func TestFanout_UserRPCFailure_ReturnsError(t *testing.T) {
	outbox := new(mockOutboxModel)
	inbox := new(mockInboxModel)
	userSvc := new(mockUserService)
	event := PostPublished{PostId: 1001, AuthorId: 9, CreatedAt: 1710000000000}

	userSvc.On("GetUser", mock.Anything, mock.Anything).
		Return(nil, errors.New("rpc unavailable")).Once()

	_, err := HandlePostPublished(context.Background(), outbox, inbox, userSvc, 10000, 500, event)

	require.Error(t, err)
}

func TestFanout_PaginatesFollowers(t *testing.T) {
	outbox := new(mockOutboxModel)
	inbox := new(mockInboxModel)
	userSvc := new(mockUserService)
	event := PostPublished{PostId: 1001, AuthorId: 9, CreatedAt: 1710000000000}

	userSvc.On("GetUser", mock.Anything, &userservice.GetUserReq{UserId: 9}).
		Return(&userservice.GetUserResp{User: &userservice.UserInfo{Id: 9, FollowerCount: 3}}, nil).Once()
	outbox.On("InsertIgnore", mock.Anything, mock.Anything).Return(nil).Once()
	userSvc.On("GetFollowers", mock.Anything, &userservice.GetFollowersReq{UserId: 9, Page: 1, PageSize: 2}).
		Return(&userservice.GetFollowersResp{Users: []*userservice.UserInfo{{Id: 1}, {Id: 2}}, Total: 3}, nil).Once()
	userSvc.On("GetFollowers", mock.Anything, &userservice.GetFollowersReq{UserId: 9, Page: 2, PageSize: 2}).
		Return(&userservice.GetFollowersResp{Users: []*userservice.UserInfo{{Id: 3}}, Total: 3}, nil).Once()
	inbox.On("BatchInsertIgnore", mock.Anything, mock.MatchedBy(func(rows []*model.FeedInbox) bool {
		return len(rows) == 3
	})).Return(int64(3), nil).Once()

	pushed, err := HandlePostPublished(context.Background(), outbox, inbox, userSvc, 10000, 2, event)

	require.NoError(t, err)
	require.Equal(t, int64(3), pushed)
}

func TestFanout_OutboxInsertFailure_ReturnsError(t *testing.T) {
	outbox := new(mockOutboxModel)
	inbox := new(mockInboxModel)
	userSvc := new(mockUserService)
	event := PostPublished{PostId: 1001, AuthorId: 9, CreatedAt: 1710000000000}

	userSvc.On("GetUser", mock.Anything, mock.Anything).
		Return(&userservice.GetUserResp{User: &userservice.UserInfo{Id: 9, FollowerCount: 99}}, nil).Once()
	outbox.On("InsertIgnore", mock.Anything, mock.Anything).
		Return(errors.New("db offline")).Once()

	_, err := HandlePostPublished(context.Background(), outbox, inbox, userSvc, 10000, 500, event)

	require.Error(t, err)
}
```

- [ ] **Step 2: 运行测试确认失败**

```bash
cd /home/bt/projects/backend/little-white-box-content-community && GOCACHE=/tmp/go-build-little-white-box go test ./app/feed/mq/internal/logic/... -v -count=1
```

Expected: 编译失败 — `HandlePostPublished`、`PostPublished` 未定义。

- [ ] **Step 3: 实现 fanout.go**

`app/feed/mq/internal/logic/fanout.go`:

```go
package logic

import (
	"context"

	"esx/app/feed/mq/internal/model"
	"user/userservice"
)

type PostPublished struct {
	PostId    int64
	AuthorId  int64
	CreatedAt int64
}

type OutboxInserter interface {
	InsertIgnore(ctx context.Context, row *model.FeedOutbox) error
}

type InboxBatchInserter interface {
	BatchInsertIgnore(ctx context.Context, rows []*model.FeedInbox) (int64, error)
}

type UserGetter interface {
	GetUser(ctx context.Context, in *userservice.GetUserReq, opts ...interface{}) (*userservice.GetUserResp, error)
	GetFollowers(ctx context.Context, in *userservice.GetFollowersReq, opts ...interface{}) (*userservice.GetFollowersResp, error)
}

func HandlePostPublished(
	ctx context.Context,
	outbox OutboxInserter,
	inbox InboxBatchInserter,
	userSvc UserGetter,
	bigVThreshold int64,
	fanoutBatchSize int64,
	event PostPublished,
) (int64, error) {
	userResp, err := userSvc.GetUser(ctx, &userservice.GetUserReq{UserId: event.AuthorId})
	if err != nil {
		return 0, err
	}
	if err := outbox.InsertIgnore(ctx, &model.FeedOutbox{
		AuthorId: event.AuthorId, PostId: event.PostId, CreatedAt: event.CreatedAt,
	}); err != nil {
		return 0, err
	}
	if userResp.User == nil || userResp.User.FollowerCount >= bigVThreshold {
		return 0, nil
	}
	pageSize := int32(fanoutBatchSize)
	if pageSize <= 0 {
		pageSize = 500
	}
	rows := make([]*model.FeedInbox, 0)
	var fetched int64
	for page := int32(1); ; page++ {
		followersResp, err := userSvc.GetFollowers(ctx, &userservice.GetFollowersReq{
			UserId: event.AuthorId, Page: page, PageSize: pageSize,
		})
		if err != nil {
			return 0, err
		}
		for _, user := range followersResp.Users {
			if user.Id > 0 {
				rows = append(rows, &model.FeedInbox{
					UserId: user.Id, AuthorId: event.AuthorId,
					PostId: event.PostId, CreatedAt: event.CreatedAt,
				})
			}
		}
		fetched += int64(len(followersResp.Users))
		if len(followersResp.Users) == 0 || int32(len(followersResp.Users)) < pageSize || fetched >= followersResp.Total {
			break
		}
	}
	return inbox.BatchInsertIgnore(ctx, rows)
}
```

- [ ] **Step 4: 运行测试确认通过**

```bash
cd /home/bt/projects/backend/little-white-box-content-community && GOCACHE=/tmp/go-build-little-white-box go test ./app/feed/mq/internal/logic/... -v -count=1
```

Expected: 5 个测试全部 PASS。

- [ ] **Step 5: Commit**

```bash
git -C /home/bt/projects/backend/little-white-box-content-community add app/feed/mq/internal/logic/fanout.go app/feed/mq/internal/logic/fanout_test.go
git -C /home/bt/projects/backend/little-white-box-content-community commit -m "feat(feed-mq): add fanout logic with TDD tests"
```

---

### Task 4: Feed MQ — Consumer 接线、main.go、yaml (TDD)

**Files:**
- Create: `app/feed/mq/internal/mqs/post_publish_consumer_test.go`
- Create: `app/feed/mq/internal/mqs/post_publish_consumer.go`
- Create: `app/feed/mq/main.go`
- Create: `app/feed/mq/etc/feed-consumer.yaml`

- [ ] **Step 1: 写 consumer 测试**

`app/feed/mq/internal/mqs/post_publish_consumer_test.go`:

```go
package mqs

import (
	"context"
	"errors"
	"testing"

	"esx/app/feed/mq/internal/logic"
	"esx/app/feed/mq/internal/model"
	"esx/app/feed/mq/internal/svc"
	"user/userservice"

	"github.com/apache/rocketmq-client-go/v2/consumer"
	"github.com/apache/rocketmq-client-go/v2/primitive"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

// --- fakes ---

type fakeOutboxModel struct{ inserted []*model.FeedOutbox }

func (m *fakeOutboxModel) InsertIgnore(ctx context.Context, row *model.FeedOutbox) error {
	m.inserted = append(m.inserted, row)
	return nil
}

type fakeInboxModel struct{ inserted []*model.FeedInbox }

func (m *fakeInboxModel) BatchInsertIgnore(ctx context.Context, rows []*model.FeedInbox) (int64, error) {
	m.inserted = append(m.inserted, rows...)
	return int64(len(rows)), nil
}

type fakeUserService struct{ followers []*userservice.UserInfo }

func (s *fakeUserService) GetUser(ctx context.Context, in *userservice.GetUserReq, opts ...grpc.CallOption) (*userservice.GetUserResp, error) {
	return &userservice.GetUserResp{User: &userservice.UserInfo{Id: in.UserId, FollowerCount: int64(len(s.followers))}}, nil
}
func (s *fakeUserService) GetFollowers(ctx context.Context, in *userservice.GetFollowersReq, opts ...grpc.CallOption) (*userservice.GetFollowersResp, error) {
	return &userservice.GetFollowersResp{Users: s.followers, Total: int64(len(s.followers))}, nil
}

// We use the real HandlePostPublished from logic, wiring fakes together

func TestPostPublishConsumer_MalformedJSON_Skips(t *testing.T) {
	outbox := &fakeOutboxModel{}
	inbox := &fakeInboxModel{}
	userSvc := &fakeUserService{followers: []*userservice.UserInfo{}}
	svcCtx := &svc.ServiceContext{
		OutboxModel: outbox, InboxModel: inbox, UserService: userSvc,
		BigVThreshold: 10000, FanoutBatchSize: 500,
	}

	result, err := consumeMessageBatch(context.Background(), svcCtx,
		&primitive.MessageExt{Message: primitive.Message{Body: []byte(`not-json`)}, MsgId: "msg-1"},
	)

	require.NoError(t, err)
	assert.Equal(t, consumer.ConsumeSuccess, result)
	assert.Empty(t, outbox.inserted)
}

func TestPostPublishConsumer_MissingFields_Skips(t *testing.T) {
	outbox := &fakeOutboxModel{}
	inbox := &fakeInboxModel{}
	userSvc := &fakeUserService{}
	svcCtx := &svc.ServiceContext{
		OutboxModel: outbox, InboxModel: inbox, UserService: userSvc,
		BigVThreshold: 10000, FanoutBatchSize: 500,
	}

	result, err := consumeMessageBatch(context.Background(), svcCtx,
		&primitive.MessageExt{Message: primitive.Message{Body: []byte(`{"post_id":0}`)}, MsgId: "msg-2"},
	)

	require.NoError(t, err)
	assert.Equal(t, consumer.ConsumeSuccess, result)
}

func TestPostPublishConsumer_UserRPCFailure_Retry(t *testing.T) {
	outbox := &fakeOutboxModel{}
	inbox := &fakeInboxModel{}
	userSvc := new(mockUserService)
	userSvc.On("GetUser", mock.Anything, mock.Anything).
		Return(nil, errors.New("rpc down")).Once()
	svcCtx := &svc.ServiceContext{
		OutboxModel: outbox, InboxModel: inbox, UserService: userSvc,
		BigVThreshold: 10000, FanoutBatchSize: 500,
	}

	result, err := consumeMessageBatch(context.Background(), svcCtx,
		&primitive.MessageExt{Message: primitive.Message{Body: []byte(`{"post_id":1,"author_id":9,"created_at":1710000000000}`)}, MsgId: "msg-3"},
	)

	require.NoError(t, err)
	assert.Equal(t, consumer.ConsumeRetryLater, result)
}

func TestPostPublishConsumer_ValidMessage_Success(t *testing.T) {
	outbox := &fakeOutboxModel{}
	inbox := &fakeInboxModel{}
	userSvc := &fakeUserService{followers: []*userservice.UserInfo{{Id: 1}, {Id: 2}}}
	svcCtx := &svc.ServiceContext{
		OutboxModel: outbox, InboxModel: inbox, UserService: userSvc,
		BigVThreshold: 10000, FanoutBatchSize: 500,
	}

	result, err := consumeMessageBatch(context.Background(), svcCtx,
		&primitive.MessageExt{Message: primitive.Message{Body: []byte(`{"post_id":1,"author_id":9,"created_at":1710000000000}`)}, MsgId: "msg-4"},
	)

	require.NoError(t, err)
	assert.Equal(t, consumer.ConsumeSuccess, result)
	assert.Len(t, outbox.inserted, 1)
	assert.Len(t, inbox.inserted, 2)
}
```

- [ ] **Step 2: 运行测试确认失败**（`consumeMessageBatch` 未定义）

```bash
cd /home/bt/projects/backend/little-white-box-content-community && GOCACHE=/tmp/go-build-little-white-box go test ./app/feed/mq/internal/mqs/... -v -count=1
```

- [ ] **Step 3: 实现 consumer 接线**

`app/feed/mq/internal/mqs/post_publish_consumer.go`:

```go
package mqs

import (
	"context"
	"encoding/json"
	"fmt"

	"esx/app/feed/mq/internal/logic"
	"esx/app/feed/mq/internal/svc"
	"mqx"

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
	if err != nil {
		return nil, fmt.Errorf("feed-consumer: create consumer: %w", err)
	}
	handler := func(ctx context.Context, msgs ...*primitive.MessageExt) (consumer.ConsumeResult, error) {
		return consumeMessageBatch(ctx, svcCtx, msgs...), nil
	}
	if err := c.SubscribeWithTopic(mqx.TopicPostCreate, mqx.TagDefault, handler); err != nil {
		return nil, fmt.Errorf("feed-consumer: subscribe %s: %w", mqx.TopicPostCreate, err)
	}
	return c, nil
}

func consumeMessageBatch(ctx context.Context, svcCtx *svc.ServiceContext, msgs ...*primitive.MessageExt) consumer.ConsumeResult {
	for _, msg := range msgs {
		var event postPublishedMessage
		if err := json.Unmarshal(msg.Body, &event); err != nil {
			logx.WithContext(ctx).Errorw("feed-consumer: unmarshal failed",
				logx.Field("msg_id", msg.MsgId), logx.Field("err", err.Error()))
			continue
		}
		if event.PostId <= 0 || event.AuthorId <= 0 || event.CreatedAt <= 0 {
			logx.WithContext(ctx).Errorw("feed-consumer: missing required fields",
				logx.Field("msg_id", msg.MsgId), logx.Field("post_id", event.PostId),
				logx.Field("author_id", event.AuthorId), logx.Field("created_at", event.CreatedAt))
			continue
		}
		_, err := logic.HandlePostPublished(ctx,
			svcCtx.OutboxModel, svcCtx.InboxModel, svcCtx.UserService,
			svcCtx.BigVThreshold, svcCtx.FanoutBatchSize,
			logic.PostPublished{
				PostId: event.PostId, AuthorId: event.AuthorId, CreatedAt: event.CreatedAt,
			})
		if err != nil {
			logx.WithContext(ctx).Errorw("feed-consumer: fanout failed",
				logx.Field("msg_id", msg.MsgId), logx.Field("post_id", event.PostId),
				logx.Field("err", err.Error()))
			return consumer.ConsumeRetryLater
		}
	}
	return consumer.ConsumeSuccess
}
```

注意：`svc.UserService` 接口使用 `grpc.CallOption` 变参，而 `logic.UserGetter` 接口使用 `...interface{}`。在 consumer 层调用时统一转换为 `logic` 兼容的签名。因为 `logic.HandlePostPublished` 直接接受拆分后的接口参数（`OutboxInserter`, `InboxBatchInserter`, `UserGetter`），consumer 层传入的 mock/fake 只要满足接口即可。

但 `logic.UserGetter` 的 `GetUser(ctx, *GetUserReq, ...interface{})` 签名无法被 `*userservice.UserServiceClient` 直接满足（它的签名是 `...grpc.CallOption`）。所以 logic 层的 `HandlePostPublished` 应该直接接受具体类型——我们在 fanout_test.go 的 mock 中已经演示了 `...grpc.CallOption`。修改 `logic.UserGetter` 接口使用 `...grpc.CallOption`：

替换 `logic/fanout.go` 中的 `UserGetter`:

```go
type UserGetter interface {
	GetUser(ctx context.Context, in *userservice.GetUserReq, opts ...grpc.CallOption) (*userservice.GetUserResp, error)
	GetFollowers(ctx context.Context, in *userservice.GetFollowersReq, opts ...grpc.CallOption) (*userservice.GetFollowersResp, error)
}
```

并在文件头添加 `import "google.golang.org/grpc"` 到 logic/fanout.go。

- [ ] **Step 4: 实现 main.go**

`app/feed/mq/main.go`:

```go
package main

import (
	"context"
	"flag"
	"fmt"

	"cleanupx"
	"esx/app/feed/mq/internal/config"
	"esx/app/feed/mq/internal/mqs"
	"esx/app/feed/mq/internal/svc"

	"github.com/zeromicro/go-zero/core/conf"
	"github.com/zeromicro/go-zero/core/logx"
)

var configFile = flag.String("f", "etc/feed-consumer.yaml", "config file")

func main() {
	flag.Parse()
	var c config.Config
	conf.MustLoad(*configFile, &c, conf.UseEnv())

	svcCtx := svc.NewServiceContext(c)

	postConsumer, err := mqs.NewPostPublishConsumer(svcCtx)
	if err != nil {
		logx.Must(err)
	}
	if err := postConsumer.Start(); err != nil {
		logx.Must(err)
	}
	defer cleanupx.Shutdown(logx.WithContext(context.Background()), "feed post-publish consumer", postConsumer.Shutdown)

	fmt.Println("Feed MQ consumer started, subscribing post-create...")
	select {}
}
```

- [ ] **Step 5: 创建 yaml 配置**

`app/feed/mq/etc/feed-consumer.yaml`:

```yaml
DataSource: "${DB_FEED}"
UserRpc: "user.rpc"
MQ:
  NameServer: "${MQ_NAMESERVER}"
  GroupName: "feed-service-group"
  Topic: "post-create"
  Tag: "default"
  ConsumeOrder: false
BigVThreshold: 10000
FanoutBatchSize: 500
```

- [ ] **Step 6: 运行测试确认通过**

```bash
cd /home/bt/projects/backend/little-white-box-content-community && GOCACHE=/tmp/go-build-little-white-box go test ./app/feed/mq/... -v -count=1
```

Expected: 所有测试 PASS（9 个：5 logic + 4 mqs）。

- [ ] **Step 7: Commit**

```bash
git -C /home/bt/projects/backend/little-white-box-content-community add app/feed/mq/internal/mqs/ app/feed/mq/main.go app/feed/mq/etc/
git -C /home/bt/projects/backend/little-white-box-content-community commit -m "feat(feed-mq): add consumer wiring, main entry, and yaml config"
```

---

### Task 5: Message MQ — Model 层

**Files:**
- Create: `app/message/mq/internal/model/vars.go`
- Create: `app/message/mq/internal/model/notification_model.go`

- [ ] **Step 1: 创建 vars.go**

```go
package model

import "github.com/zeromicro/go-zero/core/stores/sqlx"

var ErrNotFound = sqlx.ErrNotFound
```

- [ ] **Step 2: 创建 Notification model（最小，仅 Insert）**

`app/message/mq/internal/model/notification_model.go`:

```go
package model

import (
	"context"
	"database/sql"

	"github.com/zeromicro/go-zero/core/stores/sqlx"
)

type Notification struct {
	Id         int64          `db:"id"`
	UserId     int64          `db:"user_id"`
	Type       int64          `db:"type"`
	Title      sql.NullString `db:"title"`
	Content    sql.NullString `db:"content"`
	TargetId   sql.NullInt64  `db:"target_id"`
	TargetType sql.NullInt64  `db:"target_type"`
	SenderId   sql.NullInt64  `db:"sender_id"`
	Status     int64          `db:"status"`
}

type NotificationModel interface {
	Insert(ctx context.Context, data *Notification) (sql.Result, error)
}

type notificationModel struct {
	conn  sqlx.SqlConn
	table string
}

func NewNotificationModel(conn sqlx.SqlConn) NotificationModel {
	return &notificationModel{conn: conn, table: "notification"}
}

func (m *notificationModel) Insert(ctx context.Context, data *Notification) (sql.Result, error) {
	query := `INSERT INTO notification (user_id, type, title, content, target_id, target_type, sender_id, status) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`
	return m.conn.ExecCtx(ctx, query, data.UserId, data.Type, data.Title, data.Content, data.TargetId, data.TargetType, data.SenderId, data.Status)
}
```

- [ ] **Step 3: Commit**

```bash
git -C /home/bt/projects/backend/little-white-box-content-community add app/message/mq/internal/model/
git -C /home/bt/projects/backend/little-white-box-content-community commit -m "feat(message-mq): add minimal Notification model"
```

---

### Task 6: Message MQ — Config、ServiceContext、RedisStore

**Files:**
- Create: `app/message/mq/internal/config/config.go`
- Create: `app/message/mq/internal/svc/service_context.go`
- Create: `app/message/mq/internal/svc/redis_store.go`

- [ ] **Step 1: 创建 config.go**

`app/message/mq/internal/config/config.go`:

```go
package config

import "mqx"

type Config struct {
	DataSource string
	Redis      struct {
		Host string
		Pass string
	}
	MQ mqx.ConsumerConfig
}
```

- [ ] **Step 2: 创建 redis_store.go**

`app/message/mq/internal/svc/redis_store.go`:

```go
package svc

import (
	"context"
	"fmt"

	"github.com/zeromicro/go-zero/core/stores/redis"
)

type UnreadStore interface {
	DeleteUserUnread(ctx context.Context, userID int64) error
}

type redisUnreadStore struct {
	redis *redis.Redis
}

func NewRedisUnreadStore(r *redis.Redis) UnreadStore {
	return &redisUnreadStore{redis: r}
}

func (s *redisUnreadStore) DeleteUserUnread(ctx context.Context, userID int64) error {
	key := fmt.Sprintf("cache:unread:notification:%d", userID)
	_, err := s.redis.DelCtx(ctx, key)
	return err
}
```

- [ ] **Step 3: 创建 service_context.go**

`app/message/mq/internal/svc/service_context.go`:

```go
package svc

import (
	"esx/app/message/mq/internal/config"
	"esx/app/message/mq/internal/model"
	"fmt"

	"github.com/zeromicro/go-zero/core/stores/redis"
	"github.com/zeromicro/go-zero/core/stores/sqlx"
)

type ServiceContext struct {
	Config            config.Config
	Conn              sqlx.SqlConn
	NotificationModel model.NotificationModel
	UnreadStore       UnreadStore
}

func NewServiceContext(c config.Config) *ServiceContext {
	conn := sqlx.NewMysql(c.DataSource)
	var store UnreadStore
	if c.Redis.Host != "" {
		r := redis.MustNewRedis(redis.RedisConf{
			Host: c.Redis.Host,
			Pass: c.Redis.Pass,
			Type: "node",
		})
		store = NewRedisUnreadStore(r)
	}
	return &ServiceContext{
		Config:            c,
		Conn:              conn,
		NotificationModel: model.NewNotificationModel(conn),
		UnreadStore:       store,
	}
}

func MustServiceContext(c config.Config) *ServiceContext {
	ctx := NewServiceContext(c)
	if ctx.Conn == nil {
		panic(fmt.Sprintf("message-consumer: mysql connection failed"))
	}
	return ctx
}
```

- [ ] **Step 4: Commit**

```bash
git -C /home/bt/projects/backend/little-white-box-content-community add app/message/mq/internal/config/ app/message/mq/internal/svc/
git -C /home/bt/projects/backend/little-white-box-content-community commit -m "feat(message-mq): add config, ServiceContext, and Redis unread store"
```

---

### Task 7: Message MQ — Consumer Logic、接线、main.go、yaml (TDD)

**Files:**
- Create: `app/message/mq/internal/mqs/message_consumer_test.go`
- Create: `app/message/mq/internal/mqs/message_consumer.go`
- Create: `app/message/mq/main.go`
- Create: `app/message/mq/etc/message-consumer.yaml`

- [ ] **Step 1: 写 consumer 测试**

`app/message/mq/internal/mqs/message_consumer_test.go`:

```go
package mqs

import (
	"context"
	"database/sql"
	"errors"
	"testing"

	"esx/app/message/mq/internal/model"
	"esx/app/message/mq/internal/svc"

	"github.com/apache/rocketmq-client-go/v2/consumer"
	"github.com/apache/rocketmq-client-go/v2/primitive"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- fakes ---

type fakeResult struct{ id int64 }

func (r fakeResult) LastInsertId() (int64, error) { return r.id, nil }
func (r fakeResult) RowsAffected() (int64, error) { return 1, nil }

type fakeNotificationModel struct {
	inserted  []*model.Notification
	insertErr error
}

func (m *fakeNotificationModel) Insert(ctx context.Context, n *model.Notification) (sql.Result, error) {
	if m.insertErr != nil {
		return nil, m.insertErr
	}
	m.inserted = append(m.inserted, n)
	return fakeResult{id: 1}, nil
}

type fakeUnreadStore struct{ deleted []int64 }

func (s *fakeUnreadStore) DeleteUserUnread(ctx context.Context, userID int64) error {
	s.deleted = append(s.deleted, userID)
	return nil
}

// --- tests ---

func TestMessageConsumer_MalformedJSON_ReturnsSuccess(t *testing.T) {
	notifications := &fakeNotificationModel{}
	store := &fakeUnreadStore{}
	svcCtx := &svc.ServiceContext{NotificationModel: notifications, UnreadStore: store}

	result := consumeNotificationBatch(context.Background(), svcCtx,
		&primitive.MessageExt{Message: primitive.Message{Body: []byte(`not-json`)}, MsgId: "bad-1"},
	)

	assert.Equal(t, consumer.ConsumeSuccess, result)
	assert.Empty(t, notifications.inserted)
}

func TestMessageConsumer_MissingTargetUserID_ReturnsSuccess(t *testing.T) {
	svcCtx := &svc.ServiceContext{NotificationModel: &fakeNotificationModel{}, UnreadStore: &fakeUnreadStore{}}

	result := consumeNotificationBatch(context.Background(), svcCtx,
		&primitive.MessageExt{Message: primitive.Message{Body: []byte(`{"action_type":1}`)}, MsgId: "msg-1"},
	)

	assert.Equal(t, consumer.ConsumeSuccess, result)
}

func TestMessageConsumer_MissingActionType_ReturnsSuccess(t *testing.T) {
	svcCtx := &svc.ServiceContext{NotificationModel: &fakeNotificationModel{}, UnreadStore: &fakeUnreadStore{}}

	result := consumeNotificationBatch(context.Background(), svcCtx,
		&primitive.MessageExt{Message: primitive.Message{Body: []byte(`{"target_user_id":9}`)}, MsgId: "msg-2"},
	)

	assert.Equal(t, consumer.ConsumeSuccess, result)
}

func TestMessageConsumer_UnsupportedActionType_ReturnsSuccess(t *testing.T) {
	notifications := &fakeNotificationModel{}
	store := &fakeUnreadStore{}
	svcCtx := &svc.ServiceContext{NotificationModel: notifications, UnreadStore: store}

	result := consumeNotificationBatch(context.Background(), svcCtx,
		&primitive.MessageExt{Message: primitive.Message{Body: []byte(`{"target_user_id":9,"action_type":99}`)}, MsgId: "msg-3"},
	)

	assert.Equal(t, consumer.ConsumeSuccess, result)
	assert.Empty(t, notifications.inserted)
}

func TestMessageConsumer_LikeNotification_Success(t *testing.T) {
	notifications := &fakeNotificationModel{}
	store := &fakeUnreadStore{}
	svcCtx := &svc.ServiceContext{NotificationModel: notifications, UnreadStore: store}

	result := consumeNotificationBatch(context.Background(), svcCtx,
		&primitive.MessageExt{Message: primitive.Message{Body: []byte(`{"target_user_id":9,"action_type":1,"user_id":7,"username":"小白","target_id":99}`)}, MsgId: "msg-4"},
	)

	assert.Equal(t, consumer.ConsumeSuccess, result)
	require.Len(t, notifications.inserted, 1)
	assert.Equal(t, int64(9), notifications.inserted[0].UserId)
	assert.Equal(t, int64(1), notifications.inserted[0].Type)
	assert.Equal(t, "点赞", notifications.inserted[0].Title.String)
	assert.Equal(t, "小白 赞了你的帖子", notifications.inserted[0].Content.String)
	assert.Equal(t, []int64{9}, store.deleted)
}

func TestMessageConsumer_CommentNotification_Success(t *testing.T) {
	notifications := &fakeNotificationModel{}
	svcCtx := &svc.ServiceContext{NotificationModel: notifications, UnreadStore: &fakeUnreadStore{}}

	result := consumeNotificationBatch(context.Background(), svcCtx,
		&primitive.MessageExt{Message: primitive.Message{Body: []byte(`{"target_user_id":9,"action_type":2,"user_id":7,"username":"小黑","target_id":88}`)}, MsgId: "msg-5"},
	)

	assert.Equal(t, consumer.ConsumeSuccess, result)
	require.Len(t, notifications.inserted, 1)
	assert.Equal(t, "评论", notifications.inserted[0].Title.String)
	assert.Equal(t, "小黑 评论了你的帖子", notifications.inserted[0].Content.String)
}

func TestMessageConsumer_FollowNotification_Success(t *testing.T) {
	notifications := &fakeNotificationModel{}
	svcCtx := &svc.ServiceContext{NotificationModel: notifications, UnreadStore: &fakeUnreadStore{}}

	result := consumeNotificationBatch(context.Background(), svcCtx,
		&primitive.MessageExt{Message: primitive.Message{Body: []byte(`{"target_user_id":9,"action_type":3,"user_id":7,"username":"小蓝"}`)}, MsgId: "msg-6"},
	)

	assert.Equal(t, consumer.ConsumeSuccess, result)
	require.Len(t, notifications.inserted, 1)
	assert.Equal(t, "关注", notifications.inserted[0].Title.String)
	assert.Equal(t, "小蓝 关注了你", notifications.inserted[0].Content.String)
}

func TestMessageConsumer_SystemNotification_Success(t *testing.T) {
	notifications := &fakeNotificationModel{}
	svcCtx := &svc.ServiceContext{NotificationModel: notifications, UnreadStore: &fakeUnreadStore{}}

	result := consumeNotificationBatch(context.Background(), svcCtx,
		&primitive.MessageExt{Message: primitive.Message{Body: []byte(`{"target_user_id":9,"action_type":4,"content":"系统维护通知"}`)}, MsgId: "msg-7"},
	)

	assert.Equal(t, consumer.ConsumeSuccess, result)
	require.Len(t, notifications.inserted, 1)
	assert.Equal(t, "系统通知", notifications.inserted[0].Title.String)
	assert.Equal(t, "系统维护通知", notifications.inserted[0].Content.String)
}

func TestMessageConsumer_InsertFails_ReturnsRetry(t *testing.T) {
	notifications := &fakeNotificationModel{insertErr: errors.New("db offline")}
	svcCtx := &svc.ServiceContext{NotificationModel: notifications, UnreadStore: &fakeUnreadStore{}}

	result := consumeNotificationBatch(context.Background(), svcCtx,
		&primitive.MessageExt{Message: primitive.Message{Body: []byte(`{"target_user_id":9,"action_type":1,"user_id":7,"username":"小白","target_id":99}`)}, MsgId: "msg-8"},
	)

	assert.Equal(t, consumer.ConsumeRetryLater, result)
}

func TestMessageConsumer_BatchSkipsPermanentAndProcessesRest(t *testing.T) {
	notifications := &fakeNotificationModel{}
	store := &fakeUnreadStore{}
	svcCtx := &svc.ServiceContext{NotificationModel: notifications, UnreadStore: store}

	result := consumeNotificationBatch(context.Background(), svcCtx,
		&primitive.MessageExt{Message: primitive.Message{Body: []byte(`bad`)}, MsgId: "bad"},
		&primitive.MessageExt{Message: primitive.Message{Body: []byte(`{"target_user_id":9,"action_type":1,"user_id":7,"username":"小白","target_id":99}`)}, MsgId: "good"},
	)

	assert.Equal(t, consumer.ConsumeSuccess, result)
	assert.Len(t, notifications.inserted, 1)
}
```

- [ ] **Step 2: 运行测试确认失败**

```bash
cd /home/bt/projects/backend/little-white-box-content-community && GOCACHE=/tmp/go-build-little-white-box go test ./app/message/mq/internal/mqs/... -v -count=1
```

- [ ] **Step 3: 实现 consumer**

`app/message/mq/internal/mqs/message_consumer.go`:

```go
package mqs

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	"esx/app/message/mq/internal/model"
	"esx/app/message/mq/internal/svc"
	"mqx"

	"github.com/apache/rocketmq-client-go/v2/consumer"
	"github.com/apache/rocketmq-client-go/v2/primitive"
	"github.com/zeromicro/go-zero/core/logx"
)

const (
	NotificationTypeLike    = int64(1)
	NotificationTypeComment = int64(2)
	NotificationTypeFollow  = int64(3)
	NotificationTypeSystem  = int64(4)
)

type userActionEvent struct {
	TargetUserID int64  `json:"target_user_id"`
	ActionType   int64  `json:"action_type"`
	UserID       int64  `json:"user_id"`
	Username     string `json:"username"`
	TargetID     int64  `json:"target_id"`
	TargetType   int64  `json:"target_type"`
	Content      string `json:"content"`
}

func NewMessageConsumer(svcCtx *svc.ServiceContext) (*mqx.Consumer, error) {
	c, err := mqx.NewConsumer(svcCtx.Config.MQ)
	if err != nil {
		return nil, fmt.Errorf("message-consumer: create consumer: %w", err)
	}
	handler := func(ctx context.Context, msgs ...*primitive.MessageExt) (consumer.ConsumeResult, error) {
		return consumeNotificationBatch(ctx, svcCtx, msgs...), nil
	}
	if err := c.SubscribeWithTopic(mqx.TopicMessagePush, mqx.TagDefault, handler); err != nil {
		return nil, fmt.Errorf("message-consumer: subscribe %s: %w", mqx.TopicMessagePush, err)
	}
	return c, nil
}

func consumeNotificationBatch(ctx context.Context, svcCtx *svc.ServiceContext, msgs ...*primitive.MessageExt) consumer.ConsumeResult {
	for _, msg := range msgs {
		var event userActionEvent
		if err := json.Unmarshal(msg.Body, &event); err != nil {
			logx.WithContext(ctx).Errorw("message-consumer: unmarshal failed",
				logx.Field("msg_id", msg.MsgId), logx.Field("err", err.Error()))
			continue
		}
		if event.TargetUserID <= 0 {
			logx.WithContext(ctx).Errorw("message-consumer: missing target_user_id",
				logx.Field("msg_id", msg.MsgId))
			continue
		}
		if event.ActionType <= 0 {
			logx.WithContext(ctx).Errorw("message-consumer: missing action_type",
				logx.Field("msg_id", msg.MsgId))
			continue
		}
		title, content := renderNotificationContent(event)
		if title == "" {
			logx.WithContext(ctx).Errorw("message-consumer: unsupported action_type",
				logx.Field("msg_id", msg.MsgId), logx.Field("action_type", event.ActionType))
			continue
		}
		if strings.TrimSpace(content) == "" {
			logx.WithContext(ctx).Errorw("message-consumer: empty notification content",
				logx.Field("msg_id", msg.MsgId))
			continue
		}
		_, err := svcCtx.NotificationModel.Insert(ctx, &model.Notification{
			UserId:     event.TargetUserID,
			Type:       event.ActionType,
			Title:      sql.NullString{String: title, Valid: title != ""},
			Content:    sql.NullString{String: content, Valid: content != ""},
			TargetId:   sql.NullInt64{Int64: event.TargetID, Valid: event.TargetID > 0},
			TargetType: sql.NullInt64{Int64: event.TargetType, Valid: event.TargetType > 0},
			SenderId:   sql.NullInt64{Int64: event.UserID, Valid: event.UserID > 0},
			Status:     0,
		})
		if err != nil {
			logx.WithContext(ctx).Errorw("message-consumer: insert notification failed",
				logx.Field("msg_id", msg.MsgId), logx.Field("err", err.Error()))
			return consumer.ConsumeRetryLater
		}
		if svcCtx.UnreadStore != nil {
			if err := svcCtx.UnreadStore.DeleteUserUnread(ctx, event.TargetUserID); err != nil {
				logx.WithContext(ctx).Errorw("message-consumer: delete unread cache failed",
					logx.Field("msg_id", msg.MsgId), logx.Field("target_user_id", event.TargetUserID),
					logx.Field("err", err.Error()))
			}
		}
	}
	return consumer.ConsumeSuccess
}

func renderNotificationContent(event userActionEvent) (string, string) {
	username := strings.TrimSpace(event.Username)
	if username == "" {
		username = "有人"
	}
	switch event.ActionType {
	case NotificationTypeLike:
		return "点赞", fmt.Sprintf("%s 赞了你的帖子", username)
	case NotificationTypeComment:
		return "评论", fmt.Sprintf("%s 评论了你的帖子", username)
	case NotificationTypeFollow:
		return "关注", fmt.Sprintf("%s 关注了你", username)
	case NotificationTypeSystem:
		return "系统通知", strings.TrimSpace(event.Content)
	default:
		return "", ""
	}
}
```

- [ ] **Step 4: 实现 main.go**

`app/message/mq/main.go`:

```go
package main

import (
	"context"
	"flag"
	"fmt"

	"cleanupx"
	"esx/app/message/mq/internal/config"
	"esx/app/message/mq/internal/mqs"
	"esx/app/message/mq/internal/svc"

	"github.com/zeromicro/go-zero/core/conf"
	"github.com/zeromicro/go-zero/core/logx"
)

var configFile = flag.String("f", "etc/message-consumer.yaml", "config file")

func main() {
	flag.Parse()
	var c config.Config
	conf.MustLoad(*configFile, &c, conf.UseEnv())

	svcCtx := svc.NewServiceContext(c)

	msgConsumer, err := mqs.NewMessageConsumer(svcCtx)
	if err != nil {
		logx.Must(err)
	}
	if err := msgConsumer.Start(); err != nil {
		logx.Must(err)
	}
	defer cleanupx.Shutdown(logx.WithContext(context.Background()), "message notification consumer", msgConsumer.Shutdown)

	fmt.Println("Message MQ consumer started, subscribing message-push...")
	select {}
}
```

- [ ] **Step 5: 创建 yaml 配置**

`app/message/mq/etc/message-consumer.yaml`:

```yaml
DataSource: "${DB_MESSAGE}"
Redis:
  Host: "127.0.0.1:6379"
  Pass: "${REDIS_PASS}"
MQ:
  NameServer: "${MQ_NAMESERVER}"
  GroupName: "message-service-group"
  Topic: "message-push"
  Tag: "default"
  ConsumeOrder: false
```

- [ ] **Step 6: 运行测试确认通过**

```bash
cd /home/bt/projects/backend/little-white-box-content-community && GOCACHE=/tmp/go-build-little-white-box go test ./app/message/mq/... -v -count=1
```

Expected: 所有测试 PASS（10 个 mqs 测试）。

- [ ] **Step 7: Commit**

```bash
git -C /home/bt/projects/backend/little-white-box-content-community add app/message/mq/internal/mqs/ app/message/mq/main.go app/message/mq/etc/
git -C /home/bt/projects/backend/little-white-box-content-community commit -m "feat(message-mq): add consumer logic, wiring, main entry, and yaml config"
```

---

### Task 8: Media MQ — Storage 层和 Config

**Files:**
- Create: `app/media/mq/internal/storage/s3.go`
- Create: `app/media/mq/internal/config/config.go`
- Create: `app/media/mq/internal/svc/service_context.go`

- [ ] **Step 1: 创建 storage/s3.go**

`app/media/mq/internal/storage/s3.go`:

```go
package storage

import (
	"context"
	"fmt"
	"strings"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type Config struct {
	Endpoint      string
	AccessKey     string
	SecretKey     string
	UseSSL        bool
	Region        string
	Bucket        string
	PublicBaseURL string
}

type ObjectStorage interface {
	Delete(ctx context.Context, objectKey string) error
	BuildPublicURL(objectKey string) string
}

type S3Client struct {
	cli           *minio.Client
	bucket        string
	publicBaseURL string
}

func NewS3Client(cfg Config) (*S3Client, error) {
	cli, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.UseSSL,
		Region: cfg.Region,
	})
	if err != nil {
		return nil, fmt.Errorf("media-mq: init s3 client: %w", err)
	}
	client := &S3Client{
		cli:           cli,
		bucket:        cfg.Bucket,
		publicBaseURL: strings.TrimRight(cfg.PublicBaseURL, "/"),
	}
	if err = client.ensureBucket(context.Background(), cfg.Region); err != nil {
		return nil, err
	}
	return client, nil
}

func (s *S3Client) ensureBucket(ctx context.Context, region string) error {
	exists, err := s.cli.BucketExists(ctx, s.bucket)
	if err != nil {
		return fmt.Errorf("media-mq: bucket exists check: %w", err)
	}
	if exists {
		return nil
	}
	return s.cli.MakeBucket(ctx, s.bucket, minio.MakeBucketOptions{Region: region})
}

func (s *S3Client) Delete(ctx context.Context, objectKey string) error {
	if err := s.cli.RemoveObject(ctx, s.bucket, objectKey, minio.RemoveObjectOptions{}); err != nil {
		return fmt.Errorf("media-mq: remove object %s: %w", objectKey, err)
	}
	return nil
}

func (s *S3Client) BuildPublicURL(objectKey string) string {
	return s.publicBaseURL + "/" + objectKey
}
```

- [ ] **Step 2: 创建 config.go**

`app/media/mq/internal/config/config.go`:

```go
package config

import (
	"esx/app/media/mq/internal/storage"
	"mqx"
)

type Config struct {
	S3Storage storage.Config
	MQ        mqx.ConsumerConfig
}
```

- [ ] **Step 3: 创建 service_context.go**

`app/media/mq/internal/svc/service_context.go`:

```go
package svc

import (
	"esx/app/media/mq/internal/config"
	"esx/app/media/mq/internal/storage"
	"fmt"
)

type ServiceContext struct {
	Config  config.Config
	Storage storage.ObjectStorage
}

func NewServiceContext(c config.Config) *ServiceContext {
	s3Client, err := storage.NewS3Client(c.S3Storage)
	if err != nil {
		panic(fmt.Sprintf("media-mq: s3 client init failed: %v", err))
	}
	return &ServiceContext{
		Config:  c,
		Storage: s3Client,
	}
}
```

- [ ] **Step 4: Commit**

```bash
git -C /home/bt/projects/backend/little-white-box-content-community add app/media/mq/internal/
git -C /home/bt/projects/backend/little-white-box-content-community commit -m "feat(media-mq): add S3 storage adapter, config, and ServiceContext"
```

---

### Task 9: Media MQ — Consumer、main.go、yaml (TDD)

**Files:**
- Create: `app/media/mq/internal/mqs/media_cleanup_consumer_test.go`
- Create: `app/media/mq/internal/mqs/media_cleanup_consumer.go`
- Create: `app/media/mq/main.go`
- Create: `app/media/mq/etc/media-consumer.yaml`

- [ ] **Step 1: 写 consumer 测试**

`app/media/mq/internal/mqs/media_cleanup_consumer_test.go`:

```go
package mqs

import (
	"context"
	"errors"
	"testing"

	"github.com/apache/rocketmq-client-go/v2/consumer"
	"github.com/apache/rocketmq-client-go/v2/primitive"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type fakeStorage struct {
	deleted []string
	err     error
}

func (s *fakeStorage) Delete(ctx context.Context, key string) error {
	if s.err != nil {
		return s.err
	}
	s.deleted = append(s.deleted, key)
	return nil
}

func (s *fakeStorage) BuildPublicURL(key string) string { return "http://fake/" + key }

func TestMediaCleanupConsumer_MalformedJSON_Skips(t *testing.T) {
	store := &fakeStorage{}

	result := consumeMediaDeleteBatch(context.Background(), store,
		&primitive.MessageExt{Message: primitive.Message{Body: []byte(`not-json`)}, MsgId: "msg-1"},
	)

	assert.Equal(t, consumer.ConsumeSuccess, result)
	assert.Empty(t, store.deleted)
}

func TestMediaCleanupConsumer_EmptyObjectKey_Skips(t *testing.T) {
	store := &fakeStorage{}

	result := consumeMediaDeleteBatch(context.Background(), store,
		&primitive.MessageExt{Message: primitive.Message{Body: []byte(`{"media_id":1,"s3_object_key":"","bucket":"xbh-media"}`)}, MsgId: "msg-2"},
	)

	assert.Equal(t, consumer.ConsumeSuccess, result)
	assert.Empty(t, store.deleted)
}

func TestMediaCleanupConsumer_ValidMessage_DeletesObject(t *testing.T) {
	store := &fakeStorage{}

	result := consumeMediaDeleteBatch(context.Background(), store,
		&primitive.MessageExt{Message: primitive.Message{Body: []byte(`{"media_id":1,"s3_object_key":"obj/key","bucket":"xbh-media","deleted_at":1710000000}`)}, MsgId: "msg-3"},
	)

	assert.Equal(t, consumer.ConsumeSuccess, result)
	require.Len(t, store.deleted, 1)
	assert.Equal(t, "obj/key", store.deleted[0])
}

func TestMediaCleanupConsumer_DeleteFails_ReturnsRetry(t *testing.T) {
	store := &fakeStorage{err: errors.New("s3 unavailable")}

	result := consumeMediaDeleteBatch(context.Background(), store,
		&primitive.MessageExt{Message: primitive.Message{Body: []byte(`{"media_id":1,"s3_object_key":"obj/key","bucket":"xbh-media","deleted_at":1710000000}`)}, MsgId: "msg-4"},
	)

	assert.Equal(t, consumer.ConsumeRetryLater, result)
}

func TestMediaCleanupConsumer_BatchSkipsBadAndProcessesGood(t *testing.T) {
	store := &fakeStorage{}

	result := consumeMediaDeleteBatch(context.Background(), store,
		&primitive.MessageExt{Message: primitive.Message{Body: []byte(`bad`)}, MsgId: "bad"},
		&primitive.MessageExt{Message: primitive.Message{Body: []byte(`{"media_id":2,"s3_object_key":"obj/key2","bucket":"xbh-media","deleted_at":1710000000}`)}, MsgId: "good"},
	)

	assert.Equal(t, consumer.ConsumeSuccess, result)
	assert.Len(t, store.deleted, 1)
	assert.Equal(t, "obj/key2", store.deleted[0])
}
```

- [ ] **Step 2: 运行测试确认失败**

```bash
cd /home/bt/projects/backend/little-white-box-content-community && GOCACHE=/tmp/go-build-little-white-box go test ./app/media/mq/internal/mqs/... -v -count=1
```

- [ ] **Step 3: 实现 consumer**

`app/media/mq/internal/mqs/media_cleanup_consumer.go`:

```go
package mqs

import (
	"context"
	"encoding/json"
	"fmt"

	"esx/app/media/mq/internal/svc"
	"mqx"

	"github.com/apache/rocketmq-client-go/v2/consumer"
	"github.com/apache/rocketmq-client-go/v2/primitive"
	"github.com/zeromicro/go-zero/core/logx"
)

type mediaDeletedMessage struct {
	MediaId     int64  `json:"media_id"`
	S3ObjectKey string `json:"s3_object_key"`
	Bucket      string `json:"bucket"`
	DeletedAt   int64  `json:"deleted_at"`
}

// ObjectDeleter is the minimal interface for S3 deletion used by the consumer.
type ObjectDeleter interface {
	Delete(ctx context.Context, objectKey string) error
}

func NewMediaCleanupConsumer(svcCtx *svc.ServiceContext) (*mqx.Consumer, error) {
	c, err := mqx.NewConsumer(svcCtx.Config.MQ)
	if err != nil {
		return nil, fmt.Errorf("media-consumer: create consumer: %w", err)
	}
	handler := func(ctx context.Context, msgs ...*primitive.MessageExt) (consumer.ConsumeResult, error) {
		return consumeMediaDeleteBatch(ctx, svcCtx.Storage, msgs...), nil
	}
	if err := c.SubscribeWithTopic(mqx.TopicMediaDelete, mqx.TagDefault, handler); err != nil {
		return nil, fmt.Errorf("media-consumer: subscribe %s: %w", mqx.TopicMediaDelete, err)
	}
	return c, nil
}

func consumeMediaDeleteBatch(ctx context.Context, deleter ObjectDeleter, msgs ...*primitive.MessageExt) consumer.ConsumeResult {
	for _, msg := range msgs {
		var m mediaDeletedMessage
		if err := json.Unmarshal(msg.Body, &m); err != nil {
			logx.WithContext(ctx).Errorw("media-consumer: unmarshal failed",
				logx.Field("msg_id", msg.MsgId), logx.Field("err", err.Error()))
			continue
		}
		if m.S3ObjectKey == "" {
			logx.WithContext(ctx).Errorw("media-consumer: empty s3_object_key, skipping",
				logx.Field("msg_id", msg.MsgId), logx.Field("media_id", m.MediaId))
			continue
		}
		if err := deleter.Delete(ctx, m.S3ObjectKey); err != nil {
			logx.WithContext(ctx).Errorw("media-consumer: delete s3 object failed",
				logx.Field("msg_id", msg.MsgId), logx.Field("media_id", m.MediaId),
				logx.Field("object_key", m.S3ObjectKey), logx.Field("err", err.Error()))
			return consumer.ConsumeRetryLater
		}
		logx.WithContext(ctx).Infow("media-consumer: s3 object deleted",
			logx.Field("media_id", m.MediaId), logx.Field("object_key", m.S3ObjectKey))
	}
	return consumer.ConsumeSuccess
}
```

- [ ] **Step 4: 实现 main.go**

`app/media/mq/main.go`:

```go
package main

import (
	"context"
	"flag"
	"fmt"

	"cleanupx"
	"esx/app/media/mq/internal/config"
	"esx/app/media/mq/internal/mqs"
	"esx/app/media/mq/internal/svc"

	"github.com/zeromicro/go-zero/core/conf"
	"github.com/zeromicro/go-zero/core/logx"
)

var configFile = flag.String("f", "etc/media-consumer.yaml", "config file")

func main() {
	flag.Parse()
	var c config.Config
	conf.MustLoad(*configFile, &c, conf.UseEnv())

	if c.S3Storage.AccessKey == "" || c.S3Storage.SecretKey == "" {
		panic("media-consumer: S3_ACCESS_KEY and S3_SECRET_KEY must be set")
	}

	svcCtx := svc.NewServiceContext(c)

	mqConsumer, err := mqs.NewMediaCleanupConsumer(svcCtx)
	if err != nil {
		panic(fmt.Sprintf("media-consumer: MQ consumer init failed: %v", err))
	}
	if err := mqConsumer.Start(); err != nil {
		panic(fmt.Sprintf("media-consumer: MQ consumer start failed: %v", err))
	}
	defer cleanupx.Shutdown(logx.WithContext(context.Background()), "media cleanup consumer", mqConsumer.Shutdown)

	fmt.Println("Media MQ consumer started, subscribing media-deleted...")
	select {}
}
```

- [ ] **Step 5: 创建 yaml 配置**

`app/media/mq/etc/media-consumer.yaml`:

```yaml
S3Storage:
  Endpoint: "127.0.0.1:8333"
  AccessKey: "${S3_ACCESS_KEY}"
  SecretKey: "${S3_SECRET_KEY}"
  UseSSL: false
  Region: "us-east-1"
  Bucket: "xbh-media"
  PublicBaseURL: "http://127.0.0.1:8333/xbh-media"
MQ:
  NameServer: "${MQ_NAMESERVER}"
  GroupName: "media-service-group"
  Topic: "media-deleted"
  Tag: "default"
  ConsumeOrder: false
```

- [ ] **Step 6: 运行测试确认通过**

```bash
cd /home/bt/projects/backend/little-white-box-content-community && GOCACHE=/tmp/go-build-little-white-box go test ./app/media/mq/... -v -count=1
```

Expected: 所有测试 PASS（5 个 mqs 测试）。

- [ ] **Step 7: Commit**

```bash
git -C /home/bt/projects/backend/little-white-box-content-community add app/media/mq/internal/mqs/ app/media/mq/main.go app/media/mq/etc/
git -C /home/bt/projects/backend/little-white-box-content-community commit -m "feat(media-mq): add cleanup consumer, main entry, and yaml config"
```

---

### Task 10: Search MQ — 骨架 (TDD)

**Files:**
- Create: `app/search/mq/internal/indexer/indexer.go`
- Create: `app/search/mq/internal/config/config.go`
- Create: `app/search/mq/internal/svc/service_context.go`
- Create: `app/search/mq/internal/mqs/search_consumer_test.go`
- Create: `app/search/mq/internal/mqs/search_consumer.go`
- Create: `app/search/mq/main.go`
- Create: `app/search/mq/etc/search-consumer.yaml`

- [ ] **Step 1: 创建 Indexer 接口和默认实现**

`app/search/mq/internal/indexer/indexer.go`:

```go
package indexer

import "context"

// IndexDoc is a generic document for indexing.
type IndexDoc struct {
	DocID string
	Type  string
	Body  map[string]any
}

// Indexer is the future ES/Milvus write interface.
type Indexer interface {
	Index(ctx context.Context, doc IndexDoc) error
	Delete(ctx context.Context, docID string) error
}

// NoopIndexer is the default no-op implementation.
type NoopIndexer struct{}

func (n *NoopIndexer) Index(ctx context.Context, doc IndexDoc) error { return nil }
func (n *NoopIndexer) Delete(ctx context.Context, docID string) error { return nil }
```

- [ ] **Step 2: 创建 config.go**

`app/search/mq/internal/config/config.go`:

```go
package config

import "mqx"

type Config struct {
	MQ mqx.ConsumerConfig
}
```

- [ ] **Step 3: 创建 service_context.go**

`app/search/mq/internal/svc/service_context.go`:

```go
package svc

import (
	"esx/app/search/mq/internal/config"
	"esx/app/search/mq/internal/indexer"
)

type ServiceContext struct {
	Config  config.Config
	Indexer indexer.Indexer
}

func NewServiceContext(c config.Config) *ServiceContext {
	return &ServiceContext{
		Config:  c,
		Indexer: &indexer.NoopIndexer{},
	}
}
```

- [ ] **Step 4: 写 consumer 测试**

`app/search/mq/internal/mqs/search_consumer_test.go`:

```go
package mqs

import (
	"context"
	"errors"
	"testing"

	"esx/app/search/mq/internal/indexer"

	"github.com/apache/rocketmq-client-go/v2/consumer"
	"github.com/apache/rocketmq-client-go/v2/primitive"
	"github.com/stretchr/testify/assert"
)

type errorIndexer struct{ err error }

func (e *errorIndexer) Index(ctx context.Context, doc indexer.IndexDoc) error  { return e.err }
func (e *errorIndexer) Delete(ctx context.Context, docID string) error          { return e.err }

type recordingIndexer struct {
	indexed []indexer.IndexDoc
	deleted []string
}

func (r *recordingIndexer) Index(ctx context.Context, doc indexer.IndexDoc) error {
	r.indexed = append(r.indexed, doc)
	return nil
}
func (r *recordingIndexer) Delete(ctx context.Context, docID string) error {
	r.deleted = append(r.deleted, docID)
	return nil
}

func TestSearchConsumer_MalformedJSON_Skips(t *testing.T) {
	rec := &recordingIndexer{}
	result := consumeSearchBatch(context.Background(), rec,
		&primitive.MessageExt{Message: primitive.Message{Body: []byte(`bad`)}, MsgId: "msg-1"},
	)
	assert.Equal(t, consumer.ConsumeSuccess, result)
	assert.Empty(t, rec.indexed)
}

func TestSearchConsumer_SearchIndexEvent_IndexesDocument(t *testing.T) {
	rec := &recordingIndexer{}
	result := consumeSearchBatch(context.Background(), rec,
		&primitive.MessageExt{Message: primitive.Message{Body: []byte(`{"type":"index","doc_id":"doc-1","body":{"title":"hello"}}`)}, MsgId: "msg-2"},
	)
	assert.Equal(t, consumer.ConsumeSuccess, result)
	assert.Len(t, rec.indexed, 1)
	assert.Equal(t, "doc-1", rec.indexed[0].DocID)
}

func TestSearchConsumer_SearchDeleteEvent_DeletesDocument(t *testing.T) {
	rec := &recordingIndexer{}
	result := consumeSearchBatch(context.Background(), rec,
		&primitive.MessageExt{Message: primitive.Message{Body: []byte(`{"type":"delete","doc_id":"doc-2"}`)}, MsgId: "msg-3"},
	)
	assert.Equal(t, consumer.ConsumeSuccess, result)
	assert.Len(t, rec.deleted, 1)
	assert.Equal(t, "doc-2", rec.deleted[0])
}

func TestSearchConsumer_MissingDocID_Skips(t *testing.T) {
	rec := &recordingIndexer{}
	result := consumeSearchBatch(context.Background(), rec,
		&primitive.MessageExt{Message: primitive.Message{Body: []byte(`{"type":"index"}`)}, MsgId: "msg-4"},
	)
	assert.Equal(t, consumer.ConsumeSuccess, result)
	assert.Empty(t, rec.indexed)
}

func TestSearchConsumer_IndexerError_ReturnsRetry(t *testing.T) {
	errIndexer := &errorIndexer{err: errors.New("es unavailable")}
	result := consumeSearchBatch(context.Background(), errIndexer,
		&primitive.MessageExt{Message: primitive.Message{Body: []byte(`{"type":"index","doc_id":"doc-3","body":{}}`)}, MsgId: "msg-5"},
	)
	assert.Equal(t, consumer.ConsumeRetryLater, result)
}
```

- [ ] **Step 5: 运行测试确认失败**

```bash
cd /home/bt/projects/backend/little-white-box-content-community && GOCACHE=/tmp/go-build-little-white-box go test ./app/search/mq/internal/mqs/... -v -count=1
```

- [ ] **Step 6: 实现 consumer**

`app/search/mq/internal/mqs/search_consumer.go`:

```go
package mqs

import (
	"context"
	"encoding/json"
	"fmt"

	"esx/app/search/mq/internal/indexer"
	"esx/app/search/mq/internal/svc"
	"mqx"

	"github.com/apache/rocketmq-client-go/v2/consumer"
	"github.com/apache/rocketmq-client-go/v2/primitive"
	"github.com/zeromicro/go-zero/core/logx"
)

type searchEvent struct {
	Type  string         `json:"type"`
	DocID string         `json:"doc_id"`
	Body  map[string]any `json:"body"`
}

func NewSearchConsumer(svcCtx *svc.ServiceContext) (*mqx.Consumer, error) {
	c, err := mqx.NewConsumer(svcCtx.Config.MQ)
	if err != nil {
		return nil, fmt.Errorf("search-consumer: create consumer: %w", err)
	}
	handler := func(ctx context.Context, msgs ...*primitive.MessageExt) (consumer.ConsumeResult, error) {
		return consumeSearchBatch(ctx, svcCtx.Indexer, msgs...), nil
	}
	if err := c.SubscribeWithTopic(mqx.TopicSearchIndex, mqx.TagDefault, handler); err != nil {
		return nil, fmt.Errorf("search-consumer: subscribe %s: %w", mqx.TopicSearchIndex, err)
	}
	return c, nil
}

func consumeSearchBatch(ctx context.Context, idx indexer.Indexer, msgs ...*primitive.MessageExt) consumer.ConsumeResult {
	for _, msg := range msgs {
		var event searchEvent
		if err := json.Unmarshal(msg.Body, &event); err != nil {
			logx.WithContext(ctx).Errorw("search-consumer: unmarshal failed",
				logx.Field("msg_id", msg.MsgId), logx.Field("err", err.Error()))
			continue
		}
		if event.DocID == "" {
			logx.WithContext(ctx).Errorw("search-consumer: missing doc_id",
				logx.Field("msg_id", msg.MsgId), logx.Field("type", event.Type))
			continue
		}
		switch event.Type {
		case "index":
			if err := idx.Index(ctx, indexer.IndexDoc{
				DocID: event.DocID, Type: event.Type, Body: event.Body,
			}); err != nil {
				logx.WithContext(ctx).Errorw("search-consumer: index failed",
					logx.Field("msg_id", msg.MsgId), logx.Field("doc_id", event.DocID),
					logx.Field("err", err.Error()))
				return consumer.ConsumeRetryLater
			}
			logx.WithContext(ctx).Infow("search-consumer: document indexed",
				logx.Field("doc_id", event.DocID))
		case "delete":
			if err := idx.Delete(ctx, event.DocID); err != nil {
				logx.WithContext(ctx).Errorw("search-consumer: delete failed",
					logx.Field("msg_id", msg.MsgId), logx.Field("doc_id", event.DocID),
					logx.Field("err", err.Error()))
				return consumer.ConsumeRetryLater
			}
			logx.WithContext(ctx).Infow("search-consumer: document deleted",
				logx.Field("doc_id", event.DocID))
		default:
			logx.WithContext(ctx).Errorw("search-consumer: unknown event type, skipping",
				logx.Field("msg_id", msg.MsgId), logx.Field("type", event.Type))
		}
	}
	return consumer.ConsumeSuccess
}
```

- [ ] **Step 7: 实现 main.go**

`app/search/mq/main.go`:

```go
package main

import (
	"context"
	"flag"
	"fmt"

	"cleanupx"
	"esx/app/search/mq/internal/config"
	"esx/app/search/mq/internal/mqs"
	"esx/app/search/mq/internal/svc"

	"github.com/zeromicro/go-zero/core/conf"
	"github.com/zeromicro/go-zero/core/logx"
)

var configFile = flag.String("f", "etc/search-consumer.yaml", "config file")

func main() {
	flag.Parse()
	var c config.Config
	conf.MustLoad(*configFile, &c, conf.UseEnv())

	svcCtx := svc.NewServiceContext(c)

	searchConsumer, err := mqs.NewSearchConsumer(svcCtx)
	if err != nil {
		logx.Must(err)
	}
	if err := searchConsumer.Start(); err != nil {
		logx.Must(err)
	}
	defer cleanupx.Shutdown(logx.WithContext(context.Background()), "search consumer", searchConsumer.Shutdown)

	fmt.Println("Search MQ consumer started, subscribing search-index...")
	select {}
}
```

- [ ] **Step 8: 创建 yaml 配置**

`app/search/mq/etc/search-consumer.yaml`:

```yaml
MQ:
  NameServer: "${MQ_NAMESERVER}"
  GroupName: "search-service-group"
  Topic: "search-index"
  Tag: "default"
  ConsumeOrder: false
```

- [ ] **Step 9: 运行测试确认通过**

```bash
cd /home/bt/projects/backend/little-white-box-content-community && GOCACHE=/tmp/go-build-little-white-box go test ./app/search/mq/... -v -count=1
```

Expected: 所有测试 PASS（5 个 mqs 测试）。

- [ ] **Step 10: Commit**

```bash
git -C /home/bt/projects/backend/little-white-box-content-community add app/search/mq/
git -C /home/bt/projects/backend/little-white-box-content-community commit -m "feat(search-mq): add skeleton search consumer with Indexer interface"
```

---

### Task 11: Recommend MQ — 骨架 (TDD)

**Files:**
- Create: `app/recommend/mq/internal/store/behavior_store.go`
- Create: `app/recommend/mq/internal/config/config.go`
- Create: `app/recommend/mq/internal/svc/service_context.go`
- Create: `app/recommend/mq/internal/mqs/recommend_consumer_test.go`
- Create: `app/recommend/mq/internal/mqs/recommend_consumer.go`
- Create: `app/recommend/mq/main.go`
- Create: `app/recommend/mq/etc/recommend-consumer.yaml`

- [ ] **Step 1: 创建 BehaviorStore 接口和默认实现**

`app/recommend/mq/internal/store/behavior_store.go`:

```go
package store

import "context"

// BehaviorEvent is a generic user behavior event.
type BehaviorEvent struct {
	UserID    int64  `json:"user_id"`
	Action    string `json:"action"`
	TargetID  int64  `json:"target_id"`
	TargetType string `json:"target_type"`
}

// BehaviorStore is the future profile/feature write interface.
type BehaviorStore interface {
	Record(ctx context.Context, event BehaviorEvent) error
}

// NoopBehaviorStore is the default no-op implementation.
type NoopBehaviorStore struct{}

func (n *NoopBehaviorStore) Record(ctx context.Context, event BehaviorEvent) error { return nil }
```

- [ ] **Step 2: 创建 config.go**

`app/recommend/mq/internal/config/config.go`:

```go
package config

import "mqx"

type Config struct {
	MQ mqx.ConsumerConfig
}
```

- [ ] **Step 3: 创建 service_context.go**

`app/recommend/mq/internal/svc/service_context.go`:

```go
package svc

import (
	"esx/app/recommend/mq/internal/config"
	"esx/app/recommend/mq/internal/store"
)

type ServiceContext struct {
	Config        config.Config
	BehaviorStore store.BehaviorStore
}

func NewServiceContext(c config.Config) *ServiceContext {
	return &ServiceContext{
		Config:        c,
		BehaviorStore: &store.NoopBehaviorStore{},
	}
}
```

- [ ] **Step 4: 写 consumer 测试**

`app/recommend/mq/internal/mqs/recommend_consumer_test.go`:

```go
package mqs

import (
	"context"
	"errors"
	"testing"

	"esx/app/recommend/mq/internal/store"

	"github.com/apache/rocketmq-client-go/v2/consumer"
	"github.com/apache/rocketmq-client-go/v2/primitive"
	"github.com/stretchr/testify/assert"
)

type recordingStore struct{ recorded []store.BehaviorEvent }

func (r *recordingStore) Record(ctx context.Context, e store.BehaviorEvent) error {
	r.recorded = append(r.recorded, e)
	return nil
}

type errorStore struct{ err error }

func (e *errorStore) Record(ctx context.Context, ev store.BehaviorEvent) error { return e.err }

func TestRecommendConsumer_MalformedJSON_Skips(t *testing.T) {
	rec := &recordingStore{}
	result := consumeBehaviorBatch(context.Background(), rec,
		&primitive.MessageExt{Message: primitive.Message{Body: []byte(`bad`)}, MsgId: "msg-1"},
	)
	assert.Equal(t, consumer.ConsumeSuccess, result)
	assert.Empty(t, rec.recorded)
}

func TestRecommendConsumer_MissingUserID_Skips(t *testing.T) {
	rec := &recordingStore{}
	result := consumeBehaviorBatch(context.Background(), rec,
		&primitive.MessageExt{Message: primitive.Message{Body: []byte(`{"action":"view"}`)}, MsgId: "msg-2"},
	)
	assert.Equal(t, consumer.ConsumeSuccess, result)
	assert.Empty(t, rec.recorded)
}

func TestRecommendConsumer_MissingAction_Skips(t *testing.T) {
	rec := &recordingStore{}
	result := consumeBehaviorBatch(context.Background(), rec,
		&primitive.MessageExt{Message: primitive.Message{Body: []byte(`{"user_id":1}`)}, MsgId: "msg-3"},
	)
	assert.Equal(t, consumer.ConsumeSuccess, result)
	assert.Empty(t, rec.recorded)
}

func TestRecommendConsumer_ValidEvent_Records(t *testing.T) {
	rec := &recordingStore{}
	result := consumeBehaviorBatch(context.Background(), rec,
		&primitive.MessageExt{Message: primitive.Message{Body: []byte(`{"user_id":1,"action":"view","target_id":99,"target_type":"post"}`)}, MsgId: "msg-4"},
	)
	assert.Equal(t, consumer.ConsumeSuccess, result)
	assert.Len(t, rec.recorded, 1)
	assert.Equal(t, int64(1), rec.recorded[0].UserID)
	assert.Equal(t, "view", rec.recorded[0].Action)
}

func TestRecommendConsumer_StoreError_ReturnsRetry(t *testing.T) {
	errStore := &errorStore{err: errors.New("store offline")}
	result := consumeBehaviorBatch(context.Background(), errStore,
		&primitive.MessageExt{Message: primitive.Message{Body: []byte(`{"user_id":1,"action":"view","target_id":99,"target_type":"post"}`)}, MsgId: "msg-5"},
	)
	assert.Equal(t, consumer.ConsumeRetryLater, result)
}
```

- [ ] **Step 5: 运行测试确认失败**

```bash
cd /home/bt/projects/backend/little-white-box-content-community && GOCACHE=/tmp/go-build-little-white-box go test ./app/recommend/mq/internal/mqs/... -v -count=1
```

- [ ] **Step 6: 实现 consumer**

`app/recommend/mq/internal/mqs/recommend_consumer.go`:

```go
package mqs

import (
	"context"
	"encoding/json"
	"fmt"

	"esx/app/recommend/mq/internal/store"
	"esx/app/recommend/mq/internal/svc"
	"mqx"

	"github.com/apache/rocketmq-client-go/v2/consumer"
	"github.com/apache/rocketmq-client-go/v2/primitive"
	"github.com/zeromicro/go-zero/core/logx"
)

type behaviorEvent struct {
	UserID     int64  `json:"user_id"`
	Action     string `json:"action"`
	TargetID   int64  `json:"target_id"`
	TargetType string `json:"target_type"`
}

func NewRecommendConsumer(svcCtx *svc.ServiceContext) (*mqx.Consumer, error) {
	c, err := mqx.NewConsumer(svcCtx.Config.MQ)
	if err != nil {
		return nil, fmt.Errorf("recommend-consumer: create consumer: %w", err)
	}
	handler := func(ctx context.Context, msgs ...*primitive.MessageExt) (consumer.ConsumeResult, error) {
		return consumeBehaviorBatch(ctx, svcCtx.BehaviorStore, msgs...), nil
	}
	if err := c.SubscribeWithTopic(mqx.TopicUserBehavior, mqx.TagDefault, handler); err != nil {
		return nil, fmt.Errorf("recommend-consumer: subscribe %s: %w", mqx.TopicUserBehavior, err)
	}
	return c, nil
}

func consumeBehaviorBatch(ctx context.Context, bs store.BehaviorStore, msgs ...*primitive.MessageExt) consumer.ConsumeResult {
	for _, msg := range msgs {
		var event behaviorEvent
		if err := json.Unmarshal(msg.Body, &event); err != nil {
			logx.WithContext(ctx).Errorw("recommend-consumer: unmarshal failed",
				logx.Field("msg_id", msg.MsgId), logx.Field("err", err.Error()))
			continue
		}
		if event.UserID <= 0 {
			logx.WithContext(ctx).Errorw("recommend-consumer: missing user_id",
				logx.Field("msg_id", msg.MsgId))
			continue
		}
		if event.Action == "" {
			logx.WithContext(ctx).Errorw("recommend-consumer: missing action",
				logx.Field("msg_id", msg.MsgId))
			continue
		}
		if err := bs.Record(ctx, store.BehaviorEvent{
			UserID: event.UserID, Action: event.Action,
			TargetID: event.TargetID, TargetType: event.TargetType,
		}); err != nil {
			logx.WithContext(ctx).Errorw("recommend-consumer: record behavior failed",
				logx.Field("msg_id", msg.MsgId), logx.Field("user_id", event.UserID),
				logx.Field("err", err.Error()))
			return consumer.ConsumeRetryLater
		}
		logx.WithContext(ctx).Infow("recommend-consumer: behavior recorded",
			logx.Field("user_id", event.UserID), logx.Field("action", event.Action))
	}
	return consumer.ConsumeSuccess
}
```

- [ ] **Step 7: 实现 main.go**

`app/recommend/mq/main.go`:

```go
package main

import (
	"context"
	"flag"
	"fmt"

	"cleanupx"
	"esx/app/recommend/mq/internal/config"
	"esx/app/recommend/mq/internal/mqs"
	"esx/app/recommend/mq/internal/svc"

	"github.com/zeromicro/go-zero/core/conf"
	"github.com/zeromicro/go-zero/core/logx"
)

var configFile = flag.String("f", "etc/recommend-consumer.yaml", "config file")

func main() {
	flag.Parse()
	var c config.Config
	conf.MustLoad(*configFile, &c, conf.UseEnv())

	svcCtx := svc.NewServiceContext(c)

	recConsumer, err := mqs.NewRecommendConsumer(svcCtx)
	if err != nil {
		logx.Must(err)
	}
	if err := recConsumer.Start(); err != nil {
		logx.Must(err)
	}
	defer cleanupx.Shutdown(logx.WithContext(context.Background()), "recommend consumer", recConsumer.Shutdown)

	fmt.Println("Recommend MQ consumer started, subscribing user-behavior...")
	select {}
}
```

- [ ] **Step 8: 创建 yaml 配置**

`app/recommend/mq/etc/recommend-consumer.yaml`:

```yaml
MQ:
  NameServer: "${MQ_NAMESERVER}"
  GroupName: "recommend-service-group"
  Topic: "user-behavior"
  Tag: "default"
  ConsumeOrder: false
```

- [ ] **Step 9: 运行测试确认通过**

```bash
cd /home/bt/projects/backend/little-white-box-content-community && GOCACHE=/tmp/go-build-little-white-box go test ./app/recommend/mq/... -v -count=1
```

Expected: 所有测试 PASS（5 个 mqs 测试）。

- [ ] **Step 10: Commit**

```bash
git -C /home/bt/projects/backend/little-white-box-content-community add app/recommend/mq/
git -C /home/bt/projects/backend/little-white-box-content-community commit -m "feat(recommend-mq): add skeleton recommend consumer with BehaviorStore interface"
```

---

### Task 12: RPC 迁移 — 从 RPC 进程移除 MQ Consumer

**Files:**
- Modify: `app/feed/rpc/feed.go`
- Modify: `app/message/rpc/message.go`
- Modify: `app/media/rpc/media.go`

- [ ] **Step 1: 修改 feed.go — 移除 MQ consumer 启动代码**

将 `app/feed/rpc/feed.go` 中以下代码段：

```go
var postConsumer *mqx.Consumer
if c.MQ.NameServer != "" {
    var err error
    postConsumer, err = mqs.NewPostPublishConsumer(ctx)
    if err != nil {
        logx.Must(err)
    }
    if err := postConsumer.Start(); err != nil {
        logx.Must(err)
    }
    defer cleanupx.Shutdown(logx.WithContext(context.Background()), "post publish consumer", postConsumer.Shutdown)
}
```

替换为（移除整段），同时移除不再需要的 import：
- `"cleanupx"`
- `"esx/app/feed/rpc/internal/mqs"`
- `"mqx"`

修改后的 `feed.go`:

```go
package main

import (
	"flag"
	"fmt"

	"esx/app/feed/rpc/internal/config"
	"esx/app/feed/rpc/internal/server"
	"esx/app/feed/rpc/internal/svc"
	"esx/app/feed/rpc/xiaobaihe/feed/pb"

	"github.com/zeromicro/go-zero/core/conf"
	"github.com/zeromicro/go-zero/core/service"
	"github.com/zeromicro/go-zero/zrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

var configFile = flag.String("f", "etc/feed.yaml", "the config file")

func main() {
	flag.Parse()

	var c config.Config
	conf.MustLoad(*configFile, &c, conf.UseEnv())
	ctx := svc.NewServiceContext(c)

	s := zrpc.MustNewServer(c.RpcServerConf, func(grpcServer *grpc.Server) {
		pb.RegisterFeedServiceServer(grpcServer, server.NewFeedServiceServer(ctx))

		if c.Mode == service.DevMode || c.Mode == service.TestMode {
			reflection.Register(grpcServer)
		}
	})
	defer s.Stop()

	fmt.Printf("Starting rpc server at %s...\n", c.ListenOn)
	s.Start()
}
```

- [ ] **Step 2: 修改 message.go — 移除 MQ consumer 启动代码**

将 `app/message/rpc/message.go` 中：

```go
var messageConsumer *mqx.Consumer
if c.MQ.NameServer != "" && c.MQ.Topic != "" {
    var err error
    messageConsumer, err = mqs.NewRocketMQConsumer(ctx)
    if err != nil {
        logx.Must(err)
    }
    if err := messageConsumer.Start(); err != nil {
        logx.Must(err)
    }
    defer cleanupx.Shutdown(logx.WithContext(context.Background()), "message consumer", messageConsumer.Shutdown)
}
```

替换为（移除），移除 import `"cleanupx"`, `"esx/app/message/rpc/internal/mqs"`, `"mqx"`.

修改后的 `message.go`:

```go
package main

import (
	"esx/app/message/rpc/internal/config"
	"esx/app/message/rpc/internal/server"
	"esx/app/message/rpc/internal/svc"
	"esx/app/message/rpc/xiaobaihe/message/pb"
	"flag"
	"fmt"

	"github.com/zeromicro/go-zero/core/conf"
	"github.com/zeromicro/go-zero/core/service"
	"github.com/zeromicro/go-zero/zrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

var configFile = flag.String("f", "etc/message.yaml", "the config file")

func main() {
	flag.Parse()

	var c config.Config
	conf.MustLoad(*configFile, &c, conf.UseEnv())
	ctx := svc.NewServiceContext(c)

	s := zrpc.MustNewServer(c.RpcServerConf, func(grpcServer *grpc.Server) {
		pb.RegisterMessageServiceServer(grpcServer, server.NewMessageServiceServer(ctx))

		if c.Mode == service.DevMode || c.Mode == service.TestMode {
			reflection.Register(grpcServer)
		}
	})
	defer s.Stop()

	fmt.Printf("Starting rpc server at %s...\n", c.ListenOn)
	s.Start()
}
```

- [ ] **Step 3: 修改 media.go — 移除 MQ consumer 启动代码**

将 `app/media/rpc/media.go` 中：

```go
if c.MQ.NameServer != "" {
    mqConsumer, err := mqs.NewMediaCleanupConsumer(ctx)
    if err != nil {
        panic(fmt.Sprintf("media: MQ consumer initialization failed: %v", err))
    }
    if err = mqConsumer.Start(); err != nil {
        panic(fmt.Sprintf("media: MQ consumer start failed: %v", err))
    }
    defer cleanupx.Shutdown(logx.WithContext(context.Background()), "media cleanup consumer", mqConsumer.Shutdown)
}
```

替换为（移除），移除 import `"cleanupx"`, `"esx/app/media/rpc/internal/mqs"`.

修改后的 `media.go`:

```go
package main

import (
	"esx/app/media/rpc/internal/config"
	"esx/app/media/rpc/internal/server"
	"esx/app/media/rpc/internal/svc"
	"esx/app/media/rpc/pb/xiaobaihe/media/pb"
	"flag"
	"fmt"

	"github.com/zeromicro/go-zero/core/conf"
	"github.com/zeromicro/go-zero/core/service"
	"github.com/zeromicro/go-zero/zrpc"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

var configFile = flag.String("f", "etc/media.yaml", "the config file")

func main() {
	flag.Parse()

	var c config.Config
	conf.MustLoad(*configFile, &c, conf.UseEnv())

	if c.S3Storage.AccessKey == "" || c.S3Storage.SecretKey == "" {
		panic("S3_ACCESS_KEY and S3_SECRET_KEY must be set")
	}

	ctx := svc.NewServiceContext(c)

	s := zrpc.MustNewServer(c.RpcServerConf, func(grpcServer *grpc.Server) {
		pb.RegisterMediaServiceServer(grpcServer, server.NewMediaServiceServer(ctx))

		if c.Mode == service.DevMode || c.Mode == service.TestMode {
			reflection.Register(grpcServer)
		}
	})
	defer s.Stop()

	fmt.Printf("Starting rpc server at %s...\n", c.ListenOn)
	s.Start()
}
```

- [ ] **Step 4: Commit**

```bash
git -C /home/bt/projects/backend/little-white-box-content-community add app/feed/rpc/feed.go app/message/rpc/message.go app/media/rpc/media.go
git -C /home/bt/projects/backend/little-white-box-content-community commit -m "refactor: remove MQ consumer startup from Feed/Message/Media RPC processes"
```

---

### Task 13: 移除旧 mqs 代码

**Files:**
- Delete: `app/feed/rpc/internal/mqs/post_publish_consumer.go`
- Delete: `app/feed/rpc/internal/mqs/post_publish_consumer_test.go`
- Delete: `app/feed/rpc/internal/mqs/feed_integration_test.go`
- Delete: `app/feed/rpc/internal/mqs/feed_testdb_integration_test.go`
- Delete: `app/message/rpc/internal/mqs/message_consumer.go`
- Delete: `app/message/rpc/internal/mqs/message_consumer_test.go`
- Delete: `app/media/rpc/internal/mqs/media_cleanup_consumer.go`

- [ ] **Step 1: 删除旧 mqs 文件并验证编译**

```bash
rm /home/bt/projects/backend/little-white-box-content-community/app/feed/rpc/internal/mqs/post_publish_consumer.go
rm /home/bt/projects/backend/little-white-box-content-community/app/feed/rpc/internal/mqs/post_publish_consumer_test.go
rm /home/bt/projects/backend/little-white-box-content-community/app/feed/rpc/internal/mqs/feed_integration_test.go
rm /home/bt/projects/backend/little-white-box-content-community/app/feed/rpc/internal/mqs/feed_testdb_integration_test.go
rm /home/bt/projects/backend/little-white-box-content-community/app/message/rpc/internal/mqs/message_consumer.go
rm /home/bt/projects/backend/little-white-box-content-community/app/message/rpc/internal/mqs/message_consumer_test.go
rm /home/bt/projects/backend/little-white-box-content-community/app/media/rpc/internal/mqs/media_cleanup_consumer.go
```

- [ ] **Step 2: 验证编译通过**

```bash
cd /home/bt/projects/backend/little-white-box-content-community && GOCACHE=/tmp/go-build-little-white-box go build ./app/feed/rpc/ ./app/message/rpc/ ./app/media/rpc/
```

Expected: 编译成功。

- [ ] **Step 3: Commit**

```bash
git -C /home/bt/projects/backend/little-white-box-content-community add app/feed/rpc/internal/mqs/ app/message/rpc/internal/mqs/ app/media/rpc/internal/mqs/
git -C /home/bt/projects/backend/little-white-box-content-community commit -m "refactor: remove old RPC mqs code, now owned by app/*/mq"
```

---

### Task 14: 最终验证

- [ ] **Step 1: 运行所有新 MQ consumer 的测试**

```bash
cd /home/bt/projects/backend/little-white-box-content-community && GOCACHE=/tmp/go-build-little-white-box go test ./app/feed/mq/... ./app/message/mq/... ./app/media/mq/... ./app/search/mq/... ./app/recommend/mq/... -race -cover -v
```

Expected: 所有测试 PASS，共约 34 个测试。

- [ ] **Step 2: 运行 go vet**

```bash
cd /home/bt/projects/backend/little-white-box-content-community && GOCACHE=/tmp/go-build-little-white-box go vet ./app/feed/mq/... ./app/message/mq/... ./app/media/mq/... ./app/search/mq/... ./app/recommend/mq/...
```

Expected: 无错误。

- [ ] **Step 3: 验证所有 MQ consumer 可编译**

```bash
cd /home/bt/projects/backend/little-white-box-content-community && GOCACHE=/tmp/go-build-little-white-box go build ./app/feed/mq/ ./app/message/mq/ ./app/media/mq/ ./app/search/mq/ ./app/recommend/mq/
```

Expected: 编译成功。

- [ ] **Step 4: 验证 RPC 服务仍可编译**

```bash
cd /home/bt/projects/backend/little-white-box-content-community && GOCACHE=/tmp/go-build-little-white-box go build ./app/feed/rpc/ ./app/message/rpc/ ./app/media/rpc/
```

Expected: 编译成功。

- [ ] **Step 5: 运行仓库级验证（若被历史问题阻塞则报告）**

```bash
cd /home/bt/projects/backend/little-white-box-content-community && GOCACHE=/tmp/go-build-little-white-box go test ./... -race -cover 2>&1 | tail -30
cd /home/bt/projects/backend/little-white-box-content-community && GOCACHE=/tmp/go-build-little-white-box go vet ./... 2>&1 | tail -20
```

Expected: 新 MQ consumer 包通过；旧包如果存在非本分支引入的失败需明确报告。

- [ ] **Step 6: Commit (若有最终调整)**

```bash
git -C /home/bt/projects/backend/little-white-box-content-community add -A
git -C /home/bt/projects/backend/little-white-box-content-community commit -m "chore: final verification and adjustments for W5 MQ consumers"
```

---

## 自审检查表

- [x] **规格覆盖** — 5 个 consumer 入口（Task 1-11）、RPC 迁移（Task 12）、旧代码移除（Task 13）均有对应任务
- [x] **无占位符** — 所有步骤均有完整代码，无 TBD/TODO/"类似上文"
- [x] **类型一致性** — `PostPublished` 定义在 `logic` 包，consumer 和测试均引用同一类型；`UserService` 在 svc 定义接口，logic 的 `UserGetter` 使用 `...grpc.CallOption` 匹配
- [x] **TDD 顺序** — 每个 logic/consumer 任务先写测试再实现
- [x] **错误处理** — 永久错误 → ConsumeSuccess + 日志；临时错误 → ConsumeRetryLater
- [x] **Context 传递** — 所有日志使用 `logx.WithContext(ctx)`
- [x] **不共享 RPC logic** — MQ consumer 有自己独立的 logic/model/storage
- [x] **topic 硬编码** — 所有 consumer 使用 `mqx.TopicXxx` 和 `mqx.GroupXxxService` 常量
