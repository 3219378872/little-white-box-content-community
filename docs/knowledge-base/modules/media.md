---
title: media
tracks:
  - app/media/
last_synced_commit: 8ce9ecc
last_synced_date: 2026-05-30
sync_note: ""
---

# media

## 职责
媒体 RPC 服务（:9008）+ MQ 消费者。负责图片/媒体上传、批量取媒体，以及异步媒体处理（转码/落对象存储）。

## 公开接口与契约
- proto：`proto/media/media.proto`。
- RPC Logic：`upload_image_logic.go`、`get_media_logic.go`、`batch_get_media_logic.go`、`upload_common.go`。
- MQ：`mq/main.go` + `mq/internal/logic/*` 消费媒体处理事件。

## 上游
[gateway](gateway.md)（上传/查询）；MQ 侧消费媒体事件。

## 下游
- MinIO / SeaweedFS（S3 兼容对象存储）。
- MySQL + Redis；[mqx](mqx.md)（异步处理）、[errx](errx.md)（媒体段 4000-4999）。

## 关键文件
- `rpc/media.go` — RPC 入口。
- `mq/main.go` — MQ 消费者入口。
- `rpc/internal/logic/upload_*.go` — 上传逻辑。

## 注意事项与陷阱
- 上传需校验大小/类型；对象存储凭证走环境变量。
- MQ 处理失败区分可重试与永久失败（`mqx.ErrPermanentEvent`）。
