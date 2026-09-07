---
id: EVD-20260906-content-discovery
layer: evidence
title: 内容发现当前确定性验证
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
  - DISC-A01
  - DISC-A02
  - DISC-A03
  - DISC-A04
  - DISC-A05
  paths:
  - app/feed
  - app/search
  - app/recommend
  - app/embedding
  - algorithm
  - app/content/visibility
  - pkg/visibilityx
  - scripts/spec_evals.py
  - eval/dev/search_qrels.synthetic.json
  - eval/dev/recommend_samples.synthetic.json
---

# 内容发现当前确定性验证

列出的命令均在 `observed_commit` 上返回 0。`make engineering-lint` 运行 74 个治理测试；`make check`
通过格式检查、治理检查、`go vet` 与 `golangci-lint`（0 issues）；`make test` 完成全模块 race/coverage；
规格评测 47 个测试、Python 单元 10+3 个测试、三个 critical integration 包均通过。固定
`grpcio-tools==1.71.0`、`protobuf==5.29.4` 后连续两次 `make generate`，每次工作树均保持 clean。

本证据不包含浏览器、设备、真实 provider、人类质量评审或生产月度观测；这些边界在对应 IMP 中
保持 `unknown` 或 `diverged`，不得由本页提升为 `aligned`。
