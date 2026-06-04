# 新 Handler 完工检查清单

> 适用场景：新增一个 HTTP Handler（含对应 Logic、types、routes）

---

## 🚨 阻塞型检查（未通过不得合并）

### 代码生成

- [ ] `.api` 文件已更新，包含新增的 type 和 handler 定义
- [ ] 已运行 `goctl api go -api xxx.api -dir . --style go_zero` 重新生成
- [ ] 生成的 `internal/types/types.go`、`internal/handler/routes.go` 已包含新路由
- [ ] **未手动编辑** goctl 生成的文件（手写改动会在下次生成时丢失）

### 分层约定

- [ ] Handler 层**仅做**：参数绑定 → 调用 Logic → 返回响应
- [ ] Handler 层**没有**业务逻辑（条件判断、循环、数据转换）
- [ ] Logic 层通过 `svc.ServiceContext` 访问资源，没有全局变量
- [ ] Logic 层**没有直接**访问 `http.Request` / `http.ResponseWriter`
- [ ] Model 层**没有**跨 Model 直接调用（需 Logic 协调）

### Context 传递

- [ ] 所有日志使用 `logx.WithContext(ctx).Info/Error(...)`，**没有** `logx.Info/Error(...)` 裸调用
  - 验证：`grep -rn "logx\.\(Info\|Error\|Debug\)(" internal/ | grep -v WithContext`
- [ ] Logic 内发起的 zrpc 调用透传 `l.ctx`
- [ ] 内部 goroutine **没有**使用外层 ctx（避免超时误触发）
- [ ] **没有** `context.Background()` 新建 ctx（最外层入口除外）

### 错误处理

- [ ] Logic 层返回错误使用统一的错误包装（推荐 `errorx.New(code, msg)` 或项目自定义的错误类型），避免裸 `errors.New("...")` 字符串错误
  - 验证：`grep -rn "errors\.New" internal/logic/`
  - 注：`errorx` 包的具体实现取决于项目，此处为推荐模式
- [ ] 错误码集中定义（推荐 `common/errorx/codes.go`），**没有**魔法数字
- [ ] Handler 层**没有**手动 `httpx.Error` 设置 HTTP 状态码（由错误处理中间件统一处理）
- [ ] 所有错误路径都有对应的错误码和用户可读消息

### 参数校验

- [ ] 所有用户输入字段都有 `validate` tag（必填、长度、格式）
- [ ] 敏感字段（email、phone、url）使用标准校验器
- [ ] 分页参数有上限（page_size ≤ 100）
- [ ] ID 参数做了合法性检查（非零、非负）

### 测试（TDD 要求）

- [ ] 每个 Logic 方法至少一个**成功路径**单元测试
- [ ] 每个 Logic 方法至少一个**失败路径**单元测试（错误码返回）
- [ ] 涉及 DB 的测试使用 `sqlmock` 或 testcontainers（**不**直接 mock SqlConn 接口）
- [ ] 所有测试先写并见过 RED（失败）状态才写的实现
- [ ] 运行 `go test ./... -race -cover`，覆盖率 ≥ 80%
  - 命令：`go test -coverprofile=cover.out ./internal/logic/...`

### 配置

- [ ] **没有**硬编码（URL、端口、超时、Key、Secret）
  - 验证：`grep -rnE "(http://|https://|:[0-9]{4,5}|time\.Second \* [0-9]+)" internal/logic/`
- [ ] 新增配置项加入 `internal/config/config.go` 并更新 `etc/*.yaml` 示例
- [ ] 敏感配置用 `${ENV_VAR}` 占位，**没有**明文 secret

---

## ⚠️ 建议型检查（未通过需评估）

### 可观测性

- [ ] 关键业务动作有 `logx.WithContext(ctx).Infow(...)` 结构化日志
- [ ] 错误路径日志包含足够的上下文（用户 ID、请求 ID、参数）
- [ ] 关键接口有 Prometheus 指标（请求数、延迟、错误率）
- [ ] 跨服务调用有链路追踪 span

### 性能

- [ ] 数据库查询有索引支持（看 EXPLAIN）
- [ ] N+1 查询已避免（批量查询或 JOIN）
- [ ] 热点数据考虑缓存策略（参考 `references/database-patterns.md#caching-pattern`）
- [ ] 慢查询阈值配置合理

### 安全

- [ ] 鉴权：是否需要 JWT 中间件（检查 routes.go）
- [ ] 权限：是否需要业务级 ACL 校验
- [ ] SQL 使用参数化查询，**没有**字符串拼接
- [ ] 用户输入在返回前做了 XSS 过滤（如返回 HTML 片段）

### 代码质量

- [ ] `go vet ./...` 无警告
- [ ] `golangci-lint run` 无 error 级别问题
- [ ] `goctl check` 通过
- [ ] 无 `// TODO` / `// FIXME` 遗留（或已创建 issue 跟踪）

---

## 验证命令一键执行

```bash
set -e
echo "=== 代码生成检查 ==="
goctl check

echo "=== Lint ==="
go vet ./...
golangci-lint run --timeout=5m

echo "=== 测试 + 覆盖率 ==="
go test ./... -race -coverprofile=cover.out
go tool cover -func=cover.out | tail -1

echo "=== 敏感模式扫描 ==="
! grep -rn "errors\.New" internal/logic/ || (echo "❌ 存在 errors.New"; exit 1)
! grep -rn "logx\.\(Info\|Error\|Debug\)(" internal/ | grep -v WithContext || (echo "❌ 存在无 ctx 日志"; exit 1)

echo "✅ 所有检查通过"
```

---

## 如果某项失败

2. **不要修改测试让它通过**——修代码，不修测试
3. **不要注释掉失败的断言**——解决问题
