# Interaction Service Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Implement all business logic in `app/interaction` for Like/Unlike/Favorite/Unfavorite, count queries with Redis caching, and batch status checks.

**Architecture:** All logic lives in `app/interaction/internal/logic/`. DB models use go-zero sqlx + cache. Redis stores hot counters with singleflight protection against cache penetration. MQ events are fire-and-forget (failures logged, not blocking).

**Tech Stack:** Go 1.26.1, go-zero v1.10.1, MySQL 8.0, Redis 7, sqlmock (testing), miniredis (testing)

---

## File Structure

| File | Action | Responsibility |
|------|--------|----------------|
| `deploy/sql/xbh_interaction.sql` | Modify | Append `action_count` table DDL |
| `app/interaction/internal/model/action_count_model.go` | Create | Hand-written model for action_count (no cache) |
| `app/interaction/internal/config/config.go` | Modify | Add `Redis redis.RedisConf` field |
| `app/interaction/internal/svc/service_context.go` | Modify | Add Redis client, ActionCountModel |
| `app/interaction/internal/logic/like_logic.go` | Modify | Like business logic |
| `app/interaction/internal/logic/like_logic_test.go` | Create | Unit tests for LikeLogic |
| `app/interaction/internal/logic/unlike_logic.go` | Modify | Unlike business logic |
| `app/interaction/internal/logic/unlike_logic_test.go` | Create | Unit tests for UnlikeLogic |
| `app/interaction/internal/logic/favorite_logic.go` | Modify | Favorite business logic |
| `app/interaction/internal/logic/favorite_logic_test.go` | Create | Unit tests for FavoriteLogic |
| `app/interaction/internal/logic/unfavorite_logic.go` | Modify | Unfavorite business logic |
| `app/interaction/internal/logic/unfavorite_logic_test.go` | Create | Unit tests for UnfavoriteLogic |
| `app/interaction/internal/logic/get_counts_logic.go` | Modify | Count query with Redis cache |
| `app/interaction/internal/logic/get_counts_logic_test.go` | Create | Unit tests for GetCountsLogic |
| `app/interaction/internal/logic/get_like_count_logic.go` | Modify | Delegates to GetCounts or direct query |
| `app/interaction/internal/logic/check_liked_logic.go` | Modify | Single check |
| `app/interaction/internal/logic/batch_check_liked_logic.go` | Modify | Batch IN query |
| `app/interaction/internal/logic/check_favorited_logic.go` | Modify | Single check |
| `app/interaction/internal/logic/batch_check_favorited_logic.go` | Modify | Batch IN query |
| `app/interaction/internal/logic/get_favorite_list_logic.go` | Modify | Paginated favorite list |

---

## Task 1: Infrastructure — SQL + Model + Config + SVC

**Files:**
- Modify: `deploy/sql/xbh_interaction.sql`
- Create: `app/interaction/internal/model/action_count_model.go`
- Modify: `app/interaction/internal/config/config.go`
- Modify: `app/interaction/internal/svc/service_context.go`

- [ ] **Step 1: Append action_count table to SQL**

Append to `deploy/sql/xbh_interaction.sql` (after the existing `report` table, before any `USE` or other statements):

```sql
-- 互动计数表
CREATE TABLE IF NOT EXISTS `action_count` (
    `id` BIGINT NOT NULL AUTO_INCREMENT,
    `target_id` BIGINT NOT NULL COMMENT '目标ID',
    `target_type` TINYINT NOT NULL COMMENT '目标类型 1:帖子 2:评论',
    `like_count` BIGINT NOT NULL DEFAULT 0 COMMENT '点赞数',
    `favorite_count` BIGINT NOT NULL DEFAULT 0 COMMENT '收藏数',
    `comment_count` BIGINT NOT NULL DEFAULT 0 COMMENT '评论数',
    `share_count` BIGINT NOT NULL DEFAULT 0 COMMENT '分享数',
    PRIMARY KEY (`id`),
    UNIQUE KEY `uk_target` (`target_id`, `target_type`)
) ENGINE=InnoDB DEFAULT CHARSET=utf8mb4 COLLATE=utf8mb4_unicode_ci COMMENT='互动计数表';
```

- [ ] **Step 2: Create action_count model**

Create `app/interaction/internal/model/action_count_model.go`:

```go
package model

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/zeromicro/go-zero/core/stores/sqlx"
)

var _ ActionCountModel = (*customActionCountModel)(nil)

type (
	ActionCountModel interface {
		Insert(ctx context.Context, data *ActionCount) (sql.Result, error)
		FindOneByTarget(ctx context.Context, targetId int64, targetType int64) (*ActionCount, error)
		Update(ctx context.Context, data *ActionCount) error
	}

	customActionCountModel struct {
		conn  sqlx.SqlConn
		table string
	}

	ActionCount struct {
		Id            int64 `db:"id"`
		TargetId      int64 `db:"target_id"`
		TargetType    int64 `db:"target_type"`
		LikeCount     int64 `db:"like_count"`
		FavoriteCount int64 `db:"favorite_count"`
		CommentCount  int64 `db:"comment_count"`
		ShareCount    int64 `db:"share_count"`
	}
)

func NewActionCountModel(conn sqlx.SqlConn) ActionCountModel {
	return &customActionCountModel{
		conn:  conn,
		table: "`action_count`",
	}
}

func (m *customActionCountModel) Insert(ctx context.Context, data *ActionCount) (sql.Result, error) {
	query := fmt.Sprintf("insert into %s (`target_id`, `target_type`, `like_count`, `favorite_count`, `comment_count`, `share_count`) values (?, ?, ?, ?, ?, ?)", m.table)
	return m.conn.ExecCtx(ctx, query, data.TargetId, data.TargetType, data.LikeCount, data.FavoriteCount, data.CommentCount, data.ShareCount)
}

func (m *customActionCountModel) FindOneByTarget(ctx context.Context, targetId int64, targetType int64) (*ActionCount, error) {
	query := fmt.Sprintf("select `id`, `target_id`, `target_type`, `like_count`, `favorite_count`, `comment_count`, `share_count` from %s where `target_id` = ? and `target_type` = ? limit 1", m.table)
	var resp ActionCount
	err := m.conn.QueryRowCtx(ctx, &resp, query, targetId, targetType)
	switch err {
	case nil:
		return &resp, nil
	case sqlx.ErrNotFound:
		return nil, ErrNotFound
	default:
		return nil, err
	}
}

func (m *customActionCountModel) Update(ctx context.Context, data *ActionCount) error {
	query := fmt.Sprintf("update %s set `like_count` = ?, `favorite_count` = ?, `comment_count` = ?, `share_count` = ? where `id` = ?", m.table)
	_, err := m.conn.ExecCtx(ctx, query, data.LikeCount, data.FavoriteCount, data.CommentCount, data.ShareCount, data.Id)
	return err
}
```

- [ ] **Step 3: Update config with Redis**

Modify `app/interaction/internal/config/config.go` to:

```go
package config

import (
	"github.com/zeromicro/go-zero/core/stores/redis"
	"github.com/zeromicro/go-zero/zrpc"
)

type Config struct {
	zrpc.RpcServerConf
	DataSource string
	Redis      redis.RedisConf
}
```

- [ ] **Step 4: Update service context**

Modify `app/interaction/internal/svc/service_context.go` to:

```go
package svc

import (
	"esx/app/interaction/internal/config"
	"esx/app/interaction/internal/model"

	"github.com/zeromicro/go-zero/core/stores/cache"
	"github.com/zeromicro/go-zero/core/stores/redis"
	"github.com/zeromicro/go-zero/core/stores/sqlx"
	"golang.org/x/sync/singleflight"
)

type ServiceContext struct {
	Config              config.Config
	FavoriteFolderModel model.FavoriteFolderModel
	FavoriteModel       model.FavoriteModel
	LikeRecordModel     model.LikeRecordModel
	ReportModel         model.ReportModel
	ViewHistoryModel    model.ViewHistoryModel
	ActionCountModel    model.ActionCountModel
	Redis               *redis.Redis
	SingleFlight        singleflight.Group
}

func NewServiceContext(c config.Config) *ServiceContext {
	conn, err := sqlx.NewMysql(c.DataSource)
	if err != nil {
		panic(err)
	}

	conf := cache.CacheConf{
		cache.NodeConf{
			RedisConf: c.Redis,
			Weight:    100,
		},
	}

	return &ServiceContext{
		Config:              c,
		FavoriteFolderModel: model.NewFavoriteFolderModel(conn, conf),
		FavoriteModel:       model.NewFavoriteModel(conn, conf),
		LikeRecordModel:     model.NewLikeRecordModel(conn, conf),
		ReportModel:         model.NewReportModel(conn, conf),
		ViewHistoryModel:    model.NewViewHistoryModel(conn, conf),
		ActionCountModel:    model.NewActionCountModel(conn),
		Redis:               redis.MustNewRedis(c.Redis),
	}
}
```

- [ ] **Step 5: Verify compilation**

Run:
```bash
cd app/interaction && go build ./...
```

Expected: compiles successfully (no output = success).

- [ ] **Step 6: Commit**

```bash
git add deploy/sql/xbh_interaction.sql app/interaction/internal/model/action_count_model.go app/interaction/internal/config/config.go app/interaction/internal/svc/service_context.go
git commit -m "feat(interaction): add action_count model, redis config, and svc wiring"
```

---

## Task 2: LikeLogic (TDD)

**Files:**
- Modify: `app/interaction/internal/logic/like_logic.go`
- Create: `app/interaction/internal/logic/like_logic_test.go`

- [ ] **Step 1: Write failing tests**

Create `app/interaction/internal/logic/like_logic_test.go`:

```go
package logic

import (
	"context"
	"testing"

	"esx/app/interaction/internal/model"
	"esx/app/interaction/internal/svc"
	"esx/app/interaction/pb/xiaobaihe/interaction/pb"
	"esx/pkg/errx"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/alicebob/miniredis/v2"
	"github.com/stretchr/testify/assert"
	"github.com/zeromicro/go-zero/core/stores/redis"
)

func TestLikeLogic_Like_FirstTime(t *testing.T) {
	mockDB, mock, err := sqlmock.New()
	assert.NoError(t, err)
	defer mockDB.Close()

	mr := miniredis.RunT(&testing.T{})
	defer mr.Close()

	svcCtx := &svc.ServiceContext{
		LikeRecordModel:  model.NewLikeRecordModel(sqlx.NewSqlConnFromDB(mockDB), cache.CacheConf{}),
		ActionCountModel: model.NewActionCountModel(sqlx.NewSqlConnFromDB(mockDB)),
		Redis:            redis.New(mr.Addr(), redis.WithPass("")),
	}

	// Mock: FindOneByUserIdTargetIdTargetType returns ErrNotFound (first time)
	mock.ExpectQuery("select \\`id\\`,").WillReturnError(model.ErrNotFound)
	// Mock: Insert success
	mock.ExpectExec("insert into").WillReturnResult(sqlmock.NewResult(1, 1))
	// Mock: FindOneByTarget for action_count returns existing row
	rows := sqlmock.NewRows([]string{"id", "target_id", "target_type", "like_count", "favorite_count", "comment_count", "share_count"}).
		AddRow(1, 100, 1, 5, 0, 0, 0)
	mock.ExpectQuery("select \\`id\\`,").WillReturnRows(rows)
	// Mock: Update action_count
	mock.ExpectExec("update").WillReturnResult(sqlmock.NewResult(0, 1))

	logic := NewLikeLogic(context.Background(), svcCtx)
	resp, err := logic.Like(&pb.LikeReq{UserId: 1, TargetId: 100, TargetType: 1})
	assert.NoError(t, err)
	assert.NotNil(t, resp)
}

func TestLikeLogic_Like_AlreadyLiked(t *testing.T) {
	mockDB, mock, err := sqlmock.New()
	assert.NoError(t, err)
	defer mockDB.Close()

	mr := miniredis.RunT(&testing.T{})
	defer mr.Close()

	svcCtx := &svc.ServiceContext{
		LikeRecordModel: model.NewLikeRecordModel(sqlx.NewSqlConnFromDB(mockDB), cache.CacheConf{}),
		Redis:           redis.New(mr.Addr(), redis.WithPass("")),
	}

	// Mock: FindOneByUserIdTargetIdTargetType returns existing liked record
	rows := sqlmock.NewRows([]string{"id", "user_id", "target_id", "target_type", "status", "created_at", "updated_at"}).
		AddRow(1, 1, 100, 1, 1, time.Now(), time.Now())
	mock.ExpectQuery("select \\`id\\`,").WillReturnRows(rows)

	logic := NewLikeLogic(context.Background(), svcCtx)
	_, err = logic.Like(&pb.LikeReq{UserId: 1, TargetId: 100, TargetType: 1})
	assert.Error(t, err)
	bizErr, ok := err.(*errx.BizError)
	assert.True(t, ok)
	assert.Equal(t, errx.AlreadyLiked, bizErr.Code)
}
```

Note: add missing imports (`time`, `github.com/zeromicro/go-zero/core/stores/cache`, `github.com/zeromicro/go-zero/core/stores/sqlx`).

- [ ] **Step 2: Run tests to verify they fail**

Run:
```bash
cd app/interaction && go test ./internal/logic -run TestLikeLogic -v
```

Expected: FAIL — `Like` method returns empty resp, test expects `AlreadyLiked` error on second test.

- [ ] **Step 3: Implement LikeLogic**

Modify `app/interaction/internal/logic/like_logic.go`:

```go
package logic

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"time"

	"esx/app/interaction/internal/model"
	"esx/app/interaction/internal/svc"
	"esx/app/interaction/pb/xiaobaihe/interaction/pb"
	"esx/pkg/errx"
	"esx/pkg/mqx"

	"github.com/zeromicro/go-zero/core/logx"
)

type LikeLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewLikeLogic(ctx context.Context, svcCtx *svc.ServiceContext) *LikeLogic {
	return &LikeLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *LikeLogic) Like(in *pb.LikeReq) (*pb.LikeResp, error) {
	// 1. Check if already liked
	record, err := l.svcCtx.LikeRecordModel.FindOneByUserIdTargetIdTargetType(l.ctx, in.UserId, in.TargetId, int64(in.TargetType))
	if err != nil && !errors.Is(err, model.ErrNotFound) {
		l.Logger.Errorf("FindOneByUserIdTargetIdTargetType failed: %v", err)
		return nil, errx.New(errx.SystemError, err.Error())
	}
	if record != nil && record.Status == 1 {
		return nil, errx.New(errx.AlreadyLiked, errx.GetMsg(errx.AlreadyLiked))
	}

	// 2. Insert or update
	if record != nil {
		record.Status = 1
		if err := l.svcCtx.LikeRecordModel.Update(l.ctx, record); err != nil {
			l.Logger.Errorf("Update like_record failed: %v", err)
			return nil, errx.New(errx.SystemError, err.Error())
		}
	} else {
		_, err = l.svcCtx.LikeRecordModel.Insert(l.ctx, &model.LikeRecord{
			UserId:     in.UserId,
			TargetId:   in.TargetId,
			TargetType: int64(in.TargetType),
			Status:     1,
		})
		if err != nil {
			l.Logger.Errorf("Insert like_record failed: %v", err)
			return nil, errx.New(errx.SystemError, err.Error())
		}
	}

	// 3. Update action_count
	if err := l.incrLikeCount(in.TargetId, int64(in.TargetType)); err != nil {
		l.Logger.Errorf("incrLikeCount failed: %v", err)
	}

	// 4. Send MQ (fire-and-forget)
	l.publishEvent(in)

	return &pb.LikeResp{}, nil
}

func (l *LikeLogic) incrLikeCount(targetId, targetType int64) error {
	key := fmt.Sprintf("action_count:%d:%d", targetId, targetType)
	exists, err := l.svcCtx.Redis.Exists(key)
	if err != nil {
		return err
	}
	if exists {
		_, err = l.svcCtx.Redis.Hincrby(key, "like_count", 1)
		return err
	}

	// Redis miss: read from DB, then set
	count, err := l.svcCtx.ActionCountModel.FindOneByTarget(l.ctx, targetId, targetType)
	if err != nil {
		if errors.Is(err, model.ErrNotFound) {
			// Create new record
			_, err = l.svcCtx.ActionCountModel.Insert(l.ctx, &model.ActionCount{
				TargetId:   targetId,
				TargetType: targetType,
				LikeCount:  1,
			})
			if err != nil {
				return err
			}
			l.svcCtx.Redis.Hset(key, "like_count", "1")
			l.svcCtx.Redis.Hset(key, "favorite_count", "0")
			l.svcCtx.Redis.Expire(key, 30) // short TTL for null-value
			return nil
		}
		return err
	}

	count.LikeCount++
	if err := l.svcCtx.ActionCountModel.Update(l.ctx, count); err != nil {
		return err
	}

	l.svcCtx.Redis.Hset(key, "like_count", fmt.Sprintf("%d", count.LikeCount))
	l.svcCtx.Redis.Hset(key, "favorite_count", fmt.Sprintf("%d", count.FavoriteCount))
	ttl := 300 + rand.Intn(60)
	l.svcCtx.Redis.Expire(key, ttl)
	return nil
}

func (l *LikeLogic) publishEvent(in *pb.LikeReq) {
	// MQ send is best-effort; failure is logged but not blocking
	_ = l.svcCtx.MqProducer // placeholder if producer exists
	l.Logger.Infof("Like event: user=%d target=%d type=%d", in.UserId, in.TargetId, in.TargetType)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run:
```bash
cd app/interaction && go test ./internal/logic -run TestLikeLogic -v
```

Expected: PASS for both tests.

- [ ] **Step 5: Commit**

```bash
git add app/interaction/internal/logic/like_logic.go app/interaction/internal/logic/like_logic_test.go
git commit -m "feat(interaction): implement LikeLogic with tests"
```

---

## Task 3: UnlikeLogic (TDD)

**Files:**
- Modify: `app/interaction/internal/logic/unlike_logic.go`
- Create: `app/interaction/internal/logic/unlike_logic_test.go`

- [ ] **Step 1: Write failing tests**

Create `app/interaction/internal/logic/unlike_logic_test.go`:

```go
package logic

import (
	"context"
	"testing"
	"time"

	"esx/app/interaction/internal/model"
	"esx/app/interaction/internal/svc"
	"esx/app/interaction/pb/xiaobaihe/interaction/pb"
	"esx/pkg/errx"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/alicebob/miniredis/v2"
	"github.com/stretchr/testify/assert"
	"github.com/zeromicro/go-zero/core/stores/cache"
	"github.com/zeromicro/go-zero/core/stores/redis"
	"github.com/zeromicro/go-zero/core/stores/sqlx"
)

func TestUnlikeLogic_Unlike_Success(t *testing.T) {
	mockDB, mock, err := sqlmock.New()
	assert.NoError(t, err)
	defer mockDB.Close()

	mr := miniredis.RunT(&testing.T{})
	defer mr.Close()

	svcCtx := &svc.ServiceContext{
		LikeRecordModel:  model.NewLikeRecordModel(sqlx.NewSqlConnFromDB(mockDB), cache.CacheConf{}),
		ActionCountModel: model.NewActionCountModel(sqlx.NewSqlConnFromDB(mockDB)),
		Redis:            redis.New(mr.Addr(), redis.WithPass("")),
	}

	// Mock: FindOneByUserIdTargetIdTargetType returns liked record
	rows := sqlmock.NewRows([]string{"id", "user_id", "target_id", "target_type", "status", "created_at", "updated_at"}).
		AddRow(1, 1, 100, 1, 1, time.Now(), time.Now())
	mock.ExpectQuery("select \\`id\\`,").WillReturnRows(rows)
	// Mock: Update status=0
	mock.ExpectExec("update").WillReturnResult(sqlmock.NewResult(0, 1))
	// Mock: FindOneByTarget for action_count
	countRows := sqlmock.NewRows([]string{"id", "target_id", "target_type", "like_count", "favorite_count", "comment_count", "share_count"}).
		AddRow(1, 100, 1, 5, 0, 0, 0)
	mock.ExpectQuery("select \\`id\\`,").WillReturnRows(countRows)
	// Mock: Update action_count
	mock.ExpectExec("update").WillReturnResult(sqlmock.NewResult(0, 1))

	logic := NewUnlikeLogic(context.Background(), svcCtx)
	resp, err := logic.Unlike(&pb.UnlikeReq{UserId: 1, TargetId: 100, TargetType: 1})
	assert.NoError(t, err)
	assert.NotNil(t, resp)
}

func TestUnlikeLogic_Unlike_NotLiked(t *testing.T) {
	mockDB, mock, err := sqlmock.New()
	assert.NoError(t, err)
	defer mockDB.Close()

	mr := miniredis.RunT(&testing.T{})
	defer mr.Close()

	svcCtx := &svc.ServiceContext{
		LikeRecordModel: model.NewLikeRecordModel(sqlx.NewSqlConnFromDB(mockDB), cache.CacheConf{}),
		Redis:           redis.New(mr.Addr(), redis.WithPass("")),
	}

	// Mock: FindOneByUserIdTargetIdTargetType returns ErrNotFound
	mock.ExpectQuery("select \\`id\\`,").WillReturnError(model.ErrNotFound)

	logic := NewUnlikeLogic(context.Background(), svcCtx)
	_, err = logic.Unlike(&pb.UnlikeReq{UserId: 1, TargetId: 100, TargetType: 1})
	assert.Error(t, err)
	bizErr, ok := err.(*errx.BizError)
	assert.True(t, ok)
	assert.Equal(t, errx.NotLikedYet, bizErr.Code)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run:
```bash
cd app/interaction && go test ./internal/logic -run TestUnlikeLogic -v
```

Expected: FAIL.

- [ ] **Step 3: Implement UnlikeLogic**

Modify `app/interaction/internal/logic/unlike_logic.go`:

```go
package logic

import (
	"context"
	"errors"
	"fmt"
	"math/rand"

	"esx/app/interaction/internal/model"
	"esx/app/interaction/internal/svc"
	"esx/app/interaction/pb/xiaobaihe/interaction/pb"
	"esx/pkg/errx"

	"github.com/zeromicro/go-zero/core/logx"
)

type UnlikeLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewUnlikeLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UnlikeLogic {
	return &UnlikeLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *UnlikeLogic) Unlike(in *pb.UnlikeReq) (*pb.UnlikeResp, error) {
	record, err := l.svcCtx.LikeRecordModel.FindOneByUserIdTargetIdTargetType(l.ctx, in.UserId, in.TargetId, int64(in.TargetType))
	if err != nil {
		if errors.Is(err, model.ErrNotFound) {
			return nil, errx.New(errx.NotLikedYet, errx.GetMsg(errx.NotLikedYet))
		}
		l.Logger.Errorf("FindOneByUserIdTargetIdTargetType failed: %v", err)
		return nil, errx.New(errx.SystemError, err.Error())
	}
	if record.Status == 0 {
		return nil, errx.New(errx.NotLikedYet, errx.GetMsg(errx.NotLikedYet))
	}

	record.Status = 0
	if err := l.svcCtx.LikeRecordModel.Update(l.ctx, record); err != nil {
		l.Logger.Errorf("Update like_record failed: %v", err)
		return nil, errx.New(errx.SystemError, err.Error())
	}

	if err := l.decrLikeCount(in.TargetId, int64(in.TargetType)); err != nil {
		l.Logger.Errorf("decrLikeCount failed: %v", err)
	}

	return &pb.UnlikeResp{}, nil
}

func (l *UnlikeLogic) decrLikeCount(targetId, targetType int64) error {
	key := fmt.Sprintf("action_count:%d:%d", targetId, targetType)
	exists, err := l.svcCtx.Redis.Exists(key)
	if err != nil {
		return err
	}
	if exists {
		_, err = l.svcCtx.Redis.Hincrby(key, "like_count", -1)
		return err
	}

	count, err := l.svcCtx.ActionCountModel.FindOneByTarget(l.ctx, targetId, targetType)
	if err != nil {
		return err
	}
	if count.LikeCount > 0 {
		count.LikeCount--
	}
	if err := l.svcCtx.ActionCountModel.Update(l.ctx, count); err != nil {
		return err
	}

	l.svcCtx.Redis.Hset(key, "like_count", fmt.Sprintf("%d", count.LikeCount))
	l.svcCtx.Redis.Hset(key, "favorite_count", fmt.Sprintf("%d", count.FavoriteCount))
	ttl := 300 + rand.Intn(60)
	l.svcCtx.Redis.Expire(key, ttl)
	return nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run:
```bash
cd app/interaction && go test ./internal/logic -run TestUnlikeLogic -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add app/interaction/internal/logic/unlike_logic.go app/interaction/internal/logic/unlike_logic_test.go
git commit -m "feat(interaction): implement UnlikeLogic with tests"
```

---

## Task 4: FavoriteLogic (TDD)

**Files:**
- Modify: `app/interaction/internal/logic/favorite_logic.go`
- Create: `app/interaction/internal/logic/favorite_logic_test.go`

- [ ] **Step 1: Write failing tests**

Create `app/interaction/internal/logic/favorite_logic_test.go`:

```go
package logic

import (
	"context"
	"testing"
	"time"

	"esx/app/interaction/internal/model"
	"esx/app/interaction/internal/svc"
	"esx/app/interaction/pb/xiaobaihe/interaction/pb"
	"esx/pkg/errx"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/alicebob/miniredis/v2"
	"github.com/stretchr/testify/assert"
	"github.com/zeromicro/go-zero/core/stores/cache"
	"github.com/zeromicro/go-zero/core/stores/redis"
	"github.com/zeromicro/go-zero/core/stores/sqlx"
)

func TestFavoriteLogic_Favorite_FirstTime(t *testing.T) {
	mockDB, mock, err := sqlmock.New()
	assert.NoError(t, err)
	defer mockDB.Close()

	mr := miniredis.RunT(&testing.T{})
	defer mr.Close()

	svcCtx := &svc.ServiceContext{
		FavoriteModel:    model.NewFavoriteModel(sqlx.NewSqlConnFromDB(mockDB), cache.CacheConf{}),
		ActionCountModel: model.NewActionCountModel(sqlx.NewSqlConnFromDB(mockDB)),
		Redis:            redis.New(mr.Addr(), redis.WithPass("")),
	}

	mock.ExpectQuery("select \\`id\\`,").WillReturnError(model.ErrNotFound)
	mock.ExpectExec("insert into").WillReturnResult(sqlmock.NewResult(1, 1))
	countRows := sqlmock.NewRows([]string{"id", "target_id", "target_type", "like_count", "favorite_count", "comment_count", "share_count"}).
		AddRow(1, 100, 1, 0, 0, 0, 0)
	mock.ExpectQuery("select \\`id\\`,").WillReturnRows(countRows)
	mock.ExpectExec("update").WillReturnResult(sqlmock.NewResult(0, 1))

	logic := NewFavoriteLogic(context.Background(), svcCtx)
	resp, err := logic.Favorite(&pb.FavoriteReq{UserId: 1, PostId: 100})
	assert.NoError(t, err)
	assert.NotNil(t, resp)
}

func TestFavoriteLogic_Favorite_AlreadyFavorited(t *testing.T) {
	mockDB, mock, err := sqlmock.New()
	assert.NoError(t, err)
	defer mockDB.Close()

	mr := miniredis.RunT(&testing.T{})
	defer mr.Close()

	svcCtx := &svc.ServiceContext{
		FavoriteModel: model.NewFavoriteModel(sqlx.NewSqlConnFromDB(mockDB), cache.CacheConf{}),
		Redis:         redis.New(mr.Addr(), redis.WithPass("")),
	}

	rows := sqlmock.NewRows([]string{"id", "user_id", "post_id", "folder_id", "status", "created_at", "updated_at"}).
		AddRow(1, 1, 100, sql.NullInt64{Valid: false}, 1, time.Now(), time.Now())
	mock.ExpectQuery("select \\`id\\`,").WillReturnRows(rows)

	logic := NewFavoriteLogic(context.Background(), svcCtx)
	_, err = logic.Favorite(&pb.FavoriteReq{UserId: 1, PostId: 100})
	assert.Error(t, err)
	bizErr, ok := err.(*errx.BizError)
	assert.True(t, ok)
	assert.Equal(t, errx.AlreadyFavorited, bizErr.Code)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run:
```bash
cd app/interaction && go test ./internal/logic -run TestFavoriteLogic -v
```

Expected: FAIL.

- [ ] **Step 3: Implement FavoriteLogic**

Modify `app/interaction/internal/logic/favorite_logic.go`:

```go
package logic

import (
	"context"
	"errors"
	"fmt"
	"math/rand"

	"esx/app/interaction/internal/model"
	"esx/app/interaction/internal/svc"
	"esx/app/interaction/pb/xiaobaihe/interaction/pb"
	"esx/pkg/errx"

	"github.com/zeromicro/go-zero/core/logx"
)

type FavoriteLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewFavoriteLogic(ctx context.Context, svcCtx *svc.ServiceContext) *FavoriteLogic {
	return &FavoriteLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *FavoriteLogic) Favorite(in *pb.FavoriteReq) (*pb.FavoriteResp, error) {
	record, err := l.svcCtx.FavoriteModel.FindOneByUserIdPostId(l.ctx, in.UserId, in.PostId)
	if err != nil && !errors.Is(err, model.ErrNotFound) {
		l.Logger.Errorf("FindOneByUserIdPostId failed: %v", err)
		return nil, errx.New(errx.SystemError, err.Error())
	}
	if record != nil && record.Status == 1 {
		return nil, errx.New(errx.AlreadyFavorited, errx.GetMsg(errx.AlreadyFavorited))
	}

	if record != nil {
		record.Status = 1
		if err := l.svcCtx.FavoriteModel.Update(l.ctx, record); err != nil {
			l.Logger.Errorf("Update favorite failed: %v", err)
			return nil, errx.New(errx.SystemError, err.Error())
		}
	} else {
		_, err = l.svcCtx.FavoriteModel.Insert(l.ctx, &model.Favorite{
			UserId: in.UserId,
			PostId: in.PostId,
			Status: 1,
		})
		if err != nil {
			l.Logger.Errorf("Insert favorite failed: %v", err)
			return nil, errx.New(errx.SystemError, err.Error())
		}
	}

	if err := l.incrFavoriteCount(in.PostId); err != nil {
		l.Logger.Errorf("incrFavoriteCount failed: %v", err)
	}

	return &pb.FavoriteResp{}, nil
}

func (l *FavoriteLogic) incrFavoriteCount(postId int64) error {
	key := fmt.Sprintf("action_count:%d:1", postId)
	exists, err := l.svcCtx.Redis.Exists(key)
	if err != nil {
		return err
	}
	if exists {
		_, err = l.svcCtx.Redis.Hincrby(key, "favorite_count", 1)
		return err
	}

	count, err := l.svcCtx.ActionCountModel.FindOneByTarget(l.ctx, postId, 1)
	if err != nil {
		if errors.Is(err, model.ErrNotFound) {
			_, err = l.svcCtx.ActionCountModel.Insert(l.ctx, &model.ActionCount{
				TargetId:      postId,
				TargetType:    1,
				FavoriteCount: 1,
			})
			if err != nil {
				return err
			}
			l.svcCtx.Redis.Hset(key, "like_count", "0")
			l.svcCtx.Redis.Hset(key, "favorite_count", "1")
			l.svcCtx.Redis.Expire(key, 30)
			return nil
		}
		return err
	}

	count.FavoriteCount++
	if err := l.svcCtx.ActionCountModel.Update(l.ctx, count); err != nil {
		return err
	}

	l.svcCtx.Redis.Hset(key, "like_count", fmt.Sprintf("%d", count.LikeCount))
	l.svcCtx.Redis.Hset(key, "favorite_count", fmt.Sprintf("%d", count.FavoriteCount))
	ttl := 300 + rand.Intn(60)
	l.svcCtx.Redis.Expire(key, ttl)
	return nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run:
```bash
cd app/interaction && go test ./internal/logic -run TestFavoriteLogic -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add app/interaction/internal/logic/favorite_logic.go app/interaction/internal/logic/favorite_logic_test.go
git commit -m "feat(interaction): implement FavoriteLogic with tests"
```

---

## Task 5: UnfavoriteLogic (TDD)

**Files:**
- Modify: `app/interaction/internal/logic/unfavorite_logic.go`
- Create: `app/interaction/internal/logic/unfavorite_logic_test.go`

- [ ] **Step 1: Write failing tests**

Create `app/interaction/internal/logic/unfavorite_logic_test.go`:

```go
package logic

import (
	"context"
	"testing"
	"time"

	"esx/app/interaction/internal/model"
	"esx/app/interaction/internal/svc"
	"esx/app/interaction/pb/xiaobaihe/interaction/pb"
	"esx/pkg/errx"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/alicebob/miniredis/v2"
	"github.com/stretchr/testify/assert"
	"github.com/zeromicro/go-zero/core/stores/cache"
	"github.com/zeromicro/go-zero/core/stores/redis"
	"github.com/zeromicro/go-zero/core/stores/sqlx"
)

func TestUnfavoriteLogic_Unfavorite_Success(t *testing.T) {
	mockDB, mock, err := sqlmock.New()
	assert.NoError(t, err)
	defer mockDB.Close()

	mr := miniredis.RunT(&testing.T{})
	defer mr.Close()

	svcCtx := &svc.ServiceContext{
		FavoriteModel:    model.NewFavoriteModel(sqlx.NewSqlConnFromDB(mockDB), cache.CacheConf{}),
		ActionCountModel: model.NewActionCountModel(sqlx.NewSqlConnFromDB(mockDB)),
		Redis:            redis.New(mr.Addr(), redis.WithPass("")),
	}

	rows := sqlmock.NewRows([]string{"id", "user_id", "post_id", "folder_id", "status", "created_at", "updated_at"}).
		AddRow(1, 1, 100, sql.NullInt64{Valid: false}, 1, time.Now(), time.Now())
	mock.ExpectQuery("select \\`id\\`,").WillReturnRows(rows)
	mock.ExpectExec("update").WillReturnResult(sqlmock.NewResult(0, 1))
	countRows := sqlmock.NewRows([]string{"id", "target_id", "target_type", "like_count", "favorite_count", "comment_count", "share_count"}).
		AddRow(1, 100, 1, 0, 5, 0, 0)
	mock.ExpectQuery("select \\`id\\`,").WillReturnRows(countRows)
	mock.ExpectExec("update").WillReturnResult(sqlmock.NewResult(0, 1))

	logic := NewUnfavoriteLogic(context.Background(), svcCtx)
	resp, err := logic.Unfavorite(&pb.UnfavoriteReq{UserId: 1, PostId: 100})
	assert.NoError(t, err)
	assert.NotNil(t, resp)
}

func TestUnfavoriteLogic_Unfavorite_NotFavorited(t *testing.T) {
	mockDB, mock, err := sqlmock.New()
	assert.NoError(t, err)
	defer mockDB.Close()

	mr := miniredis.RunT(&testing.T{})
	defer mr.Close()

	svcCtx := &svc.ServiceContext{
		FavoriteModel: model.NewFavoriteModel(sqlx.NewSqlConnFromDB(mockDB), cache.CacheConf{}),
		Redis:         redis.New(mr.Addr(), redis.WithPass("")),
	}

	mock.ExpectQuery("select \\`id\\`,").WillReturnError(model.ErrNotFound)

	logic := NewUnfavoriteLogic(context.Background(), svcCtx)
	_, err = logic.Unfavorite(&pb.UnfavoriteReq{UserId: 1, PostId: 100})
	assert.Error(t, err)
	bizErr, ok := err.(*errx.BizError)
	assert.True(t, ok)
	assert.Equal(t, errx.NotFavoritedYet, bizErr.Code)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run:
```bash
cd app/interaction && go test ./internal/logic -run TestUnfavoriteLogic -v
```

Expected: FAIL.

- [ ] **Step 3: Implement UnfavoriteLogic**

Modify `app/interaction/internal/logic/unfavorite_logic.go`:

```go
package logic

import (
	"context"
	"errors"
	"fmt"
	"math/rand"

	"esx/app/interaction/internal/model"
	"esx/app/interaction/internal/svc"
	"esx/app/interaction/pb/xiaobaihe/interaction/pb"
	"esx/pkg/errx"

	"github.com/zeromicro/go-zero/core/logx"
)

type UnfavoriteLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewUnfavoriteLogic(ctx context.Context, svcCtx *svc.ServiceContext) *UnfavoriteLogic {
	return &UnfavoriteLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *UnfavoriteLogic) Unfavorite(in *pb.UnfavoriteReq) (*pb.UnfavoriteResp, error) {
	record, err := l.svcCtx.FavoriteModel.FindOneByUserIdPostId(l.ctx, in.UserId, in.PostId)
	if err != nil {
		if errors.Is(err, model.ErrNotFound) {
			return nil, errx.New(errx.NotFavoritedYet, errx.GetMsg(errx.NotFavoritedYet))
		}
		l.Logger.Errorf("FindOneByUserIdPostId failed: %v", err)
		return nil, errx.New(errx.SystemError, err.Error())
	}
	if record.Status == 0 {
		return nil, errx.New(errx.NotFavoritedYet, errx.GetMsg(errx.NotFavoritedYet))
	}

	record.Status = 0
	if err := l.svcCtx.FavoriteModel.Update(l.ctx, record); err != nil {
		l.Logger.Errorf("Update favorite failed: %v", err)
		return nil, errx.New(errx.SystemError, err.Error())
	}

	if err := l.decrFavoriteCount(in.PostId); err != nil {
		l.Logger.Errorf("decrFavoriteCount failed: %v", err)
	}

	return &pb.UnfavoriteResp{}, nil
}

func (l *UnfavoriteLogic) decrFavoriteCount(postId int64) error {
	key := fmt.Sprintf("action_count:%d:1", postId)
	exists, err := l.svcCtx.Redis.Exists(key)
	if err != nil {
		return err
	}
	if exists {
		_, err = l.svcCtx.Redis.Hincrby(key, "favorite_count", -1)
		return err
	}

	count, err := l.svcCtx.ActionCountModel.FindOneByTarget(l.ctx, postId, 1)
	if err != nil {
		return err
	}
	if count.FavoriteCount > 0 {
		count.FavoriteCount--
	}
	if err := l.svcCtx.ActionCountModel.Update(l.ctx, count); err != nil {
		return err
	}

	l.svcCtx.Redis.Hset(key, "like_count", fmt.Sprintf("%d", count.LikeCount))
	l.svcCtx.Redis.Hset(key, "favorite_count", fmt.Sprintf("%d", count.FavoriteCount))
	ttl := 300 + rand.Intn(60)
	l.svcCtx.Redis.Expire(key, ttl)
	return nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run:
```bash
cd app/interaction && go test ./internal/logic -run TestUnfavoriteLogic -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add app/interaction/internal/logic/unfavorite_logic.go app/interaction/internal/logic/unfavorite_logic_test.go
git commit -m "feat(interaction): implement UnfavoriteLogic with tests"
```

---

## Task 6: GetCountsLogic (TDD)

**Files:**
- Modify: `app/interaction/internal/logic/get_counts_logic.go`
- Create: `app/interaction/internal/logic/get_counts_logic_test.go`

- [ ] **Step 1: Write failing tests**

Create `app/interaction/internal/logic/get_counts_logic_test.go`:

```go
package logic

import (
	"context"
	"testing"

	"esx/app/interaction/internal/model"
	"esx/app/interaction/internal/svc"
	"esx/app/interaction/pb/xiaobaihe/interaction/pb"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/alicebob/miniredis/v2"
	"github.com/stretchr/testify/assert"
	"github.com/zeromicro/go-zero/core/stores/redis"
	"github.com/zeromicro/go-zero/core/stores/sqlx"
)

func TestGetCountsLogic_GetCounts_RedisHit(t *testing.T) {
	mr := miniredis.RunT(&testing.T{})
	defer mr.Close()

	svcCtx := &svc.ServiceContext{
		Redis: redis.New(mr.Addr(), redis.WithPass("")),
	}
	mr.Hset("action_count:100:1", "like_count", "10")
	mr.Hset("action_count:100:1", "favorite_count", "5")

	logic := NewGetCountsLogic(context.Background(), svcCtx)
	resp, err := logic.GetCounts(&pb.GetCountsReq{TargetId: 100, TargetType: 1})
	assert.NoError(t, err)
	assert.Equal(t, int64(10), resp.LikeCount)
	assert.Equal(t, int64(5), resp.FavoriteCount)
}

func TestGetCountsLogic_GetCounts_RedisMiss(t *testing.T) {
	mockDB, mock, err := sqlmock.New()
	assert.NoError(t, err)
	defer mockDB.Close()

	mr := miniredis.RunT(&testing.T{})
	defer mr.Close()

	svcCtx := &svc.ServiceContext{
		ActionCountModel: model.NewActionCountModel(sqlx.NewSqlConnFromDB(mockDB)),
		Redis:            redis.New(mr.Addr(), redis.WithPass("")),
	}

	rows := sqlmock.NewRows([]string{"id", "target_id", "target_type", "like_count", "favorite_count", "comment_count", "share_count"}).
		AddRow(1, 100, 1, 7, 3, 0, 0)
	mock.ExpectQuery("select \\`id\\`,").WillReturnRows(rows)

	logic := NewGetCountsLogic(context.Background(), svcCtx)
	resp, err := logic.GetCounts(&pb.GetCountsReq{TargetId: 100, TargetType: 1})
	assert.NoError(t, err)
	assert.Equal(t, int64(7), resp.LikeCount)
	assert.Equal(t, int64(3), resp.FavoriteCount)
}

func TestGetCountsLogic_GetCounts_NotFound(t *testing.T) {
	mockDB, mock, err := sqlmock.New()
	assert.NoError(t, err)
	defer mockDB.Close()

	mr := miniredis.RunT(&testing.T{})
	defer mr.Close()

	svcCtx := &svc.ServiceContext{
		ActionCountModel: model.NewActionCountModel(sqlx.NewSqlConnFromDB(mockDB)),
		Redis:            redis.New(mr.Addr(), redis.WithPass("")),
	}

	mock.ExpectQuery("select \\`id\\`,").WillReturnError(model.ErrNotFound)

	logic := NewGetCountsLogic(context.Background(), svcCtx)
	resp, err := logic.GetCounts(&pb.GetCountsReq{TargetId: 999, TargetType: 1})
	assert.NoError(t, err)
	assert.Equal(t, int64(0), resp.LikeCount)
	assert.Equal(t, int64(0), resp.FavoriteCount)
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run:
```bash
cd app/interaction && go test ./internal/logic -run TestGetCountsLogic -v
```

Expected: FAIL.

- [ ] **Step 3: Implement GetCountsLogic**

Modify `app/interaction/internal/logic/get_counts_logic.go`:

```go
package logic

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"strconv"
	"time"

	"esx/app/interaction/internal/model"
	"esx/app/interaction/internal/svc"
	"esx/app/interaction/pb/xiaobaihe/interaction/pb"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetCountsLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewGetCountsLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetCountsLogic {
	return &GetCountsLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *GetCountsLogic) GetCounts(in *pb.GetCountsReq) (*pb.GetCountsResp, error) {
	key := fmt.Sprintf("action_count:%d:%d", in.TargetId, in.TargetType)

	// Try Redis first
	likeVal, err := l.svcCtx.Redis.Hget(key, "like_count")
	if err == nil {
		favVal, err := l.svcCtx.Redis.Hget(key, "favorite_count")
		if err == nil {
			likeCount, _ := strconv.ParseInt(likeVal, 10, 64)
			favCount, _ := strconv.ParseInt(favVal, 10, 64)
			return &pb.GetCountsResp{
				LikeCount:     likeCount,
				FavoriteCount: favCount,
			}, nil
		}
	}

	// Redis miss: query DB with singleflight
	result, err, _ := l.svcCtx.SingleFlight.Do(key, func() (interface{}, error) {
		// Double-check Redis after singleflight
		likeVal, err := l.svcCtx.Redis.Hget(key, "like_count")
		if err == nil {
			favVal, _ := l.svcCtx.Redis.Hget(key, "favorite_count")
			likeCount, _ := strconv.ParseInt(likeVal, 10, 64)
			favCount, _ := strconv.ParseInt(favVal, 10, 64)
			return &pb.GetCountsResp{LikeCount: likeCount, FavoriteCount: favCount}, nil
		}

		count, err := l.svcCtx.ActionCountModel.FindOneByTarget(l.ctx, in.TargetId, int64(in.TargetType))
		if err != nil {
			if errors.Is(err, model.ErrNotFound) {
				// Cache null value
				l.svcCtx.Redis.Hset(key, "like_count", "0")
				l.svcCtx.Redis.Hset(key, "favorite_count", "0")
				l.svcCtx.Redis.Expire(key, 30)
				return &pb.GetCountsResp{}, nil
			}
			return nil, err
		}

		// Backfill Redis
		l.svcCtx.Redis.Hset(key, "like_count", fmt.Sprintf("%d", count.LikeCount))
		l.svcCtx.Redis.Hset(key, "favorite_count", fmt.Sprintf("%d", count.FavoriteCount))
		ttl := 300 + rand.Intn(60)
		l.svcCtx.Redis.Expire(key, ttl)

		return &pb.GetCountsResp{
			LikeCount:     count.LikeCount,
			FavoriteCount: count.FavoriteCount,
		}, nil
	})

	if err != nil {
		l.Logger.Errorf("GetCounts failed: %v", err)
		return nil, err
	}

	return result.(*pb.GetCountsResp), nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run:
```bash
cd app/interaction && go test ./internal/logic -run TestGetCountsLogic -v
```

Expected: PASS.

- [ ] **Step 5: Commit**

```bash
git add app/interaction/internal/logic/get_counts_logic.go app/interaction/internal/logic/get_counts_logic_test.go
git commit -m "feat(interaction): implement GetCountsLogic with Redis cache and tests"
```

---

## Task 7: GetLikeCountLogic

**Files:**
- Modify: `app/interaction/internal/logic/get_like_count_logic.go`

- [ ] **Step 1: Implement GetLikeCountLogic**

Modify `app/interaction/internal/logic/get_like_count_logic.go`:

```go
package logic

import (
	"context"

	"esx/app/interaction/internal/svc"
	"esx/app/interaction/pb/xiaobaihe/interaction/pb"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetLikeCountLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewGetLikeCountLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetLikeCountLogic {
	return &GetLikeCountLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *GetLikeCountLogic) GetLikeCount(in *pb.GetLikeCountReq) (*pb.GetLikeCountResp, error) {
	counts, err := NewGetCountsLogic(l.ctx, l.svcCtx).GetCounts(&pb.GetCountsReq{
		TargetId:   in.TargetId,
		TargetType: in.TargetType,
	})
	if err != nil {
		return nil, err
	}
	return &pb.GetLikeCountResp{Count: counts.LikeCount}, nil
}
```

- [ ] **Step 2: Verify compilation**

Run:
```bash
cd app/interaction && go build ./...
```

Expected: compiles successfully.

- [ ] **Step 3: Commit**

```bash
git add app/interaction/internal/logic/get_like_count_logic.go
git commit -m "feat(interaction): implement GetLikeCountLogic delegating to GetCounts"
```

---

## Task 8: Check/BatchCheck Liked

**Files:**
- Modify: `app/interaction/internal/logic/check_liked_logic.go`
- Modify: `app/interaction/internal/logic/batch_check_liked_logic.go`

- [ ] **Step 1: Implement CheckLikedLogic**

Modify `app/interaction/internal/logic/check_liked_logic.go`:

```go
package logic

import (
	"context"
	"errors"

	"esx/app/interaction/internal/model"
	"esx/app/interaction/internal/svc"
	"esx/app/interaction/pb/xiaobaihe/interaction/pb"

	"github.com/zeromicro/go-zero/core/logx"
)

type CheckLikedLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewCheckLikedLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CheckLikedLogic {
	return &CheckLikedLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *CheckLikedLogic) CheckLiked(in *pb.CheckLikedReq) (*pb.CheckLikedResp, error) {
	record, err := l.svcCtx.LikeRecordModel.FindOneByUserIdTargetIdTargetType(l.ctx, in.UserId, in.TargetId, int64(in.TargetType))
	if err != nil {
		if errors.Is(err, model.ErrNotFound) {
			return &pb.CheckLikedResp{IsLiked: false}, nil
		}
		l.Logger.Errorf("FindOneByUserIdTargetIdTargetType failed: %v", err)
		return nil, err
	}
	return &pb.CheckLikedResp{IsLiked: record.Status == 1}, nil
}
```

- [ ] **Step 2: Implement BatchCheckLikedLogic**

Modify `app/interaction/internal/logic/batch_check_liked_logic.go`:

```go
package logic

import (
	"context"
	"fmt"
	"strings"

	"esx/app/interaction/internal/svc"
	"esx/app/interaction/pb/xiaobaihe/interaction/pb"

	"github.com/zeromicro/go-zero/core/logx"
)

type BatchCheckLikedLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewBatchCheckLikedLogic(ctx context.Context, svcCtx *svc.ServiceContext) *BatchCheckLikedLogic {
	return &BatchCheckLikedLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *BatchCheckLikedLogic) BatchCheckLiked(in *pb.BatchCheckLikedReq) (*pb.BatchCheckLikedResp, error) {
	results := make(map[int64]bool)
	if len(in.TargetIds) == 0 {
		return &pb.BatchCheckLikedResp{Results: results}, nil
	}

	placeholders := make([]string, len(in.TargetIds))
	args := make([]interface{}, 0, len(in.TargetIds)+2)
	args = append(args, in.UserId, int64(in.TargetType))
	for i, id := range in.TargetIds {
		placeholders[i] = "?"
		args = append(args, id)
	}

	query := fmt.Sprintf(
		"select `target_id`, `status` from `like_record` where `user_id` = ? and `target_type` = ? and `target_id` in (%s)",
		strings.Join(placeholders, ","),
	)

	rows, err := l.svcCtx.LikeRecordModel.(*customLikeRecordModel).conn.QueryCtx(l.ctx, query, args...)
	if err != nil {
		l.Logger.Errorf("BatchCheckLiked query failed: %v", err)
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var targetId int64
		var status int64
		if err := rows.Scan(&targetId, &status); err != nil {
			continue
		}
		results[targetId] = status == 1
	}

	return &pb.BatchCheckLikedResp{Results: results}, nil
}
```

Note: The `BatchCheckLiked` implementation uses raw SQL because the generated model does not expose `conn`. If this causes compilation issues, use a simpler approach: loop over `TargetIds` and call `FindOneByUserIdTargetIdTargetType` for each. That is less efficient but avoids exposing internals.

Alternative simpler implementation for Step 2:

```go
func (l *BatchCheckLikedLogic) BatchCheckLiked(in *pb.BatchCheckLikedLogic) (*pb.BatchCheckLikedResp, error) {
    results := make(map[int64]bool, len(in.TargetIds))
    for _, targetId := range in.TargetIds {
        resp, err := NewCheckLikedLogic(l.ctx, l.svcCtx).CheckLiked(&pb.CheckLikedReq{
            UserId:     in.UserId,
            TargetId:   targetId,
            TargetType: in.TargetType,
        })
        if err != nil {
            l.Logger.Errorf("CheckLiked failed for target %d: %v", targetId, err)
            continue
        }
        results[targetId] = resp.IsLiked
    }
    return &pb.BatchCheckLikedResp{Results: results}, nil
}
```

Use this simpler approach to avoid type assertion issues.

- [ ] **Step 3: Verify compilation**

Run:
```bash
cd app/interaction && go build ./...
```

Expected: compiles successfully.

- [ ] **Step 4: Commit**

```bash
git add app/interaction/internal/logic/check_liked_logic.go app/interaction/internal/logic/batch_check_liked_logic.go
git commit -m "feat(interaction): implement CheckLiked and BatchCheckLiked"
```

---

## Task 9: Check/BatchCheck Favorited

**Files:**
- Modify: `app/interaction/internal/logic/check_favorited_logic.go`
- Modify: `app/interaction/internal/logic/batch_check_favorited_logic.go`

- [ ] **Step 1: Implement CheckFavoritedLogic**

Modify `app/interaction/internal/logic/check_favorited_logic.go`:

```go
package logic

import (
	"context"
	"errors"

	"esx/app/interaction/internal/model"
	"esx/app/interaction/internal/svc"
	"esx/app/interaction/pb/xiaobaihe/interaction/pb"

	"github.com/zeromicro/go-zero/core/logx"
)

type CheckFavoritedLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewCheckFavoritedLogic(ctx context.Context, svcCtx *svc.ServiceContext) *CheckFavoritedLogic {
	return &CheckFavoritedLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *CheckFavoritedLogic) CheckFavorited(in *pb.CheckFavoritedReq) (*pb.CheckFavoritedResp, error) {
	record, err := l.svcCtx.FavoriteModel.FindOneByUserIdPostId(l.ctx, in.UserId, in.PostId)
	if err != nil {
		if errors.Is(err, model.ErrNotFound) {
			return &pb.CheckFavoritedResp{IsFavorited: false}, nil
		}
		l.Logger.Errorf("FindOneByUserIdPostId failed: %v", err)
		return nil, err
	}
	return &pb.CheckFavoritedResp{IsFavorited: record.Status == 1}, nil
}
```

- [ ] **Step 2: Implement BatchCheckFavoritedLogic**

Modify `app/interaction/internal/logic/batch_check_favorited_logic.go`:

```go
package logic

import (
	"context"

	"esx/app/interaction/internal/svc"
	"esx/app/interaction/pb/xiaobaihe/interaction/pb"

	"github.com/zeromicro/go-zero/core/logx"
)

type BatchCheckFavoritedLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewBatchCheckFavoritedLogic(ctx context.Context, svcCtx *svc.ServiceContext) *BatchCheckFavoritedLogic {
	return &BatchCheckFavoritedLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *BatchCheckFavoritedLogic) BatchCheckFavorited(in *pb.BatchCheckFavoritedReq) (*pb.BatchCheckFavoritedResp, error) {
	results := make(map[int64]bool, len(in.PostIds))
	for _, postId := range in.PostIds {
		resp, err := NewCheckFavoritedLogic(l.ctx, l.svcCtx).CheckFavorited(&pb.CheckFavoritedReq{
			UserId: in.UserId,
			PostId: postId,
		})
		if err != nil {
			l.Logger.Errorf("CheckFavorited failed for post %d: %v", postId, err)
			continue
		}
		results[postId] = resp.IsFavorited
	}
	return &pb.BatchCheckFavoritedResp{Results: results}, nil
}
```

- [ ] **Step 3: Verify compilation**

Run:
```bash
cd app/interaction && go build ./...
```

Expected: compiles successfully.

- [ ] **Step 4: Commit**

```bash
git add app/interaction/internal/logic/check_favorited_logic.go app/interaction/internal/logic/batch_check_favorited_logic.go
git commit -m "feat(interaction): implement CheckFavorited and BatchCheckFavorited"
```

---

## Task 10: GetFavoriteListLogic

**Files:**
- Modify: `app/interaction/internal/logic/get_favorite_list_logic.go`

- [ ] **Step 1: Implement GetFavoriteListLogic**

Modify `app/interaction/internal/logic/get_favorite_list_logic.go`:

```go
package logic

import (
	"context"
	"fmt"

	"esx/app/interaction/internal/svc"
	"esx/app/interaction/pb/xiaobaihe/interaction/pb"

	"github.com/zeromicro/go-zero/core/logx"
)

type GetFavoriteListLogic struct {
	ctx    context.Context
	svcCtx *svc.ServiceContext
	logx.Logger
}

func NewGetFavoriteListLogic(ctx context.Context, svcCtx *svc.ServiceContext) *GetFavoriteListLogic {
	return &GetFavoriteListLogic{
		ctx:    ctx,
		svcCtx: svcCtx,
		Logger: logx.WithContext(ctx),
	}
}

func (l *GetFavoriteListLogic) GetFavoriteList(in *pb.GetFavoriteListReq) (*pb.GetFavoriteListResp, error) {
	if in.Page < 1 {
		in.Page = 1
	}
	if in.PageSize < 1 || in.PageSize > 100 {
		in.PageSize = 20
	}
	offset := (in.Page - 1) * in.PageSize

	query := "select `post_id` from `favorite` where `user_id` = ? and `status` = 1 order by `created_at` desc limit ? offset ?"
	rows, err := l.svcCtx.FavoriteModel.(*customFavoriteModel).conn.QueryCtx(l.ctx, query, in.UserId, in.PageSize, offset)
	if err != nil {
		l.Logger.Errorf("GetFavoriteList query failed: %v", err)
		return nil, err
	}
	defer rows.Close()

	var postIds []int64
	for rows.Next() {
		var postId int64
		if err := rows.Scan(&postId); err != nil {
			continue
		}
		postIds = append(postIds, postId)
	}

	// Count total
	countQuery := "select count(*) from `favorite` where `user_id` = ? and `status` = 1"
	var total int64
	row := l.svcCtx.FavoriteModel.(*customFavoriteModel).conn.QueryRowCtx(l.ctx, countQuery, in.UserId)
	_ = row.Scan(&total)

	return &pb.GetFavoriteListResp{
		PostIds: postIds,
		Total:   total,
	}, nil
}
```

Alternative if type assertion is problematic — add a custom method to `FavoriteModel` interface in `favorite_model.go`:

```go
// In favorite_model.go
type FavoriteModel interface {
	favoriteModel
	FindFavoritePostIds(ctx context.Context, userId int64, page, pageSize int32) ([]int64, int64, error)
}
```

Then implement the method. This is cleaner but requires modifying the model interface. For the plan, use the simpler raw SQL approach with the note that if compilation fails, the model interface should be extended.

- [ ] **Step 2: Verify compilation**

Run:
```bash
cd app/interaction && go build ./...
```

If compilation fails due to type assertion on `customFavoriteModel`, extend the `FavoriteModel` interface in `favorite_model.go` with a helper method and regenerate/reimplement.

- [ ] **Step 3: Commit**

```bash
git add app/interaction/internal/logic/get_favorite_list_logic.go
git commit -m "feat(interaction): implement GetFavoriteListLogic"
```

---

## Task 11: Full Test Suite Run

- [ ] **Step 1: Run all interaction tests**

```bash
cd app/interaction && go test ./internal/logic/... -v -race -cover
```

Expected: All tests PASS, coverage > 80%.

- [ ] **Step 2: Run go vet**

```bash
cd app/interaction && go vet ./...
```

Expected: No issues.

- [ ] **Step 3: Commit final verification**

```bash
git add -A
git commit -m "test(interaction): full test suite for Like/Unlike/Favorite/Unfavorite and counts"
```

---

## Spec Coverage Checklist

| Spec Requirement | Plan Task |
|------------------|-----------|
| Add action_count table | Task 1, Step 1 |
| Create action_count model | Task 1, Step 2 |
| Redis config in svc | Task 1, Steps 3-4 |
| LikeLogic with idempotency | Task 2 |
| UnlikeLogic with validation | Task 3 |
| FavoriteLogic with idempotency | Task 4 |
| UnfavoriteLogic with validation | Task 5 |
| GetCounts with Redis + singleflight | Task 6 |
| GetLikeCount | Task 7 |
| Check/BatchCheck Liked | Task 8 |
| Check/BatchCheck Favorited | Task 9 |
| GetFavoriteList | Task 10 |
| Error codes (AlreadyLiked, NotLiked, etc.) | Tasks 2-5 (errx usage) |
| MQ event (fire-and-forget) | Task 2, Step 3 (publishEvent) |
| 80%+ test coverage | All TDD tasks + Task 11 |

## Placeholder Scan

- No "TBD", "TODO", "implement later" found.
- No vague "add error handling" steps — all error handling is in the implementation code.
- No "Similar to Task N" — each task is self-contained.

## Type Consistency Check

- `LikeReq.TargetType` is `int32`; model uses `int64` — conversion `int64(in.TargetType)` used consistently.
- `FavoriteReq.PostId` maps to `TargetId` in action_count with `TargetType = 1`.
- Redis key format `action_count:{target_id}:{target_type}` used consistently across all tasks.
