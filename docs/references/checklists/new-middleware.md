# 新中间件完工检查清单

> 适用场景：新增 go-zero HTTP 或 RPC 中间件

---

## 🚨 阻塞型检查

### 中间件签名与注册

- [ ] HTTP 中间件签名为 `func(http.HandlerFunc) http.HandlerFunc`
- [ ] RPC 中间件使用 `grpc.UnaryServerInterceptor` 或 `StreamServerInterceptor`
- [ ] 已在 `svc.ServiceContext` 中实例化
- [ ] 已在 `api/*.api` 的 `@server(middleware:)` 中注册
- [ ] goctl 重新生成后路由正确应用中间件

### Context 处理

- [ ] 中间件**必须**将必要信息通过 `context.WithValue` 传递到后续 handler
- [ ] 新增的 ctx key 使用**自定义类型**（避免字符串冲突）
  ```go
  type ctxKey int
  const userIDKey ctxKey = iota
  ```
- [ ] **不要**丢失原 ctx（始终从 `r.Context()` 派生）
- [ ] 在 handler 侧通过类型断言取值，处理断言失败情况

### 错误处理

- [ ] 中间件拦截请求时使用 `httpx.Error(w, err)` 返回标准错误
- [ ] 错误码走 `common/errorx` 统一定义
- [ ] **不要**在中间件里 `panic`（除非有 recover middleware 兜底）
- [ ] 日志使用 `logx.WithContext(r.Context())`

### 执行顺序

- [ ] 中间件顺序符合预期（见 `best-practices/middleware.md`）：
  ```
  Recover → RequestID → Tracing → Metrics → Auth → RateLimit → 业务
  ```
- [ ] **早退出**中间件（auth / ratelimit）放在业务中间件之前
- [ ] **兜底**中间件（recover / metrics）放在最外层

### 性能

- [ ] 中间件**不做**阻塞 IO（除非业务必需）
- [ ] 缓存可缓存的结果（如 JWT 公钥、权限列表）
- [ ] **不要**在每个请求里重新解析配置

### 测试

- [ ] 单元测试覆盖：
  - [ ] 正常通过场景
  - [ ] 拒绝场景（返回错误）
  - [ ] ctx 值正确注入
  - [ ] 异常场景（panic、超时）
- [ ] 使用 `httptest.NewRecorder` + `httptest.NewRequest` 构造测试
- [ ] 至少一个集成测试验证中间件链路顺序

---

## ⚠️ 建议型检查

### 可配置性

- [ ] 中间件行为可通过配置控制（启用 / 禁用、白名单、阈值）
- [ ] 配置走 `etc/*.yaml`，不硬编码

### 可观测性

- [ ] 中间件自身有 metrics（通过次数、拒绝次数、延迟）
- [ ] 关键决策点有 debug 日志（生产默认关闭）

### 幂等性

- [ ] 中间件**不应**修改请求体（除非显式设计，如解压、解密）
- [ ] 如修改请求体，确保 `r.Body` 可重新读取

---

## 常见陷阱

### JWT 中间件特有

- [ ] Token 过期时间校验
- [ ] 签名算法白名单（防止 `alg: none` 攻击）
- [ ] Refresh token 与 Access token 分开
- [ ] 黑名单 / 吊销列表机制（如需要）

### 限流中间件特有

- [ ] 限流维度明确（IP / user / tenant / global）
- [ ] 达到阈值返回 `429 Too Many Requests`，带 `Retry-After` header
- [ ] 使用 go-zero 内置 `periodlimit` / `tokenlimit` 而非自造轮子

### 鉴权中间件特有

- [ ] 401 vs 403 语义清晰（未认证 vs 无权限）
- [ ] 不在响应体暴露鉴权内部细节
- [ ] 失败次数限制（防爆破）
