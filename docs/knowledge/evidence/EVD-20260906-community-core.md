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
commands:
  - python3 -m unittest discover -s scripts -p 'test_engineering_lint.py' -q
  - python3 -m unittest discover -s scripts -p 'test_spec_evals.py' -q
  - python3 -m unittest discover -s scripts -p 'test_gateway_performance.py' -q
  - python3 scripts/engineering-lint.py
observed_commit: f706309f860621e7d9079333cf33e81557253b73
result: partial
---

# 社区核心当前确定性验证

`observed_commit` 是本次迁移开始前的后端基线。列出的命令只检查迁移工作树中的治理规则、评测客户端和
性能客户端，尚未在一个已提交的迁移版本上复验 Go 静态、race 与关键集成范围，因此结果为 `partial`。

本证据不包含浏览器、设备、真实 provider、人类质量评审或生产月度观测；这些边界在对应 IMP 中
保持 `unknown` 或 `diverged`，不得由本页提升为 `aligned`。
