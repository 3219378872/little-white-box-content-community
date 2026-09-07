---
id: EVD-20260907-module-refactor
layer: evidence
title: 后端职责拆分与本地回归验证
status: active
owner: agent
updated_at: '2026-09-07'
scope:
- static
- unit
- integration
commands:
- make fmt-check vet lint vulncheck spec-evals-test python-unit integration-critical
  KNOWLEDGE_PYTHON=/home/dev/projects/little/little-white-box-content-community/.venv-knowledge/bin/python
- make test ARGS=-count=1
- NO_PROXY=localhost,127.0.0.1,172.28.0.1 no_proxy=localhost,127.0.0.1,172.28.0.1
  go test -p 1 -tags=integration -race -count=1 -timeout=10m ./app/assistant/internal/runtime
  ./app/assistant/internal/store
observed_commit: 7a2cf3adb4ccabc3922827ca02bf6bafb721a44b
result: passed
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
- requirements:
  - WCH-001
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
  - WCH-A02
  - WCH-A04
  - WCH-A06
  paths:
  - app/assistant/watch
  - app/assistant/mq
  - app/assistant/internal/runtime
  - app/assistant/internal/store
  - app/assistant/internal/tool
  - app/gateway/internal/logic/assistant
  - proto/assistant/assistant.proto
  - deploy/sql/xbh_assistant.sql
  - go.mod
  - go.sum
- requirements:
  - MEM-001
  - MEM-002
  - MEM-003
  - MEM-004
  - MEM-010
  - MEM-011
  - MEM-012
  - MEM-013
  - MEM-014
  - MEM-020
  - MEM-021
  - MEM-022
  - MEM-023
  - MEM-024
  - MEM-025
  - MEM-030
  - MEM-031
  - MEM-032
  - MEM-033
  - MEM-A01
  - MEM-A02
  - MEM-A03
  - MEM-A04
  - MEM-A06
  paths:
  - app/assistant/internal/memory
  - app/assistant/internal/runtime
  - app/assistant/internal/store
  - app/assistant/internal/prompt
  - app/gateway/internal/logic/assistant
  - proto/assistant/assistant.proto
  - deploy/sql/xbh_assistant.sql
  - go.mod
  - go.sum
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
  - app/behavior
  - app/pipeline/behaviorlog
  - app/recommend/mq
  - pkg/outboxx
  - app/gateway
  - app/assistant
  - deploy/loki/loki-config.yaml
  - deploy/docker-compose.production.yml
  - scripts/spec_evals.py
  - scripts/gateway_performance.py
  - go.mod
  - go.sum
- requirements:
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
  paths:
  - app/content
  - app/interaction
  - app/message
  - app/media
  - app/user
  - app/gateway
  - pkg/idempotencyx
  - pkg/outboxx
  - pkg/visibilityx
  - proto/content/content.proto
  - proto/interaction/interaction.proto
  - proto/message/message.proto
  - proto/media/media.proto
  - go.mod
  - go.sum
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
  - go.mod
  - go.sum
---

# 后端职责拆分与本地回归验证

上述命令在 observed_commit 上全部返回 0。全模块单测使用 race 与包级覆盖率；golangci-lint 为
0 issues，govulncheck 无可达漏洞（仍报告未被调用的间接包/模块漏洞，不据此声明依赖全部无漏洞）。
规格评测工具 47 项、Python 工具 10+3 项与三个 critical integration 包通过。额外的 Assistant
runtime/store 隔离 MySQL 集成 race 测试分别在 21.594s、22.674s 通过，覆盖原事务、租约、journal、
终态发布回滚与 Watch 配额/重试测试；没有使用 MemoryStore 冒充 SQL 回滚证明。

第一阶段按 Go AST 声明移动 store/sql、store/fake、runtime/loop 与 tool/registry，301 个声明体未变。
第二阶段按运行轮、工具准备、压缩摘要、终态消息/outbox、接收重放、来源解析、Watch 可见性、内容
校验与构造、媒体持久化和推荐排序提取私有步骤。新增并发 Execute 状态隔离、缺失 session 提前终止、
图片/视频已提交但 SendAndClose 失败仍保留对象并清理临时文件的回归测试。

六个覆盖组保留原领域输入范围并纳入 Go 依赖清单，重新验证移动后的实现；历史 EVD 不改写。IMP 中
既有 unknown/diverged 不自动升级，尤其社区既有全链验收缺口、真实模型质量、人类评审和生产 SLO。
本页仅记录本地 static/unit/integration，不包含真实联调栈、浏览器、设备或 live-provider 结论。
