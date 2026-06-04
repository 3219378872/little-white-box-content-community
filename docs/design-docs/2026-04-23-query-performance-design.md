# 查询性能修复设计

**日期**: 2026-04-23
**范围**: app/interaction, app/content, app/gateway
**关联审查项**: H5 (N+1 查询), H6 (输入校验缺失 — 批量 ID 上限)

---

## 问题描述

### N+1 查询 (Interaction)

`BatchCheckLikedLogic` 和 `BatchCheckFavoritedLogic` 的实现：

```go
for _, targetID := range in.TargetIds {
    resp, err := NewCheckLikedLogic(...).CheckLiked(...) // 每条一次 DB 查询
}
```

当 `TargetIds` 长度为 100 时，会产生 100 次数据库查询 + 100 次 RPC 调用（若走 RPC）。

### 批量查询 ID 无上限 (Content / Gateway)

`GetPostsByIds` 接口未限制传入 ID 数量，可被恶意调用传入极大数组，导致：
- SQL `IN (...)` 子句过长，数据库拒绝执行
- 返回数据量过大，内存压力与网络拥塞

## 目标

1. `BatchCheckLiked` / `BatchCheckFavorited` 改为单次批量查询
2. 所有批量 ID 查询接口增加上限校验（`max 100`）

## 方案

### 1. Interaction 批量检查改为 IN 查询

在 `LikeRecordModel` / `FavoriteModel` 中新增批量查询方法：

```go
// FindStatusByUserAndTargets 批量查询用户对多个目标的点赞状态
func (m *customLikeRecordModel) FindStatusByUserAndTargets(
    ctx context.Context, userId int64, targetIds []int64, targetType int64,
) (map[int64]bool, error)
```

实现：
```sql
SELECT target_id, status FROM like_record
WHERE user_id = ? AND target_type = ? AND target_id IN (...)
```

返回 `map[int64]bool`（targetId → isActive）。未在结果中的 targetId 视为未点赞。

`BatchCheckLikedLogic` 改为直接调用此方法，不再循环调用 `CheckLikedLogic`。

### 2. 批量 ID 上限校验

在以下入口增加校验：

**Gateway 层**（`app/gateway/internal/logic/posts/get_post_list_logic.go` 等若存在批量接口）：
```go
if len(req.Ids) > 100 {
    return nil, errx.NewWithCode(errx.ParamError)
}
```

**Content RPC 层**（`app/content/internal/logic/get_posts_by_ids_logic.go`）：
```go
if len(in.Ids) == 0 || len(in.Ids) > 100 {
    return nil, errx.NewWithCode(errx.ParamError)
}
```

常量提取到 `pkg/validator` 或各模块配置中：
```go
const MaxBatchQueryIds = 100
```

## 文件变更

| 文件 | 变更 |
|------|------|
| `app/interaction/internal/model/like_record_model.go` | 新增 `FindStatusByUserAndTargets` |
| `app/interaction/internal/model/favorite_model.go` | 新增 `FindStatusByUserAndTargets` |
| `app/interaction/internal/logic/batch_check_liked_logic.go` | 改为单次批量查询 |
| `app/interaction/internal/logic/batch_check_favorited_logic.go` | 改为单次批量查询 |
| `app/content/internal/logic/get_posts_by_ids_logic.go` | 增加 ID 上限校验 |
| `app/gateway/internal/logic/posts/get_post_list_logic.go` | 增加 ID 上限校验（若适用） |
| `pkg/validator/limits.go` | 新增 `MaxBatchQueryIds` 常量 |

## 验收标准

- `BatchCheckLiked` 100 个 targetId 仅产生 1 次数据库查询
- 传入 101 个 ID 时返回 `ParamError`
- 所有新增代码覆盖率 >= 80%
