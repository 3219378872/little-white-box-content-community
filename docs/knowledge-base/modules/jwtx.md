---
title: jwtx
tracks:
  - pkg/jwtx/
last_synced_commit: 8ce9ecc
last_synced_date: 2026-05-30
sync_note: ""
---

# jwtx

## 职责
JWT 令牌的签发与校验，以及把已认证身份（userId / username）通过 `context.Context`
在调用链中透传。

## 公开接口与契约
- `GenerateToken(...)` / `ParseToken(...)` — 签发与解析。
- `WithUserIdContext` / `WithUsernameContext` / `WithClaimsContext` — 写入 ctx。
- `GetUserIdFromContext` / `GetUsernameFromContext` — 必选读取（缺失即错误）。
- `GetOptionalUserIdFromContext` — 可选读取（匿名可访问的接口用）。

## 上游
gateway 鉴权中间件解析 Token 后写入 ctx；各 Logic 从 ctx 读取身份。

## 下游
`context.Context`，被 [middleware](middleware.md) 与各服务 Logic 消费。

## 关键文件
- `jwt.go` — 签发/解析与 claims。
- `context.go` — ctx 读写辅助。

## 注意事项与陷阱
- 身份只走 ctx，禁止从 `http.Request` 直接取。
- 必选与可选读取分开：可选接口用 `GetOptionalUserIdFromContext`，避免匿名请求被误判为错误。
- 签名密钥走配置/环境变量，禁止硬编码。
