---
implementation: IMP-content-community-backend
verified_at: 2026-08-14
verified_commit: 42347e7
commands:
  - go test ./app/content/...
  - go test -tags integration ./app/content/rpc/internal/logic/
  - make check
  - make test
result: passed
---

# 2026-08-14 评论幂等命令哈希覆盖回复目标（CORE-050/051）

## 缺陷

`CreateComment` 的 `CommandHash` 只覆盖 content + post_id，未覆盖
`parent_id`（回复目标评论）与 `reply_user_id`（被回复用户）。同一调用者以
相同键、相同内容回复不同父评论（或不同被回复用户）会被当作"同一命令"静默
返回旧评论，而不是按 CORE-051 返回可区分的幂等冲突；与 2026-08-14 帖子/
媒体幂等哈希完整性修复同一方向。

## 修复

- `app/content/rpc/internal/logic/create_comment_logic.go`：提取
  `commentIdempotencyRecord(in)`，`CommandHash` 覆盖内容、帖子、回复目标
  评论与被回复用户。

## 测试

- 新增 `TestCommentIdempotencyRecord`：同键同命令 → 相同哈希；回复目标评论 /
  被回复用户 / 帖子 / 内容任一变化 → 哈希变化（异命令冲突语义）。

## 审查证据

- 幂等哈希完整性扫描（CORE-042/050/051）：帖子（status+媒体引用）、消息
  （media_id）、媒体（本次修复含文件内容）、评论（本次修复含 parent/reply）
  全部覆盖命令维度；媒体修复见 media-idem-hash。
- `go test ./app/content/...`、内容集成测试（MySQL/Redis testcontainers）
  全部通过；`make check` exit 0；`make test` 85 包 0 失败。

## 结果

- 评论同键异命令按 CORE-051 返回 409 幂等冲突，不再静默复用旧评论。

## 未覆盖边界

- 行为变更影响真实重试客户端：同键改变回复目标将得到 409，符合幂等契约；
  客户端重试应发送相同命令或更换键。
