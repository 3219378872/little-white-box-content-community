# Interaction RMW 竞态修复计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development or superpowers:executing-plans.

**Goal:** 消除 Interaction 模块 Like/Unlike/Favorite/Unfavorite 的 Read-Modify-Write 竞态，使用 UPSERT 和条件 UPDATE 替代 Find+Update。

**Architecture:** 数据库原生原子操作替代应用层 RMW；计数调整通过 RowsAffected 判断，删除实时缓存同步以消除二次 RMW。

**Tech Stack:** Go 1.26.1, go-zero, MySQL 8.0 (ON DUPLICATE KEY UPDATE)

---

## 文件变更总览

| 文件 | 变更 |
|------|------|
| `app/interaction/internal/model/like_record_model.go` | 新增 `UpsertLikeStatus` |
| `app/interaction/internal/model/like_record_model_gen.go` | 新增 `FindStatusByUserAndTargets` |
| `app/interaction/internal/model/favorite_model.go` | 新增 `UpsertFavoriteStatus`、`FindFavoriteStatusByUserAndPosts` |
| `app/interaction/internal/logic/like_logic.go` | 使用 UPSERT，按 RowsAffected 调计数 |
| `app/interaction/internal/logic/unlike_logic.go` | 使用条件 UPDATE，按 RowsAffected 调计数 |
| `app/interaction/internal/logic/favorite_logic.go` | 使用 UPSERT，按 RowsAffected 调计数 |
| `app/interaction/internal/logic/unfavorite_logic.go` | 使用条件 UPDATE，按 RowsAffected 调计数 |

---

### Task 1: LikeRecordModel UPSERT 与批量查询

**Files:**
- Modify: `app/interaction/internal/model/like_record_model.go`
- Modify: `app/interaction/internal/model/like_record_model_gen.go`

- [ ] **Step 1: 在 `LikeRecordModel` 接口中新增方法签名**

在 `app/interaction/internal/model/like_record_model.go` 中：

```go
type LikeRecordModel interface {
	likeRecordModel
	UpsertLikeStatus(ctx context.Context, userId, targetId, targetType, status int64) (sql.Result, error)
	FindStatusByUserAndTargets(ctx context.Context, userId int64, targetIds []int64, targetType int64) (map[int64]bool, error)
}
```

需要添加 import：`"context"`、`"database/sql"`、`"fmt"`、`"strings"`。

- [ ] **Step 2: 实现 `UpsertLikeStatus`**

在 `customLikeRecordModel` 中实现：

```go
func (m *customLikeRecordModel) UpsertLikeStatus(ctx context.Context, userId, targetId, targetType, status int64) (sql.Result, error) {
	query := fmt.Sprintf(
		"insert into %s (`user_id`,`target_id`,`target_type`,`status`) values (?,?,?,?) on duplicate key update `status`=values(`status`)",
		m.table,
	)
	return m.conn.ExecCtx(ctx, query, userId, targetId, targetType, status)
}
```

- [ ] **Step 3: 实现 `FindStatusByUserAndTargets`**

```go
func (m *customLikeRecordModel) FindStatusByUserAndTargets(ctx context.Context, userId int64, targetIds []int64, targetType int64) (map[int64]bool, error) {
	if len(targetIds) == 0 {
		return map[int64]bool{}, nil
	}
	placeholders := strings.Repeat("?,", len(targetIds))
	placeholders = placeholders[:len(placeholders)-1]
	args := make([]interface{}, 0, len(targetIds)+2)
	args = append(args, userId, targetType)
	for _, id := range targetIds {
		args = append(args, id)
	}

	var rows []struct {
		TargetId int64 `db:"target_id"`
		Status   int64 `db:"status"`
	}
	query := fmt.Sprintf("select `target_id`,`status` from %s where `user_id`=? and `target_type`=? and `target_id` in (%s)", m.table, placeholders)
	if err := m.CachedConn.QueryRowsNoCacheCtx(ctx, &rows, query, args...); err != nil {
		return nil, err
	}

	results := make(map[int64]bool, len(rows))
	for _, r := range rows {
		results[r.TargetId] = r.Status == StatusActive
	}
	return results, nil
}
```

- [ ] **Step 4: Commit**

```bash
git add app/interaction/internal/model/like_record_model.go
git commit -m "feat(interaction): add UpsertLikeStatus and FindStatusByUserAndTargets

- UpsertLikeStatus uses INSERT ... ON DUPLICATE KEY UPDATE for atomic like/unlike
- FindStatusByUserAndTargets enables batch status lookup with single query

Refs H1"
```

---

### Task 2: FavoriteModel UPSERT 与批量查询

**Files:**
- Modify: `app/interaction/internal/model/favorite_model.go`

- [ ] **Step 1: 在 `FavoriteModel` 接口中新增方法签名**

```go
type FavoriteModel interface {
	favoriteModel
	FindActivePostIds(ctx context.Context, userID int64, page, pageSize int32) ([]int64, int64, error)
	UpsertFavoriteStatus(ctx context.Context, userId, postId, status int64) (sql.Result, error)
	FindFavoriteStatusByUserAndPosts(ctx context.Context, userId int64, postIds []int64) (map[int64]bool, error)
}
```

- [ ] **Step 2: 实现 `UpsertFavoriteStatus`**

```go
func (m *customFavoriteModel) UpsertFavoriteStatus(ctx context.Context, userId, postId, status int64) (sql.Result, error) {
	query := fmt.Sprintf(
		"insert into %s (`user_id`,`post_id`,`status`) values (?,?,?) on duplicate key update `status`=values(`status`)",
		m.table,
	)
	return m.conn.ExecCtx(ctx, query, userId, postId, status)
}
```

- [ ] **Step 3: 实现 `FindFavoriteStatusByUserAndPosts`**

```go
func (m *customFavoriteModel) FindFavoriteStatusByUserAndPosts(ctx context.Context, userId int64, postIds []int64) (map[int64]bool, error) {
	if len(postIds) == 0 {
		return map[int64]bool{}, nil
	}
	placeholders := strings.Repeat("?,", len(postIds))
	placeholders = placeholders[:len(placeholders)-1]
	args := make([]interface{}, 0, len(postIds)+1)
	args = append(args, userId)
	for _, id := range postIds {
		args = append(args, id)
	}

	var rows []struct {
		PostId int64 `db:"post_id"`
		Status int64 `db:"status"`
	}
	query := fmt.Sprintf("select `post_id`,`status` from %s where `user_id`=? and `post_id` in (%s)", m.table, placeholders)
	if err := m.CachedConn.QueryRowsNoCacheCtx(ctx, &rows, query, args...); err != nil {
		return nil, err
	}

	results := make(map[int64]bool, len(rows))
	for _, r := range rows {
		results[r.PostId] = r.Status == StatusActive
	}
	return results, nil
}
```

- [ ] **Step 4: Commit**

```bash
git add app/interaction/internal/model/favorite_model.go
git commit -m "feat(interaction): add UpsertFavoriteStatus and batch query

- Atomic upsert for favorite status
- Batch favorite status lookup for N+1 fix

Refs H1"
```

---

### Task 3: 重写 LikeLogic 消除 RMW

**Files:**
- Modify: `app/interaction/internal/logic/like_logic.go`
- Test: `app/interaction/internal/logic/like_logic_test.go`

- [ ] **Step 1: 修改 `Like` 方法使用 UPSERT**

替换整个 `Like` 方法：

```go
func (l *LikeLogic) Like(in *pb.LikeReq) (*pb.LikeResp, error) {
	if in.UserId <= 0 || in.TargetId <= 0 {
		return nil, errx.NewWithCode(errx.ParamError)
	}

	result, err := l.svcCtx.LikeRecordModel.UpsertLikeStatus(l.ctx, in.UserId, in.TargetId, int64(in.TargetType), model.StatusActive)
	if err != nil {
		l.Logger.Errorw("UpsertLikeStatus failed",
			logx.Field("userId", in.UserId),
			logx.Field("targetId", in.TargetId),
			logx.Field("err", err.Error()),
		)
		return nil, errx.NewWithCode(errx.SystemError)
	}

	rowsAffected, _ := result.RowsAffected()
	// RowsAffected: 1 = inserted (new like), 2 = updated (was inactive)
	// Only increment count when the status actually changed to active
	if rowsAffected > 0 {
		if err := l.svcCtx.ActionCountModel.IncrLikeCount(l.ctx, in.TargetId, int64(in.TargetType)); err != nil {
			l.Logger.Errorw("IncrLikeCount failed",
				logx.Field("targetId", in.TargetId),
				logx.Field("err", err.Error()),
			)
			// Do not fail the request; count inconsistency can be repaired
		}
	} else {
		// Already active, return already liked
		return nil, errx.NewWithCode(errx.AlreadyLiked)
	}

	return &pb.LikeResp{}, nil
}
```

- [ ] **Step 2: 删除 `incrLikeCount` 和 `syncLikeCountCache` 方法**

这两个方法引入二次 RMW（先原子 incr，再 find，再写 redis）。删除后仅保留数据库原子 incr。

- [ ] **Step 3: 编写并发 Like 测试**

```go
func TestLikeLogic_ConcurrentLike(t *testing.T) {
	// Setup testcontainers MySQL + mock svcCtx
	// Spawn 2 goroutines calling Like for same (userId, targetId)
	// Verify only 1 like_record exists with status=1
	// Verify action_count.like_count = 1
}
```

- [ ] **Step 4: 运行测试**

Run: `go test ./app/interaction/internal/logic/... -v -run TestLikeLogic -race`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add app/interaction/internal/logic/like_logic.go
git commit -m "fix(interaction): replace Like RMW with UPSERT

- Use UpsertLikeStatus for atomic insert-or-update
- Increment count only when RowsAffected > 0
- Remove incrLikeCount and syncLikeCountCache to eliminate secondary RMW

Fixes H1"
```

---

### Task 4: 重写 UnlikeLogic 消除 RMW

**Files:**
- Modify: `app/interaction/internal/logic/unlike_logic.go`
- Test: `app/interaction/internal/logic/unlike_logic_test.go`

- [ ] **Step 1: 在 `LikeRecordModel` 接口中新增 `UpdateStatusById`**

在 `like_record_model.go` 中添加：

```go
type LikeRecordModel interface {
	likeRecordModel
	UpsertLikeStatus(ctx context.Context, userId, targetId, targetType, status int64) (sql.Result, error)
	FindStatusByUserAndTargets(ctx context.Context, userId int64, targetIds []int64, targetType int64) (map[int64]bool, error)
	UpdateStatusById(ctx context.Context, id, expectedStatus, newStatus int64) (sql.Result, error)
}
```

实现：
```go
func (m *customLikeRecordModel) UpdateStatusById(ctx context.Context, id, expectedStatus, newStatus int64) (sql.Result, error) {
	query := fmt.Sprintf("update %s set `status`=? where `id`=? and `status`=?, m.table)
	return m.conn.ExecCtx(ctx, query, newStatus, id, expectedStatus)
}
```

- [ ] **Step 2: 修改 `Unlike` 使用条件 UPDATE**

```go
func (l *UnlikeLogic) Unlike(in *pb.UnlikeReq) (*pb.UnlikeResp, error) {
	if in.UserId <= 0 || in.TargetId <= 0 {
		return nil, errx.NewWithCode(errx.ParamError)
	}

	record, err := l.svcCtx.LikeRecordModel.FindOneByUserIdTargetIdTargetType(l.ctx, in.UserId, in.TargetId, int64(in.TargetType))
	if errors.Is(err, model.ErrNotFound) {
		return nil, errx.NewWithCode(errx.NotLikedYet)
	}
	if err != nil {
		l.Logger.Errorw("FindOneByUserIdTargetIdTargetType failed", logx.Field("err", err.Error()))
		return nil, errx.NewWithCode(errx.SystemError)
	}
	if record.Status == model.StatusInactive {
		return nil, errx.NewWithCode(errx.NotLikedYet)
	}

	result, err := l.svcCtx.LikeRecordModel.UpdateStatusById(l.ctx, record.Id, model.StatusActive, model.StatusInactive)
	if err != nil {
		l.Logger.Errorw("UpdateStatusById failed", logx.Field("err", err.Error()))
		return nil, errx.NewWithCode(errx.SystemError)
	}

	rowsAffected, _ := result.RowsAffected()
	if rowsAffected > 0 {
		if err := l.svcCtx.ActionCountModel.DecrLikeCount(l.ctx, in.TargetId, int64(in.TargetType)); err != nil {
			l.Logger.Errorw("DecrLikeCount failed", logx.Field("err", err.Error()))
		}
	}

	return &pb.UnlikeResp{}, nil
}
```

- [ ] **Step 3: 删除 `decrLikeCount` 和 `syncLikeCountCache`**

- [ ] **Step 4: Commit**

```bash
git add app/interaction/internal/logic/unlike_logic.go app/interaction/internal/model/like_record_model.go
git commit -m "fix(interaction): replace Unlike RMW with conditional UPDATE

- Add UpdateStatusById with expectedStatus guard
- Decrement count only when RowsAffected > 0

Fixes H1"
```

---

### Task 5: 重写 FavoriteLogic / UnfavoriteLogic

**Files:**
- Modify: `app/interaction/internal/logic/favorite_logic.go`
- Modify: `app/interaction/internal/logic/unfavorite_logic.go`

按照 Task 3 / Task 4 的相同模式修改：
- `FavoriteLogic.Favorite` → `UpsertFavoriteStatus`，`RowsAffected > 0` 时 `IncrFavoriteCount`
- `UnfavoriteLogic.Unfavorite` → 先 `FindOneByUserIdPostId` 获取记录，再 `UpdateStatusById`（需要在 `favorite_model.go` 中添加），`RowsAffected > 0` 时 `DecrFavoriteCount`
- 删除两个文件中的 `incrFavoriteCount` / `decrFavoriteCount` / `syncFavoriteCountCache`

- [ ] **Step 1: 在 `FavoriteModel` 添加 `UpdateStatusById`**

```go
func (m *customFavoriteModel) UpdateStatusById(ctx context.Context, id, expectedStatus, newStatus int64) (sql.Result, error) {
	query := fmt.Sprintf("update %s set `status`=? where `id`=? and `status`=?, m.table)
	return m.conn.ExecCtx(ctx, query, newStatus, id, expectedStatus)
}
```

- [ ] **Step 2: 修改 `favorite_logic.go` 和 `unfavorite_logic.go`**

（代码模式与 Like/Unlike 一致，略。具体代码参考 Task 3/4。）

- [ ] **Step 3: Commit**

```bash
git add app/interaction/internal/logic/favorite_logic.go app/interaction/internal/logic/unfavorite_logic.go app/interaction/internal/model/favorite_model.go
git commit -m "fix(interaction): replace Favorite/Unfavorite RMW with atomic ops

Fixes H1"
```

---

## 验证清单

- [ ] `go test ./app/interaction/... -race -cover` 通过
- [ ] 覆盖率 >= 80%
