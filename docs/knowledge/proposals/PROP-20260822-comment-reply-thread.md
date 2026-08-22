---
id: PROP-20260822-comment-reply-thread
layer: proposal
title: 楼中楼回复读取与计数一致性（拟 CORE-070）
status: open
owner: agent
target_layer: spec
upstream:
  - SPEC-community-core
---

# 观察到的缺口

- 写入侧自 MVP 起即支持创建楼中楼回复（`comment.parent_id` / `reply_user_id`，
  CORE-050/051 幂等哈希已覆盖回复目标），但读取侧没有任何接口能返回子评论：
  评论列表 SQL 固定过滤 `parent_id IS NULL`，`comment.reply_count` 列自建表起
  无维护方、无读取方（死字段）。前端楼中楼渲染因此长期为空。
- 2026-08-22 人类在对话中决策补全该能力，选择：评论列表内嵌前 3 条回复预览 +
  独立全量分页接口；删除顶级评论级联软删全部子回复并修正计数；严格两层结构。
  该语义目前无对应 SPEC-community-core 条款，实现已先行（见
  [证据](../implementation/evidence/2026-08-22-content-community-comment-replies.md)）。

# 建议条款（请人类评审后决定是否纳入 SPEC-community-core）

- **CORE-070a 楼中楼读取契约**：`GetCommentList` 每条顶级评论携带
  `reply_count` 与前 3 条回复预览（时间正序）；`GET /api/v1/comments/:commentId/replies`
  分页返回全部可见回复（时间正序，确定性二级键 `created_at asc, id asc`）。
  回复列表与评论列表同等适用 CORE-015/016：父评论不存在/已删除、或父帖未发布
  时统一 `ContentNotFound`。
- **CORE-070b 两层不变式**：回复目标必须是当前帖子上可见的顶级评论；
  请求必须同时携带 `parent_id` 与 `reply_user_id`；对楼中楼再嵌套返回参数错误。
- **CORE-070c 计数一致性**：创建/删除回复与父评论 `reply_count`、帖子
  `comment_count` 的增减在同一事务内完成；删除顶级评论时级联软删其全部可见
  子回复，帖子计数按实际影响行数回减（GREATEST 保护不为负）。

# 影响

- 若采纳：`IMP-content-community-backend` 新增对应追踪行并转 `aligned`；
- 若调整（如预览条数、排序方向）：实现按新条款收敛，当前实现标记过渡基线；
- 若否决：实现回退或在实现层显式登记 `diverged` 及理由。
