---
implementation: IMP-content-community-backend
verified_at: 2026-08-14
verified_commit: a4fab42
commands:
  - python3 -m unittest scripts.test_coverage_report
  - python3 scripts/coverage_report.py --help
  - make coverage-no-gate
result: passed
---

# 2026-08-14 coverage_report 工具补测试（工具质量）

## 缺陷

`scripts/coverage_report.py` 是覆盖率门禁（`make coverage`）的核心
工具（分层聚合 + 门禁判定），但无任何单元测试，回归时只有运行
`make coverage` 才能暴露；且 `main()` 无 argv 注入，不可测。

## 改动

- `main(argv=None)`：支持 argv 注入（argparse 改为 `parse_args(argv)`）。
- 新增 `test_coverage_report.py`（10 项）：
  - `category`：handler/logic/model/mq_consumer/wiring/shared/other/
    generated 分层与 Windows 路径归一化；
  - `load_profiles`：按层聚合、重复块并集、不一致块抛错；
  - `add_percentages`：百分比与 handwritten 汇总、零语句；
  - `main` 门禁：none 恒过、baseline 达标/不达标退出码。

## 结果

- 10 项单测全过；`--help` 正常；`make coverage-no-gate` 各层报告正常。

## 未覆盖边界

- 外部输入门禁不变（ASST/REL/PROP/DISC-062 复评），见
  [IMP-todo-blocked-gates](../IMP-todo-blocked-gates.md)。
