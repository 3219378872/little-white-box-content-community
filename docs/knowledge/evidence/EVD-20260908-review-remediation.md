---
id: EVD-20260908-review-remediation
layer: evidence
title: 全面审查后端修复与隔离集成回归
status: active
owner: agent
updated_at: '2026-09-08'
scope: [static, unit, integration]
commands:
- make fmt-check vet lint vulncheck spec-evals-test python-unit
- make test ARGS=-count=1
- env NO_PROXY=localhost,127.0.0.1,172.28.0.1 no_proxy=localhost,127.0.0.1,172.28.0.1 go test -p 1 -tags=integration -race -count=1 -timeout=10m ./app/assistant/internal/runtime ./app/assistant/internal/store
- env NO_PROXY=localhost,127.0.0.1,172.28.0.1 no_proxy=localhost,127.0.0.1,172.28.0.1 make integration-critical
- NO_PROXY=127.0.0.1,localhost,::1 go test -race -tags integration -count=1 ./app/feed/rpc/internal/svc -run '^TestFeedServiceReadsCurrentNegativeFeaturesFromConfiguredRedis$'
observed_commit: 35a93a164461d03a905605e791a580265bbf7be9
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

# 全面审查后端修复与隔离集成回归

本页记录两个实现提交合并后的最终代码，所有声明命令在观察提交上实际取得 exit 0，开始和结束时
工作树干净。证据和 IMP 更新在随后独立提交完成，不使用自指 SHA。公开 `gateway.api` 未改变，
内部 Content proto 的 presence 字段由固定生成依赖正常生成；Flutter SDK 同步和真实栈归根仓验证。

| 审查分项 | 修复与正式回归入口 |
| --- | --- |
| R02 | 固定原 claim fence；`lease_ownership_test.go` 覆盖正常、取消和失败收尾，`lease_ownership_integration_test.go` 以真实 SQL 租约过期、B Claim、A 继续执行验证只有 B 能写唯一答案和 done |
| R03 | 十个 SQL service context 在连接前关闭参数日志；`deploy/log_retention_test.go` 检查实际生产 wiring，Message `sql_privacy_test.go` 验证正常、慢查询、含敏感合成值的错误均不记录私信原文 |
| R04 / R13 | `post_media_commands_test.go` 验证新增 URL 不能绕过媒体归属、同帖保留、失效旧 ID 拒绝与清空/整体替换恢复；Gateway `post_v2_revision_logic_test.go` 使用真实 JSON 解析和 protobuf 往返验证省略与空数组 |
| R06 | `evidence_excerpt_test.go` 验证正文、评论和中文长片段为原文精确子串，合法发布成功、改文仍拒绝；旧错误 evidence 不篡改 |
| R07 | `compact_summary_test.go` 连续两次 compact，验证旧条件参与后续摘要请求与最终快照；旧摘要作为不可信历史输入 |
| R09 / R10 | Recommend `cursor_visibility_test.go` 验证过滤后原始消费位置；Feed 测试验证合法空结果、负反馈及可见性失败不伪装成功 |
| R11 / R12 | `fallback_feed_test.go`、`fallback_cursor_test.go`、`fallback_state_test.go` 验证缓冲、耗尽、失败来源重试、链路去重/位置、全 binding、固定 600 秒及旧/坏游标拒绝 |
| R14 | `memory_review_terminal_test.go` 从真实 Execute 调用工具进入成功、取消、失败和触顶终态，验证已成功 change 的通知、去重与实际 undo |
| R17 | `budget_execution_test.go` 验证主/辅助剩余配额、末轮触顶、流式 partial、同轮多工具、结构化发布、review 累计输入以及无收益辅助调用记账 |

修复后的交叉检查还补齐两项接线/兼容保护：Feed 与 Recommend 三个实际 YAML 统一使用 `v2` 特征，
真实 Redis 测试通过 `NewServiceContext` 读到 `v2` 隐藏记录而非 `v1`；媒体更新只为显式空集合增加
幂等 presence，`post_command_idempotency_test.go` 验证四类历史 hash/重放保持兼容，并拒绝空数组
复用缺省命令。验证码成功路径返回 typed `{}`，对应测试检查非 nil 与序列化，避免严格前端把成功误报失败。

| 实际检查 | 最终结果 |
| --- | --- |
| 格式、vet、lint、govulncheck | 全部 exit 0；lint 0 issues，当前可调用符号无已知漏洞 |
| 全模块 `make test ARGS=-count=1` | exit 0；113 个有测试包通过，启用 race 与包级覆盖率输出 |
| 规格评测器 / Python 工具 | 47 / 10+3 项通过；负向 fixture 的失败指标不是产品质量失败或成功结论 |
| Assistant runtime/store 隔离 MySQL 集成 | 全部 exit 0，分别 35.442s / 20.528s；包含存储 fencing、journal、并发 Memory、Watch 与终态事务回滚 |
| critical 集成 | Interaction logic / User logic / User model 全部通过，分别 14.723s / 16.971s / 15.537s |
| Feed 真实 Redis 接线集成 | 1 项通过，3.485s；独立容器已清理 |

原始运行日志在 `/tmp/little-review-fix-validation-20260908-ka9yNl` 的 `backend-*-final.log`；本页结果、
命令与入库回归共同形成持久记录，不以临时产物作为唯一证明。sqlx 当前 guard 同时控制 SQL timing
metric，禁用参数日志后不声称该计时仍可用；请求/RPC/领域指标继续生效。

六组 coverage 保留 `EVD-20260907-module-refactor` 的原 requirements 与 paths，并为日志隐私组补入
各 SQL 服务与 wiring 测试输入。只刷新原有 aligned 的有效证据；社区、研究语义、真实模型、
discussion_spike、人类评审、设备、性能和生产 SLO 的既有 unknown/diverged 不因本次局部门禁升级。

本次数据库测试使用独立 MySQL 8/Redis 7 测试容器，模型仍为确定性替身，没有启动、重置或清理业务栈。
实际进程崩溃、真实 provider 的长研究质量、引用逐条支持关系、浏览器/设备和生产容量不在本页 scope。
依赖扫描仍有一个包级及两个模块级非可达告警，不能把零可达漏洞解释为所有依赖均无风险。
