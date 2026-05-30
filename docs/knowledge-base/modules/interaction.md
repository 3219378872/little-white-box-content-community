---
title: interaction
tracks:
  - app/interaction/
last_synced_commit: 8ce9ecc
last_synced_date: 2026-05-30
sync_note: ""
---

# interaction

## 职责
交互 RPC 服务。负责点赞/取消点赞、收藏/取消收藏，以及各类计数与状态查询（含批量）。

## 公开接口与契约
- proto：`proto/interaction/interaction.proto`。
- 代表性 Logic：`like_logic.go`、`unfavorite_logic.go`、`check_liked_logic.go`、`batch_check_favorited_logic.go`、`get_like_count_logic.go`、`get_counts_logic.go`。

## 上游
[gateway](gateway.md)；[content](content.md) 帖子列表附带交互状态。

## 下游
- MySQL + Redis（计数热点强缓存，[cachex](cachex.md)）。
- 交互动作产生事件经 [mqx](mqx.md) 供 [recommend](recommend.md) 使用。
- [errx](errx.md)（交互段 3000-3999）。

## 关键文件
- `rpc/interaction.go` — 服务入口。
- `rpc/internal/logic/*like*` / `*favorite*` / `*count*` — 业务逻辑。

## 注意事项与陷阱
- 点赞/收藏需幂等：重复点赞不应重复计数。
- 计数读多写多，注意缓存与 DB 一致性；批量查询避免 N+1。
