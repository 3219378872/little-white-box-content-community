---
implementation: IMP-content-community-backend
verified_at: 2026-08-14
verified_commit: 2d5db50
commands:
  - make python-unit
  - python3 -c "import yaml; yaml.safe_load(open('.github/workflows/ci.yml'))"
result: passed
---

# 2026-08-14 Python 工具单测统一入口与 CI 接入（工具质量）

## 缺陷

- `test_coverage_report.py`（上轮新增）无任何 Makefile/CI 入口——
  测试存在但从不自动运行。
- `test_gateway_performance.py` 仅挂在 `make performance-gateway`
  （依赖 live 服务），CI 不跑 performance-gateway——其单测部分也
  不进 CI。

## 改动

- Makefile 新增 `make python-unit`：运行 `test_coverage_report` +
  `test_gateway_performance`（engineering/spec-evals 已有独立目标，
  不重复）。
- CI test job 在 Algorithm unit tests 前增加
  `make python-unit`。
- 顺带修正 `test_coverage_report.MainGateTest` 的构造：thresholds
  含 handler 层但 profile 曾缺该层（handler 0% 导致门禁误判），
  现显式构造 logic/handler 双层并校准失败（45.5%<50%）与通过
  （62.5%/100%）场景。

## 结果

- `make python-unit`：coverage_report 10 项 + gateway_performance
  3 项全过；ci.yml YAML 解析通过。

## 未覆盖边界

- 外部输入门禁不变（ASST/REL/PROP/DISC-062 复评），见
  [IMP-todo-blocked-gates](../IMP-todo-blocked-gates.md)。
