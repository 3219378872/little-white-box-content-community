---
implementation: IMP-content-community-backend
verified_at: 2026-08-22
verified_commit: 23295da
commands:
  - go build ./...
  - go vet ./...
  - go vet -tags=integration ./...
  - make check
  - make test
  - go test -tags=integration -count=1 ./app/content/rpc/internal/logic
  - go test -tags=integration -count=1 ./app/content/rpc/internal/model
result: passed
---

# 2026-08-22 楼中楼回复读取与计数一致性

## 背景

写入侧长期支持创建楼中楼回复，但读取侧无任何出口：评论列表 SQL 固定
`parent_id IS NULL`，`reply_count` 为死字段。人类 2026-08-22 决策补全读取能力
（列表内嵌前 3 条预览 + 独立全量分页接口 + 删除级联修正计数），语义条款以
[PROP-20260822-comment-reply-thread](../../proposals/PROP-20260822-comment-reply-thread.md)
登记为 draft，实现先行。

## 改动（提交 23295da）

- proto/api 契约：`CommentInfo` 增加 `reply_count` 与内嵌 `replies`（仅顶级评论
  填充前 3 条、时间正序）；新增 `GetCommentReplies` RPC 与
  `GET /api/v1/comments/:commentId/replies`（OptionalAuth，匿名可读）。
- Model：`FindByParentId`（分页正序 + count）与 `FindByParentIds`（单条 IN 批量
  取本页预览），均走 `idx_parent_id` 且过滤 `status=1`。
- RPC logic：回复可见性对齐 CORE-015/016——父评论不存在/已删除、父帖未发布统一
  `ContentNotFound`；SQL 过滤后内存二次过滤并回减 Total（纵深防御与评论列表一致）。
- 写路径：严格两层不变式（回复必须同时携带父评论与被回复用户，父必须是可见顶级
  评论，对楼中楼再嵌套报参数错误）；创建/删除同事务维护父 `reply_count`；删除
  顶级评论级联软删全部可见子回复，`post.comment_count` 按实际行数回减。
- Gateway：新路由 + logic（ClampPage/ClampPageSize 语义与列表一致），
  `CommentItem` 增加 `replyCount`/`replies`；REST 决策表登记新路由成功决策
  （匿名 + x-auth-state: anonymous）。

## 验证

- 单元：`TestGetCommentListLogic` 新增内嵌预览/回复总数用例；新增
  `TestGetCommentRepliesLogic` 覆盖分页正序、二次过滤回减 Total、父不存在/
  已删/楼中楼父/草稿帖统一不存在、参数非法。
- 集成：创建回复后 `reply_count`/`comment_count` 递增；回复缺少被回复用户、
  对楼中楼再回复、对已删评论回复分别报错；删除子回复父计数回减；删除顶级评论
  级联后 `FindByParentId` 为空且帖子计数归零；`TestGetCommentReplies` 全量分页
  正序翻页、已删父/草稿帖统一不存在。
- 门禁：`make check`（fmt/文档策略/vet/golangci-lint 0 issues）、`make test`
  全绿；content rpc logic/model 集成测试全绿。
- 已知边界：gateway `userName/userAvatar` 维持现状（空）；历史三层脏数据（若有）
  因父不在顶级列表而自然不可达，无迁移；集成测试 fixture 作者 id 使用 9501+
  独立段，避免与共享库内其他用例冲突。

## 未覆盖

- `make integration-critical` 仅覆盖 interaction/user 关键路径，content 集成为
  手动全量运行；前端接入与真实网关联调见前端仓证据
  （EVD-comment-replies-2026-08-22）。
