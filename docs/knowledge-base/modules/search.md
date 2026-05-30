---
title: search
tracks:
  - app/search/
last_synced_commit: 8ce9ecc
last_synced_date: 2026-05-30
sync_note: ""
---

# search

## 职责
搜索 MQ 消费者。消费内容变更事件，维护 Elasticsearch 全文索引。当前以 MQ 消费者形态存在，尚无独立 RPC 服务。

## 公开接口与契约
- MQ 入口：`mq/main.go` + `mq/internal/config/config.go`。
- 消费 [content](content.md) 经 [mqx](mqx.md) 投递的发帖/删帖事件。

## 上游
content 事件流。

## 下游
Elasticsearch 8.x。

## 关键文件
- `mq/main.go` — 消费者入口。
- `mq/internal/config/config.go` — 配置。

## 注意事项与陷阱
- 索引更新需与帖子删除对齐：删帖事件必须移除文档，避免搜到已删内容。
- 索引写入应幂等并容忍乱序（以版本/时间戳兜底）。
