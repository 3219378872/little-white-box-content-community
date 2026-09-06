---
id: EVD-20260906-community-core
layer: evidence
title: 社区核心当前确定性验证
status: active
owner: agent
upstream:
  - IMP-community-core
updated_at: 2026-09-06
covers:
  - CORE-001
  - CORE-002
  - CORE-003
  - CORE-004
  - CORE-005
  - CORE-010
  - CORE-011
  - CORE-012
  - CORE-013
  - CORE-014
  - CORE-015
  - CORE-016
  - CORE-020
  - CORE-021
  - CORE-022
  - CORE-023
  - CORE-024
  - CORE-030
  - CORE-031
  - CORE-033
  - CORE-034
  - CORE-040
  - CORE-041
  - CORE-042
  - CORE-043
  - CORE-044
  - CORE-050
  - CORE-051
  - CORE-052
  - CORE-053
  - CORE-054
  - CORE-060
  - CORE-061
  - CORE-062
  - CORE-063
  - CORE-A01
  - CORE-A02
  - CORE-A03
  - CORE-A04
  - CORE-A05
  - CORE-A06
  - CORE-A07
scope:
  - static
  - unit
  - integration
commands:
  - make engineering-lint
  - make check
  - make test
  - make spec-evals-test
  - make python-unit
  - make integration-critical
  - PATH=/tmp/xbh-codegen-v2.M2q1RU/bin:$PATH make generate
  - git status --short
  - git diff --check 0632db5395d0450e05eb7b21b1d96e769a66e6f3^ 0632db5395d0450e05eb7b21b1d96e769a66e6f3
observed_commit: 0632db5395d0450e05eb7b21b1d96e769a66e6f3
result: passed
---

# 社区核心当前确定性验证

列出的命令均在 `observed_commit` 上返回 0。`make engineering-lint` 运行 74 个治理测试；`make check`
通过格式检查、治理检查、`go vet` 与 `golangci-lint`（0 issues）；`make test` 完成全模块 race/coverage；
规格评测 47 个测试、Python 单元 10+3 个测试、三个 critical integration 包均通过。固定
`grpcio-tools==1.71.0`、`protobuf==5.29.4` 后连续两次 `make generate`，每次工作树均保持 clean。

本证据不包含浏览器、设备、真实 provider、人类质量评审或生产月度观测；这些边界在对应 IMP 中
保持 `unknown` 或 `diverged`，不得由本页提升为 `aligned`。
