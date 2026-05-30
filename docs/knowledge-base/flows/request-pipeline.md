---
title: request-pipeline
tracks:
  - app/gateway/
last_synced_commit: 8ce9ecc
last_synced_date: 2026-05-30
sync_note: ""
---

# 请求处理链（REST → RPC）

一个外部 REST 请求从进入网关到返回响应的规范路径。

## 步骤

1. **路由 + 中间件**：请求命中 goctl 生成的 routes，先过 CORS，再过
   [middleware](../modules/middleware.md) 的鉴权（强制或可选）。鉴权成功后由
   [jwtx](../modules/jwtx.md) 把 userId/username 写入 `context.Context`。
2. **Handler（生成代码）**：仅做参数绑定（`httpx.Parse`）并调用对应 Logic，不含业务逻辑。
3. **Logic**：从 `svc.ServiceContext` 取 zrpc 客户端，**透传入参 ctx** 调用下游
   RPC（[user](../modules/user.md) / [content](../modules/content.md) /
   [media](../modules/media.md) / [interaction](../modules/interaction.md) 等）。
4. **RPC 服务端**：[interceptor](../modules/interceptor.md) 把 Logic 返回的
   [errx](../modules/errx.md) `BizError` 编码进 gRPC `status`。
5. **错误归一**：gateway 用 `errx.FromGRPCError` 还原业务码，再由 `errx` 中间件
   映射为 HTTP 状态码与统一响应体。

## 不变量
- Handler 不写业务逻辑、不手动设置 HTTP 状态码。
- 所有日志带 ctx（`logx.WithContext`）；所有 zrpc 调用透传 ctx。
- 业务错误码全程保持（不被降级为裸字符串错误）。

详见 [error-propagation](error-propagation.md)。
