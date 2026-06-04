---
title: event
tracks:
  - pkg/event/
last_synced_commit: b3d788b
last_synced_date: 2026-06-04
sync_note: "refreshed after PostEvent and InteractionEvent definitions stabilized"
---

# event

## 职责
跨服务事件载荷（payload）的共享定义，作为 MQ 生产方与消费方之间的契约 schema。

## 公开接口与契约
- `behavior.go` — 行为事件载荷结构定义（生产者序列化、消费者反序列化共用）。
- `post.go` — 帖子生命周期事件（`PostEvent`：Created/Updated/Deleted），含 `Validate()` 校验。
- `interaction.go` — 交互事件（`InteractionEvent`：like/favorite/view 等），含 `Validate()` 与 `ToBehaviorEvent()` 转换。

## 上游
产生事件的服务（如行为采集、内容/交互动作）。

## 下游
- [mqx](mqx.md) 负责传输。
- [pipeline](pipeline.md) 等消费方按本包结构反序列化。

## 关键文件
- `behavior.go` — 行为事件载荷。
- `post.go` — 帖子生命周期事件定义与校验。
- `interaction.go` — 交互事件定义、校验与行为事件转换。

## 注意事项与陷阱
- 事件结构是跨服务契约：字段变更需保证向后兼容（新增可选字段优先，避免删除/改名）。
- 序列化字段标签（json）必须与消费方约定一致。
