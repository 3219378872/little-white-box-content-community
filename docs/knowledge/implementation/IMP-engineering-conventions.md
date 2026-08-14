---
id: IMP-engineering-conventions
layer: implementation
title: 工程约定（分层、安全、可靠性、质量）
status: aligned
owner: agent
upstream:
  - DES-content-community-backend
tracks:
  - pkg/errx
  - pkg/jwtx
  - pkg/middleware
  - pkg/interceptor
  - pkg/validator
  - deploy/
verified_at: 2026-08-14
verified_commit: bea6c09
---

# 工程约定（分层、安全、可靠性、质量）

本文档合并旧顶层 DESIGN、SECURITY、RELIABILITY 与 QUALITY_SCORE 规范文档。
AGENTS.md 与开发流程引用本页；实现入口以代码为准。

## 三层架构

| 层 | 职责 | 禁止 |
| --- | --- | --- |
| Handler | 参数绑定、调用 Logic、返回响应 | 写业务逻辑 |
| Logic | 业务逻辑，通过 `svc.ServiceContext` 获取资源 | 直接访问 `http.Request` |
| Model | 数据访问 | 跨 Model 直接调用 |
| Svc | 依赖注入容器（DB / Redis / RPC 客户端等） | — |

## Context 传递

- **必须** `logx.WithContext(ctx)` — 禁止不带 ctx 的日志。
- **必须** 所有 zrpc 调用透传入参 ctx。
- **必须** goroutine 内使用 ctx 的拷贝并处理取消。
- **禁止** `context.Background()` 新建 ctx（除非最外层入口）。

## 错误处理

- Logic 统一返回 `errx.New(code, msg)`；错误码集中于 `pkg/errx/codes.go`。
- HTTP 状态码映射由 errx 中间件统一处理；**禁止**裸 `errors.New` 字符串错误与
  Handler 手动设置 HTTP 错误状态。
- 框架 gRPC 错误只保留业务码，不暴露原始消息（CORE-054）。

## 配置管理

- 配置走 `etc/*.yaml` → `config.Config`；secret 只能通过环境变量。
- **禁止** 硬编码任何配置值；新增依赖需经用户批准。

## 安全

- JWT 由 `pkg/jwtx` 签发/校验并写入 context；业务层不复制 token 解析。
- 可选鉴权与 CORS 使用 `pkg/middleware` 已有实现。
- 用户输入经过 `pkg/validator`；错误响应不泄露内部堆栈、凭据或存储细节。
- secret 只来自环境变量；配置文件只保留占位或非敏感默认值。
- 完整客户端 IP 不得写入行为分析表（只存 SHA-256 哈希，REL-021）。

## 弹性与可靠性

- go-zero 内置防护：Load Shedding → Rate Limiting → Circuit Breaker → Timeout。
- 事务 outbox：权威写入与 outbox 同事务提交，relay 保证投递与幂等。
- RocketMQ 消费者处理重试、幂等与不可恢复错误；outbox relay 有界指数退避。
- 权威写入已提交后，缓存失效/索引/通知等异步效果失败不改变成功响应（CORE-053）。

## 质量门禁

- 每个 Logic 至少一个失败路径测试；表驱动覆盖成功/分支/边界/依赖失败。
- 单元测试 + 集成测试（DB/Redis/RPC）；**禁止** mock `sqlx.SqlConn`。
- Gateway REST 决策表至少覆盖每条路由一条成功规则；JWT 路由另有未认证规则。
- 覆盖率门槛按 Handler/Logic/MQ consumer/手写 Model/共享库分别计算；生成文件不计入。
- 本地验证：`make check`、`make test`、`make coverage`；按变更范围执行并报告实际结果。
