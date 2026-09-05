# AGENTS.md

这是 esx（little-white-box）社交内容平台仓库。`AGENTS.md` 是唯一的项目规则入口；
知识与任务资料分别按 [docs/knowledge/README.md](docs/knowledge/README.md) 和
[docs/INDEX.md](docs/INDEX.md) 路由，禁止默认读取整个 `docs/`。

## 项目事实

- 根模块为 `esx`，Go 版本见 `go.mod`，框架为 go-zero。
- `app/` 包含 gateway、用户/内容/媒体/互动/feed/message 等 RPC 或 MQ 模块。
- `pkg/` 包含错误、鉴权、中间件、事件、MQ、缓存和测试工具等共享库。
- `deploy/docker-compose.middleware.yml` 提供本地 MySQL、Redis、etcd、RocketMQ、搜索、对象存储和观测组件。
- 运行时事实以代码、配置、`.api`、`.proto` 和测试为准；设计文档不能覆盖代码行为。

## 不可违反的规则

- Handler 只绑定参数、调用 Logic、返回响应；业务逻辑放在 Logic，数据访问放在 Model。
- 所有请求上下文必须透传；日志使用 `logx.WithContext(ctx)`，RPC 调用使用入参 ctx。
- Logic 返回 `pkg/errx` 中定义的业务错误；禁止裸 `errors.New` 和 Handler 手动设置 HTTP 错误状态。
- 配置从 `etc/*.yaml` 和 `config.Config` 注入；secret 只能通过环境变量，禁止硬编码。
- 禁止手动编辑 goctl/protobuf 生成文件；修改 `.api` 或 `.proto` 后必须重新生成并检查差异。
- 不为通过测试修改测试；修复实现并覆盖至少一个失败路径。
- 不引入新依赖，除非用户明确批准。

## 知识权限与加载顺序

- 正式知识链为“意图 → 规范 → 设计 → 实现”；先从知识总路由按需读取，不遍历目录。
- 意图定义产品价值与边界；规范定义工程约束、质量指标和验收条件，内部实现机制归设计。
- 意图、规范和治理入口的语义决定权属于人类开发者；agent 默认可以编辑和维护。修改
  [知识总路由](docs/knowledge/README.md) 中的受保护路径前，只需获得当前对话中的人类自然语言授权，
  不要求授权文件、签名或额外审批记录。
- 授权只覆盖指令明确的目标、内容及必要索引；超出范围或需要新增语义决定时必须再次询问。
  未获授权的意图或规范建议只能写入 `docs/knowledge/proposals/`，不能成为正式上游。
- `owner: human` 表示语义所有权，不限制经授权的 agent 编辑；只有人类明确接受、批准或要求发布
  正式内容时，agent 才能把 `INT-*`、`SPEC-*` 标记为 `approved`。
- 设计必须引用已批准规范或登记的过渡基线，实现必须引用设计。上层缺失、冲突或含糊时停止
  设计/实现并请求人类决定，禁止在未授权范围内从代码反推并写入正式意图或规范。
- 当前行为以代码、配置、`.api`、`.proto` 和测试为准；偏离上层时在实现层标记 `diverged`，
  不得反向改写设计、规范或意图。

## 任务资料与地址

先通过 [docs/INDEX.md](docs/INDEX.md) 判断任务类型，只打开相关页面：

| 任务知识 | 文档地址 | 内容范围 |
| --- | --- | --- |
| REST API | [docs/knowledge/implementation/IMP-development-quickstart.md](docs/knowledge/implementation/IMP-development-quickstart.md) | Handler、参数校验和 `.api` 修改流程 |
| RPC | [docs/knowledge/implementation/IMP-development-quickstart.md](docs/knowledge/implementation/IMP-development-quickstart.md) | proto、服务间调用和代码生成要求 |
| 数据 | [docs/knowledge/implementation/IMP-development-quickstart.md](docs/knowledge/implementation/IMP-development-quickstart.md) | MySQL、Redis、事务和缓存边界 |
| 安全 | [docs/knowledge/implementation/IMP-engineering-conventions.md](docs/knowledge/implementation/IMP-engineering-conventions.md) | 鉴权、业务错误和 secret 约束 |
| 运维 | [docs/knowledge/implementation/IMP-development-quickstart.md](docs/knowledge/implementation/IMP-development-quickstart.md) | MQ、重试、降级和部署排查 |
| 测试 | [docs/knowledge/implementation/IMP-development-quickstart.md](docs/knowledge/implementation/IMP-development-quickstart.md) | 测试分层、覆盖率和质量门禁 |
| 整体架构 | [docs/knowledge/implementation/IMP-architecture.md](docs/knowledge/implementation/IMP-architecture.md) | 服务、存储和主要数据流概览 |
| 实现映射 | [docs/knowledge/implementation/IMP-content-community-backend.md](docs/knowledge/implementation/IMP-content-community-backend.md) | 设计到代码的逐条追踪与状态 |
| 剩余门禁 | [docs/knowledge/implementation/IMP-todo-blocked-gates.md](docs/knowledge/implementation/IMP-todo-blocked-gates.md) | 人类评测集与月度 SLO |

迁移状态见 [docs/knowledge/TRANSITION.md](docs/knowledge/TRANSITION.md)。

## 命令入口

所有日常检查统一通过根目录 `Makefile`，先运行 `make help` 查看目标和参数。

- `make check`：格式、文档策略、`go vet` 和 golangci-lint。
- `make test`：所有 module 的 race 测试与包级覆盖率；额外参数用 `ARGS` 传入。
- `make coverage`、`make coverage-target`、`make coverage-no-gate`：覆盖率基线、终态或无门禁报告。
- `make integration-critical`：PR 核心集成测试；`make integration-init/run/clear`：分步管理完整集成测试；`make integration-all`：隔离环境内运行并无条件清理。
- `make fuzz FUZZ_TIME=10s`：限时 fuzz；`make quality`：运行标准本地质量门禁。

根据变更范围执行相关命令；完成时报告实际执行的命令和结果，不用未执行的全量检查代替证据。


## 工作流程

- 使用git worktree 在.worktree目录创建任务分支task/task-name 
- 在该工作树完成task
- 切回main分支，rebase该任务分支
- 处理冲突，提交并推送
- 删除task工作树和分支
