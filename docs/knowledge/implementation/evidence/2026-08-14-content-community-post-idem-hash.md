---
implementation: IMP-content-community-backend
verified_at: 2026-08-14
verified_commit: 7964307
commands:
  - go test -count=1 ./app/content/rpc/internal/logic/
  - make check
  - make test
result: passed
---

# 2026-08-14 帖子幂等命令哈希完整性修复（CORE-050/051）

## 缺陷

`CreatePost` 的 `CommandHash` 只覆盖 title/content/images/tags，未覆盖
`status` 与 `MediaIds`。同一幂等键下改变 status（如 draft→published）
或媒体引用会被当作"同一命令"返回旧资源（旧草稿），而不是按
CORE-051 返回可区分的幂等冲突；这与 CORE-042 对消息"异命令（含不同
media_id）冲突"的语义不一致。

## 修复

- `create_post_logic.go`：`CommandHash` 追加排序去重后的媒体 ID 列表
  与 `status`。
- `convert.go`：新增 `sortedMediaIDs`（排序去重），媒体引用顺序无关。

## 测试

- 新增 `TestCreatePostCommandHashCoversStatusAndMediaIDs`：status 变化
  必须改变哈希；媒体顺序无关；媒体集合变化必须改变哈希。

## 审查证据（本轮深入扫描）

- assistant safety（归一化/扫描限制）、会话状态（Lua 原子性/归属/配额）、
  behavior publisher、recommend 特征存储（匿名不建画像/opt-out 清理）、
  feed fanout（BigV 阈值限制规模）、gateway SSE（EOF/完整性语义）、
  comment/media 幂等哈希（已覆盖各自全部命令参数）均审查通过。

## 结果

- `go test ./app/content/rpc/internal/logic/` 全过；`make check` 通过；
  `make test` 全部模块通过（含 race）。

## 未覆盖边界

- 既有幂等记录按旧哈希格式存储：升级后同键重试相同命令会因哈希变化
  得到 409 而非原资源（幂等键重试窗口通常极短，影响可忽略）；外部
  输入门禁不变，见 [IMP-todo-blocked-gates](../IMP-todo-blocked-gates.md)。
