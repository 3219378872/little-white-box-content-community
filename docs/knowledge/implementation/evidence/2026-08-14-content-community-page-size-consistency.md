---
implementation: IMP-content-community-backend
verified_at: 2026-08-14
verified_commit: 6fe17f0
commands:
  - go test -count=1 ./internal/logic/posts/ ./internal/logic/user/ ./internal/logic/comment/ ./internal/logic/pageutil/
  - make check
  - make test
result: passed
---

# 2026-08-14 gateway 页大小响应一致性修复

## 缺陷

gateway 的帖子/用户帖子/评论列表 logic 直接透传 `req.PageSize` 给
content RPC，内容服务按 `normalizePage` 语义 clamp（非正数→20、
>50→50），但 gateway 响应 `PageSize` 回传**原始请求值**——当客户端
请求超大 pageSize（如 999）时，响应返回 50 条却标注 `PageSize=999`，
元数据与实际不一致。

## 修复

- 新增共享包 `gateway/internal/logic/pageutil`：`ClampPageSize`
  （非正数→20、>50→50，与内容 RPC clamp 语义一致）。
- `get_post_list` / `get_user_posts` / `get_comment_list`：函数开头
  clamp 一次，请求与响应均使用 clamp 后的值。

## 测试

- `pageutil` 表驱动单测（pageSize 0/-1/20/50/51/9999；page 0/-5/1/42）。
- 既有 posts/user/comment logic 测试不受影响。

## 补充（同一缺陷的 Page 部分）

初版只处理 pageSize；复查发现 `Page` 同样透传未归一（内容 RPC
page<=0→1，gateway 回传原始值）。追加 `pageutil.ClampPage` 并应用到
三个 content 系 logic 的请求与响应，保持页码回显一致。

## 结果

- gateway 相关包测试全过；`make check` 通过；`make test` 全部模块
  通过（含 race）。

## 补充（互动收藏列表，同一缺陷模式）

复查 `get_user_favorites`：互动 RPC `GetFavoriteList` 也是 clamp 语义
（page<1→1、pageSize 非正数或 >100→20），gateway 同样透传原始值。
新增通用 `pageutil.ClampPageSizeTo(pageSize, def, max)`（内容 20/50、
互动 20/100 复用），`get_user_favorites` 请求与响应均使用归一值。

## 未覆盖边界

- search（RPC 拒绝超大 pageSize 而非 clamp）保持拒绝语义，不在本次
  改动范围；外部输入门禁不变，见
  [IMP-todo-blocked-gates](../IMP-todo-blocked-gates.md)。
