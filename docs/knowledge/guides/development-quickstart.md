# 开发与运维速查

本文档是非正式操作指南，合并旧 `docs/active/*.md` 速查，作为
API/RPC/数据/安全/运维/测试的按需入口。

## API 开发

- Gateway 负责 HTTP 参数绑定、鉴权中间件与 RPC 编排；Logic 负责业务流程。
- 修改 `app/gateway/openapi.yaml` 后运行 `make generate`；DTO 与路由为生成文件，
  普通 Handler 手写维护。生成器不会覆盖业务 Handler。
- 用户输入经 `pkg/validator` 或 API 声明校验；响应不暴露内部堆栈、secret 或数据库细节。

## RPC 开发

- proto 定义在 `proto/`，生成代码不手动编辑；修改后运行仓库使用的 OpenAPI/Kitex/protobuf 命令。
- 所有 Kitex 调用透传原始 ctx；goroutine 使用 ctx 副本并处理取消。
- 跨服务业务错误使用 `pkg/errx` 与现有 interceptor，不重新定义错误协议。
- Assistant 先检索用户可见的已发布社区内容，并对帖子与评论回源验证；社区资料不足时，按当前
  capability 和授权使用互联网搜索补充。社区检索故障、可见性不可验证与资料不足必须区分，
  任何帖子或网页来源都只有在实际取得、登记并通过发布校验后才能展示。

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
  RocketMQ、Elasticsearch、Milvus、SeaweedFS、ClickHouse、观测组件）。SeaweedFS 是唯一对象
  存储：业务媒体、广告素材、Milvus 与模型仓库各用独立桶和凭据；不再部署 MinIO。
- RocketMQ 消费者必须处理重试、幂等与不可恢复错误；不静默吞错。
- 超时/取消/下游错误沿 ctx 传播；日志使用 `logging.WithContext(ctx)`。
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

## Compose 网络地址

中间件网络默认 `172.30.240.0/24`，动态分配池 `172.30.240.128/25`，网关 `172.30.240.1`。
服务使用 Compose DNS 名称，不指定固定 IP。环境重叠时一起覆盖 `XBH_NETWORK_SUBNET`、
`XBH_NETWORK_IP_RANGE`、`XBH_NETWORK_GATEWAY`，确保地址池及网关属于子网且不重叠。
Docker 不能原地修改现有网络 IPAM；已有环境需停止应用后以原 Compose project 执行
`docker compose down`（绝不加 `-v`），再按原配置重新启动。数据卷保留，旧容器的历史固定 IP
随重建移除。生产环境的维护窗口与运行验收应另行安排。

### 框架迁移后的配置兼容

`RestConf.Timeout`（默认 3000 毫秒）与 `RestConf.MaxBytes`（默认 10 MiB）控制普通
Gateway 请求；显式零值关闭对应普通请求限制。媒体 multipart 和 SSE 使用 OpenAPI 中的独立
路由策略，普通 HTTP 超时不截断 SSE。RPC `Timeout` 默认 2000 毫秒，服务端和客户端仅作用于
普通调用，显式零值不设置截止时间。流式上传和事件订阅保留调用方的取消与生命周期。
`Etcd.Key` 是 Kitex 注册/发现的逻辑服务名，允许与日志展示的 `Name` 不同。

### Streaming request ingestion deadlines

Ordinary API and multipart route deadlines now cancel the request context and set an absolute
connection read deadline, which interrupts blocked JSON/multipart reads. The read deadline
is reset before the request returns, preserving subsequent keep-alive requests and allowing
the existing JSON error response after downstream cancellation. Explicit zero timeout and SSE retain their existing exemptions.
The proxy must
stream ordinary request bodies (`proxy_request_buffering off`) so slow clients are visible
to this absolute deadline; nginx `client_body_timeout` is only an inactivity guard.

Loopback tests cover slow chunked JSON and multipart on default and standard Hertz
transports, plus successful keep-alive and SSE. Real nginx/full-stack verification remains
open until the required runtime is available.
