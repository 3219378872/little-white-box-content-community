---
implementation: IMP-content-community-backend
verified_at: 2026-08-14
verified_commit: f2434fb
commands:
  - make spec-evals-test
  - make check
result: passed
---

# 2026-08-14 spec_evals 测试加固（死 import + 直接单测）

## 本批次改动

- `scripts/test_spec_evals.py`：
  - 删除 `test_recommend_subcommand_dispatches_to_samples` 中未使用的
    `import os`（全仓 Python AST 扫描的唯一死 import）。
  - 新增 `ReportFunctionTest`（8 项直接单测）：
    - `percentile`：空列表→0、单元素、p95 索引（ceil(0.95*100)-1=94）、
      小列表 clamp；
    - `report_slo`：met→0 / 未 met→1 返回码；
    - `report_recommendation`：达标（relative_improvement≥5% 且 CI lower≥0）
      →0、不达标→1。

## 审查证据

- 全仓 Python AST 未使用 import 扫描：仅此一处。
- `spec_evals.py` 的 `percentile`/`report_slo`/`report_recommendation`
  此前仅经 `monthly_slo_report` 与 CLI dispatch 间接覆盖；`live_search`/
  `live_assistant` 需真实 Gateway，维持不测（合理边界）。
- 本轮同时完成的正向验证：gateway 决策表覆盖全部 33 条路由（按 AddRoutes
  块解析 prefix 后比对，含查询参数归一化）；DES 设计文档与实现一致；
  ES 索引器/gateway_performance/coverage_report 无质量问题。

## 结果

- `make spec-evals-test`：26 项全过（原 18 + 新 8）。
- `make check` 通过。

## 未覆盖边界

- 外部输入门禁不变（ASST/REL/PROP/DISC-062 复评），见
  [IMP-todo-blocked-gates](../IMP-todo-blocked-gates.md)。
