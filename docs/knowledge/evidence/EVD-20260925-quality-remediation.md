---
id: EVD-20260925-quality-remediation
layer: evidence
title: 后端隐私、一致性与订阅错误修复验证
status: active
owner: agent
updated_at: '2026-09-25'
scope:
- static
- unit
- integration
commands:
- GOMAXPROCS=4 make fmt-check vet lint vulncheck spec-evals-test python-unit
- GOMAXPROCS=4 make test ARGS='-count=1 -p=2' TEST_JSON_DIR=/tmp/little-remediation-final/backend-json
- GOMAXPROCS=4 GOFLAGS=-race make integration-critical
- GOMAXPROCS=4 go test -p 1 -tags=integration -race -count=1 -timeout=10m ./app/assistant/internal/runtime
  ./app/assistant/internal/store ./app/content/rpc/internal/logic ./app/feed/rpc/internal/svc
  ./app/recommend/mq/internal/store
observed_commit: fca6d503942f63f677aaedd8ff4acd3fd56da234
result: passed
artifacts:
- docs/knowledge/evidence/assets/quality-remediation-20260925/validation-summary.json
- app/assistant/internal/runtime/delete_history_test.go
- app/assistant/internal/runtime/delete_history_snapshot_integration_test.go
- app/gateway/internal/handler/assistant/assistant_run_events_lifecycle_test.go
- app/recommend/mq/internal/store/personalization_integration_test.go
- app/user/rpc/internal/logic/login_lock_redis_test.go
coverage:
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
  - pkg/interceptor
  - pkg/jwtx
  - pkg/testutil
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
  - proto/user/user.proto
  - Makefile
  - scripts/test.sh
  - scripts/integration-test.sh
  - scripts/_lib.sh
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
  - pkg/interceptor
  - pkg/jwtx
  - pkg/testutil
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
  - Makefile
  - scripts/test.sh
  - scripts/integration-test.sh
  - scripts/_lib.sh
  - app/recommend/rpc
  - app/recommend/mq
  - app/user
  - deploy/docker-compose.production.yml
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
  - pkg/interceptor
  - pkg/jwtx
  - pkg/testutil
  - app/assistant/internal/memory
  - app/assistant/internal/runtime
  - app/assistant/internal/store
  - app/assistant/internal/prompt
  - app/gateway/internal/logic/assistant
  - proto/assistant/assistant.proto
  - deploy/sql/xbh_assistant.sql
  - go.mod
  - go.sum
  - Makefile
  - scripts/test.sh
  - scripts/integration-test.sh
  - scripts/_lib.sh
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
  - pkg/interceptor
  - pkg/jwtx
  - pkg/testutil
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
  - Makefile
  - scripts/test.sh
  - scripts/integration-test.sh
  - scripts/_lib.sh
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
---

# 后端隐私、一致性与订阅错误修复验证

全部命令在页头固定实现提交执行；本页及 IMP 引用随后提交。格式、vet、lint、govulncheck、
规格评测器与 Python 工具均通过。全模块 race 为 1790 个测试/子测试通过、195 个有测试包通过，
另有 7 个无测试包。核心与补充隔离 MySQL/Redis 集成全部通过；精确命令与耗时见入库摘要。
漏洞扫描为 0 个可达漏洞，另有 1 个导入包和 2 个依赖模块级非调用路径提示，不能解释为依赖完全无漏洞。

| 问题 | 修复与实际回归 |
| --- | --- |
| 删除历史后仍复用压缩摘要 | 原子取消/fencing 并清除历史派生 prompt；下一次模型请求不含已删摘要，MEMORY/USER 保留；compact 迟到结果拒写，隔离 MySQL 注入 outbox 失败整笔回滚 |
| 并发旧快照恢复历史 | 隔离 MySQL 重现请求先建立 RR 快照、删除提交、再获得线程锁并重开旧 session；事务 session 当前读保留删除后的摘要与 epoch |
| 个性化关闭标记过期 | 缺标记/缓存故障回查 User RPC；状态未知不生成画像；真实 Redis 验证过期、写标记丢失、周期清理、场景召回与关注分发成员清除，作者新帖不得重建关闭用户画像 |
| 用户名变体绕过登录锁 | 以 user.Id 锁定；真实 MySQL 排序规则验证同账户变体共用尝试次数 |
| INCR 与 EXPIRE 分离留下永久锁 | Lua 原子维护固定窗口，真实 Redis 并发 32 次累加、不续期及既存无 TTL 计数修复 |
| 推荐续页忽略新隐藏反馈 | 每页重读明确负反馈，按原快照偏移前进；依赖故障关闭；关闭个性化使旧个性化游标失效，新规则游标仍可翻页 |
| run 终态竞态漏最终事件 | 终态观察后补读持久事件；done/error 两种提交交错及最终读取失败回归 |
| SSE 后端错误变成空 200 | 真实 go-zero WithSSE 包装下初始 403/404/503 保持 JSON HTTP 错误，起流后 transport_error 不占持久游标；写失败取消订阅，panic 不悬挂或泄露内部内容 |

六个 coverage 组保留既有输入并纳入新增网关 handler 与偏好查询路径。刷新既有 192 个 aligned
声明，包括原先因 confirmation_revision_test.go 变化过期的 MEM-001；不修改历史 EVD 结果，
不提升任何 unknown/diverged 门禁。单元/隔离集成不代替真实接口、浏览器、设备、模型或生产证明。

数据库均由 testcontainers 独立创建并回收，不连接业务库。公开 REST/proto 未变；推荐消费者新增
带内部签名的 User RPC 依赖及生产配置注入。偏好权威回查增加 RPC/DB 访问，未运行容量压测。
旧用户名登录锁键不沿用；代码维持既有 Redis 故障 fail-open 登录策略。
