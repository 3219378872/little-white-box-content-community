# Best Practices

go-zero 项目最佳实践速查。每个主题只列核心规则，详细代码示例见对应 reference 文件。

## Code Organization

### Project Structure

```
service-name/
├── etc/
│   └── config.yaml           # 配置文件
├── internal/
│   ├── config/
│   │   └── config.go         # Config 结构体
│   ├── handler/
│   │   └── *handler.go       # HTTP handlers（薄层）
│   ├── logic/
│   │   └── *logic.go         # 业务逻辑（厚层）
│   ├── middleware/
│   │   └── *middleware.go     # 自定义中间件
│   ├── svc/
│   │   └── servicecontext.go # 依赖注入
│   ├── types/
│   │   └── types.go          # 请求/响应类型
│   └── model/
│       └── *model.go         # 数据库 model
├── service.go                # 入口
└── service.api               # API 定义
```

### File Naming

```
# Handlers: <resource><action>handler.go
createuserhandler.go, getuserhandler.go

# Logic: <resource><action>logic.go
createuserlogic.go, getuserlogic.go

# Models: <table>model.go
usermodel.go, ordermodel.go

# Middleware: <purpose>middleware.go
authmiddleware.go, ratelimitmiddleware.go
```

## Configuration

- 嵌入 `rest.RestConf`（REST）或 `zrpc.RpcServerConf`（RPC）
- 使用 `json:",default=..."` 设默认值，`json:",optional"` 标记可选字段
- 敏感值走环境变量 `${ENV_VAR}`，不在 yaml 明文
- 用 `conf.MustLoad` 加载，注入到 ServiceContext

详细示例：[rest-api-patterns.md](../references/rest-api-patterns.md#configuration-pattern)

## Error Handling

- 包级别定义 `var ErrXxx = errors.New("...")`，或使用自定义 `BusinessError` 类型
- 用 `fmt.Errorf("context: %w", err)` 包装错误
- 用 `errors.Is()` 检查错误类型
- 注册 `httpx.SetErrorHandler` 统一映射 HTTP 状态码
- Logic 层返回领域错误，Handler 层不直接设置状态码

详细示例：[rest-api-patterns.md](../references/rest-api-patterns.md#error-handling)、[api-governance-patterns.md](../references/api-governance-patterns.md)

## Logging

- 使用 `logx.WithContext(ctx)` 结构化日志，**禁止** `logx.Info/Error` 裸调用
- **禁止**日志记录敏感信息（密码、token、身份证），PII 字段使用 mask 函数
- 生产环境用 JSON 编码，不开 DEBUG 级别
- 批量操作记录摘要而非逐条日志

详细示例：[observability-patterns.md](../references/observability-patterns.md)

## Testing

- 覆盖率目标 80%+，运行 `go test ./... -race -cover`
- Logic 层是主要测试对象，Mock ServiceContext 依赖
- 使用 table-driven tests
- 集成测试用 testcontainers 跑真实数据库
- 每个 Logic 方法至少一个成功路径 + 一个失败路径测试

详细示例：[testing-patterns.md](../references/testing-patterns.md)

## Performance

- 在 ServiceContext 中初始化连接池，**禁止**在 handler/logic 中创建连接
- 避免 N+1 查询，使用批量查询
- 用 `mr.MapReduce` 做并行处理（限制 worker 数量）
- 读热数据启用 cache（`NewUsersModel(conn, c.Cache)`）
- **禁止**创建无界 goroutine，使用 `threading.NewTaskRunner(n)`

详细示例：[resilience-patterns.md](../references/resilience-patterns.md)、[database-patterns.md](../references/database-patterns.md#caching-pattern)

## Security

- 所有用户输入加 `validate` tag，Logic 层用 `validator.New().Struct(req)` 校验
- 密码使用 `bcrypt` 哈希存储，**禁止**明文
- JWT 认证走 go-zero 内置 `jwt` 中间件
- **禁止**暴露内部错误给客户端（返回泛化消息）
- 文件路径参数用 `filepath.Clean` 防路径穿越

详细示例：[security-patterns.md](../references/security-patterns.md)

## Database

- 使用 `goctl model` 生成 Model，自定义方法加在 `customXxxModel` 上
- 传递 `context.Context` 到所有数据库操作
- 事务用 `DB.TransactCtx`，闭包内返回 error 自动回滚
- 更新操作优先用直接 SQL `UPDATE ... SET`，避免 read-modify-write
- 并发写用乐观锁（`version` 字段）或悲观锁（`SELECT FOR UPDATE`）
- `sql.Null*` 类型必须检查 `.Valid` 再访问值
- 动态 SQL 用 `[]string` 拼接 SET/WHERE 子句，**禁止**为每种字段组合硬编码

详细示例：[database-patterns.md](../references/database-patterns.md)、[concurrency-patterns.md](../references/concurrency-patterns.md)

## Deployment

- 多阶段 Docker 构建（builder + alpine），`CGO_ENABLED=0`
- Kubernetes 配置 liveness + readiness probe
- 设置资源 requests/limits
- 生产环境 `Mode: pro`（启用 load shedding）

详细示例：[deployment-patterns.md](../references/deployment-patterns.md)

---

## Summary

### Always Do

1. Handler 薄、Logic 厚，三层分离
2. 结构化日志 + context 传递
3. 显式处理所有错误
4. 彻底校验输入
5. 复用连接池
6. 读热数据启用缓存
7. 写单元测试 + 集成测试
8. 事务保证原子操作
9. 安全措施到位
10. 监控生产指标
11. `sql.Null*` 用辅助函数转换，动态 SQL 用 `[]string` 拼接

### Never Do

1. 业务逻辑放 Handler
2. 日志记录敏感信息
3. 忽略错误
4. Handler/Logic 中创建连接
5. 循环内逐条查询
6. 生产环境禁用 resilience 特性
7. 使用全局变量
8. 无超时阻塞
9. 创建无界 goroutine
10. 不校验就信任用户输入
11. 直接访问 `sql.NullString.String` 不检查 `.Valid`
