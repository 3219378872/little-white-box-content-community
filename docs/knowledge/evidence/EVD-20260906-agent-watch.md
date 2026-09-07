---
id: EVD-20260906-agent-watch
layer: evidence
title: Agent Watch 当前确定性验证
status: active
owner: agent
updated_at: 2026-09-06
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
coverage:
- requirements:
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
  paths:
  - app/assistant/watch
  - app/assistant/mq
  - app/assistant/internal/runtime
  - app/assistant/internal/store
  - app/gateway/internal/logic/assistant
  - proto/assistant/assistant.proto
  - deploy/sql/xbh_assistant.sql
---

# Agent Watch 当前确定性验证

列出的命令均在 `observed_commit` 上返回 0。`make engineering-lint` 运行 74 个治理测试；`make check`
通过格式检查、治理检查、`go vet` 与 `golangci-lint`（0 issues）；`make test` 完成全模块 race/coverage；
规格评测 47 个测试、Python 单元 10+3 个测试、三个 critical integration 包均通过。固定
`grpcio-tools==1.71.0`、`protobuf==5.29.4` 后连续两次 `make generate`，每次工作树均保持 clean。

本证据不包含浏览器、设备、真实 provider、人类质量评审或生产月度观测；这些边界在对应 IMP 中
保持 `unknown` 或 `diverged`，不得由本页提升为 `aligned`。
