---
title: user
tracks:
  - app/user/
last_synced_commit: 8ce9ecc
last_synced_date: 2026-05-30
sync_note: ""
---

# user

## 职责
用户 RPC 服务（:9090）。负责注册、登录、验证码、资料更新、批量取用户、关注关系等。

## 公开接口与契约
- proto：`proto/user/user.proto`（改后须重新生成 rpc 代码）。
- 代表性 Logic：`register_logic.go`、`send_verify_code_logic.go`、`update_profile_logic.go`、`follow_logic.go`、`batch_get_users_logic.go`。
- 服务端注册 [interceptor](interceptor.md) 的 `BizErrorUnaryInterceptor`。

## 上游
[gateway](gateway.md) 经 zrpc 调用。

## 下游
- MySQL（go-zero `CachedConn`）+ Redis 缓存（[cachex](cachex.md)）。
- [util](util.md)（雪花 ID/密码哈希）、[validator](validator.md)（输入校验）、[errx](errx.md)（错误码）。

## 关键文件
- `rpc/user.go` — 服务入口。
- `rpc/internal/logic/*` — 业务逻辑。
- `rpc/internal/model/*` — 数据访问。
- `rpc/internal/config/config.go` — 配置结构体。

## 注意事项与陷阱
- 密码只存哈希（`util.HashPassword`），禁止记录明文。
- 注册/资料更新前置 `validator` 校验，失败转 `errx` 用户段（1000-1999）。
- 跨 Model 访问由 Logic 协调，Model 间不得直接互调。
