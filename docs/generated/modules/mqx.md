---
title: mqx
tracks:
  - pkg/mqx/
last_synced_commit: 8ce9ecc
last_synced_date: 2026-05-30
sync_note: ""
---

# mqx

## 职责
RocketMQ 5.x 的生产者/消费者封装与主题常量集中定义，统一消息发送与消费的样板。

## 公开接口与契约
- `NewProducer(...)` — 构造生产者。
- `NewConsumer(...)` — 构造消费者并绑定处理回调。
- `ErrPermanentEvent(...)` / `IsPermanentEvent(...)` — 区分可重试错误与永久失败
  （永久失败不再重投，避免毒消息无限重试）。
- `topics.go` — 主题/Tag 常量。

## 上游
所有 MQ 服务（feed/media/message/recommend/search/pipeline）与产生事件的 Logic。

## 下游
RocketMQ broker；消费侧回调进入各服务 `internal/logic`。

## 关键文件
- `producer.go` / `consumer.go` — 收发封装。
- `consumer_error.go` — 永久/可重试错误语义。
- `topics.go` — 主题常量。

## 注意事项与陷阱
- 消费回调内必须使用传入 ctx 派生的日志（`logx.WithContext`）。
- 永久失败要显式标记为 `ErrPermanentEvent`，否则会触发重投。
- 主题/Tag 一律走 `topics.go` 常量，禁止字符串字面量散落。
