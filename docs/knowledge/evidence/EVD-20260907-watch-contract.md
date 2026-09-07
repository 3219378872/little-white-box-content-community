---
id: EVD-20260907-watch-contract
layer: evidence
title: Watch 工具边界维护与共享输入复验
status: active
owner: agent
updated_at: 2026-09-07
scope:
- static
- unit
- integration
commands:
- KNOWLEDGE_PYTHON=/home/dev/projects/little/little-white-box-content-community/.venv-knowledge/bin/python make check spec-evals-test python-unit integration-critical
- make test ARGS=-count=1
- NO_PROXY=localhost,127.0.0.1,172.28.0.1 no_proxy=localhost,127.0.0.1,172.28.0.1 go test -p 1 -tags=integration -race -count=1 -timeout=10m ./app/assistant/internal/runtime ./app/assistant/internal/store
- git status --short
- git diff --check 8ae6dfa773898af81462440b8004369313db45f1 0179d81bcbf1ba07fd3271b45a2f0a486c97632c
observed_commit: 0179d81bcbf1ba07fd3271b45a2f0a486c97632c
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
---

# Watch 工具边界维护与共享输入复验

上述命令均在 `observed_commit` 上返回 0，验证结束工作树为空。知识与工程规则测试为 40+11 项，
`go vet` 与 `golangci-lint` 通过（0 issues）；全模块测试使用 `-race -cover -count=1`。规格评测工具
47 项、Python 单元 10+3 项与三个 critical integration 包通过。额外对 Assistant runtime/store 执行
带 integration 标签的 race 测试，两包分别在 20.173s、27.582s 通过，使用隔离 MySQL/Redis 测试环境。

`TestWatchCapabilityBoundary` 核对完整 Watch metadata 边界、新旧协议、冻结快照兼容及 user/review
隔离。`TestWatchExecutionRejectsUnadvertisedCapabilities` 验证未裁剪 registry 也会拒绝 Watch 的
澄清、平台业务写、个人列表与网络搜索调用，不能仅靠模型广告限制权限。

`TestWatchResearchDelivery` 使用确定性模型 fixture 执行 get_post、read_source 和 publish_answer，
覆盖成功、非结构化正文、取消与最终来源不可见；核对来源快照、answer_committed、可见消息、outbox、
未读、小时/日发送次数以及失败后配额释放。新输入不再要求旧 present_sources。SQL 集成另行覆盖回答
发布事务回滚、租约/journal、Watch 配额、重试和过期 finalizer，不将内存 fixture 当作 SQL 并发证明。

四个覆盖组保留此前对应领域的输入范围，Watch 补入直接决定权限与协议过滤的工具目录。共享输入改变
后重新执行本地门禁，不复用过期组，也不改写旧 EVD 的历史结果。WCH-002/WCH-A01 因实际 matcher 未
注入 SpikeJudge 保持 diverged，不在本页覆盖范围。WCH-A03 的专门 SQL 取消交错门禁继续 unknown。

本次未改变 API/proto/SQL、业务工具白名单、冻结快照恢复或 ask_questions 的任务来源限制；未执行
代码生成、浏览器、设备、真实 provider、人类语义质量或生产 SLO 验证，不能用本页关闭这些门禁。
