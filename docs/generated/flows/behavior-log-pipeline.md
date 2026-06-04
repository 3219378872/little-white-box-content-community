---
title: behavior-log-pipeline
tracks:
  - app/pipeline/
last_synced_commit: 8ce9ecc
last_synced_date: 2026-05-30
sync_note: ""
---

# 行为日志管道（事件 → 去重 → ClickHouse）

用户行为事件从 RocketMQ 进入，到去重后落 ClickHouse 的链路。

## 步骤

1. **投递**：行为事件以 [event](../modules/event.md) 定义的载荷经
   [mqx](../modules/mqx.md) 消费者进入 [pipeline](../modules/pipeline.md)。
2. **去重**：`record_behavior.go` 用 Redis 上的 Bloom Filter 判重，已见过的事件丢弃。
   Redis 故障时的降级路径由测试覆盖，去重状态持久化。
3. **落库**：去重后的行为经 [clickhousex](../modules/clickhousex.md) 批量写入 ClickHouse。
4. **错误处理**：可重试错误交由 MQ 重投；永久失败标记为 `mqx.ErrPermanentEvent`，
   不再重投以免毒消息循环。

## 不变量
- 去重幂等：同一事件多次到达只落库一次。
- 批量写入失败要么整体可重试，要么明确永久失败。
- 集成测试用 testcontainers 跑真实 ClickHouse；无容器时跳过并记录原因。
