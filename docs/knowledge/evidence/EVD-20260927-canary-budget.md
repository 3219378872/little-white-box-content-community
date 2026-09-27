---
layer: evidence
status: active
owner: agent
id: EVD-20260927-canary-budget
title: Assistant 就绪探针推理预算回归
updated_at: '2026-09-27'
scope:
- static
- unit
commands:
- GOMAXPROCS=4 make fmt-check vet lint vulncheck
- GOMAXPROCS=4 go test -race -count=1 ./app/assistant/...
observed_commit: 5d5a7481a6d5b73afb8bd4661344e7404725a492
result: passed
artifacts:
- docs/knowledge/evidence/assets/canary-budget-20260927/validation-summary.json
- app/assistant/internal/llm/canary_test.go
coverage:
- requirements:
  - AGENT-001
  - AGENT-002
  - AGENT-003
  - AGENT-004
  - AGENT-010
  - AGENT-011
  - AGENT-012
  - AGENT-013
  - AGENT-014
  - AGENT-015
  - AGENT-020
  - AGENT-021
  - AGENT-022
  - AGENT-023
  - AGENT-024
  - AGENT-025
  - AGENT-026
  - AGENT-030
  - AGENT-031
  - AGENT-032
  - AGENT-033
  - AGENT-034
  - AGENT-035
  - AGENT-036
  - AGENT-037
  - AGENT-040
  - AGENT-041
  - AGENT-042
  - AGENT-043
  - AGENT-044
  - AGENT-045
  - AGENT-050
  - AGENT-051
  - AGENT-052
  - AGENT-053
  - AGENT-054
  - AGENT-060
  - AGENT-061
  - AGENT-062
  - AGENT-063
  - AGENT-080
  - AGENT-081
  - AGENT-082
  - AGENT-083
  paths:
  - pkg/interceptor
  - pkg/jwtx
  - pkg/testutil
  - app/assistant/internal/runtime
  - app/assistant/internal/store
  - app/assistant/internal/lease
  - app/assistant/internal/llm
  - app/assistant/internal/prompt
  - app/assistant/internal/tool
  - app/assistant/worker
  - app/assistant/rpc
  - app/gateway/internal/logic/assistant
  - app/gateway/gateway.api
  - proto/assistant/assistant.proto
  - deploy/sql/xbh_assistant.sql
  - deploy/sql/patches/20260905_agent_research.sql
  - scripts/spec_evals.py
  - eval/dev/assistant_cases.synthetic.json
  - go.mod
  - go.sum
  - Makefile
  - scripts/test.sh
  - scripts/integration-test.sh
  - scripts/_lib.sh
  - app/gateway/internal/handler/assistant
  - app/gateway/internal/httpxconfig
- requirements:
  - REL-001
  - REL-002
  - REL-003
  - REL-004
  - REL-005
  - REL-006
  - REL-007
  - REL-008
  - REL-010
  - REL-011
  - REL-012
  - REL-013
  - REL-020
  - REL-021
  - REL-022
  - REL-023
  - REL-024
  - REL-044
  - REL-045
  - REL-050
  - REL-051
  - REL-052
  - REL-053
  - REL-054-01
  - REL-054-02
  - REL-054-03
  - REL-054-04
  - REL-054-05
  - REL-054-06
  - REL-054-07
  - REL-054-08
  - REL-054-09
  - REL-054-11
  - REL-054-12
  - REL-060
  - REL-061
  - REL-A01
  paths:
  - pkg/interceptor
  - pkg/jwtx
  - pkg/testutil
  - app/behavior
  - app/pipeline/behaviorlog
  - app/recommend/mq
  - pkg/outboxx
  - app/gateway
  - app/assistant
  - app/content
  - app/interaction
  - app/message
  - app/media
  - app/user
  - app/feed
  - deploy/log_retention_test.go
  - deploy/loki/loki-config.yaml
  - deploy/docker-compose.production.yml
  - scripts/spec_evals.py
  - scripts/gateway_performance.py
  - go.mod
  - go.sum
  - proto/user/user.proto
  - Makefile
  - scripts/test.sh
  - scripts/integration-test.sh
  - scripts/_lib.sh
  - app/gateway/internal/handler/assistant
  - app/gateway/internal/httpxconfig
  - app/recommend/rpc
  - deploy/docker-compose.middleware.yml
---

# 推理模型就绪探针预算

上游是 [运行时设计](../design/DES-assistant-agent-runtime.md)，实现回链
[Agent](../implementation/IMP-assistant-agent.md) 与 [可靠性](../implementation/IMP-feedback-reliability.md)。

实际端点测试显示工具轮可消耗 65 token，超过旧首轮 64；确认轮的旧 32 token 也可能全部消耗在推理。
两轮预算改为 256，保留所有 schema/call/result 验证；新增低预算耗尽模拟及错误协议拒绝回归。
本提交的 Assistant race 单测与全仓静态检查通过，未更换模型、协议、凭据或 fallback。

仅刷新包含 canary 输入的原覆盖组，范围不缩小；未改动领域的完整回归见
[EVD-20260927-media-uploads](EVD-20260927-media-uploads.md)。本页是 static/unit 证据，
端点延迟是单次样本，不是性能基准，实际 worker readiness 与媒体验收另由根仓记录。
