---
implementation: IMP-content-community-backend
verified_at: 2026-08-14
verified_commit: 8e3524d
commands:
  - make vet
  - make lint
  - make check
  - bash -n scripts/*.sh
result: passed
---

# 2026-08-14 门禁脚本模块枚举去重（工具质量）

## 缺陷

`vet.sh`/`lint.sh`/`test.sh`/`coverage.sh`/`integration-test.sh` 各自
重复实现"枚举工作区 Go module"的 find 命令（5 处拷贝），且
`integration-test.sh` 的排除路径拼写为 `.worktrees`（复数）而实际
目录是 `.worktree`（单数）——一旦工作树内嵌套 go.mod，集成测试会
误扫；各脚本后续改动也容易漂移。

## 改动

- 新增共享库 `scripts/_lib.sh`：`list_modules()` 单一实现模块枚举
  （`.worktree` 单数，排除 vendor）。
- 5 个门禁脚本 `source` 共享库并调用 `list_modules()`，同时修正
  integration-test.sh 的路径拼写。

## 验证

- `bash -n` 语法全过；`make vet`/`make lint`/`make check` 全绿
  （模块枚举结果与重构前一致）。

## 未覆盖边界

- 外部输入门禁不变（ASST/REL/PROP/DISC-062 复评），见
  [IMP-todo-blocked-gates](../IMP-todo-blocked-gates.md)。
