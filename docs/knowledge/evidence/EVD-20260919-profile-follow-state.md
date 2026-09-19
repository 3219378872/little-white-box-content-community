---
id: EVD-20260919-profile-follow-state
layer: evidence
title: 资料访问者关注状态与共享契约回归
status: active
owner: agent
updated_at: '2026-09-19'
scope: [static, unit, integration]
commands:
- make fmt-check vet lint vulncheck
- make test ARGS=-count=1
- GOFLAGS=-race make integration-critical
- make spec-evals-test python-unit
- env NO_PROXY=localhost,127.0.0.1,172.28.0.1 no_proxy=localhost,127.0.0.1,172.28.0.1 go test -p 1 -tags=integration -race -count=1 -timeout=10m ./app/assistant/internal/runtime ./app/assistant/internal/store ./app/content/rpc/internal/logic ./app/feed/rpc/internal/svc
observed_commit: 0184c600d8a0fcfb8e7701464fa3e5ef330f7faf
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
- requirements: [CORE-030, CORE-032, CORE-062, CORE-063]
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
  - proto/user/user.proto
  - go.mod
  - go.sum
---

# 资料访问者关注状态与共享契约回归

命令均在 rebase 后实现提交 `0184c600d8a0fcfb8e7701464fa3e5ef330f7faf` 的源码、测试与配置上重新
执行。验证期间仅待提交证据、IMP 和生成索引有变动，全部被验证输入与该提交一致；后续提交记录证据。
本次改动为 User/GetUser RPC 与 Gateway 资料响应，不改变
关注写路径、公共 UserInfo 或数据库结构。生成源为 `gateway.api` 与 `proto/user/user.proto`，执行
`make generate` 后检查生成差异，未手改生成物。

`GetUserReq.viewer_id` 由 Gateway 的 OptionalAuth 上下文提供；User RPC 每次从关注表读取，
`GetUserResp.isFollowing` 为确定的布尔值。匿名和本人不查询关系，缺失关系返回 false，缺少 store
或数据库失败返回业务错误，不能返回伪造的 false 成功。旧 RPC 调用者 viewer_id 默认为 0，新增字段
保持 protobuf 字段编号兼容；公共 REST 为加字段，客户端同步后必须校验其存在与类型。

| 实际验证 | 结果 |
| --- | --- |
| User logic 单测 | 匿名、本人、已关注、未关注、数据库失败和依赖缺失均通过 |
| Gateway logic 单测 | 认证访问者透传、响应字段映射及 RPC 失败传播通过 |
| 新增隔离 MySQL 集成 | `TestGetUserReadsCurrentViewerRelationIntegration` 验证关注前/后、匿名/本人/其他账号以及取消后读取 |
| 全模块 `make test ARGS=-count=1` | 113 个有测试包通过，启用 race 与包级覆盖率 |
| fmt / vet / lint / govulncheck | 全部 exit 0，lint 0 issues，无可达符号漏洞；仍有 1 个包级、2 个模块级非可达告警 |
| critical race 集成 | Interaction logic、User logic、User model 全部通过，包括 main 新增的认证原子操作和旧缓存回归 |
| Assistant runtime/store 隔离 SQL race 集成 | 两包通过，包含原有租约接管、Memory/Watch、journal 与事务回滚回归 |
| Content / Feed race 集成 | 通过，使用独立 MySQL/Redis 容器 |
| 规格评测器 / Python 工具 | 47 / 10+3 项通过 |

`gateway.api` 和 `app/user` 的修改使原 Agent/可靠性覆盖组过期，因此重跑其全模块与隔离集成检查，
保留原 coverage requirements 和输入 paths（可靠性组补入 User proto），只刷新这两个 IMP 的既有
aligned 引用，输入边界继承新 main 的 `EVD-20260919-backend-quality-fixes`，包含共享 RPC/JWT/测试
工具。Memory/Watch/Discovery 的新 main 覆盖组输入未变，继续使用其证据。CORE 覆盖保留其原有
输入边界并补入 User proto，仅补充关注与兼容性局部证明，不把 CORE 条款或任何 unknown/diverged
升级为整体对齐。

测试使用自动销毁的 MySQL/Redis 容器，没有连接或清理业务库。模型调用为确定性替身，不证明真实
provider、浏览器、实体设备、完整 HTTP E2E、容量、生产 SLO 或人工语义质量。原始日志暂存于
`/tmp/little-quality-tools-nsA9h9/backend-*.log`；本页实际命令、结果和入库回归共同形成持久证据。
