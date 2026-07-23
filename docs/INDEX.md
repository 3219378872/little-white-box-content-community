# Agent 文档入口

这是 agent 的按需路由表，不是要求全文阅读的目录。先判断任务类型，再只打开对应的一个或两个文档。

## 任务路由

| 任务 | 先读 | 需要时再读 |
| --- | --- | --- |
| REST API、Handler、参数校验 | [active/api.md](active/api.md) | `app/gateway/*.api` |
| RPC、proto、服务间调用 | [active/rpc.md](active/rpc.md) | `proto/**/*.proto`、对应 `app/` 代码 |
| MySQL、Redis、事务、缓存 | [active/data.md](active/data.md) | 对应 Model 和迁移文件 |
| 鉴权、错误、安全边界 | [active/security.md](active/security.md) | `pkg/jwtx/`、`pkg/middleware/`、`pkg/errx/` |
| MQ、重试、降级、部署排查 | [active/operations.md](active/operations.md) | `deploy/` 和对应消费者 |
| 测试、质量、发布前验证 | [active/testing.md](active/testing.md) | 相关测试文件和 CI |
| 需要了解整体结构 | [ARCHITECTURE.md](ARCHITECTURE.md) | [generated/INDEX.md](generated/INDEX.md) |

## 文档层级

- `docs/active/`：当前实现的短规范，允许作为任务上下文加载。
- `docs/generated/`：由代码生成的模块和流程事实，只在定位具体模块时加载。
- 历史设计和已完成计划不属于当前文档面，必要时通过 Git 历史追溯，不作为默认规则来源。

## 来源优先级

1. 代码、配置、API/proto 定义和测试结果。
2. `docs/generated/` 的当前快照。
3. `docs/active/` 的约束和操作说明。
4. 历史设计仅用于解释决策，不得覆盖当前实现。
