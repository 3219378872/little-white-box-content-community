---
id: EVD-20260906-content-discovery
layer: evidence
title: 内容发现当前确定性验证
status: active
owner: agent
upstream:
  - IMP-content-discovery
updated_at: 2026-09-06
covers:
  - DISC-001
  - DISC-002
  - DISC-003
  - DISC-004
  - DISC-010
  - DISC-011
  - DISC-012
  - DISC-020
  - DISC-021
  - DISC-022
  - DISC-023
  - DISC-030
  - DISC-031
  - DISC-032
  - DISC-033
  - DISC-034
  - DISC-035
  - DISC-036
  - DISC-040
  - DISC-041
  - DISC-042
  - DISC-050
  - DISC-051
  - DISC-052
  - DISC-061
  - DISC-062
  - DISC-A01
  - DISC-A02
  - DISC-A03
  - DISC-A04
  - DISC-A05
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

# 内容发现当前确定性验证

`observed_commit` 是本次迁移开始前的后端基线。列出的命令只检查迁移工作树中的治理规则、评测客户端和
性能客户端，尚未在一个已提交的迁移版本上复验 Go 静态、race 与关键集成范围，因此结果为 `partial`。

本证据不包含浏览器、设备、真实 provider、人类质量评审或生产月度观测；这些边界在对应 IMP 中
保持 `unknown` 或 `diverged`，不得由本页提升为 `aligned`。
