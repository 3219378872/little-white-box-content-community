---
title: event
tracks:
  - pkg/event/
last_synced_commit: 8ce9ecc
last_synced_date: 2026-05-30
sync_note: ""
---

# event

## 职责
跨服务事件载荷（payload）的共享定义，作为 MQ 生产方与消费方之间的契约 schema。

## 公开接口与契约
- `behavior.go` — 行为事件载荷结构定义（生产者序列化、消费者反序列化共用）。

## 上游
产生事件的服务（如行为采集、内容/交互动作）。

## 下游
- [mqx](mqx.md) 负责传输。
- [pipeline](pipeline.md) 等消费方按本包结构反序列化。

## 关键文件
- `behavior.go` — 行为事件载荷。

## 注意事项与陷阱
- 事件结构是跨服务契约：字段变更需保证向后兼容（新增可选字段优先，避免删除/改名）。
- 序列化字段标签（json）必须与消费方约定一致。
