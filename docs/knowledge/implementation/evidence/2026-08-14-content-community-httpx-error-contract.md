---
implementation: IMP-content-community-backend
verified_at: 2026-08-14
verified_commit: 5386689
commands:
  - go test ./app/gateway/internal/httpxconfig/
  - make check
  - make test
result: passed
---

# 2026-08-14 网关公开 JSON 错误契约可测试化（CORE-051 收尾）

## 背景

errx-http-status 批次修复了业务码 HTTP 映射，但网关的
`ConfigureErrors`（安装公开 JSON 错误信封）本身没有直接单测——客户端可见的
(code, message) 信封与状态映射只有间接覆盖。

## 本批次改动

- `app/gateway/internal/httpxconfig/errors.go`：把错误 → (HTTP 状态, JSON 信封)
  映射提取为 `MapError(err)`，`ConfigureErrors` 安装的 handler 直接调用。
- 新增 `errors_test.go` 四组测试：
  - BizError 透传（PasswordError → 401 + code/message 信封）；
  - 认证/验证码/搜索状态表（401/400/400/504/401/409/404）；
  - 包装 BizError（fmt.Errorf %w）保持业务码与状态；
  - 普通错误经 `FromHTTPError` 映射为 ParamError/400。

## 审查证据

- `go test ./app/gateway/internal/httpxconfig/`：4 项全过。
- `make check`：exit 0；`make test`：86 包（新增 httpxconfig 包测试）0 失败。

## 结果

- 网关公开错误契约（状态 + 信封）具备直接单测，未来业务码映射变化有回归保护。

## 未覆盖边界

- JWT 失败路径由 `Unauthorized` 独立处理（LoginRequired/401 信封），未纳入本测试。
