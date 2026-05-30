---
title: recommend
tracks:
  - app/recommend/
last_synced_commit: 8ce9ecc
last_synced_date: 2026-05-30
sync_note: ""
---

# recommend

## 职责
推荐 MQ 消费者。消费内容/交互事件，维护推荐所需的向量与特征（Milvus 向量库）。当前以 MQ 消费者形态存在，尚无独立 RPC 服务。

## 公开接口与契约
- MQ 入口：`mq/main.go` + `mq/internal/config/config.go`。
- 消费 [content](content.md) / [interaction](interaction.md) 经 [mqx](mqx.md) 投递的事件。

## 上游
content/interaction 事件流。

## 下游
Milvus 向量数据库。

## 关键文件
- `mq/main.go` — 消费者入口。
- `mq/internal/config/config.go` — 配置。

## 注意事项与陷阱
- 该服务仍在演进，业务 Logic 较薄；扩展为 RPC 前需走 brainstorming 流程。
- 向量写入应幂等，重复事件不应造成特征污染。
