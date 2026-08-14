---
implementation: IMP-content-community-backend
verified_at: 2026-08-14
verified_commit: 55c308d
commands:
  - make spec-evals-test
  - make algorithm-test
  - make check
  - python3 -c 'import yaml,sys; yaml.safe_load(open(".github/workflows/ci.yml"))'
result: passed
---

# 2026-08-14 CI 补齐 Python 质量门禁

## 本批次改动

- `.github/workflows/ci.yml`：test job 增加 `setup-python@v5`（3.12），并在
  coverage gate 后新增两个门禁步骤：
  - `make spec-evals-test`：规范质量门禁评估器（`scripts/spec_evals.py`，
    DISC/ASST/REL 共用）的单元测试；
  - `make algorithm-test`：算法模块单元测试（`algorithm/`，15 项，3 项因
    缺少 grpc 依赖按守卫跳过）。

## 审查证据

- 原 CI 缺口：ci.yml 只守护 Go 侧门禁（fmt/vet/lint/engineering-lint/test/
  coverage/integration-critical），Python 侧工具（评测门禁评估器、算法模块）
  的测试仅存在于本地 Makefile 目标，CI 无覆盖。
- 依赖检查：`spec_evals.py` 纯标准库；`algorithm` 测试中 grpc 依赖用例有
  skip 守卫，无第三方环境（如 CI ubuntu-latest + setup-python）可直接通过
  ——本地无 grpc 环境实测 `make algorithm-test` 15 过 3 skip。
- ci.yml 经 YAML 解析校验，test job 步骤顺序正确。

## 结果

- `make spec-evals-test` 通过；`make algorithm-test` 通过（15 过 3 skip）。
- `make check` 通过；ci.yml 语法校验通过。

## 未覆盖边界

- `make integration-all`（完整集成）与 `make fuzz` 仍不纳入 CI——保持现有
  PR 规模取舍；`make coverage-target` 目标门槛仍由人工发布流程把关。
