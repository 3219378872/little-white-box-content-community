---
title: message
tracks:
  - app/message/
last_synced_commit: 8ce9ecc
last_synced_date: 2026-05-30
sync_note: ""
---

# message

## 职责
消息 RPC 服务 + MQ 消费者。负责私信发送与会话、以及系统/互动通知的异步生成与拉取。

## 公开接口与契约
- proto：`proto/message/message.proto`。
- RPC Logic：`send_message_logic.go`、`get_notifications_logic.go`、`convert.go`。
- MQ：`mq/main.go` + `mq/internal/logic/notification.go` 异步生成通知。

## 上游
- [gateway](gateway.md)（发私信/拉通知）。
- 其他服务的互动事件经 [mqx](mqx.md) 触发通知。

## 下游
MySQL + Redis；[errx](errx.md)、[util](util.md)。

## 关键文件
- `rpc/internal/logic/send_message_logic.go` — 私信发送。
- `mq/internal/logic/notification.go` — 通知生成。

## 注意事项与陷阱
- 通知生成为异步：消费失败需可重试，避免重复通知。
- 会话/通知分页需稳定排序（按时间 + ID），避免漏读重读。
