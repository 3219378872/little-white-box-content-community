---
title: feed
tracks:
  - app/feed/
last_synced_commit: 8ce9ecc
last_synced_date: 2026-05-30
sync_note: ""
---

# feed

## 职责
Feed RPC 服务 + MQ 消费者。负责发帖后的写扩散（fan-out）到粉丝收件箱，以及关注流读取。

## 公开接口与契约
- proto：`proto/feed/feed.proto`。
- RPC Logic：`fanout_post_logic.go`、`push_to_inbox_logic.go`、`get_follow_feed_logic.go`。
- MQ：`mq/main.go` + `mq/internal/logic/fanout.go` 异步消费发帖事件做扩散。

## 上游
- [content](content.md) 发帖事件经 [mqx](mqx.md) 触发。
- [gateway](gateway.md) 读取关注流。

## 下游
- Redis 收件箱（粉丝 timeline）+ MySQL。
- 回查 [user](user.md)（粉丝关系）、[content](content.md)（帖子内容）。

## 关键文件
- `mq/internal/logic/fanout.go` — 写扩散核心。
- `rpc/internal/logic/get_follow_feed_logic.go` — 关注流读取。

## 注意事项与陷阱
- 大 V 写扩散成本高：注意是否采用推拉结合，避免对超大粉丝量同步扩散。
- 扩散为异步消费，失败需可重试；幂等避免重复 push。
