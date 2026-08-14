---
implementation: IMP-content-community-backend
verified_at: 2026-08-14
verified_commit: c727522
commands:
  - go test -count=1 ./app/feed/...
  - make check
  - make test
result: passed
---

# 2026-08-14 关注流 inbox 残留分页缺陷修复（DISC-011）

## 缺陷

`GetFollowFeed` 的 inbox 分页存在数据可见性死路：

1. `inboxHasMore` 依赖 `keptCurrentInbox`（本页是否有保留行）：当本页
   `FindByUserBefore` 返回的 `limit` 行**全部属于已取关作者**（取关后
   inbox 行残留，DISC-011），`keptCurrentInbox=false` → `hasMore=false`。
2. 本页无可见项时 `lastScanned=nil` → 不返回 `NextCursor*`。

后果：若残留行时间上盖过当前关注作者的行，第一页返回空且无游标，
客户端无法翻页，更早的可见行被**永久跳过**。

## 修复（get_follow_feed_logic.go）

1. `inboxHasMore := len(inboxRows) == int(limit)`——不再依赖本页保留行：
   只要取满 `limit` 行，更早的行就可能存在，必须允许翻页。
2. 空可见页但 `hasMore` 时，用本页已扫描的最旧行
   （`oldestScannedRow`：inbox/outbox 均为降序返回，取字典序最小者）
   推进 `NextCursorCreatedAt/PostId`，避免死路。
3. 移除不再使用的 `keptCurrentInbox`。

## 测试（新增 2 个失败路径用例）

- `TestGetFollowFeedLogic_FullUnfollowedInboxPageStillAdvancesCursor`：
  第一页 limit 行全属已取关作者 → `HasMore=true` 且游标推进到最旧扫描行。
- `TestGetFollowFeedLogic_UnpublishedCandidatesStillAdvanceCursor`：
  候选行全部未发布（enrich 过滤后不可见）→ 同样必须推进游标。
- 既有 `TestGetFollowFeedLogic_ExcludesUnfollowedInboxAuthors`
  （行数 < limit 的真实空页）断言保持不变，语义不受影响。

## 结果

- `go test ./app/feed/...` 全过；`make check` 通过；`make test` 全部模块
  通过（含 race）。

## 未覆盖边界

- 外部输入门禁不变（ASST/REL/PROP/DISC-062 复评），见
  [IMP-todo-blocked-gates](../IMP-todo-blocked-gates.md)。
