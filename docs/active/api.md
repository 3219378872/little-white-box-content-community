# API 开发速查

## 边界

- Gateway 负责 HTTP 参数绑定、鉴权中间件和 RPC 编排。
- Logic 负责业务流程；Handler 不写业务逻辑，不直接访问 Model。
- 错误由 Logic 返回 `errx.New(code, message)`，统一中间件负责 HTTP 映射。

## 修改流程

1. 先检查现有 `app/gateway/*.api` 和相邻 Logic/Model。
2. 修改 `.api` 后运行项目约定的 goctl API 生成命令。
3. 只在生成前修改 `.api`；不要手改 `internal/handler` 或 `internal/types`。
4. 为成功和失败路径补测试，再运行 `python3 scripts/engineering-lint.py`。

## 输入与响应

- 用户输入经过 `pkg/validator` 或 API 声明的校验。
- 不在响应中暴露内部堆栈、secret 或数据库细节。
- 需要认证的接口使用现有 JWT middleware；不要在 Handler 重复解析 token。
