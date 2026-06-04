# 查询性能修复计划

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development or superpowers:executing-plans.

**Goal:** 将 Interaction 批量检查改为单次 IN 查询，为所有批量 ID 接口增加上限校验。

**Architecture:** Model 层新增批量查询方法，Logic 层直接调用；Gateway 和 Content 层在入口处校验 ID 数量。

**Tech Stack:** Go 1.26.1, go-zero, MySQL 8.0

---

## 文件变更总览

| 文件 | 变更 |
|------|------|
| `app/interaction/internal/logic/batch_check_liked_logic.go` | 改为单次批量查询 |
| `app/interaction/internal/logic/batch_check_favorited_logic.go` | 改为单次批量查询 |
| `app/content/internal/logic/get_posts_by_ids_logic.go` | 增加 ID 上限校验 |
| `app/gateway/internal/logic/posts/get_post_list_logic.go` | 增加 ID 上限校验（若存在批量接口） |
| `pkg/validator/limits.go` | 新增 `MaxBatchQueryIds` 常量 |

---

### Task 1: 重写 BatchCheckLikedLogic

**Files:**
- Modify: `app/interaction/internal/logic/batch_check_liked_logic.go`

- [ ] **Step 1: 修改 `BatchCheckLiked` 方法**

```go
func (l *BatchCheckLikedLogic) BatchCheckLiked(in *pb.BatchCheckLikedReq) (*pb.BatchCheckLikedResp, error) {
	if in.UserId <= 0 {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	if len(in.TargetIds) > validator.MaxBatchQueryIds {
		return nil, errx.NewWithCode(errx.ParamError)
	}

	results := make(map[int64]bool, len(in.TargetIds))
	if len(in.TargetIds) == 0 {
		return &pb.BatchCheckLikedResp{Results: results}, nil
	}

	statusMap, err := l.svcCtx.LikeRecordModel.FindStatusByUserAndTargets(l.ctx, in.UserId, in.TargetIds, int64(in.TargetType))
	if err != nil {
		l.Logger.Errorw("FindStatusByUserAndTargets failed",
			logx.Field("userId", in.UserId),
			logx.Field("err", err.Error()),
		)
		return nil, errx.NewWithCode(errx.SystemError)
	}

	for _, targetID := range in.TargetIds {
		results[targetID] = statusMap[targetID]
	}

	return &pb.BatchCheckLikedResp{Results: results}, nil
}
```

需要添加 import：`"validator"`（或项目中的校验包路径）。

- [ ] **Step 2: Commit**

```bash
git add app/interaction/internal/logic/batch_check_liked_logic.go
git commit -m "perf(interaction): replace N+1 with single query in BatchCheckLiked

- Use FindStatusByUserAndTargets for batch IN query
- Add MaxBatchQueryIds limit

Fixes H5"
```

---

### Task 2: 重写 BatchCheckFavoritedLogic

**Files:**
- Modify: `app/interaction/internal/logic/batch_check_favorited_logic.go`

- [ ] **Step 1: 修改 `BatchCheckFavorited` 方法**

模式与 BatchCheckLiked 完全一致，使用 `FindFavoriteStatusByUserAndPosts`：

```go
func (l *BatchCheckFavoritedLogic) BatchCheckFavorited(in *pb.BatchCheckFavoritedReq) (*pb.BatchCheckFavoritedResp, error) {
	if in.UserId <= 0 {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	if len(in.PostIds) > validator.MaxBatchQueryIds {
		return nil, errx.NewWithCode(errx.ParamError)
	}

	results := make(map[int64]bool, len(in.PostIds))
	if len(in.PostIds) == 0 {
		return &pb.BatchCheckFavoritedResp{Results: results}, nil
	}

	statusMap, err := l.svcCtx.FavoriteModel.FindFavoriteStatusByUserAndPosts(l.ctx, in.UserId, in.PostIds)
	if err != nil {
		l.Logger.Errorw("FindFavoriteStatusByUserAndPosts failed", logx.Field("err", err.Error()))
		return nil, errx.NewWithCode(errx.SystemError)
	}

	for _, postID := range in.PostIds {
		results[postID] = statusMap[postID]
	}

	return &pb.BatchCheckFavoritedResp{Results: results}, nil
}
```

- [ ] **Step 2: Commit**

```bash
git add app/interaction/internal/logic/batch_check_favorited_logic.go
git commit -m "perf(interaction): replace N+1 with single query in BatchCheckFavorited

Fixes H5"
```

---

### Task 3: 批量 ID 上限校验

**Files:**
- Create: `pkg/validator/limits.go`
- Modify: `app/content/internal/logic/get_posts_by_ids_logic.go`

- [ ] **Step 1: 创建 `pkg/validator/limits.go`**

```go
package validator

const MaxBatchQueryIds = 100
```

- [ ] **Step 2: 在 `get_posts_by_ids_logic.go` 添加上限校验**

```go
func (l *GetPostsByIdsLogic) GetPostsByIds(in *pb.GetPostsByIdsReq) (*pb.GetPostsByIdsResp, error) {
	if len(in.Ids) == 0 || len(in.Ids) > validator.MaxBatchQueryIds {
		return nil, errx.NewWithCode(errx.ParamError)
	}
	// ... existing logic ...
}
```

- [ ] **Step 3: Commit**

```bash
git add pkg/validator/limits.go app/content/internal/logic/get_posts_by_ids_logic.go
git commit -m "fix(content): limit batch query IDs to prevent abuse

- Add MaxBatchQueryIds = 100
- Reject requests with empty or oversized ID lists

Fixes H6"
```

---

## 验证清单

- [ ] `go test ./app/interaction/... ./app/content/... -race -cover` 通过
- [ ] BatchCheck 测试验证仅产生 1 次查询
