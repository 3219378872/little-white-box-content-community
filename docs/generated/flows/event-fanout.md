---
title: event-fanout
tracks:
  - pkg/mqx/
last_synced_commit: 8ce9ecc
last_synced_date: 2026-05-30
sync_note: ""
---

# 事件驱动写扩散与异步处理

写操作如何通过 RocketMQ 触发下游服务的异步处理。

## 步骤

1. **产生事件**：写动作（如 [content](../modules/content.md) 发帖、
   [interaction](../modules/interaction.md) 点赞）在 Logic 中经
   [mqx](../modules/mqx.md) 生产者发送事件，主题/Tag 取自 `topics.go` 常量。
2. **扇出消费**：同一事件被多个消费者按需消费：
   - [feed](../modules/feed.md)：发帖写扩散到粉丝收件箱。
   - [search](../modules/search.md)：更新 Elasticsearch 索引。
   - [recommend](../modules/recommend.md)：更新 Milvus 向量/特征。
   - [message](../modules/message.md)：生成互动通知。
3. **失败语义**：可重试错误由 broker 重投；永久失败标记 `mqx.ErrPermanentEvent`。

## 不变量
- 消费幂等：重复投递不产生重复扩散/索引/通知。
- 主题/Tag 走常量，禁止字面量散落。
- 消费回调内日志带 ctx。
