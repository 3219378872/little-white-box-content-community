# 运行与可靠性速查

## 本地依赖

中间件由 `deploy/docker-compose.middleware.yml` 管理，包含 MySQL、Redis、etcd、RocketMQ、Elasticsearch、Milvus、MinIO/SeaweedFS、DTM、ClickHouse 和观测组件。启动或排查前先读取该 compose 文件的实际服务名、端口和健康检查。

## 异步与失败处理

- RocketMQ 消费者必须处理重试、幂等和不可恢复错误；不要静默吞错。
- DTM 或 outbox 类流程必须保留补偿/重入路径，并验证数据库与事件的一致性边界。
- 超时、取消和下游错误沿 ctx 传播；日志使用 `logx.WithContext(ctx)`。

## 排查入口

先看服务日志和配置，再验证依赖健康、注册发现和消息主题；最后定位具体消费者或 RPC 调用。不要从历史计划推断当前运行状态。
