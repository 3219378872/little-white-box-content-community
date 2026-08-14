---
implementation: IMP-content-community-backend
verified_at: 2026-08-14
verified_commit: 78bf31e
commands:
  - go test -count=1 ./app/assistant/...
  - make check
  - make test
result: passed
---

# 2026-08-14 ASST-032 LLM 降级返回证据摘要（规格层验证）

## 规格偏离

`SPEC-grounded-assistant` ASST-032 要求：检索成功且证据有效、但 LLM
不可用时，返回**结构化证据摘要和来源**并标记降级。原实现 Generator
失败走 `sendPersistedDegraded`——只发固定 "temporarily unable" 错误，
**丢弃了已检索的证据摘要与来源**。

## 修复

- `app/assistant/rpc/internal/logic/chat_logic.go`：新增 `sendEvidenceDegraded`——持久化证据摘要
  与来源，流式发送：证据摘要 TOKEN 片段 + SOURCE 来源引用 + 降级
  ERROR 结束事件（ASST-022 协议：失败以唯一降级事件结束）。

## 测试

- 更新 `TestChatLLMFailureIsPersistedStructuredDegradedEvent`：断言
  证据摘要 TOKEN + 降级 ERROR、持久化内容为证据摘要而非固定消息。
- 新增 `TestChatLLMFailureStreamsEvidenceSources`：含来源场景断言
  TOKEN + SOURCE(id/revision) + ERROR 三事件、来源被持久化。

## 结果

- assistant 包测试全过；`make check` 通过；`make test` 全部模块通过
  （含 race）。

## 未覆盖边界

- 外部输入门禁不变（ASST/REL/PROP/DISC-062 复评），见
  [IMP-todo-blocked-gates](../IMP-todo-blocked-gates.md)。
