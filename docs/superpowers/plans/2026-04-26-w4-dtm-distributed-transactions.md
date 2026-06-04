# W4 DTM Distributed Transactions Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement W4 DTM distributed transactions for post creation plus Feed fanout, keep interaction likes inside local MySQL consistency, and store DTM transaction state in MySQL.

**Architecture:** Content starts a DTM reliable message transaction that commits the post/tag local transaction and then invokes a Feed fanout branch. Interaction likes are not distributed transactions because `like_record` and `action_count` are owned by the same service and database; if strict counter consistency is required, use one local MySQL transaction around the state transition and counter update. DTM SDK calls are hidden behind small service-local interfaces for unit tests.

**Tech Stack:** Go 1.26.1, go-zero v1.10.1 RPC, MySQL 8.0, DTM gRPC SDK, testify, goctl.

---

## File Structure

- Modify `deploy/docker-compose.middleware.yml`: switch DTM storage from Redis to MySQL.
- Create `deploy/dtm_storage_test.go`: guard test for MySQL-backed DTM compose config.
- Create `deploy/sql/xbh_dtm.sql`: create durable DTM store database.
- Modify `deploy/sql/xbh_content.sql`, `deploy/sql/xbh_feed.sql`: add DTM branch barrier table to business DBs that participate in the content-to-feed flow.
- Modify `proto/content/content.proto`: add DTM `QueryPrepared` endpoint for reliable message checkback.
- Modify `proto/feed/feed.proto`: add `FanoutPost` branch endpoint.
- Regenerate goctl output under `app/content` and `app/feed`.
- Modify `app/content/internal/config/config.go` and `app/content/etc/content.yaml`: add DTM config.
- Modify `app/content/internal/svc/service_context.go`: keep raw `*sql.DB`, inject DTM message factory.
- Create `app/content/internal/svc/dtm.go`: DTM reliable message adapter.
- Modify `app/content/internal/model/post_model.go`: add `InsertPostTx`.
- Modify `app/content/internal/model/post_tag_model.go`: add `BatchInsertTagsByPostIdTx`.
- Modify `app/content/internal/logic/create_post_logic.go`: use DTM reliable message transaction.
- Implement generated `app/content/internal/logic/query_prepared_logic.go`: DTM query prepared handler.
- Modify `app/feed/internal/config/config.go` and `app/feed/etc/feed.yaml`: add `FeedBusiServer` if needed for branch URL clarity.
- Create `app/feed/internal/fanout/post_fanout.go`: shared fanout workflow used by RPC and MQ.
- Modify `app/feed/internal/mqs/post_publish_consumer.go`: delegate to shared fanout package.
- Implement generated `app/feed/internal/logic/fanout_post_logic.go`: DTM branch endpoint.
- Modify `app/interaction/internal/svc/service_context.go`: expose the interaction `sqlx.SqlConn` for local transactions.
- Modify `app/interaction/internal/model/like_record_model.go`: add transaction-aware like status upsert and cache invalidation helpers.
- Modify `app/interaction/internal/model/action_count_model.go`: add transaction-aware like counter increment helper.
- Modify `app/interaction/internal/logic/like_logic.go`: keep Like local; wrap like state and counter update in one local transaction if strict consistency is implemented.
- Do not `git add` or commit `docs/superpowers/specs/*` or `docs/superpowers/plans/*`.

### Task 1: MySQL-Backed DTM Deployment

**Files:**
- Create: `deploy/dtm_storage_test.go`
- Create: `deploy/sql/xbh_dtm.sql`
- Modify: `deploy/docker-compose.middleware.yml`
- Modify: `deploy/sql/xbh_content.sql`
- Modify: `deploy/sql/xbh_feed.sql`

- [ ] **Step 1: Write failing DTM storage test**

Create `deploy/dtm_storage_test.go`:

```go
package deploy

import (
	"os"
	"strings"
	"testing"
)

func TestDTMServiceUsesMySQLStorage(t *testing.T) {
	body, err := os.ReadFile("docker-compose.middleware.yml")
	if err != nil {
		t.Fatal(err)
	}
	content := string(body)
	start := strings.Index(content, "  dtm:")
	if start < 0 {
		t.Fatal("dtm service block not found")
	}
	end := strings.Index(content[start+1:], "\n  # Jaeger")
	if end < 0 {
		t.Fatal("dtm service block terminator not found")
	}
	block := content[start : start+1+end]
	if !strings.Contains(block, "STORE_DRIVER: mysql") {
		t.Fatalf("dtm must use mysql storage, block:\n%s", block)
	}
	if strings.Contains(block, "STORE_DRIVER: redis") || strings.Contains(block, "STORE_HOST: redis") {
		t.Fatalf("dtm block still contains redis storage config:\n%s", block)
	}
	if !strings.Contains(block, `STORE_DSN: "${DTM_STORE_DSN}"`) {
		t.Fatalf("dtm block must use DTM_STORE_DSN env placeholder, block:\n%s", block)
	}
	if !strings.Contains(block, "mysql:") || !strings.Contains(block, "condition: service_healthy") {
		t.Fatalf("dtm must depend on healthy mysql, block:\n%s", block)
	}
}
```

- [ ] **Step 2: Run test and verify RED**

Run: `GOCACHE=/tmp/go-build go test ./deploy -run TestDTMServiceUsesMySQLStorage -count=1`

Expected: FAIL with message that the DTM block contains Redis storage config.

- [ ] **Step 3: Update compose and SQL**

Change the DTM service block in `deploy/docker-compose.middleware.yml` to:

```yaml
  # DTM 分布式事务
  dtm:
    image: yedf/dtm:latest
    container_name: xbh-dtm
    restart: unless-stopped
    environment:
      IS_DOCKER: "1"
      STORE_DRIVER: mysql
      STORE_DSN: "${DTM_STORE_DSN}"
    ports:
      - "36789:36789"
      - "36790:36790"
    depends_on:
      mysql:
        condition: service_healthy
    networks:
      - xbh-network
```

Create `deploy/sql/xbh_dtm.sql`:

```sql
CREATE DATABASE IF NOT EXISTS `dtm` DEFAULT CHARACTER SET utf8mb4 COLLATE utf8mb4_unicode_ci;
```

Append this table to `deploy/sql/xbh_content.sql` and `deploy/sql/xbh_feed.sql` after each file's `USE` statement or table definitions:

```sql
CREATE TABLE IF NOT EXISTS `dtm_barrier` (
    `id` BIGINT NOT NULL AUTO_INCREMENT,
    `trans_type` VARCHAR(45) NOT NULL DEFAULT '',
    `gid` VARCHAR(128) NOT NULL DEFAULT '',
    `branch_id` VARCHAR(128) NOT NULL DEFAULT '',
    `op` VARCHAR(45) NOT NULL DEFAULT '',
    `barrier_id` VARCHAR(45) NOT NULL DEFAULT '',
    `reason` VARCHAR(45) NOT NULL DEFAULT '',
    `create_time` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
    `update_time` DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP ON UPDATE CURRENT_TIMESTAMP,
    PRIMARY KEY (`id`),
    UNIQUE KEY `uniq_barrier` (`gid`, `branch_id`, `op`, `barrier_id`),
    KEY `idx_create_time` (`create_time`),
    KEY `idx_update_time` (`update_time`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='DTM branch barrier table';
```

- [ ] **Step 4: Run test and verify GREEN**

Run: `GOCACHE=/tmp/go-build go test ./deploy -run TestDTMServiceUsesMySQLStorage -count=1`

Expected: PASS.

- [ ] **Step 5: Commit code-only deployment changes**

Run:

```bash
git add deploy/docker-compose.middleware.yml deploy/dtm_storage_test.go deploy/sql/xbh_dtm.sql deploy/sql/xbh_content.sql deploy/sql/xbh_feed.sql
git commit -m "feat: use mysql-backed dtm storage"
```

Do not add `docs/superpowers/specs/*` or `docs/superpowers/plans/*`.

### Task 2: Proto Contracts And Generated RPC Code

**Files:**
- Modify: `proto/content/content.proto`
- Modify: `proto/feed/feed.proto`
- Generated by goctl: `app/content/**`, `app/feed/**`

- [ ] **Step 1: Write failing compile references**

Add temporary compile expectations to existing tests after proto edits are planned, or rely on goctl generation as the RED gate. The intended missing symbols before generation are:

```go
var _ = (*contentpb.QueryPreparedReq)(nil)
var _ = (*feedpb.FanoutPostReq)(nil)
```

- [ ] **Step 2: Modify proto files**

Add to `proto/content/content.proto` service:

```protobuf
  // DTM reliable message query-prepared checkback
  rpc QueryPrepared(QueryPreparedReq) returns (QueryPreparedResp);
```

Add to `proto/content/content.proto` messages:

```protobuf
message QueryPreparedReq {}
message QueryPreparedResp {}
```

Add to `proto/feed/feed.proto` service:

```protobuf
  rpc FanoutPost(FanoutPostReq) returns (FanoutPostResp);
```

Add to `proto/feed/feed.proto` messages:

```protobuf
message FanoutPostReq {
  int64 author_id = 1;
  int64 post_id = 2;
  int64 created_at = 3;
}

message FanoutPostResp {
  int64 pushed_count = 1;
}
```

Do not add interaction Saga branch RPCs. `proto/interaction/interaction.proto` remains unchanged for W4 DTM because likes are interaction-local writes.

- [ ] **Step 3: Regenerate goctl output**

Run:

```bash
goctl rpc protoc proto/content/content.proto --go_out=app/content --go-grpc_out=app/content --zrpc_out=app/content --style go_zero
goctl rpc protoc proto/feed/feed.proto --go_out=app/feed --go-grpc_out=app/feed --zrpc_out=app/feed --style go_zero
```

Expected: generated client/server/logic stubs for the new content and feed RPC methods. Do not hand-edit generated files.

- [ ] **Step 4: Verify generated code compiles far enough**

Run: `GOCACHE=/tmp/go-build go test ./app/content/... ./app/feed/... -run TestDoesNotExist -count=1`

Expected: compile may fail because new generated logic stubs are not implemented yet; missing protobuf symbols should be gone.

- [ ] **Step 5: Commit proto and generated code**

Run:

```bash
git add proto/content/content.proto proto/feed/feed.proto app/content app/feed
git commit -m "feat: add dtm rpc branch contracts"
```

Do not add ignored docs.

### Task 3: Content Reliable Message Infrastructure

**Files:**
- Modify: `app/content/internal/config/config.go`
- Modify: `app/content/etc/content.yaml`
- Modify: `app/content/internal/svc/service_context.go`
- Create: `app/content/internal/svc/dtm.go`
- Modify: `app/content/internal/model/post_model.go`
- Modify: `app/content/internal/model/post_tag_model.go`
- Modify: `app/content/internal/logic/mock_models_test.go`
- Modify: `app/content/internal/logic/create_post_event_test.go`
- Modify: `app/content/internal/logic/create_post_logic.go`
- Modify: `app/content/internal/logic/query_prepared_logic.go`

- [ ] **Step 1: Write failing CreatePost DTM test**

Extend `app/content/internal/logic/create_post_event_test.go` with fake DTM types:

```go
type fakePostCreateMsg struct {
	actions []string
	payloads []proto.Message
	didSubmit bool
}

func (m *fakePostCreateMsg) Add(action string, payload proto.Message) {
	m.actions = append(m.actions, action)
	m.payloads = append(m.payloads, payload)
}

func (m *fakePostCreateMsg) DoAndSubmitDB(queryPrepared string, fn func(*sql.Tx) error) error {
	m.didSubmit = true
	return fn(nil)
}

type fakePostCreateMsgFactory struct{ msg *fakePostCreateMsg }

func (f fakePostCreateMsgFactory) NewPostCreateMsg(gid string) svc.PostCreateMsg {
	return f.msg
}
```

Add test:

```go
func TestCreatePostLogic_UsesDTMFeedFanoutBranch(t *testing.T) {
	msg := &fakePostCreateMsg{}
	pm := new(MockPostModel)
	ptm := new(MockPostTagModel)
	pm.On("InsertPostTx", mock.Anything, mock.Anything, mock.AnythingOfType("*model.Post")).Return(nil).Once()
	ptm.On("BatchInsertTagsByPostIdTx", mock.Anything, mock.Anything, mock.AnythingOfType("int64"), mock.Anything, mock.Anything).Return(nil).Once()
	svcCtx := newUnitSvcCtx(pm, nil, nil, ptm)
	svcCtx.Config.FeedBusiServer = "feed:9091"
	svcCtx.PostCreateMsgFactory = fakePostCreateMsgFactory{msg: msg}
	logic := NewCreatePostLogic(context.Background(), svcCtx)

	resp, err := logic.CreatePost(&pb.CreatePostReq{AuthorId: 9, Title: "t", Content: "content"})

	require.NoError(t, err)
	require.NotZero(t, resp.PostId)
	require.True(t, msg.didSubmit)
	require.Equal(t, []string{"feed:9091/feed.FeedService/FanoutPost"}, msg.actions)
}
```

- [ ] **Step 2: Run test and verify RED**

Run: `GOCACHE=/tmp/go-build go test ./app/content/internal/logic -run TestCreatePostLogic_UsesDTMFeedFanoutBranch -count=1`

Expected: compile failure for missing `PostCreateMsgFactory`, `InsertPostTx`, and DTM config fields.

- [ ] **Step 3: Add content config and DTM interfaces**

In `app/content/internal/config/config.go`, add:

```go
DtmServer         string
ContentBusiServer string
FeedBusiServer    string
```

In `app/content/etc/content.yaml`, add:

```yaml
DtmServer: "${DTM_GRPC_SERVER}"
ContentBusiServer: "${CONTENT_BUSI_SERVER}"
FeedBusiServer: "${FEED_BUSI_SERVER}"
```

In `app/content/internal/svc/service_context.go`, add fields:

```go
DB                   *sql.DB
PostCreateMsgFactory PostCreateMsgFactory
```

Initialize with one raw DB:

```go
db, err := sql.Open("mysql", c.DataSource)
if err != nil {
	panic(fmt.Sprintf("数据库连接失败: %v", err))
}
conn := sqlx.NewSqlConnFromDB(db)
```

Create `app/content/internal/svc/dtm.go`:

```go
package svc

import (
	"database/sql"

	"github.com/dtm-labs/dtm/client/dtmgrpc"
	"github.com/google/uuid"
	"google.golang.org/protobuf/proto"
)

type PostCreateMsg interface {
	Add(action string, payload proto.Message)
	DoAndSubmitDB(queryPrepared string, fn func(*sql.Tx) error) error
}

type PostCreateMsgFactory interface {
	NewGID() string
	NewPostCreateMsg(gid string) PostCreateMsg
}

type DTMPostCreateMsgFactory struct {
	DtmServer string
	DB        *sql.DB
}

func (f DTMPostCreateMsgFactory) NewGID() string {
	return uuid.NewString()
}

func (f DTMPostCreateMsgFactory) NewPostCreateMsg(gid string) PostCreateMsg {
	return dtmPostCreateMsg{msg: dtmgrpc.NewMsgGrpc(f.DtmServer, gid), db: f.DB}
}

type dtmPostCreateMsg struct {
	msg *dtmgrpc.MsgGrpc
	db  *sql.DB
}

func (m dtmPostCreateMsg) Add(action string, payload proto.Message) {
	m.msg.Add(action, payload)
}

func (m dtmPostCreateMsg) DoAndSubmitDB(queryPrepared string, fn func(*sql.Tx) error) error {
	return m.msg.DoAndSubmitDB(queryPrepared, m.db, func(tx *sql.Tx) error {
		return fn(tx)
	})
}
```

If the DTM SDK signature differs at compile time, adjust only this adapter, not business logic.

- [ ] **Step 4: Add transactional model methods**

Add to `PostModel` interface and `customPostModel`:

```go
InsertPostTx(ctx context.Context, tx *sql.Tx, post *Post) error
```

The implementation must use the same columns as `InsertPost`, but call `tx.ExecContext(ctx, query, ...)`.

Add to `PostTagModel` interface and `customPostTagModel`:

```go
BatchInsertTagsByPostIdTx(ctx context.Context, tx *sql.Tx, postId int64, tags []string, ids []int64) error
```

The implementation must insert all tag rows through `tx.ExecContext`. Empty `tags` returns nil.

- [ ] **Step 5: Modify CreatePost to use DTM branch**

In `CreatePost`, build the branch URL:

```go
fanoutAction := l.svcCtx.Config.FeedBusiServer + "/feed.FeedService/FanoutPost"
queryPrepared := l.svcCtx.Config.ContentBusiServer + "/content.ContentService/QueryPrepared"
```

Use the DTM factory when configured:

```go
factory := l.svcCtx.PostCreateMsgFactory
if factory == nil {
	return nil, errx.NewWithCode(errx.SystemError)
}
gid := factory.NewGID()
msg := factory.NewPostCreateMsg(gid)
createdAt := time.Now().UnixMilli()
msg.Add(fanoutAction, &feedpb.FanoutPostReq{AuthorId: in.AuthorId, PostId: id, CreatedAt: createdAt})
err = msg.DoAndSubmitDB(queryPrepared, func(tx *sql.Tx) error {
	if err := l.svcCtx.PostModel.InsertPostTx(l.ctx, tx, post); err != nil {
		return err
	}
	return l.svcCtx.PostTagModel.BatchInsertTagsByPostIdTx(l.ctx, tx, id, validTags, tagIds)
})
```

The real adapter passes `*sql.Tx` to model methods. Unit fakes pass nil and mocks assert that the method was called.

- [ ] **Step 6: Implement QueryPrepared**

In `query_prepared_logic.go`, use DTM barrier query prepared with the content raw DB:

```go
func (l *QueryPreparedLogic) QueryPrepared(in *pb.QueryPreparedReq) (*pb.QueryPreparedResp, error) {
	barrier, err := dtmgrpc.BarrierFromGrpc(l.ctx)
	if err != nil {
		l.Errorw("DTM BarrierFromGrpc failed", logx.Field("err", err.Error()))
		return nil, errx.NewWithCode(errx.SystemError)
	}
	if err := barrier.QueryPrepared(l.svcCtx.DB); err != nil {
		l.Errorw("DTM QueryPrepared failed", logx.Field("err", err.Error()))
		return nil, errx.NewWithCode(errx.SystemError)
	}
	return &pb.QueryPreparedResp{}, nil
}
```

- [ ] **Step 7: Run content tests and verify GREEN**

Run: `GOCACHE=/tmp/go-build go test ./app/content/internal/logic ./app/content/internal/model -run 'TestCreatePostLogic|TestQueryPrepared' -count=1`

Expected: PASS.

- [ ] **Step 8: Commit content changes**

Run:

```bash
git add app/content proto/content/content.proto
git commit -m "feat: wrap post creation in dtm message"
```

Do not add ignored docs.

### Task 4: Feed Fanout Branch

**Files:**
- Create: `app/feed/internal/fanout/post_fanout.go`
- Create: `app/feed/internal/fanout/post_fanout_test.go`
- Modify: `app/feed/internal/mqs/post_publish_consumer.go`
- Modify: `app/feed/internal/mqs/post_publish_consumer_test.go`
- Modify: `app/feed/internal/logic/fanout_post_logic.go`

- [ ] **Step 1: Write failing fanout package test**

Create `app/feed/internal/fanout/post_fanout_test.go` with mocks equivalent to existing MQ consumer tests and assert:

```go
func TestHandlePostPublished_SmallVFanout(t *testing.T) {
	// author follower_count below threshold writes outbox and inbox rows
}

func TestHandlePostPublished_BigVOutboxOnly(t *testing.T) {
	// author follower_count at threshold writes only outbox
}
```

- [ ] **Step 2: Run test and verify RED**

Run: `GOCACHE=/tmp/go-build go test ./app/feed/internal/fanout -run TestHandlePostPublished -count=1`

Expected: FAIL because package/function does not exist.

- [ ] **Step 3: Create shared fanout package**

Create `app/feed/internal/fanout/post_fanout.go`:

```go
package fanout

import (
	"context"

	"esx/app/feed/internal/model"
	"esx/app/feed/internal/svc"
	"user/userservice"
)

type PostPublished struct {
	PostId    int64
	AuthorId  int64
	CreatedAt int64
}

func HandlePostPublished(ctx context.Context, svcCtx *svc.ServiceContext, event PostPublished) (int64, error) {
	userResp, err := svcCtx.UserService.GetUser(ctx, &userservice.GetUserReq{UserId: event.AuthorId})
	if err != nil {
		return 0, err
	}
	if err := svcCtx.OutboxModel.InsertIgnore(ctx, &model.FeedOutbox{AuthorId: event.AuthorId, PostId: event.PostId, CreatedAt: event.CreatedAt}); err != nil {
		return 0, err
	}
	if userResp.User == nil || userResp.User.FollowerCount >= svcCtx.BigVThreshold {
		return 0, nil
	}
	pageSize := int32(svcCtx.FanoutBatchSize)
	if pageSize <= 0 {
		pageSize = 500
	}
	followersResp, err := svcCtx.UserService.GetFollowers(ctx, &userservice.GetFollowersReq{UserId: event.AuthorId, Page: 1, PageSize: pageSize})
	if err != nil {
		return 0, err
	}
	rows := make([]*model.FeedInbox, 0, len(followersResp.Users))
	for _, user := range followersResp.Users {
		if user.Id > 0 {
			rows = append(rows, &model.FeedInbox{UserId: user.Id, AuthorId: event.AuthorId, PostId: event.PostId, CreatedAt: event.CreatedAt})
		}
	}
	return svcCtx.InboxModel.BatchInsertIgnore(ctx, rows)
}
```

- [ ] **Step 4: Modify MQ consumer to use shared package**

In `app/feed/internal/mqs/post_publish_consumer.go`, replace inline `handlePostPublished` body with a call to `fanout.HandlePostPublished`.

- [ ] **Step 5: Implement FanoutPost logic**

In `app/feed/internal/logic/fanout_post_logic.go`:

```go
func (l *FanoutPostLogic) FanoutPost(in *pb.FanoutPostReq) (*pb.FanoutPostResp, error) {
	if in.AuthorId <= 0 || in.PostId <= 0 || in.CreatedAt <= 0 {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	pushed, err := fanout.HandlePostPublished(l.ctx, l.svcCtx, fanout.PostPublished{
		AuthorId: in.AuthorId,
		PostId: in.PostId,
		CreatedAt: in.CreatedAt,
	})
	if err != nil {
		l.Errorw("FanoutPost failed", logx.Field("err", err.Error()))
		return nil, errx.NewWithCode(errx.SystemError)
	}
	return &pb.FanoutPostResp{PushedCount: pushed}, nil
}
```

- [ ] **Step 6: Run feed tests and verify GREEN**

Run: `GOCACHE=/tmp/go-build go test ./app/feed/internal/fanout ./app/feed/internal/mqs ./app/feed/internal/logic -run 'TestHandlePostPublished|TestFanoutPostLogic' -count=1`

Expected: PASS.

- [ ] **Step 7: Commit feed changes**

Run:

```bash
git add app/feed proto/feed/feed.proto
git commit -m "feat: add dtm feed fanout branch"
```

Do not add ignored docs.

### Task 5: Interaction Like Local Transaction

**Files:**
- Modify: `app/interaction/internal/svc/service_context.go`
- Modify: `app/interaction/internal/model/like_record_model.go`
- Modify: `app/interaction/internal/model/action_count_model.go`
- Modify: `app/interaction/internal/logic/like_logic.go`
- Modify: `app/interaction/internal/logic/like_logic_test.go`
- Optional integration test: `app/interaction/internal/logic/interaction_integration_test.go`

- [ ] **Step 1: Write failing rollback test for local consistency**

Add a focused test that proves a counter failure does not leave an active like. Prefer a MySQL-backed integration test because this is transaction behavior. If using the existing interaction integration harness, add this case:

```go
func TestLikeLogic_Like_CountFailureRollsBackLikeState(t *testing.T) {
	ctx := context.Background()
	conn := sqlx.NewSqlConnFromDB(testDB)
	svcCtx := &svc.ServiceContext{
		Conn:             conn,
		LikeRecordModel:  model.NewLikeRecordModel(conn, cache.CacheConf{}),
		ActionCountModel: model.NewActionCountModel(conn),
	}

	_, err := testDB.Exec("DROP TABLE action_count")
	require.NoError(t, err)
	t.Cleanup(ensureActionCountTable)

	logic := NewLikeLogic(ctx, svcCtx)
	_, err = logic.Like(&pb.LikeReq{UserId: 1, TargetId: 100, TargetType: 1})
	require.Error(t, err)
	require.True(t, errx.Is(err, errx.SystemError))

	var count int
	err = testDB.QueryRow("SELECT COUNT(*) FROM like_record WHERE user_id=? AND target_id=? AND target_type=? AND status=?",
		1, 100, 1, model.StatusActive).Scan(&count)
	require.NoError(t, err)
	require.Zero(t, count)
}
```

- [ ] **Step 2: Run test and verify RED**

Run: `GOCACHE=/tmp/go-build go test ./app/interaction/internal/logic -run TestLikeLogic_Like_CountFailureRollsBackLikeState -count=1`

Expected: FAIL because current `Like` writes `like_record`, logs count failures, and returns success.

- [ ] **Step 3: Expose interaction DB connection**

Modify `app/interaction/internal/svc/service_context.go`:

```go
type ServiceContext struct {
	Config              config.Config
	Conn                sqlx.SqlConn
	FavoriteFolderModel model.FavoriteFolderModel
	FavoriteModel       model.FavoriteModel
	LikeRecordModel     model.LikeRecordModel
	ReportModel         model.ReportModel
	ViewHistoryModel    model.ViewHistoryModel
	ActionCountModel    model.ActionCountModel
	Redis               *redis.Redis
	RedisStore          RedisStore
	SingleFlight        singleflight.Group
}
```

Initialize the field with the existing `conn`:

```go
return &ServiceContext{
	Config:              c,
	Conn:                conn,
	FavoriteFolderModel: model.NewFavoriteFolderModel(conn, conf),
	FavoriteModel:       model.NewFavoriteModel(conn, conf),
	LikeRecordModel:     model.NewLikeRecordModel(conn, conf),
	ReportModel:         model.NewReportModel(conn, conf),
	ViewHistoryModel:    model.NewViewHistoryModel(conn, conf),
	ActionCountModel:    model.NewActionCountModel(conn),
	Redis:               redisClient,
	RedisStore:          NewRedisStore(redisClient),
}
```

- [ ] **Step 4: Add transaction-aware model helpers**

Extend `LikeRecordModel` in `app/interaction/internal/model/like_record_model.go`:

```go
UpsertLikeStatusTx(ctx context.Context, conn sqlx.SqlConn, userId, targetId, targetType, status int64) (sql.Result, int64, error)
InvalidateLikeRecordCache(ctx context.Context, id, userId, targetId, targetType int64) error
```

Implement:

```go
func (m *customLikeRecordModel) UpsertLikeStatusTx(ctx context.Context, conn sqlx.SqlConn, userId, targetId, targetType, status int64) (sql.Result, int64, error) {
	query := fmt.Sprintf(
		"insert into %s (`user_id`,`target_id`,`target_type`,`status`) values (?,?,?,?) on duplicate key update `id`=last_insert_id(`id`), `status`=values(`status`)",
		m.table,
	)
	result, err := conn.ExecCtx(ctx, query, userId, targetId, targetType, status)
	if err != nil {
		return nil, 0, err
	}
	id, err := result.LastInsertId()
	if err != nil {
		return nil, 0, err
	}
	return result, id, nil
}

func (m *customLikeRecordModel) InvalidateLikeRecordCache(ctx context.Context, id, userId, targetId, targetType int64) error {
	keys := []string{fmt.Sprintf("%s%v:%v:%v", cacheLikeRecordUserIdTargetIdTargetTypePrefix, userId, targetId, targetType)}
	if id > 0 {
		keys = append(keys, fmt.Sprintf("%s%v", cacheLikeRecordIdPrefix, id))
	}
	return m.DelCacheCtx(ctx, keys...)
}
```

Extend `ActionCountModel` in `app/interaction/internal/model/action_count_model.go`:

```go
IncrLikeCountTx(ctx context.Context, conn sqlx.SqlConn, targetID, targetType int64) error
```

Implement it with the same atomic SQL as `IncrLikeCount`, but execute through the transaction connection:

```go
func (m *customActionCountModel) IncrLikeCountTx(ctx context.Context, conn sqlx.SqlConn, targetID, targetType int64) error {
	query := fmt.Sprintf("insert into %s (`target_id`, `target_type`, `like_count`, `favorite_count`, `comment_count`, `share_count`) values (?, ?, 1, 0, 0, 0) on duplicate key update `like_count` = `like_count` + 1", m.table)
	_, err := conn.ExecCtx(ctx, query, targetID, targetType)
	return err
}
```

- [ ] **Step 5: Modify Like logic to use a local transaction**

In `app/interaction/internal/logic/like_logic.go`, keep validation unchanged and replace the current two independent writes with:

```go
var likeRecordID int64
err := l.svcCtx.Conn.TransactCtx(l.ctx, func(ctx context.Context, session sqlx.Session) error {
	txConn := sqlx.NewSqlConnFromSession(session)
	result, id, err := l.svcCtx.LikeRecordModel.UpsertLikeStatusTx(ctx, txConn, in.UserId, in.TargetId, int64(in.TargetType), model.StatusActive)
	if err != nil {
		return err
	}
	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return errx.NewWithCode(errx.AlreadyLiked)
	}
	likeRecordID = id
	return l.svcCtx.ActionCountModel.IncrLikeCountTx(ctx, txConn, in.TargetId, int64(in.TargetType))
})
if err != nil {
	if errx.Is(err, errx.AlreadyLiked) {
		return nil, err
	}
	l.Errorw("local like transaction failed",
		logx.Field("userId", in.UserId),
		logx.Field("targetId", in.TargetId),
		logx.Field("err", err.Error()),
	)
	return nil, errx.NewWithCode(errx.SystemError)
}
if err := l.svcCtx.LikeRecordModel.InvalidateLikeRecordCache(l.ctx, likeRecordID, in.UserId, in.TargetId, int64(in.TargetType)); err != nil {
	l.Errorw("InvalidateLikeRecordCache failed", logx.Field("err", err.Error()))
}
```

Do not add DTM config, Saga factories, interaction branch RPCs, or marker tables.

- [ ] **Step 6: Run interaction tests and verify GREEN**

Run: `GOCACHE=/tmp/go-build go test ./app/interaction/internal/logic ./app/interaction/internal/model -run 'TestLikeLogic|TestLikeRecord|TestActionCount' -count=1`

Expected: PASS.

- [ ] **Step 7: Commit local interaction consistency changes**

Run:

```bash
git add app/interaction/internal/svc/service_context.go app/interaction/internal/model/like_record_model.go app/interaction/internal/model/action_count_model.go app/interaction/internal/logic/like_logic.go app/interaction/internal/logic/like_logic_test.go app/interaction/internal/logic/interaction_integration_test.go
git commit -m "fix: keep like count update in local transaction"
```

Do not add ignored docs.

### Task 6: Dependency, Integration, And Verification

**Files:**
- Modify: `go.mod`
- Modify: `go.sum`
- Add or extend integration tests under `app/feed/internal/model` and, if Task 5 is implemented, `app/interaction/internal/logic`

- [ ] **Step 1: Add DTM dependency after plan approval**

Run:

```bash
go get github.com/dtm-labs/dtm@latest
go mod tidy
```

Expected: `go.mod` and `go.sum` include the DTM SDK and its transitive dependencies.

- [ ] **Step 2: Run focused unit tests**

Run:

```bash
GOCACHE=/tmp/go-build go test ./deploy ./app/content/internal/logic ./app/content/internal/model ./app/feed/internal/fanout ./app/feed/internal/mqs ./app/feed/internal/logic ./app/interaction/internal/logic ./app/interaction/internal/model -count=1
```

Expected: PASS.

- [ ] **Step 3: Run service package tests**

Run:

```bash
GOCACHE=/tmp/go-build go test ./app/content/... ./app/feed/... ./app/interaction/... -count=1
```

Expected: PASS.

- [ ] **Step 4: Run race tests for touched services**

Run:

```bash
GOCACHE=/tmp/go-build go test -race ./app/content/... ./app/feed/... ./app/interaction/... -count=1
```

Expected: PASS.

- [ ] **Step 5: Run vet for touched services**

Run:

```bash
GOCACHE=/tmp/go-build go vet ./app/content/... ./app/feed/... ./app/interaction/...
```

Expected: no output.

- [ ] **Step 6: Run changed-scope lint**

Run:

```bash
golangci-lint run ./app/content/... ./app/feed/... ./app/interaction/... ./deploy/...
```

Expected: no new lint errors in touched packages.

- [ ] **Step 7: Commit dependency and verification updates**

Run:

```bash
git add go.mod go.sum app/content app/feed app/interaction deploy
git commit -m "test: verify dtm distributed transaction flows"
```

Do not add ignored docs.
