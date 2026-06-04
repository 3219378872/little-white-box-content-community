---
title: embedding
tracks:
  - app/embedding/
last_synced_commit: b3d788b
last_synced_date: 2026-06-04
sync_note: ""
---

# embedding

## 职责
Embedding MQ 消费者。消费内容变更事件，将帖子文本向量化后写入 Milvus 向量数据库，供推荐服务近邻检索。

## 公开接口与契约
- MQ 入口：`mq/main.go` + `mq/internal/config/config.go`。
- 消费 [content](content.md) 经 [mqx](mqx.md) 投递的发帖/更新/删帖事件。
- 向量存储契约：`mq/internal/vectorstore/vectorstore.go`（`VectorStore` 接口）。
- 嵌入契约：`mq/internal/embedder/embedder.go`（`Embedder` 接口）。

## 上游
content 事件流。

## 下游
- Milvus（向量数据库）。
- [recommend](recommend.md) 通过 Milvus 做近邻检索。

## 关键文件
- `mq/main.go` — 消费者入口。
- `mq/internal/config/config.go` — Milvus 连接与集合配置。
- `mq/internal/mqs/embedding_consumer.go` — 消费逻辑。
- `mq/internal/vectorstore/milvus.go` — Milvus 实现。
- `mq/internal/embedder/embedder.go` — 嵌入接口（当前 NoopEmbedder）。

## 注意事项与陷阱
- 当前 Embedder 为 Noop（随机向量），生产需替换为真实模型。
- Milvus 集合维度（Dim）需与实际嵌入模型输出维度一致。
- 删帖事件应从 Milvus 中删除对应向量，避免推荐到已删内容。
