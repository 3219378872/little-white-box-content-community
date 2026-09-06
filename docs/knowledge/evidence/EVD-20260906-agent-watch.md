---
id: EVD-20260906-agent-watch
layer: evidence
title: Agent Watch 当前确定性验证
status: active
owner: agent
upstream:
  - IMP-agent-watch
updated_at: 2026-09-06
covers:
  - WCH-001
  - WCH-002
  - WCH-003
  - WCH-004
  - WCH-010
  - WCH-011
  - WCH-012
  - WCH-013
  - WCH-020
  - WCH-021
  - WCH-022
  - WCH-023
  - WCH-024
  - WCH-A01
  - WCH-A02
  - WCH-A04
  - WCH-A06
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

# Agent Watch 当前确定性验证

`observed_commit` 是本次迁移开始前的后端基线。列出的命令只检查迁移工作树中的治理规则、评测客户端和
性能客户端，尚未在一个已提交的迁移版本上复验 Go 静态、race 与关键集成范围，因此结果为 `partial`。

本证据不包含浏览器、设备、真实 provider、人类质量评审或生产月度观测；这些边界在对应 IMP 中
保持 `unknown` 或 `diverged`，不得由本页提升为 `aligned`。
