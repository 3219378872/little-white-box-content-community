---
implementation: IMP-content-community-backend
verified_at: 2026-08-14
verified_commit: 4eef6ba
commands:
  - go test ./pkg/errx/ ./app/gateway/...
  - make check
  - make test
result: passed
---

# 2026-08-14 业务错误 HTTP 状态映射补齐（CORE-051）

## 缺陷

全仓 39 个业务码逐一对表 `BizError.HTTPStatus()`：`PasswordError`（登录密码
错误）、`VerifyCodeError`/`VerifyCodeExpired`（验证码错误/过期）未在映射中，
落入 default 500——客户端可正常触发的条件（输错密码、输错验证码）返回
HTTP 500，误导客户端与监控（错误率告警）。`SearchEmpty`/`SearchTimeout`
此前依赖网关 `httpxconfig` 的特殊 switch 覆盖，属于双处维护。

## 修复

- `pkg/errx/errors.go`：`HTTPStatus()` 成为唯一映射——
  - `PasswordError` → 401（认证失败，与 TokenExpired/TokenInvalid 一致）；
  - `VerifyCodeError`/`VerifyCodeExpired` → 400（客户端输入错误）；
  - `SearchEmpty` → 400、`SearchTimeout` → 504（从网关迁移到单一来源）。
- `app/gateway/internal/httpxconfig/errors.go`：删除冗余的 SearchEmpty/
  SearchTimeout 特殊 switch，完全依赖 `HTTPStatus()`。
- 新增 `TestHTTPStatus_MapsEveryBusinessCode`：全部 39 个业务码的表驱动断言，
  防止未来新增业务码落入默认 500。

## 审查证据

- gRPC 语义派生自 `HTTPStatus()`（`resolve_grpc.go`），密码错误 → Unauthenticated、
  验证码错误 → InvalidArgument、搜索超时 → Internal，与 HTTP 一致。
- `go test ./pkg/errx/ ./app/gateway/...`：全部通过。
- `make check`：exit 0；`make test`：85 包 0 失败。

## 结果

- 客户端可触发的业务条件全部映射为明确 4xx/5xx；HTTP 与 gRPC 语义一致。

## 未覆盖边界

- `UploadFailed`/`MediaProcessFailed` 保持 500（服务端故障）；网关 JSON 错误
  信封（code/message）未变，客户端按业务码解析不受影响。
