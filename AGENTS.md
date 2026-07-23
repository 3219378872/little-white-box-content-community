# AGENTS.md

这是 esx（little-white-box）社交内容平台仓库。`AGENTS.md` 是唯一的项目规则入口；
任务资料按 [docs/INDEX.md](docs/INDEX.md) 路由，禁止默认读取整个 `docs/`。

## 项目事实

- 根模块为 `esx`，Go 版本见 `go.mod`，框架为 go-zero。
- `app/` 包含 gateway、用户/内容/媒体/互动/feed/message 等 RPC 或 MQ 模块。
- `pkg/` 包含错误、鉴权、中间件、事件、MQ、缓存和测试工具等共享库。
- `deploy/docker-compose.middleware.yml` 提供本地 MySQL、Redis、etcd、RocketMQ、搜索、对象存储和观测组件。
- 运行时事实以代码、配置、`.api`、`.proto` 和 `docs/generated/` 为准；设计文档不能覆盖代码行为。

## 不可违反的规则

- Handler 只绑定参数、调用 Logic、返回响应；业务逻辑放在 Logic，数据访问放在 Model。
- 所有请求上下文必须透传；日志使用 `logx.WithContext(ctx)`，RPC 调用使用入参 ctx。
- Logic 返回 `pkg/errx` 中定义的业务错误；禁止裸 `errors.New` 和 Handler 手动设置 HTTP 错误状态。
- 配置从 `etc/*.yaml` 和 `config.Config` 注入；secret 只能通过环境变量，禁止硬编码。
- 禁止手动编辑 goctl/protobuf 生成文件；修改 `.api` 或 `.proto` 后必须重新生成并检查差异。
- 不为通过测试修改测试；修复实现并覆盖至少一个失败路径。
- 不引入新依赖，除非用户明确批准。

## 按任务加载资料

只打开 [docs/INDEX.md](docs/INDEX.md) 指定的 1～2 个 active 文档。历史计划和日期设计文档不是当前规则，除非用户明确要求追溯。

## 验证入口

```bash
python3 scripts/engineering-lint.py
scripts/test.sh
scripts/vet.sh
scripts/lint.sh
```

根据变更范围执行相关命令；完成时报告实际执行的命令和结果，不用未执行的全量检查代替证据。
