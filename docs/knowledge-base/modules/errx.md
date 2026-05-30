---
title: errx
tracks:
  - pkg/errx/
last_synced_commit: 8ce9ecc
last_synced_date: 2026-05-30
sync_note: ""
---

# errx

## 职责
统一业务错误模型。提供 `BizError`（携带数值错误码 + 消息）、集中错误码表，以及
RPC↔HTTP 边界上的错误转换，使错误码在 gRPC `status` 与 HTTP 响应体之间无损往返。

## 公开接口与契约
- `New(code, msg)` / `NewWithCode(code)` / `Wrap(...)` — 构造业务错误，Logic 层唯一允许的错误返回方式。
- `Is`、`GetCode`、`GetMsg` — 错误码判定与提取。
- `FromGRPCError(err)` — 把 gRPC `status` 还原为 `BizError`（gateway 调 RPC 后用）。
- `FromHTTPError` / `resolve_http.go` — 把 `BizError` 映射为 HTTP 状态码与统一响应体。
- 错误码分段见 `codes.go`：通用 1-999 · 用户 1000-1999 · 内容 2000-2999 · 交互 3000-3999 · 媒体 4000-4999 · 搜索 5000-5999。

## 上游
所有 RPC 服务的 Logic 层、gateway 的 Logic/中间件。

## 下游
- [interceptor](interceptor.md) 在 gRPC 服务端把 `BizError` 编码进 `status`。
- gateway 错误中间件把 `BizError` 转成 HTTP 响应。

## 关键文件
- `codes.go` — 集中错误码常量。
- `errors.go` — `BizError` 类型与构造函数。
- `resolve_grpc.go` / `resolve_http.go` — 双向转换。

## 注意事项与陷阱
- Logic 层禁止裸 `errors.New()`；必须 `errx.New(code, msg)`。
- 新增错误码必须落在对应业务段，不可跨段复用。
- HTTP 状态码映射由中间件统一处理，Handler 不得手动 `httpx.Error`。
