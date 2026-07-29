# AGENTS.md

这是 esx（little-white-box）社交内容平台仓库。`AGENTS.md` 是唯一的项目规则入口；
任务资料按 [docs/INDEX.md](docs/INDEX.md) 路由，禁止默认读取整个 `docs/`。

## 项目事实

- 根模块为 `esx`，Go 版本见 `go.mod`，框架为 go-zero。
- `app/` 包含 gateway、用户/内容/媒体/互动/feed/message 等 RPC 或 MQ 模块。
- `pkg/` 包含错误、鉴权、中间件、事件、MQ、缓存和测试工具等共享库。
- `deploy/docker-compose.middleware.yml` 提供本地 MySQL、Redis、etcd、RocketMQ、搜索、对象存储和观测组件。
- 运行时事实以代码、配置、`.api`、`.proto` 和 `docs/generated/` 为准；设计文档不能覆盖代码行为。

## 不可违反的规则

- Handler 只绑定参数、调用 Logic、返回响应；业务逻辑放在 Logic，数据访问放在 Model。
- 所有请求上下文必须透传；日志使用 `logx.WithContext(ctx)`，RPC 调用使用入参 ctx。
- Logic 返回 `pkg/errx` 中定义的业务错误；禁止裸 `errors.New` 和 Handler 手动设置 HTTP 错误状态。
- 配置从 `etc/*.yaml` 和 `config.Config` 注入；secret 只能通过环境变量，禁止硬编码。
- 禁止手动编辑 goctl/protobuf 生成文件；修改 `.api` 或 `.proto` 后必须重新生成并检查差异。
- 不为通过测试修改测试；修复实现并覆盖至少一个失败路径。
- 不引入新依赖，除非用户明确批准。

## 文档知识与地址

先通过 [docs/INDEX.md](docs/INDEX.md) 判断任务类型，只打开对应的 1～2 个 active 文档：

| 任务知识 | 文档地址 | 内容范围 |
| --- | --- | --- |
| REST API | [docs/active/api.md](docs/active/api.md) | Handler、参数校验和 `.api` 修改流程 |
| RPC | [docs/active/rpc.md](docs/active/rpc.md) | proto、服务间调用和代码生成要求 |
| 数据 | [docs/active/data.md](docs/active/data.md) | MySQL、Redis、事务和缓存边界 |
| 安全 | [docs/active/security.md](docs/active/security.md) | 鉴权、业务错误和 secret 约束 |
| 运维 | [docs/active/operations.md](docs/active/operations.md) | MQ、重试、降级和部署排查 |
| 测试 | [docs/active/testing.md](docs/active/testing.md) | 测试分层、覆盖率和质量门禁 |
| 整体架构 | [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) | 服务、存储和主要数据流概览 |
| 模块与流程事实 | [docs/generated/INDEX.md](docs/generated/INDEX.md) | 按代码生成的模块页和链路快照 |

`docs/active/` 是当前短规范，`docs/generated/` 是定位具体模块时使用的代码事实快照。运行时判断仍以代码、配置、API/proto 定义和测试结果为先；历史计划和日期设计文档仅在明确追溯时读取。

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
