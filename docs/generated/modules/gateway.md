---
title: gateway
tracks:
  - app/gateway/
last_synced_commit: 8ce9ecc
last_synced_date: 2026-05-30
sync_note: ""
---

# gateway

## 职责
唯一对外的 REST API 网关（:8888）。负责参数绑定、鉴权、把 HTTP 请求聚合/转发到
user / content / media 等 RPC 服务，并把 `errx` 业务错误映射为统一 HTTP 响应体。

## 公开接口与契约
- API 定义：`app/gateway/gateway.api`（goctl 生成 handler/types/routes）。
- 业务分组 Logic：`internal/logic/login`、`internal/logic/posts` 等。
- 通过 zrpc 客户端调用各 RPC 服务，调用时透传 ctx。

## 上游
外部客户端（Web/App）。

## 下游
- [user](user.md) / [content](content.md) / [media](media.md) / [interaction](interaction.md) / [feed](feed.md) / [message](message.md) RPC。
- [middleware](middleware.md) 鉴权/CORS、[jwtx](jwtx.md) 身份、[errx](errx.md) 错误映射。

## 关键文件
- `gateway.go` — 服务入口。
- `gateway.api` — 接口契约（改后须重新 `goctl api go`）。
- `internal/logic/**` — 各分组业务编排。
- `internal/svc/servicecontext.go` — RPC 客户端等依赖注入。

## 注意事项与陷阱
- Handler 只做参数绑定/调用 Logic/返回；业务逻辑在 Logic。
- 禁止手改 `internal/handler/*`、`internal/types/types.go`、`internal/handler/routes.go`。
- 错误统一由 `errx` 中间件映射状态码，Handler 不得手动 `httpx.Error`。
- 可选鉴权接口须用 `NewOptionalAuthMiddleware`。
