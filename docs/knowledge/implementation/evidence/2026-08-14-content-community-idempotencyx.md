---
implementation: IMP-content-community-backend
verified_at: 2026-08-14
verified_commit: 0dbe2f1
commands:
  - go test -count=1 ./app/content/... ./app/media/...
  - make check
  - make test
result: passed
---

# 2026-08-14 幂等模型共享包提取（DRY 重构）

## 缺陷

content 与 media 两个 RPC 服务各自的幂等模型文件（`idempotency_model.go`）
是同一 root module 内的几乎相同复制（类型/命令哈希/唯一键并发回查/重复键判断），仅
函数名大小写与错误消息不同——CORE-050 幂等语义的两份实现易漂移。

## 改动

- 新增 `pkg/idempotencyx`：提取 `IdempotencyRecord`/`Valid`/
  `CommandHash`/`ResolveIdempotencySession`/`ErrIdempotencyConflict`
  与内部 findIdempotencySession/isDuplicateKeyError。
- content 的 post/comment command model 与 logic、media 的
  media_command_model 与 upload logic 改用共享包 `pkg/idempotencyx`；
  删除两份复制文件。
- 测试引用同步迁移。

## 结果

- content/media/interaction/message/pkg 测试全过；`make check` 通过；
  `make test` 全部模块通过（含 race）。

## 未覆盖边界

- 外部输入门禁不变（ASST/REL/PROP/DISC-062 复评），见
  [IMP-todo-blocked-gates](../IMP-todo-blocked-gates.md)。
