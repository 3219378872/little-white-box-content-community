# Agent 文档入口

这是 agent 的按需路由表，不是要求全文阅读的目录。先沿正式知识链确认上层要求，再只打开与任务相关的页面。

## 正式知识链

| 层 | 入口 | 权限与作用 |
| --- | --- | --- |
| 意图 | [knowledge/intent/README.md](knowledge/intent/README.md) | 人类决定语义，agent 经对话授权可维护 |
| 规范 | [knowledge/spec/README.md](knowledge/spec/README.md) | 人类决定语义，agent 经对话授权可维护 |
| 设计 | [knowledge/design/README.md](knowledge/design/README.md) | agent 维护，说明如何满足已批准规范 |
| 实现 | [knowledge/implementation/README.md](knowledge/implementation/README.md) | agent 维护，映射源码、状态和验证证据 |

所有权、状态、引用方向和提案流程以 [knowledge/README.md](knowledge/README.md) 为准。

## 任务路由

| 任务 | 先读 | 需要时再读 |
| --- | --- | --- |
| REST API、Handler、参数校验 | [implementation/IMP-development-quickstart.md](knowledge/implementation/IMP-development-quickstart.md) | `app/gateway/*.api` |
| RPC、proto、服务间调用 | [implementation/IMP-development-quickstart.md](knowledge/implementation/IMP-development-quickstart.md) | `proto/**/*.proto`、对应 `app/` 代码 |
| MySQL、Redis、事务、缓存 | [implementation/IMP-development-quickstart.md](knowledge/implementation/IMP-development-quickstart.md) | 对应 Model 和迁移文件 |
| 鉴权、错误、安全边界 | [implementation/IMP-engineering-conventions.md](knowledge/implementation/IMP-engineering-conventions.md) | `pkg/jwtx/`、`pkg/middleware/`、`pkg/errx/` |
| MQ、重试、降级、部署排查 | [implementation/IMP-development-quickstart.md](knowledge/implementation/IMP-development-quickstart.md) | `deploy/` 和对应消费者 |
| 测试、质量、发布前验证 | [implementation/IMP-development-quickstart.md](knowledge/implementation/IMP-development-quickstart.md) | 相关测试文件和 CI |
| 需要了解整体结构 | [implementation/IMP-architecture.md](knowledge/implementation/IMP-architecture.md) | 对应 `app/`、`pkg/` 代码 |
| 规格对齐与逐条状态 | [implementation/IMP-content-community-backend.md](knowledge/implementation/IMP-content-community-backend.md) | 设计到代码台账 |
| Assistant Agent、记忆、条件追踪 | [spec/SPEC-assistant-agent.md](knowledge/spec/SPEC-assistant-agent.md) | `SPEC-agent-memory`、`SPEC-agent-watch`、`DES-assistant-agent-runtime` |
| 剩余外部门禁 | [implementation/IMP-todo-blocked-gates.md](knowledge/implementation/IMP-todo-blocked-gates.md) | 人类评测集与月度 SLO |

## 文档层级

- `docs/knowledge/`：正式四层知识、非权威提案和过渡登记。
- 旧 `docs/active/` 速查、顶层规范文档、服务架构文档与 generated 旧快照的内容
  已并入实现层页面并从仓库移除。
- 历史设计和已完成计划不属于当前知识链，必要时通过 Git 历史追溯。

## 来源优先级

1. “应该做什么”由已批准意图、已批准规范和活跃设计依次约束。
2. “当前做了什么”以代码、配置、API/proto 定义和测试结果为准。
3. 实现文档和证据记录对齐或偏离状态，但不能覆盖上层要求或源码事实。
4. 提案、旧实现快照和历史材料均不具有正式约束力。
