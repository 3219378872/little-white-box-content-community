---
implementation: IMP-content-community-backend
verified_at: 2026-08-14
verified_commit: e3b1cbe
commands:
  - go test -count=1 ./app/assistant/...
  - make check
  - make test
result: passed
---

# 2026-08-14 ASST-031 历史来源清理实现（规格层验证）

## 规格偏离

`SPEC-grounded-assistant` ASST-031 要求：来源删除、取消发布或受限后，
历史会话**删除保存的标题和片段**并标记"来源不可用"。原实现
`verifyHistoricalSources` 只发送 `[source-unavailable]` 警告，历史
消息中保存的 title/snippet 未删除——规格偏离（测试也只断言警告文本）。

## 修复

- 当时的 Assistant Redis `ConversationStore` 新增 `RemoveUnavailableSourceTitles`，
  以 Lua 脚本遍历会话消息，
  将不可用 post 来源的 `title`/`snippet` 置空（来源 id 保留用于标记）。
- 当时的同步 chat logic 在 `verifyHistoricalSources` 检测到不可用来源后
  实际调用清理，再发警告。

这些实现路径已随 Hermes 异步迁移移除；本文件只保留历史结果，不作为当前代码证据。

## 测试

- store：Lua 调用参数断言（owner/messages keys、userID/ttl/postIDs）、
  空 postIDs 不调 Redis。
- logic：扩展 `TestChatWarnsWhenHistoricalSourcesChangedOrUnavailable`
  断言来源 11 的 title/snippet 已清空、来源 10（可用）保留。

## 结果

- assistant 包测试全过；`make check` 通过；`make test` 全部模块通过
  （含 race）。

## 未覆盖边界

- 外部输入门禁不变（ASST/REL/PROP/DISC-062 复评），见
  [IMP-todo-blocked-gates](../IMP-todo-blocked-gates.md)。
