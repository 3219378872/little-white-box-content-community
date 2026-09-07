# AGENTS.md

这是 esx（little-white-box）社交内容平台仓库。`AGENTS.md` 是唯一规则入口；知识从
[docs/knowledge/README.md](docs/knowledge/README.md) 按层加载，任务资料从
[docs/INDEX.md](docs/INDEX.md) 按需定位，禁止默认遍历整个 `docs/`。

## 项目事实

- 根模块为 `esx`，Go 版本见 `go.mod`，框架为 go-zero。
- `app/` 包含 Gateway、用户、内容、互动、Feed、搜索、推荐、行为和 Assistant 等服务。
- `pkg/` 包含错误、鉴权、中间件、事件、MQ、缓存和测试工具等共享代码。
- 运行时事实以源码、配置、`.api`、`.proto` 和测试为准，知识页面不能覆盖实际行为。

## 不可违反的代码规则

- Handler 只绑定参数、调用 Logic、返回响应；业务逻辑放 Logic，数据访问放 Model。
- 请求上下文必须透传；日志用 `logx.WithContext(ctx)`，RPC 调用使用入参 `ctx`。
- Logic 返回 `pkg/errx` 业务错误；禁止裸 `errors.New` 和 Handler 手动设置 HTTP 错误状态。
- 配置从 `etc/*.yaml` 和 `config.Config` 注入；secret 只经环境变量，禁止硬编码。
- 禁止手改 goctl/protobuf 生成文件；改 `.api` 或 `.proto` 后运行 `make generate` 并检查差异。
- 不为通过测试修改测试；修复实现并覆盖至少一个失败路径。
- 不引入新依赖，除非用户明确批准。

## 知识治理

- 正式链路为 `INT -> SPEC -> DES -> IMP <-> EVD`；先读已批准意图/规格，再读当前设计、唯一实现映射
  和对应证据。指南、状态页、提案与 legacy 证据不属于正式证明链。
- 意图与规格由人类决定语义；agent 修改受保护路径只需当前对话的自然语言授权。范围不清、冲突或
  需要新增产品语义时停止并询问；未授权建议只写 `docs/knowledge/proposals/`。
- `owner: human` 表示语义所有权，不限制获授权的 agent 执笔；只有人类明确批准才能把 INT/SPEC 标为
  `approved`。设计不得从代码反推并改写上层语义。
- 当前 DES 必须逐条 `tracks` approved SPEC requirement；每条 requirement 恰有一个当前 IMP owner。
- IMP 页头只记录 `active/retired` 生命周期；条款行在有效的 passed 覆盖组支持下才可 `aligned`。
  未验证用 `unknown`，已知偏离用 `diverged` 并写 `gap: ...`；引用、反向关系和聚合状态由工具推导。
- EVD 只记录实际运行命令、受控 scope、可达的完整 commit SHA 与真实结果；不得用旧证据、合成数据或
  未执行命令关闭浏览器、设备、真人评审、live-provider 或生产门禁。

## 按需路由

| 任务 | 入口 |
| --- | --- |
| REST/RPC、数据、运维、测试 | [开发与运维指南](docs/knowledge/guides/development-quickstart.md) |
| 鉴权、错误、安全与工程约定 | [工程约定](docs/knowledge/guides/engineering-conventions.md) |
| 服务与数据流概览 | [架构指南](docs/knowledge/guides/architecture.md) |
| 产品与工程要求 | [意图](docs/knowledge/intent/README.md) / [规格](docs/knowledge/spec/README.md) |
| 当前设计 | [设计索引](docs/knowledge/design/README.md) |
| 逐条实现状态 | [实现索引](docs/knowledge/implementation/README.md) |
| 验证结果与开放门禁 | [证据索引](docs/knowledge/evidence/README.md) / [状态页](docs/knowledge/status/open-gates.md) |

## 命令入口

先运行 `make help`，首次用 `make knowledge-setup` 安装隔离工具依赖。公共门禁是 `make engineering-lint`；
`make knowledge-index` 更新生成索引，`make knowledge-export REF=<sha>` 只读导出历史清单。按范围再运行：

- `make check`：格式、文档策略、`go vet`、golangci-lint 和 `govulncheck ./...`。
- `make test`：所有 module 的 race 测试与包级覆盖率；额外参数用 `ARGS`。
- `make integration-critical`：PR 核心集成测试；完整隔离集用 `make integration-all`。
- `make fuzz FUZZ_TIME=10s`；标准本地质量门禁用 `make quality`。

完成时只报告实际执行的命令和结果，不用未执行检查代替证据。

## 工作流程

- 在 `.worktree/task-<name>` 创建 `task/<name>` 分支并完成任务。
- 回到 main 更新基线，在任务树 rebase 并解决冲突，复跑相关检查。
- main 只 fast-forward 合并；验证后提交并推送，再删除任务工作树和分支。
