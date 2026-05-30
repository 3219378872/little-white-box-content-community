---
title: error-propagation
tracks:
  - pkg/errx/
last_synced_commit: 8ce9ecc
last_synced_date: 2026-05-30
sync_note: ""
---

# 错误码传播（Logic → gRPC → HTTP）

业务错误码如何在不丢失语义的前提下跨 RPC 与 HTTP 边界传播。

## 步骤

1. **产生**：RPC 服务 Logic 返回 `errx.New(code, msg)`（[errx](../modules/errx.md)），
   错误码取自 `pkg/errx/codes.go` 的对应业务段。
2. **编码**：[interceptor](../modules/interceptor.md) 的
   `BizErrorUnaryInterceptor` 在服务端把 `BizError` 写入 gRPC `status`（保留数值码）。
3. **解码**：[gateway](../modules/gateway.md) 调用 RPC 后用 `errx.FromGRPCError`
   把 `status` 还原成 `BizError`。
4. **映射**：gateway 的 `errx` HTTP 中间件（`resolve_http.go`）把业务码映射为 HTTP
   状态码与统一响应体，错误消息对外友好、不泄露内部细节。

## 错误码分段
通用 1-999 · 用户 1000-1999 · 内容 2000-2999 · 交互 3000-3999 · 媒体 4000-4999 · 搜索 5000-5999。

## 不变量
- Logic 层禁止裸 `errors.New()`。
- Handler 禁止手动 `httpx.Error` 设状态码。
- 新增错误码必须落在正确业务段并集中定义。
