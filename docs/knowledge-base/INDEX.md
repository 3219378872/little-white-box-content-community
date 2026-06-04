# Knowledge Base Index

The esx (little-white-box) social content platform knowledge base. See
[README](README.md) for how pages are kept current.

## Services (`app/`)

- [gateway](modules/gateway.md) — REST API 网关，聚合三个 RPC 服务。
- [user](modules/user.md) — 用户 RPC：注册/登录/资料/关注关系。
- [content](modules/content.md) — 内容 RPC：帖子与评论。
- [media](modules/media.md) — 媒体 RPC + MQ：上传/转码/对象存储。
- [interaction](modules/interaction.md) — 交互 RPC：点赞/收藏/计数。
- [feed](modules/feed.md) — Feed RPC + MQ：写扩散与关注流。
- [message](modules/message.md) — 消息 RPC + MQ：私信与通知。
- [recommend](modules/recommend.md) — 推荐 MQ 消费者（Milvus 向量）。
- [search](modules/search.md) — 搜索 MQ 消费者（Elasticsearch 索引）。
- [embedding](modules/embedding.md) — 向量化 MQ 消费者（Milvus 向量库）。
- [pipeline](modules/pipeline.md) — 行为日志管道：去重 + ClickHouse。

## Shared Libraries (`pkg/`)

- [errx](modules/errx.md) — 业务错误码与 HTTP/gRPC 错误转换。
- [jwtx](modules/jwtx.md) — JWT 签发/校验与 context 透传。
- [middleware](modules/middleware.md) — HTTP 鉴权/可选鉴权/CORS 中间件。
- [interceptor](modules/interceptor.md) — gRPC 业务错误拦截器。
- [mqx](modules/mqx.md) — RocketMQ 生产者/消费者封装与主题常量。
- [cachex](modules/cachex.md) — 缓存键前缀构造。
- [clickhousex](modules/clickhousex.md) — ClickHouse 客户端封装。
- [event](modules/event.md) — 跨服务事件载荷定义。
- [validator](modules/validator.md) — 输入校验（手机号/密码/用户名）。
- [util](modules/util.md) — 雪花 ID、哈希、时间、JSON 字段工具。
- [cleanupx](modules/cleanupx.md) — 资源关闭与优雅停机辅助。
- [testutil](modules/testutil.md) — 集成测试容器装配。

## Flows

- [request-pipeline](flows/request-pipeline.md) — REST 请求经网关到 RPC 的处理链。
- [error-propagation](flows/error-propagation.md) — `errx` 错误码跨 RPC/HTTP 的传播。
- [behavior-log-pipeline](flows/behavior-log-pipeline.md) — 行为日志去重落 ClickHouse。
- [event-fanout](flows/event-fanout.md) — RocketMQ 事件驱动的写扩散与异步处理。
