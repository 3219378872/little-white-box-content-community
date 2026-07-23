# 安全与错误速查

## 必须遵守

- JWT 由 `pkg/jwtx` 签发、校验并写入 context；不要在业务层复制 token 解析。
- 鉴权、可选鉴权和 CORS 使用 `pkg/middleware` 中已有实现。
- 用户输入经过 `pkg/validator`；错误响应不泄露内部堆栈、凭据或存储细节。
- secret 只能来自环境变量，配置文件只保留占位或非敏感默认值。
- 错误码集中在 `pkg/errx/codes.go`；Logic 不返回裸字符串错误。

## 排查顺序

先确认请求是否经过正确 middleware，再检查 context claim、Logic 错误码和 HTTP/gRPC 转换；不要直接在 Handler 改状态码来掩盖错误。
