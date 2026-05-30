---
title: middleware
tracks:
  - pkg/middleware/
last_synced_commit: 8ce9ecc
last_synced_date: 2026-05-30
sync_note: ""
---

# middleware

## 职责
gateway 的 HTTP 中间件集合：强制鉴权、可选鉴权、CORS。

## 公开接口与契约
- `AuthMiddleware(...)` — 强制鉴权；校验 JWT 后把身份写入 ctx，失败返回 401。
- `NewOptionalAuthMiddleware(...)` — 可选鉴权；有 Token 则解析，无 Token 放行（匿名）。
- `CORSMiddleware(...)` — 跨域响应头。
- `GetUserId` / `GetUsername` — 从中间件设置的 ctx 取身份（兼容旧 key）。

## 上游
gateway 路由装配（goctl 生成的 routes 绑定中间件）。

## 下游
- [jwtx](jwtx.md) 做实际解析与 ctx 读写。
- 下游 Logic 通过 ctx 取身份。

## 关键文件
- `auth.go` — 强制鉴权。
- `optional_auth.go` — 可选鉴权。
- `cors.go` — CORS。

## 注意事项与陷阱
- 强制与可选鉴权语义不同：可选接口必须用 optional 版本，否则匿名访问被拒。
- context key 兼容性由 `auth_compat_test.go` 守护，改动 key 需保持向后兼容。
