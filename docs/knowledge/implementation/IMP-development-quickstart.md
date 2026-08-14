---
id: IMP-development-quickstart
layer: implementation
title: 开发与运维速查（API/RPC/数据/安全/运维/测试）
status: aligned
owner: agent
upstream:
  - DES-content-community-backend
tracks:
  - Makefile
  - scripts/
verified_at: 2026-08-14
verified_commit: bea6c09
---

# 开发与运维速查

本文档合并旧 `docs/active/*.md` 速查，作为 API/RPC/数据/安全/运维/测试的按需入口。

## API 开发

- Gateway 负责 HTTP 参数绑定、鉴权中间件与 RPC 编排；Logic 负责业务流程。
- 修改 `.api` 后运行项目约定的 goctl API 生成命令；**禁止**手改
  `internal/handler` 或 `internal/types`（生成文件）。
- 用户输入经 `pkg/validator` 或 API 声明校验；响应不暴露内部堆栈、secret 或数据库细节。

## RPC 开发

- proto 定义在 `proto/`，生成代码不手动编辑；修改后运行仓库使用的 goctl/protobuf 命令。
- 所有 zrpc 调用透传原始 ctx；goroutine 使用 ctx 副本并处理取消。
- 跨服务业务错误使用 `pkg/errx` 与现有 interceptor，不重新定义错误协议。
- Assistant 只使用已发布帖子证据：Search 元数据不等于证据；Content 回源确认
  `status=1` 且正文非空才形成 `community_evidence`。

## 数据访问

- Model 只负责数据访问；跨 Model 协调由 Logic 完成。
- 客户端（DB/Redis/ClickHouse/搜索/对象存储）经 ServiceContext 或显式依赖注入。
- 更新操作必须考虑并发、幂等与缓存失效；不要用无保护的读改写覆盖并发更新。
- 数据结构变更说明兼容性、回滚与索引影响；事务失败返回统一业务错误并记录带 ctx 日志。

## 安全与错误排查

- 排查顺序：先确认 middleware，再检查 context claim、Logic 错误码与 HTTP/gRPC 转换；
  不在 Handler 改状态码掩盖错误。
- 不硬编码 secret；错误码集中在 `pkg/errx/codes.go`。

## 运行与可靠性

- 本地中间件由 `deploy/docker-compose.middleware.yml` 管理（MySQL、Redis、etcd、
  RocketMQ、Elasticsearch、Milvus、MinIO/SeaweedFS、ClickHouse、观测组件）。
- RocketMQ 消费者必须处理重试、幂等与不可恢复错误；不静默吞错。
- 超时/取消/下游错误沿 ctx 传播；日志使用 `logx.WithContext(ctx)`。
- 排查入口：先日志与配置，再验证依赖健康、注册发现与消息主题，最后定位消费者/RPC。

## 测试与交付

- 每个 Logic 至少覆盖一个失败路径；表驱动覆盖成功、条件分支、等价类、边界值与依赖失败。
- 边界值按适用类型覆盖 `N-1/N/N+1`、空值、零值、负值和 `max/max+1`。
- 纯 SQL 断言可用 sqlmock；真实 DB/Redis/RPC 行为用仓库已有 testcontainers 工具。
- `//go:build integration` 测试不属于默认套件，必须通过集成测试入口显式执行。
- 测试分层：`make test`（race + 覆盖率）、`make coverage*`、`make integration-*`、
  `make fuzz`、`make spec-evals-test`、`make algorithm-test`、`make python-unit`
  （Python 工具单测）、`make gen-frozen-evals`/`gen-recommend-samples`/
  `gen-slo-synthetic`（评测数据生成）。
- 验证命令：`make check`、`make test`、`make coverage`；报告实际执行结果。
