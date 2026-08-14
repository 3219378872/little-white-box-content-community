---
implementation: IMP-content-community-backend
verified_at: 2026-08-14
verified_commit: 07fc700
commands:
  - python3 -m unittest scripts.test_engineering_lint
  - python3 scripts/engineering-lint.py
  - make check
  - make coverage-no-gate
result: passed
---

# 2026-08-14 文档治理批次（证据登记检查 + fmt 回归修复）

## 本批次改动

- `scripts/engineering-lint.py`：`_check_evidence_pages` 新增两项检查——
  1) `evidence/` 下每份证据文件必须登记在 `README.md`（防止证据文件漏登记）；
  2) README 中链接必须指向存在的文件（防死链）。
- `scripts/test_engineering_lint.py`：为两项新检查补充单测
  （`test_evidence_must_be_registered_in_readme`、
  `test_evidence_readme_dead_link_fails`），测试夹具同步写入 README。
- `docs/knowledge/implementation/evidence/README.md`：补登记上一轮漏掉的
  `2026-08-14-content-community-quality-review.md`，并登记本证据。
- `app/content/rpc/internal/logic/page_test.go`：gofmt 修复（上一轮 7af6c6a 提交时
  未格式化，导致 main 上 `make fmt-check` 自该提交起失败）。
- `IMP-architecture`、`IMP-engineering-conventions`、`IMP-development-quickstart`：
  内容经本轮复查与代码/Makefile 一致，刷新 `verified_at/verified_commit`。

## 审查发现（本轮复查）

- **生成同步验证（正向）**：在临时 worktree 按 `scripts/generate.sh` 的实际命令
  重新生成 11 个 protoc 服务 + user proto + gateway API，`git status` 零差异；
  唯一新增文件是 goctl 的 `optionalauth_middleware.go` 占位（仓库有意用共享
  `pkg/middleware` 实现替代，不提交占位）。生成代码与 `.proto`/`.api` 一致。
- **fmt 回归（已修复）**：`main@07fc700` 上 `make check` 的 `fmt-check` 失败，
  根因是 7af6c6a 提交的 `page_test.go` 未通过 gofmt（结构体字段对齐）。上一轮证据
  `2026-08-14-content-community-quality-review.md` 的 `make check 通过` 表述
  不准确；按治理规则不回写旧证据，本证据记录修复与修正。
- **证据登记缺口（已修复）**：上一轮新增证据未登记进 `README.md`，且 lint 不检查
  登记；本轮补登记并加检查。
- **覆盖率**：`make coverage-no-gate` 结果与上轮一致——handler 89.0%、logic 78.7%、
  model 12.2%、mq_consumer 74.4%、shared 45.9%，全部高于 baseline 门槛；model 层
  低覆盖率按设计由集成测试承载（未引入 sqlmock 依赖）。

## 结果

- `python3 -m unittest scripts.test_engineering_lint`：18 项全过。
- `python3 scripts/engineering-lint.py`：all checks passed。
- `make check`：本批次修复后通过（fmt、engineering-lint、vet、golangci-lint）。
- `make coverage-no-gate`：各层均高于 baseline。
