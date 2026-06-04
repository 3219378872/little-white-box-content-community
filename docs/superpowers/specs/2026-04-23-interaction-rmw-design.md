# Interaction RMW 竞态修复设计

**日期**: 2026-04-23
**范围**: app/interaction
**关联审查项**: H1 (RMW 竞态)

---

## 问题描述

`Like` / `Unlike` / `Favorite` / `Unfavorite` 逻辑层均采用 `FindOne → 修改 Status → Update` 的 Read-Modify-Write 模式：

1. `FindOneByUserIdTargetIdTargetType` 查询记录
2. 内存中修改 `record.Status`
3. `Update` 全字段写回数据库

并发场景下（如用户快速双击点赞、或网络重试），两个请求可能同时执行到步骤1获得相同记录，随后各自在步骤2修改状态并执行步骤3。后执行的 `Update` 会覆盖先执行的结果，导致：
- 重复插入记录（唯一索引冲突，或产生重复数据）
- 计数与状态不一致（如状态为已点赞但计数未增加）
- `syncLikeCountCache` 基于查询结果写入 Redis，进一步放大不一致

## 目标

消除 Interaction 模块所有写操作的 RMW 竞态，确保并发下状态与计数的一致性。

## 方案

### 1. Like / Favorite — 替换为 UPSERT

当前逻辑：
```
if record == nil { Insert(...) } else { record.Status = Active; Update(record) }
```

改为数据库原生 UPSERT（MySQL `INSERT ... ON DUPLICATE KEY UPDATE`）：

```sql
INSERT INTO like_record (user_id, target_id, target_type, status)
VALUES (?, ?, ?, 1)
ON DUPLICATE KEY UPDATE status = VALUES(status)
```

- 利用 `(user_id, target_id, target_type)` 唯一索引保证幂等
- 单条原子语句，无 RMW 窗口
- 返回 `RowsAffected`：1 表示新插入，2 表示状态被更新（可用于判断是否需要递增计数）

### 2. Unlike / Unfavorite — 替换为条件 UPDATE

当前逻辑：
```
record.Status = Inactive
Update(record)
```

改为条件更新：
```sql
UPDATE like_record SET status = 0 WHERE id = ? AND status = 1
```

- `RowsAffected = 0` 表示已经是 Inactive（幂等，无需报错）
- `RowsAffected = 1` 表示成功取消，此时递减计数

### 3. 计数同步去耦合

当前 `incrLikeCount` / `decrLikeCount` 在事务后执行：
1. `IncrLikeCount` / `DecrLikeCount`（数据库原子更新）
2. `FindOneByTarget`（再次查询）
3. `syncLikeCountCache`（写 Redis）

步骤2-3 是额外的 RMW 来源。改为：
- 数据库计数由 `IncrLikeCount` / `DecrLikeCount` 保证原子性（已是原子操作，保留）
- 删除步骤2（`FindOneByTarget`）和步骤3（立即同步 Redis）
- Redis 缓存通过 `go-zero cache` 的 `CachedConn` 自动失效，或依赖短 TTL + 懒加载
- 若业务需要强一致实时计数，改为在数据库事务内完成计数更新，由调用方根据 UPSERT / UPDATE 的 `RowsAffected` 决定是否调用计数调整

### 4. 事务边界

UPSERT 和条件 UPDATE 均为单条 SQL，无需显式事务。计数调整（`IncrLikeCount`）作为独立原子操作在之后执行。即使计数调整失败，主状态已正确，计数可通过后台对账修复。

## 文件变更

| 文件 | 变更 |
|------|------|
| `app/interaction/internal/model/like_record_model.go` | 新增 `UpsertLikeStatus` 方法 |
| `app/interaction/internal/model/favorite_model.go` | 新增 `UpsertFavoriteStatus` 方法 |
| `app/interaction/internal/logic/like_logic.go` | 使用 UPSERT，根据 RowsAffected 决定是否递增计数 |
| `app/interaction/internal/logic/unlike_logic.go` | 使用条件 UPDATE，根据 RowsAffected 决定是否递减计数 |
| `app/interaction/internal/logic/favorite_logic.go` | 使用 UPSERT，根据 RowsAffected 决定是否递增计数 |
| `app/interaction/internal/logic/unfavorite_logic.go` | 使用条件 UPDATE，根据 RowsAffected 决定是否递减计数 |
| `app/interaction/internal/logic/like_logic_test.go` | 新增并发测试 |

## 验收标准

- `go test ./app/interaction/... -race -cover` 通过，覆盖率不降低
- 并发 Like 测试（2 个 goroutine 同时点赞同一目标）仅产生 1 条记录，计数为 1
- 并发 Unlike 测试（2 个 goroutine 同时取消点赞）计数最低为 0，不报错
