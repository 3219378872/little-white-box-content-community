---
title: content
tracks:
  - app/content/
last_synced_commit: cbbc743
last_synced_date: 2026-05-30
sync_note: "synced including the gofmt normalization of tracked files in cbbc743"
---

# content

## 职责
内容 RPC 服务（:8088）。管理帖子与评论的增删查：发布/删除帖子、帖子列表、按 ID 批量取帖、评论列表与删除。

## 公开接口与契约
- proto：`proto/content/content.proto`。
- 代表性 Logic：`get_post_list_logic.go`、`get_posts_by_ids_logic.go`、`delete_post_logic.go`、`get_comment_list_logic.go`、`delete_comment_logic.go`。
- `internal/logic/convert.go` — model↔pb 转换。

## 上游
[gateway](gateway.md)；[feed](feed.md) 写扩散时回查帖子。

## 下游
- MySQL + Redis 缓存（[cachex](cachex.md)）。
- 发布动作产生事件经 [mqx](mqx.md) 触发 feed/search/recommend。
- [errx](errx.md)（内容段 2000-2999）、[util](util.md)。

## 关键文件
- `rpc/content.go` — 服务入口。
- `rpc/internal/logic/*` — 业务逻辑。
- `rpc/internal/logic/convert.go` — 类型转换。

## 注意事项与陷阱
- 批量取帖接口注意空 ID 列表与缺失项的处理，避免缓存击穿。
- 错误码落内容段；删除操作需校验归属权限。
