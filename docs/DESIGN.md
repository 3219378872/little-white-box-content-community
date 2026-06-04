# 设计原则与约定

本文档汇总 esx 项目的核心设计原则和硬性约定。CLAUDE.md / AGENTS.md 中的规则引用本文档。

## 三层架构

| 层 | 职责 | 禁止 |
|----|------|------|
| Handler | 参数绑定、调用 Logic、返回响应 | 写业务逻辑 |
| Logic | 业务逻辑，通过 `svc.ServiceContext` 获取资源 | 直接访问 `http.Request` |
| Model | 数据访问 | 跨 Model 直接调用 |
| Svc | 依赖注入容器（DB / Redis / RPC 客户端等） | — |

## Context 传递

- **必须** `logx.WithContext(ctx).Info(...)` — 禁止不带 ctx 的日志
- **必须** 所有 zrpc 调用透传入参 ctx
- **必须** goroutine 内使用 ctx 的拷贝
- **禁止** `context.Background()` 创建新 ctx（除非最外层入口）

## 错误处理

- Logic 层统一返回 `errx.New(code, msg)`
- 错误码集中定义于 `pkg/errx/codes.go`，按业务域分段
- HTTP 状态码映射由 errx 中间件统一处理
- **禁止** `return errors.New("xxx")` 裸字符串错误
- **禁止** Handler 层手动 `httpx.Error(w, err)`

## 配置管理

- 所有配置走 `etc/*.yaml` → `config.Config` 结构体
- 敏感值走环境变量，yaml 只放 `${ENV_VAR}` 占位
- **禁止** 硬编码任何配置值

## 代码生成

- Handler 和 types 由 goctl 生成，**禁止**手动编辑
- 修改 `.api` 后：`goctl api go -api xxx.api -dir . --style go_zero`
- 修改 `.proto` 后：运行对应 goctl rpc 命令

## 详细参考

- [REST API 模式](references/rest-api.md)
- [RPC 模式](references/rpc.md)
- [数据库模式](references/database.md)
- [错误治理模式](references/api-governance.md)
- [最佳实践速查](references/best-practices.md)
