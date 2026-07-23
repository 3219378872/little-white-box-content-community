# 测试与交付速查

## 最低要求

- 每个 Logic 至少覆盖一个失败路径。
- 新增或修改的 Logic 使用表驱动用例，覆盖成功、条件分支、等价类、边界值和依赖失败。
- 边界值按适用类型覆盖 `N-1/N/N+1`、空值、零值、负值和 `max/max+1`。
- Gateway REST 路由必须在可执行决策表中至少有一条成功规则；JWT 路由还必须有无凭证规则。
- 纯 SQL 断言可以使用 sqlmock；涉及真实 DB/Redis/RPC 的集成行为使用仓库已有 testcontainers 工具。
- 不 mock `sqlx.SqlConn` 来伪造集成测试。
- `//go:build integration` 测试不属于默认套件，必须通过集成测试入口显式执行。

## 测试分层

- `make test`：所有 module 的默认测试，启用 race 和包级覆盖率；额外 go test 参数通过 `ARGS` 传入。
- `make coverage`：生成 `.artifacts/coverage/` 分层报告并执行当前基线门禁；`make coverage-target` 检查终态门槛，`make coverage-no-gate` 不执行门禁。
- `make integration-critical`：PR 使用的自包含核心 MySQL/Redis 集成测试，不依赖常驻中间件。
- `make integration-init`：创建隔离的 Docker network，启动 DTM 和使用 `deploy/seaweedfs/s3_config.json` 的 SeaweedFS S3，并等待端口就绪。
- `make integration-run`：在已经准备好的 DTM 和 SeaweedFS 环境中运行全部 integration-tag 测试，不创建或清理外部依赖。
- `make integration-clear`：删除 `integration-init` 创建的 DTM、SeaweedFS 容器和隔离 network；重复执行是安全的。
- `make integration-all`：创建隔离环境、运行全部 integration-tag 测试，并在成功、失败或收到终止信号后清理环境；CI 和本地使用同一入口。
- `make fuzz`：夜间限时 fuzz；通过 `FUZZ_TIME` 调整每个目标时长。

当测试进程运行在容器内并通过宿主机 Docker socket 启动 testcontainers 时，集成脚本会将 Docker bridge 网关追加到 `NO_PROXY/no_proxy`，避免本地健康探针被 HTTP 代理转发。

覆盖率只用于发现盲区，不能替代业务断言。门槛按 Handler、Logic、MQ consumer、手写 Model 和共享库分别计算，protobuf、goctl types/routes 等生成文件不计入。

REST 决策表以 Case ID 标识规则，至少记录方法、路径、认证状态、输入、预期 HTTP 状态和业务错误码。测试必须经过真实 Router、JWT/OptionalAuth 和统一错误处理；内部 RPC 可以使用可控 fake，夜间系统测试再替换为真实服务。

当前定时任务属于全量组件集成测试，不等同于启动 Gateway、RPC、MQ 与中间件后的系统黑盒测试。系统黑盒测试需要独立环境编排，并以真实 REST/RPC 服务替换决策表中的 fake RPC。

## 验证命令

```bash
make check
make test
make coverage
```

根据变更范围执行相关命令；报告实际执行结果。生成文件、文档链接和代码质量由 `make engineering-lint`、`make check` 与 CI 检查。
