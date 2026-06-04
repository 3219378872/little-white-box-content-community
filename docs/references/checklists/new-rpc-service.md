# 新 RPC 服务完工检查清单

> 适用场景：新增一个 go-zero zrpc 服务，或新增一个 RPC 方法

---

## 🚨 阻塞型检查

### Proto 定义

- [ ] `.proto` 文件语法正确（`protoc --dry-run` 通过）
- [ ] 消息字段编号**稳定**（已分配的编号不可修改或复用）
- [ ] 字段命名使用 `snake_case`
- [ ] 必填字段明确，可选字段使用 `optional`（proto3）
- [ ] 枚举第一个值为 `UNSPECIFIED = 0`
- [ ] 已运行 `goctl rpc protoc xxx.proto --go_out=. --go-grpc_out=. --zrpc_out=.`

### 服务端实现

- [ ] `internal/logic/xxxlogic.go` 实现 RPC 方法
- [ ] 所有 RPC 方法签名与生成的 interface 一致
- [ ] 入参校验在进入业务逻辑前完成
- [ ] 返回 `error` 使用 `status.Error(codes.XXX, "msg")` 或 `errorx` 包装
- [ ] 日志使用 `logx.WithContext(ctx)`

### 客户端使用

- [ ] 调用方通过 `zrpc.MustNewClient(c.XxxRpc).XxxService()` 获取 client
- [ ] **禁止**在请求处理路径中 `zrpc.NewClient()`（每次新建连接）
- [ ] Client 配置走 `etc/*.yaml`（含 etcd 或 target 地址）
- [ ] 客户端调用透传 `ctx`

### 服务发现与注册

- [ ] 服务注册使用 etcd（配置 `Etcd.Hosts` 和 `Etcd.Key`）
- [ ] 开发环境 / 生产环境使用不同的 etcd key 前缀（避免串调用）
- [ ] Health check 已启用（`Mode: dev/pro`）
- [ ] 服务名遵循命名规范（`业务.服务.rpc` 或 `business.service.rpc`）

### Resilience

- [ ] 客户端配置 `Timeout`（默认 2s 通常不够，评估业务实际）
- [ ] 客户端配置 `NonBlock: true`（避免启动阻塞）
- [ ] 关键调用考虑添加重试策略（谨慎，避免放大故障）
- [ ] 熔断器行为符合预期（go-zero 内置 breaker）

### 测试

- [ ] 单元测试覆盖所有 RPC 方法（成功 + 失败路径）
- [ ] 集成测试启动 mini etcd + 真实 server + 真实 client 链路
- [ ] Mock 只用于外部依赖，**不**直接 mock `pb.XxxServiceClient`
- [ ] 覆盖率 ≥ 80%

---

## ⚠️ 建议型检查

### 可观测性

- [ ] RPC 方法有 Prometheus metrics（`rpc_server_duration_seconds` 等）
- [ ] 链路追踪 ctx 正确透传（Jaeger/SkyWalking span）
- [ ] 错误日志包含 method name、peer address

### 性能

- [ ] 大对象考虑流式 RPC（避免单次响应过大）
- [ ] 频繁调用考虑批量 API（减少网络往返）
- [ ] 客户端连接池配置合理

### 安全

- [ ] 内部 RPC 考虑 mTLS
- [ ] 敏感方法有鉴权拦截器
- [ ] 入参对象大小有上限

### Proto 向后兼容

- [ ] **禁止**删除已发布的字段
- [ ] **禁止**修改已发布字段的编号或类型
- [ ] 新增字段使用未使用的编号

---

## 验证命令

```bash
# Proto 语法检查
protoc --dry-run xxx.proto

# goctl 校验
goctl check

# 连接测试
grpcurl -plaintext localhost:8080 list

# 完整测试
go test ./... -race -cover
```
