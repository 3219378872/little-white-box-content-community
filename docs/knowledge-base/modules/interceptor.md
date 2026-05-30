---
title: interceptor
tracks:
  - pkg/interceptor/
last_synced_commit: 8ce9ecc
last_synced_date: 2026-05-30
sync_note: ""
---

# interceptor

## 职责
gRPC 服务端一元拦截器，把 Logic 层返回的 `errx.BizError` 编码进 gRPC `status`，
使错误码可跨 RPC 边界传播。

## 公开接口与契约
- `BizErrorUnaryInterceptor(...)` — `grpc.UnaryServerInterceptor`，在响应返回前把
  `BizError` 转为带错误码的 `status`。

## 上游
各 RPC 服务在 `internal/server` / zrpc 启动时注册该拦截器。

## 下游
- [errx](errx.md)：消费 `BizError`，产出可被 `errx.FromGRPCError` 还原的 `status`。

## 关键文件
- `biz_error.go` — 拦截器实现。

## 注意事项与陷阱
- 与 gateway 侧 `errx.FromGRPCError` 成对使用，二者错误码编码方式必须一致。
- 拦截器只负责错误编码，不吞掉非业务错误（系统错误仍按原样返回）。
