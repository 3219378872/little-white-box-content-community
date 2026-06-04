# 安全

esx 项目安全模式概要。详细模式与代码示例参见 [references/security.md](references/security.md)。

## 核心安全措施

- **JWT 鉴权**：`pkg/jwtx/` — HS256 签名，防 `alg=none` 攻击，context 透传 userId
- **可选鉴权中间件**：`pkg/middleware/` — 公开接口有 token 则解析，无 token 不拦截
- **CORS**：`pkg/middleware/` — 白名单 origin 控制
- **gRPC 拦截器**：`pkg/interceptor/` — 业务错误跨进程传播，不泄露内部堆栈
- **输入校验**：`pkg/validator/` — 手机号/密码/用户名格式校验

## 必须遵守

- 不硬编码 secret，敏感值走环境变量
- 所有用户输入经过 validator 校验
- 错误消息不泄露敏感数据（errx 统一包装）
- 新增依赖需经用户批准

## 详细参考

- [安全模式完整文档](references/security.md)
- [生产就绪检查清单](references/checklists/production-readiness.md)
