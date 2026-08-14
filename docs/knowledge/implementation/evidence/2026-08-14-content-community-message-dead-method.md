---
implementation: IMP-content-community-backend
verified_at: 2026-08-14
verified_commit: 4885129
commands:
  - go test -count=1 ./app/message/...
  - make check
  - make test
result: passed
---

# 2026-08-14 message 会话死方法清理（UpsertPairForMessage）

## 缺陷/清理

`ConversationModel`（svc 接口 + model 接口）声明了 `UpsertPairForMessage`
及私有 `upsertOne`，但消息发送走 `CreateMessageWithConversations`
（事务内 `upsertConversationForMessage` 直写 SQL），全仓无 logic 调用
这两个方法——纯死方法链。其内部还调用带缓存的
`FindOneByUserIdTargetUserId`（`QueryRowIndexCtx`），一旦未来被误接入，
会话更新路径（事务直写、不失效缓存）与缓存读取路径会互相污染。

## 改动

- `app/message/rpc/internal/svc/service_context.go`：接口删除
  `UpsertPairForMessage`。
- `app/message/rpc/internal/model/conversation_model.go`：接口与实现删除
  `UpsertPairForMessage` + `upsertOne`。
- `app/message/rpc/internal/logic/message_logic_test.go`：删除 fake
  对应方法及 unused 字段。

保留：`FindByUser`（会话列表，NoCache）、`FindOneForUser`（读权限校验，
NoCache）、`MarkConversationRead`/事务直写路径（`CreateMessageWithConversations`）。

## 审查证据（本轮深入扫描）

- message 命令模型事务边界（会话+消息同事务、幂等唯一键竞争回查、
  MarkRead affected 递减）审查通过。
- 媒体上传孤儿对象（上一轮已修）、关注流 inbox 残留分页（上上轮已修）
  之外，本轮未发现新的正确性问题。

## 结果

- `go test ./app/message/...` 全过；`make check` 通过；`make test` 全部
  模块通过（含 race）。

## 未覆盖边界

- 外部输入门禁不变（ASST/REL/PROP/DISC-062 复评），见
  [IMP-todo-blocked-gates](../IMP-todo-blocked-gates.md)。
