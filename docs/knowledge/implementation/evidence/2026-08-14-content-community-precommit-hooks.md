---
implementation: IMP-content-community-backend
verified_at: 2026-08-14
verified_commit: 6e1ce27
commands:
  - pre-commit validate-config
  - pre-commit run go-vet --all-files
  - pre-commit run python-unit --all-files
result: passed
---

# 2026-08-14 pre-commit 钩子补齐（工具质量）

## 缺陷

`.pre-commit-config.yaml` 只有 gofmt/golangci-lint/python-compile/
engineering-lint 四个钩子——`make check` 中的 `go vet` 与 Python 工具
单测（`make python-unit`）在本地 pre-commit 无对应钩子，本地提交前
覆盖与 CI/门禁不一致。

## 改动

- 新增 `go-vet` 钩子（scripts/vet.sh，types: [go]）。
- 新增 `python-unit` 钩子（make python-unit，scripts/*.py 触发）。

## 验证

- `pre-commit validate-config` 通过；`go-vet`/`python-unit`/
  `engineering-lint` 钩子 --all-files 运行通过。

## 未覆盖边界

- 外部输入门禁不变（ASST/REL/PROP/DISC-062 复评），见
  [IMP-todo-blocked-gates](../IMP-todo-blocked-gates.md)。
