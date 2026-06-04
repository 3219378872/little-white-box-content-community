# 弹性与可靠性

esx 项目弹性模式概要。详细模式与代码示例参见 [references/resilience.md](references/resilience.md)。

## go-zero 内置防护（默认启用）

```
Request → Load Shedding → Rate Limiting → Circuit Breaker → Timeout → Service
```

- **熔断器**：Google SRE 算法，自动保护 RPC/DB/Redis 调用
- **限流**：令牌桶，按服务/接口粒度配置
- **过载保护**：自适应降载，CPU/内存超阈值自动拒绝
- **超时控制**：zrpc 全链路超时透传

## 分布式事务

- DTM 二阶段消息保证发帖写库与 Feed Fanout 最终一致性
- 屏障表 `QueryPrepared` 实现幂等

## MQ 可靠消费

- RocketMQ SendOneWay 用于非关键异步事件（media-deleted）
- 消费者 ConsumeRetryLater 自动重试

## 详细参考

- [弹性模式完整文档](references/resilience.md)
- [并发模式](references/concurrency.md)
- [事件驱动模式](references/event-driven.md)
