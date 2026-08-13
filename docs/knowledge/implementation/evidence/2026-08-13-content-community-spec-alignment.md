---
implementation: IMP-content-community-backend
verified_at: 2026-08-13
verified_commit: 314aa67
commands:
  - make check
  - make test
  - make coverage
  - make integration-critical
  - make spec-evals-test
result: passed
---

# 2026-08-13 规格对齐批次验证（门禁恢复 + REL-023 主动清理）

## 环境

- Go 1.26.1（GOTOOLCHAIN=go1.26.1）；分支 `task/spec-alignment-batch1`，验证提交 `314aa67`
  （rebase 到 main 后 SHA 不变）；testcontainers 启动 MySQL 8.0 + Redis 7 用于集成测试。

## 本批次改动

- 修复 `make check`：`recommend_logic_test.go` goimports 分组错误；`vet/lint/test/coverage`
  脚本与 `.golangci.yml` 的 `.worktrees/` → `.worktree/` 路径排除修正，本地门禁不再扫描
  残留任务工作树。
- 修复 `make coverage`：网关 REST 决策表新增 RPC 失败与版本冲突用例（CORE-051 可区分
  业务结果），handler 层覆盖率 87.5% → 89.3%（基线 88.0%）。
- REL-023：recommend-mq 新增定时主动清理（`PurgeOptedOutFeatures` + main ticker，
  默认 1h 周期），删除已关闭个性化用户的在线特征不再依赖其下一次行为事件；
  配置 `OptOutCleanupInterval`（0 禁用）。
- 文档：IMP 台账新增 21 条验收标准（A0x）追踪；REL-023 行补充主动清理说明。

## 结果

- `make check` 通过（fmt-check、engineering-lint 16 测试+脚本、vet、golangci-lint 0 issues）。
- `make test` 全部模块通过（含 race）。
- `make coverage` 通过：handler 89.3%（≥88.0）、logic 78.8%（≥76.0）、model 12.2%（≥0.2）、
  mq_consumer 74.4%（≥72.0）、shared 45.9%（≥42.0）。
- `make integration-critical` 通过（interaction/user 集成测试）。
- `make spec-evals-test` 通过（13 测试）。

## 未覆盖边界

- CORE-013/CORE-062 `expected_revision=0` 迁移兼容冲突：等待人类决定，仍为 partial。
- CORE-015/DISC-001 `Total` 只按本页回减：设计取舍，仍为 partial。
- DISC-060~063 / ASST-050~051 冻结评测集与 REL-030~043 月度观测：待人类/生产输入，
  登记于 [IMP-todo-blocked-gates](../IMP-todo-blocked-gates.md)。
