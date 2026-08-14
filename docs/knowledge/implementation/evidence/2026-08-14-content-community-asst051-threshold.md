---
implementation: IMP-content-community-backend
verified_at: 2026-08-14
verified_commit: 06a8750
commands:
  - python3 -m unittest scripts.test_spec_evals
  - make check
  - make test
result: passed
---

# 2026-08-14 ASST-051 来源有效率阈值修正（规格层验证）

## 规格偏离

`SPEC-grounded-assistant` ASST-051 要求来源有效率**必须为 100%**；
`scripts/spec_evals.py` 的 `report_assistant` 用 `source_accuracy >= 0.95`
判定——95%~100% 的来源有效率会被误判为通过，**门禁放水**。

## 修复

- `report_assistant`：`source_accuracy >= 1.0`（100% 要求），
  证据不足召回仍 ≥95%（不变）。

## 测试

- 新增 `AssistantSourceAccuracyThresholdTest`（2 项）：
  - source_accuracy=0.99 → 门禁失败（100% 要求）；
  - source_accuracy=1.0 但证据不足召回 0 → 门禁失败（召回仍 ≥95%）。

## 结果

- `test_spec_evals` 30 项全过；`make check` 通过；`make test` 全部模块
  通过（含 race）。

## 未覆盖边界

- 外部输入门禁不变（ASST/REL/PROP/DISC-062 复评），见
  [IMP-todo-blocked-gates](../IMP-todo-blocked-gates.md)。
