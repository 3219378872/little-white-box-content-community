# 数据访问速查

## 约定

- Model 只负责数据访问；跨 Model 协调由 Logic 完成。
- 数据库、Redis、ClickHouse、搜索和对象存储客户端通过 ServiceContext 或显式依赖注入提供。
- 配置值从 `etc/*.yaml` / `config.Config` 读取，secret 使用环境变量。
- 更新操作必须考虑并发、幂等和缓存失效；不要使用无保护的读改写覆盖并发更新。

## 变更流程

1. 先检查现有表、索引、Model 方法和缓存 key。
2. 纯 SQL 逻辑使用 SQL 断言测试；涉及真实 DB/Redis 的行为使用现有 testcontainers 工具。
3. 数据结构变更同时说明兼容性、回滚和索引影响。
4. 事务边界由 Logic 明确控制，失败时返回统一业务错误并记录带 ctx 的日志。
