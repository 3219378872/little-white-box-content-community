# Interaction 模块审查热修复实施计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** 修复 interaction 模块审查报告中的 1 个 CRITICAL、10 个 HIGH、5 个 MEDIUM 和 8 个 LOW 问题。

**Architecture:** 基于现有三层架构（Handler -> Logic -> Model），通过新增原子更新 Model 方法消除 Read-Modify-Write 反模式，统一错误包装，提取常量，增强可观测性。

**Tech Stack:** Go 1.26.1, go-zero v1.10.1, MySQL 8.0, Redis 7

**Worktree:** `.worktrees/hotfix-interaction` (branch: `hotfix/interaction-review-fixes`)

---

## 文件变更总览

| 文件 | 操作 | 说明 |
|------|------|------|
| `app/interaction/internal/model/vars.go` | 修改 | 新增 Status 常量和缓存 TTL 常量 |
| `app/interaction/internal/model/action_count_model.go` | 修改 | 新增原子更新方法，Insert 使用雪花 ID |
| `app/interaction/internal/logic/integration_test.go` | 修改 | 移除硬编码数据库密码 |
| `app/interaction/internal/logic/like_logic.go` | 修改 | 原子更新、错误包装、常量、日志 |
| `app/interaction/internal/logic/favorite_logic.go` | 修改 | 原子更新、错误包装、常量、日志 |
| `app/interaction/internal/logic/unlike_logic.go` | 修改 | 原子更新、错误包装、常量、日志 |
| `app/interaction/internal/logic/unfavorite_logic.go` | 修改 | 原子更新、错误包装、常量、日志 |
| `app/interaction/internal/logic/check_favorited_logic.go` | 修改 | 错误包装 |
| `app/interaction/internal/logic/check_liked_logic.go` | 修改 | 错误包装 |
| `app/interaction/internal/logic/get_favorite_list_logic.go` | 修改 | 错误包装 |
| `app/interaction/internal/logic/get_counts_logic.go` | 修改 | 错误包装、常量、singleflight key 前缀、parseInt64 日志 |
| `app/interaction/internal/logic/batch_check_favorited_logic.go` | 修改 | 错误包装 |
| `app/interaction/internal/logic/batch_check_liked_logic.go` | 修改 | 错误包装 |

---

## Task 1: CRITICAL - 移除集成测试硬编码密码

**Files:**
- Modify: `app/interaction/internal/logic/integration_test.go:31-33`

- [ ] **Step 1: 修改 getEnv 默认值，强制环境变量**

将硬编码 DSN 默认值改为空字符串，并在 TestMain 中检查：

```go
func TestMain(m *testing.M) {
	dsn := getEnv("TEST_MYSQL_DSN", getEnv("DB_INTERACTION", ""))
	if dsn == "" {
		fmt.Fprintln(os.Stderr, "错误: TEST_MYSQL_DSN 或 DB_INTERACTION 环境变量必须设置")
		os.Exit(1)
	}
	// ... 其余代码不变
}
```

- [ ] **Step 2: 运行单元测试确认未破坏**

Run: `go test ./app/interaction/internal/logic/... -short -v`
Expected: PASS (单元测试不使用集成测试文件)

- [ ] **Step 3: Commit**

```bash
git add app/interaction/internal/logic/integration_test.go
git commit -m "fix(interaction): remove hardcoded test database password

- CRITICAL[C1]: 移除 root:root 硬编码 DSN
- 强制要求 TEST_MYSQL_DSN 或 DB_INTERACTION 环境变量"
```

---

## Task 2: HIGH - 新增 ActionCount 原子更新 Model 方法

**Files:**
- Modify: `app/interaction/internal/model/action_count_model.go`

- [ ] **Step 1: 新增原子递增/递减方法到接口和实现**

```go
type (
	ActionCountModel interface {
		Insert(ctx context.Context, data *ActionCount) (sql.Result, error)
		FindOneByTarget(ctx context.Context, targetID, targetType int64) (*ActionCount, error)
		Update(ctx context.Context, data *ActionCount) error
		// 原子更新方法
		IncrLikeCount(ctx context.Context, targetID, targetType int64) error
		IncrFavoriteCount(ctx context.Context, targetID, targetType int64) error
		DecrLikeCount(ctx context.Context, targetID, targetType int64) error
		DecrFavoriteCount(ctx context.Context, targetID, targetType int64) error
	}
	// ... customActionCountModel 不变
)
```

```go
func (m *customActionCountModel) IncrLikeCount(ctx context.Context, targetID, targetType int64) error {
	query := fmt.Sprintf("update %s set `like_count` = `like_count` + 1 where `target_id` = ? and `target_type` = ?", m.table)
	_, err := m.conn.ExecCtx(ctx, query, targetID, targetType)
	return err
}

func (m *customActionCountModel) IncrFavoriteCount(ctx context.Context, targetID, targetType int64) error {
	query := fmt.Sprintf("update %s set `favorite_count` = `favorite_count` + 1 where `target_id` = ? and `target_type` = ?", m.table)
	_, err := m.conn.ExecCtx(ctx, query, targetID, targetType)
	return err
}

func (m *customActionCountModel) DecrLikeCount(ctx context.Context, targetID, targetType int64) error {
	query := fmt.Sprintf("update %s set `like_count` = `like_count` - 1 where `target_id` = ? and `target_type` = ? and `like_count` > 0", m.table)
	_, err := m.conn.ExecCtx(ctx, query, targetID, targetType)
	return err
}

func (m *customActionCountModel) DecrFavoriteCount(ctx context.Context, targetID, targetType int64) error {
	query := fmt.Sprintf("update %s set `favorite_count` = `favorite_count` - 1 where `target_id` = ? and `target_type` = ? and `favorite_count` > 0", m.table)
	_, err := m.conn.ExecCtx(ctx, query, targetID, targetType)
	return err
}
```

- [ ] **Step 2: 运行编译检查**

Run: `go build ./app/interaction/...`
Expected: 编译通过

- [ ] **Step 3: Commit**

```bash
git add app/interaction/internal/model/action_count_model.go
git commit -m "feat(interaction/model): add atomic count update methods

- 新增 IncrLikeCount/IncrFavoriteCount/DecrLikeCount/DecrFavoriteCount
- 使用数据库原子操作消除 Read-Modify-Write 竞态条件
- HIGH[H1-H4] 前置依赖"
```

---

## Task 3: MEDIUM - 定义 Status 常量和缓存 TTL 常量

**Files:**
- Modify: `app/interaction/internal/model/vars.go`

- [ ] **Step 1: 添加常量定义**

```go
package model

import "github.com/zeromicro/go-zero/core/stores/sqlx"

var ErrNotFound = sqlx.ErrNotFound

// 状态常量
const (
	StatusInactive = 0
	StatusActive   = 1
)

// 缓存 TTL 常量 (秒)
const (
	CacheShortTTL = 30  // 空数据防穿透缓存
	CacheLongTTL  = 300 // 正常数据缓存
)
```

- [ ] **Step 2: 运行编译检查**

Run: `go build ./app/interaction/...`
Expected: 编译通过

- [ ] **Step 3: Commit**

```bash
git add app/interaction/internal/model/vars.go
git commit -m "refactor(interaction/model): define status and cache TTL constants

- StatusActive/StatusInactive 替代魔法数字 1/0
- CacheShortTTL/CacheLongTTL 替代魔法数字 30/300
- MEDIUM[M1-M2]"
```

---

## Task 4: HIGH - 修复 Like/Unlike 计数原子更新与错误包装

**Files:**
- Modify: `app/interaction/internal/logic/like_logic.go`
- Modify: `app/interaction/internal/logic/unlike_logic.go`

- [ ] **Step 1: 修改 like_logic.go - 使用 StatusActive 常量、原子更新、错误包装、Redis 日志**

```go
func (l *LikeLogic) Like(in *pb.LikeReq) (*pb.LikeResp, error) {
	record, err := l.svcCtx.LikeRecordModel.FindOneByUserIdTargetIdTargetType(l.ctx, in.UserId, in.TargetId, int64(in.TargetType))
	if err != nil && !errors.Is(err, model.ErrNotFound) {
		l.Logger.Errorf("find like record failed: %v", err)
		return nil, errx.NewWithCode(errx.SystemError)
	}
	if record != nil && record.Status == model.StatusActive {
		return nil, errx.NewWithCode(errx.AlreadyLiked)
	}

	if record == nil {
		_, err = l.svcCtx.LikeRecordModel.Insert(l.ctx, &model.LikeRecord{
			UserId:     in.UserId,
			TargetId:   in.TargetId,
			TargetType: int64(in.TargetType),
			Status:     model.StatusActive,
		})
	} else {
		record.Status = model.StatusActive
		err = l.svcCtx.LikeRecordModel.Update(l.ctx, record)
	}
	if err != nil {
		l.Logger.Errorf("persist like record failed: %v", err)
		return nil, errx.NewWithCode(errx.SystemError)
	}

	if err := l.incrLikeCount(in.TargetId, int64(in.TargetType)); err != nil {
		l.Logger.Errorf("increase like count failed: %v", err)
		return nil, errx.NewWithCode(errx.SystemError)
	}

	return &pb.LikeResp{}, nil
}

func (l *LikeLogic) incrLikeCount(targetID, targetType int64) error {
	if l.svcCtx.ActionCountModel == nil {
		return nil
	}

	if err := l.svcCtx.ActionCountModel.IncrLikeCount(l.ctx, targetID, targetType); err != nil {
		return err
	}

	// 更新缓存需要读取最新值
	count, err := l.svcCtx.ActionCountModel.FindOneByTarget(l.ctx, targetID, targetType)
	if err != nil {
		return err
	}
	l.syncLikeCountCache(count)
	return nil
}

func (l *LikeLogic) syncLikeCountCache(count *model.ActionCount) {
	store := l.svcCtx.RedisStore
	if store == nil && l.svcCtx.Redis != nil {
		store = svc.NewRedisStore(l.svcCtx.Redis)
	}
	if store == nil {
		return
	}

	key := fmt.Sprintf("action_count:%d:%d", count.TargetId, count.TargetType)
	if err := store.Hset(key, "like_count", fmt.Sprintf("%d", count.LikeCount)); err != nil {
		l.Logger.Errorf("sync like_count cache failed: %v", err)
	}
	if err := store.Hset(key, "favorite_count", fmt.Sprintf("%d", count.FavoriteCount)); err != nil {
		l.Logger.Errorf("sync favorite_count cache failed: %v", err)
	}
	if err := store.Hset(key, "comment_count", fmt.Sprintf("%d", count.CommentCount)); err != nil {
		l.Logger.Errorf("sync comment_count cache failed: %v", err)
	}
	if err := store.Hset(key, "share_count", fmt.Sprintf("%d", count.ShareCount)); err != nil {
		l.Logger.Errorf("sync share_count cache failed: %v", err)
	}
	if err := store.Expire(key, model.CacheLongTTL); err != nil {
		l.Logger.Errorf("set cache expire failed: %v", err)
	}
}
```

- [ ] **Step 2: 修改 unlike_logic.go - 使用 StatusInactive 常量、原子更新、错误包装、Redis 日志**

```go
func (l *UnlikeLogic) Unlike(in *pb.UnlikeReq) (*pb.UnlikeResp, error) {
	record, err := l.svcCtx.LikeRecordModel.FindOneByUserIdTargetIdTargetType(l.ctx, in.UserId, in.TargetId, int64(in.TargetType))
	if errors.Is(err, model.ErrNotFound) {
		return nil, errx.NewWithCode(errx.NotLikedYet)
	}
	if err != nil {
		l.Logger.Errorf("find like record failed: %v", err)
		return nil, errx.NewWithCode(errx.SystemError)
	}
	if record.Status == model.StatusInactive {
		return nil, errx.NewWithCode(errx.NotLikedYet)
	}

	record.Status = model.StatusInactive
	if err := l.svcCtx.LikeRecordModel.Update(l.ctx, record); err != nil {
		l.Logger.Errorf("update like record failed: %v", err)
		return nil, errx.NewWithCode(errx.SystemError)
	}

	if err := l.decrLikeCount(in.TargetId, int64(in.TargetType)); err != nil {
		l.Logger.Errorf("decrease like count failed: %v", err)
		return nil, errx.NewWithCode(errx.SystemError)
	}

	return &pb.UnlikeResp{}, nil
}

func (l *UnlikeLogic) decrLikeCount(targetID, targetType int64) error {
	if l.svcCtx.ActionCountModel == nil {
		return nil
	}

	if err := l.svcCtx.ActionCountModel.DecrLikeCount(l.ctx, targetID, targetType); err != nil {
		return err
	}

	count, err := l.svcCtx.ActionCountModel.FindOneByTarget(l.ctx, targetID, targetType)
	if err != nil {
		return err
	}
	l.syncLikeCountCache(count)
	return nil
}
```

同步方法的 Redis 错误日志同 like_logic.go 模式。

- [ ] **Step 3: 运行测试**

Run: `go test ./app/interaction/internal/logic/... -run "Like|Unlike" -v`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add app/interaction/internal/logic/like_logic.go app/interaction/internal/logic/unlike_logic.go
git commit -m "fix(interaction): atomic like/unlike count updates and error wrapping

- 使用 IncrLikeCount/DecrLikeCount 原子方法替代 RMW
- Status 使用 model.StatusActive/StatusInactive 常量
- 计数更新失败现在返回错误而非仅记录日志
- Redis 缓存同步错误现在记录日志
- HIGH[H1,H3,H5,H6] MEDIUM[M1,M3] LOW[L1,L3]"
```

---

## Task 5: HIGH - 修复 Favorite/Unfavorite 计数原子更新与错误包装

**Files:**
- Modify: `app/interaction/internal/logic/favorite_logic.go`
- Modify: `app/interaction/internal/logic/unfavorite_logic.go`

- [ ] **Step 1: 修改 favorite_logic.go - 使用常量、原子更新、错误包装、Redis 日志**

```go
func (l *FavoriteLogic) Favorite(in *pb.FavoriteReq) (*pb.FavoriteResp, error) {
	record, err := l.svcCtx.FavoriteModel.FindOneByUserIdPostId(l.ctx, in.UserId, in.PostId)
	if err != nil && !errors.Is(err, model.ErrNotFound) {
		l.Logger.Errorf("find favorite record failed: %v", err)
		return nil, errx.NewWithCode(errx.SystemError)
	}
	if record != nil && record.Status == model.StatusActive {
		return nil, errx.NewWithCode(errx.AlreadyFavorited)
	}

	if record == nil {
		_, err = l.svcCtx.FavoriteModel.Insert(l.ctx, &model.Favorite{
			UserId:   in.UserId,
			PostId:   in.PostId,
			FolderId: sql.NullInt64{},
			Status:   model.StatusActive,
		})
	} else {
		record.Status = model.StatusActive
		err = l.svcCtx.FavoriteModel.Update(l.ctx, record)
	}
	if err != nil {
		l.Logger.Errorf("persist favorite record failed: %v", err)
		return nil, errx.NewWithCode(errx.SystemError)
	}

	if err := l.incrFavoriteCount(in.PostId); err != nil {
		l.Logger.Errorf("increase favorite count failed: %v", err)
		return nil, errx.NewWithCode(errx.SystemError)
	}

	return &pb.FavoriteResp{}, nil
}

func (l *FavoriteLogic) incrFavoriteCount(postID int64) error {
	if l.svcCtx.ActionCountModel == nil {
		return nil
	}

	if err := l.svcCtx.ActionCountModel.IncrFavoriteCount(l.ctx, postID, 1); err != nil {
		return err
	}

	count, err := l.svcCtx.ActionCountModel.FindOneByTarget(l.ctx, postID, 1)
	if err != nil {
		return err
	}
	l.syncFavoriteCountCache(count)
	return nil
}
```

同步方法的 Redis 错误日志同 like_logic.go 模式，使用 `model.CacheLongTTL`。

- [ ] **Step 2: 修改 unfavorite_logic.go - 使用常量、原子更新、错误包装、Redis 日志**

```go
func (l *UnfavoriteLogic) Unfavorite(in *pb.UnfavoriteReq) (*pb.UnfavoriteResp, error) {
	record, err := l.svcCtx.FavoriteModel.FindOneByUserIdPostId(l.ctx, in.UserId, in.PostId)
	if errors.Is(err, model.ErrNotFound) {
		return nil, errx.NewWithCode(errx.NotFavoritedYet)
	}
	if err != nil {
		l.Logger.Errorf("find favorite record failed: %v", err)
		return nil, errx.NewWithCode(errx.SystemError)
	}
	if record.Status == model.StatusInactive {
		return nil, errx.NewWithCode(errx.NotFavoritedYet)
	}

	record.Status = model.StatusInactive
	if err := l.svcCtx.FavoriteModel.Update(l.ctx, record); err != nil {
		l.Logger.Errorf("update favorite record failed: %v", err)
		return nil, errx.NewWithCode(errx.SystemError)
	}

	if err := l.decrFavoriteCount(in.PostId); err != nil {
		l.Logger.Errorf("decrease favorite count failed: %v", err)
		return nil, errx.NewWithCode(errx.SystemError)
	}

	return &pb.UnfavoriteResp{}, nil
}

func (l *UnfavoriteLogic) decrFavoriteCount(postID int64) error {
	if l.svcCtx.ActionCountModel == nil {
		return nil
	}

	if err := l.svcCtx.ActionCountModel.DecrFavoriteCount(l.ctx, postID, 1); err != nil {
		return err
	}

	count, err := l.svcCtx.ActionCountModel.FindOneByTarget(l.ctx, postID, 1)
	if err != nil {
		return err
	}
	l.syncFavoriteCountCache(count)
	return nil
}
```

- [ ] **Step 3: 运行测试**

Run: `go test ./app/interaction/internal/logic/... -run "Favorite|Unfavorite" -v`
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add app/interaction/internal/logic/favorite_logic.go app/interaction/internal/logic/unfavorite_logic.go
git commit -m "fix(interaction): atomic favorite/unfavorite count updates and error wrapping

- 使用 IncrFavoriteCount/DecrFavoriteCount 原子方法替代 RMW
- Status 使用 model.StatusActive/StatusInactive 常量
- 计数更新失败现在返回错误
- Redis 缓存同步错误现在记录日志
- HIGH[H2,H4,H5,H6] MEDIUM[M1,M3] LOW[L2,L4]"
```

---

## Task 6: HIGH - 修复查询类 Logic 错误包装

**Files:**
- Modify: `app/interaction/internal/logic/check_favorited_logic.go:35`
- Modify: `app/interaction/internal/logic/check_liked_logic.go:35`
- Modify: `app/interaction/internal/logic/get_favorite_list_logic.go:37`
- Modify: `app/interaction/internal/logic/get_counts_logic.go:64`
- Modify: `app/interaction/internal/logic/batch_check_favorited_logic.go:34`
- Modify: `app/interaction/internal/logic/batch_check_liked_logic.go:34`

- [ ] **Step 1: 修改 check_favorited_logic.go**

```go
	if err != nil {
		l.Logger.Errorf("check favorited failed: %v", err)
		return nil, errx.NewWithCode(errx.SystemError)
	}
```

- [ ] **Step 2: 修改 check_liked_logic.go**

```go
	if err != nil {
		l.Logger.Errorf("check liked failed: %v", err)
		return nil, errx.NewWithCode(errx.SystemError)
	}
```

- [ ] **Step 3: 修改 get_favorite_list_logic.go**

```go
	if err != nil {
		l.Logger.Errorf("get favorite list failed: %v", err)
		return nil, errx.NewWithCode(errx.SystemError)
	}
```

- [ ] **Step 4: 修改 get_counts_logic.go**

```go
	if err != nil {
		l.Logger.Errorf("get counts failed: %v", err)
		return nil, errx.NewWithCode(errx.SystemError)
	}
```

同时修改 singleflight key 添加前缀：

```go
	key := fmt.Sprintf("interaction:action_count:%d:%d", in.TargetId, in.TargetType)
```

修改 writeCountsToCache 使用常量：

```go
	l.writeCountsToCache(key, &model.ActionCount{TargetId: in.TargetId, TargetType: int64(in.TargetType)}, model.CacheShortTTL)
	// ...
	l.writeCountsToCache(key, count, model.CacheLongTTL)
```

修改 writeCountsToCache 记录 Redis 错误：

```go
func (l *GetCountsLogic) writeCountsToCache(key string, count *model.ActionCount, ttlSeconds int) {
	store := l.redisStore()
	if store == nil {
		return
	}

	if err := store.Hset(key, "like_count", fmt.Sprintf("%d", count.LikeCount)); err != nil {
		l.Logger.Errorf("write like_count cache failed: %v", err)
	}
	if err := store.Hset(key, "favorite_count", fmt.Sprintf("%d", count.FavoriteCount)); err != nil {
		l.Logger.Errorf("write favorite_count cache failed: %v", err)
	}
	if err := store.Hset(key, "comment_count", fmt.Sprintf("%d", count.CommentCount)); err != nil {
		l.Logger.Errorf("write comment_count cache failed: %v", err)
	}
	if err := store.Hset(key, "share_count", fmt.Sprintf("%d", count.ShareCount)); err != nil {
		l.Logger.Errorf("write share_count cache failed: %v", err)
	}
	if err := store.Expire(key, ttlSeconds); err != nil {
		l.Logger.Errorf("set cache expire failed: %v", err)
	}
}
```

修改 parseInt64 添加日志：

```go
func parseInt64(value string) int64 {
	parsed, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		logx.Errorf("parseInt64 failed: value=%s, err=%v", value, err)
		return 0
	}
	return parsed
}
```

注意：由于 `parseInt64` 不是方法，没有 `l.Logger`，使用 `logx.Errorf`。

- [ ] **Step 5: 修改 batch_check_favorited_logic.go**

```go
		if err != nil {
			l.Logger.Errorf("batch check favorited failed for post %d: %v", postID, err)
			return nil, errx.NewWithCode(errx.SystemError)
		}
```

- [ ] **Step 6: 修改 batch_check_liked_logic.go**

```go
		if err != nil {
			l.Logger.Errorf("batch check liked failed for target %d: %v", targetID, err)
			return nil, errx.NewWithCode(errx.SystemError)
		}
```

- [ ] **Step 7: 运行测试**

Run: `go test ./app/interaction/internal/logic/... -v`
Expected: PASS

- [ ] **Step 8: Commit**

```bash
git add app/interaction/internal/logic/check_favorited_logic.go \
  app/interaction/internal/logic/check_liked_logic.go \
  app/interaction/internal/logic/get_favorite_list_logic.go \
  app/interaction/internal/logic/get_counts_logic.go \
  app/interaction/internal/logic/batch_check_favorited_logic.go \
  app/interaction/internal/logic/batch_check_liked_logic.go
git commit -m "fix(interaction): wrap bare errors in query logics

- 所有查询类 Logic 的数据库错误统一包装为 errx.SystemError
- singleflight key 添加 interaction: 前缀避免冲突
- parseInt64 解析失败记录日志
- Redis 缓存写入失败记录日志
- 使用 CacheShortTTL/CacheLongTTL 常量
- HIGH[H5-H10] MEDIUM[M2,M4] LOW[L5,L7]"
```

---

## Task 7: MEDIUM - ActionCount Insert 使用雪花 ID

**Files:**
- Modify: `app/interaction/internal/model/action_count_model.go`

- [ ] **Step 1: 修改 Insert 方法生成雪花 ID**

```go
import (
	"context"
	"database/sql"
	"fmt"

	"esx/pkg/util"

	"github.com/zeromicro/go-zero/core/stores/sqlx"
)
```

```go
func (m *customActionCountModel) Insert(ctx context.Context, data *ActionCount) (sql.Result, error) {
	if data.Id == 0 {
		data.Id = util.GenId()
	}
	query := fmt.Sprintf("insert into %s (`id`, `target_id`, `target_type`, `like_count`, `favorite_count`, `comment_count`, `share_count`) values (?, ?, ?, ?, ?, ?, ?)", m.table)
	return m.conn.ExecCtx(ctx, query, data.Id, data.TargetId, data.TargetType, data.LikeCount, data.FavoriteCount, data.CommentCount, data.ShareCount)
}
```

- [ ] **Step 2: 运行编译检查**

Run: `go build ./app/interaction/...`
Expected: 编译通过

- [ ] **Step 3: Commit**

```bash
git add app/interaction/internal/model/action_count_model.go
git commit -m "fix(interaction/model): use snowflake ID for ActionCount insert

- Insert 前自动调用 util.GenId() 生成雪花 ID
- 避免依赖数据库自增 ID，便于分库分表
- MEDIUM[M5]"
```

---

## Task 8: LOW - 集成测试 SQL 拼接改进

**Files:**
- Modify: `app/interaction/internal/logic/integration_test.go:77`

- [ ] **Step 1: 使用 fmt.Sprintf 替代字符串拼接**

```go
func resetIntegrationState() {
	for _, table := range []string{"like_record", "favorite", "action_count", "favorite_folder", "view_history", "report"} {
		if _, err := testDB.Exec(fmt.Sprintf("DELETE FROM `%s`", table)); err != nil {
			fmt.Fprintf(os.Stderr, "清理 %s 失败: %v\n", table, err)
			os.Exit(1)
		}
	}
	// ...
}
```

- [ ] **Step 2: Commit**

```bash
git add app/interaction/internal/logic/integration_test.go
git commit -m "style(interaction/test): use fmt.Sprintf for SQL in tests

- LOW[L6]"
```

---

## Task 9: 最终验证

- [ ] **Step 1: 全量单元测试**

Run: `go test ./app/interaction/internal/logic/... -v`
Expected: ALL PASS

- [ ] **Step 2: 编译检查**

Run: `go build ./app/interaction/...`
Expected: 编译通过

- [ ] **Step 3: 静态检查**

Run: `go vet ./app/interaction/...`
Expected: 无问题

- [ ] **Step 4: 运行 goimports 检查格式**

Run: `goimports -l app/interaction/internal/logic/*.go app/interaction/internal/model/*.go`
Expected: 无输出（所有文件已格式化）

- [ ] **Step 5: 最终 Commit（如需要）**

如有任何格式修复：
```bash
git commit -m "style(interaction): goimports formatting"
```

---

## Self-Review Checklist

**1. Spec coverage:**
- [x] C1 硬编码密码 → Task 1
- [x] H1-H4 RMW 反模式 → Task 2 + Task 4 + Task 5
- [x] H5-H10 裸错误 → Task 4 + Task 5 + Task 6
- [x] M1 魔法数字 Status → Task 3 + Task 4 + Task 5
- [x] M2 缓存 TTL 魔法数字 → Task 3 + Task 6
- [x] M3 计数错误不返回 → Task 4 + Task 5
- [x] M4 parseInt64 静默失败 → Task 6
- [x] M5 雪花 ID → Task 7
- [x] L1-L5 Redis 错误忽略 → Task 4 + Task 5 + Task 6
- [x] L6 SQL 拼接 → Task 8
- [x] L7 singleflight key 前缀 → Task 6

**2. Placeholder scan:** 无 TBD/TODO/占位符。

**3. Type consistency:**
- `model.StatusActive` / `model.StatusInactive` 在所有文件一致
- `model.CacheShortTTL` / `model.CacheLongTTL` 在所有文件一致
- `errx.NewWithCode(errx.SystemError)` 包装模式一致

---

## Execution Handoff

**Plan complete and saved to `docs/superpowers/plans/2026-04-22-interaction-hotfix.md`.**

**Two execution options:**

**1. Subagent-Driven (recommended)** - Dispatch a fresh subagent per task, review between tasks, fast iteration

**2. Inline Execution** - Execute tasks in this session using executing-plans, batch execution with checkpoints for review

**Which approach?**
