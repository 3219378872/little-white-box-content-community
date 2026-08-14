---
implementation: IMP-content-community-backend
verified_at: 2026-08-14
verified_commit: a100cc0
commands:
  - go test -count=1 ./app/gateway/internal/logic/...
  - make check
  - make test
result: passed
---

# 2026-08-14 gateway RPC 错误映射 helper 去重（DRY）

## 缺陷

gateway 的 search/message/feed/behavior 四个子包各自实现
`searchRPCError`/`messageRPCError`/`feedRPCError`/`behaviorRPCError`，
实现与 `pkg/errx.FromRPCError` **完全相同**（BizError 透传 + 非业务
错误 Wrap SystemError，CORE-054）——4 份重复，行为漂移风险。

## 改动

- 4 个子包共 12 处调用点改用 `errx.FromRPCError`；
- 删除 4 个重复 helper；清理未使用 import。

## 结果

- gateway internal/logic 12 包测试全过；`make check` 通过；`make test`
  全部模块通过（含 race）。

## 未覆盖边界

- assistant 的 `sendRPCError`（SSE 事件发送语义，非纯错误映射）保留；
  外部输入门禁不变，见
  [IMP-todo-blocked-gates](../IMP-todo-blocked-gates.md)。
