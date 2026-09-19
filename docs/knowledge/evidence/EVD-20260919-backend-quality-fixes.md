---
id: EVD-20260919-backend-quality-fixes
layer: evidence
title: 后端质量审查六项修复与故障回归
status: active
owner: agent
updated_at: '2026-09-19'
scope: [static, unit, integration]
commands:
- make fmt-check vet lint vulncheck spec-evals-test python-unit
- make test ARGS='-count=1 -p 4'
- GOFLAGS=-race make integration-critical
- env NO_PROXY=localhost,127.0.0.1,172.28.0.1 no_proxy=localhost,127.0.0.1,172.28.0.1 go test -p 1 -tags=integration -race -count=1 -timeout=10m ./app/assistant/internal/runtime ./app/assistant/internal/store ./app/content/rpc/internal/logic ./app/feed/rpc/internal/svc
observed_commit: 297bbcbbd82616ac9d58a3a9f76884a5d966c8d1
result: passed
artifacts:
- pkg/interceptor/rpc_logging_test.go
- deploy/log_retention_test.go
- app/assistant/internal/index/history_relay_test.go
- app/assistant/internal/runtime/delete_history_integration_test.go
- app/content/rpc/internal/logic/post_media_commands_test.go
- app/interaction/rpc/internal/logic/stale_cache_integration_test.go
- app/user/rpc/internal/logic/auth_atomic_integration_test.go
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
---

# 后端质量审查六项修复与故障回归

上列命令均在观察提交上实际执行并取得 exit 0，执行期间受跟踪工作树干净。证据与 IMP 引用在独立
后续提交记录，不使用自指 SHA。公开 REST、内部 RPC 契约、生成文件、依赖和数据库结构均未改变。

| 审查问题 | 实现与回归证据 |
| --- | --- |
| RPC 请求与响应泄漏 | 十个服务入口通过生成的 ServiceDesc 排除全部方法的请求正文；全部 go-zero 客户端使用共享安全构造器，错误仅记方法、耗时和状态码。真实本地 go-zero RPC 对每个生成的 unary 方法及一个新增模拟方法执行普通、慢调用和错误调用，断言请求、响应与错误详情的合成私密标记均不进入日志；保留 Stat 和负载保护配置，既有客户端拦截器仍被调用。AST 测试约束生产接线。 |
| Assistant 删除任务丢失 | 真实 Elasticsearch Go 客户端连接受控 HTTP 服务，200/404 完成，403/429/503 保留 outbox 并可在恢复后重试。真实 MySQL 中注入 InsertOutbox 失败，验证历史删除回滚；恢复后历史不可见且恰有一条 delete intent。查询侧归属和删除过滤继续保留。 |
| JSON 图片局部更新失败 | 使用真实 newPost 序列化生成 JSON 图片字段，再验证部分保留、重排和外来 URL 拒绝；兼容已有逗号格式及媒体归属检查。 |
| 缓存裁决取消互动 | 手写 Model 对单关系直接读 MySQL；真实 MySQL/Redis 先提交点赞及收藏，再分别写入旧 negative index 与 inactive primary cache。取消及重复取消后关系为 inactive、计数归零，四次状态变更各入 outbox 一次。 |
| 验证码重复消费 | 登录和手机号注册共用原子 Lua 校验、失败计数与消费；真实 Redis 的 32 并发登录只有一个成功，其他返回过期。验证错误次数上限、TTL、成功清理；Redis 故障时注册不插入用户。 |
| 刷新失败使会话丢失 | 先生成后继令牌，Lua 校验旧归属、SET EX/NX 新 jti 后再 DEL 旧 jti。真实 Redis 以无效后继 TTL 触发脚本运行错误，旧 jti 仍有效；恢复后 32 并发只有一个成功，旧令牌拒绝重放，成功返回的新令牌可继续刷新。另验证 owner 不匹配、后继冲突与 TTL。 |

| 固定提交验证 | 结果 |
| --- | --- |
| fmt / vet / lint / govulncheck | exit 0；lint 0 issues，0 个调用可达的已知漏洞。另有 1 个导入包与 2 个依赖模块级非可达提示，不解释为所有依赖无漏洞。 |
| 全模块 race 测试 | 113 个有测试包通过，输出包级覆盖率；未执行分层 coverage 门槛，不据此声称其通过。 |
| 规格评测器和 Python 工具 | 47 + 10 + 3 项通过；负向 fixture 的模拟失败用于验证评测器，不是产品质量验收。 |
| critical 隔离集成（race） | Interaction logic 19.995s、User logic 45.548s、User model 15.461s，全部通过。 |
| 补充隔离集成（race） | Assistant runtime 55.301s、Assistant store 20.449s、Content logic 18.196s、Feed svc 3.313s，全部通过。 |

六个覆盖组保留前次证据的 requirements 与输入路径，并补入共享 RPC、JWT 与隔离测试工具输入。
本次刷新既有 aligned 行的确定性自动化证据；原有 unknown/diverged 行及其上层验收缺口不升级。
历史 EVD 的观察结果不改写。本页结果和入库回归是持久记录，原始执行日志归档在
`/tmp/little-backend-quality-validation-20260919-nrGhau/backend-review/final-*.log`，不以临时产物作为唯一证明。

数据库测试只使用独立 MySQL 8 / Redis 7 测试容器。索引删除故障使用受控 HTTP 服务，RPC 日志测试
使用真实中间件和合成内容，不调用业务服务或模型。未启动当前停止的本地联调栈，未执行全栈 E2E、
真实短信、真实 Elasticsearch 集群故障、live-provider、浏览器、设备、容量或生产验证。
刷新修复覆盖新凭据登记失败；提交后响应丢失的幂等结果重放仍不在本次实现范围。直接读取互动关系
增加该查询的数据库访问，未进行容量压测。
