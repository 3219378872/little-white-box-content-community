---
implementation: IMP-content-community-backend
verified_at: 2026-08-14
verified_commit: fe7a73c
commands:
  - go build ./app/assistant/...
  - make check
  - make test
  - docker run redis:7-alpine + EVAL（真实 Redis 验证 Lua 去重）
result: passed
---

# 2026-08-14 ASST-035 同请求标识去重扩展（规格层验证）

## 规格偏离

`SPEC-grounded-assistant` ASST-035 要求"同一请求标识重试不得形成彼此
矛盾的完成状态"。`appendConversationScript` 只检查**列表最后一条**
消息的 request_id——并发/交错下同 request_id 的消息不在末尾时会
被重复追加，形成矛盾完成状态。

## 修复

- `state.go` 的 `appendConversationScript`：改为遍历整个消息列表
  （LLEN+LINDEX，O(n)≤100）检查 request_id，任一匹配即去重不追加。

## 验证

- 真实 Redis（redis:7-alpine 容器 EVAL）分步验证：
  1) req-1 首次 append → 1；2) req-2 追加 → 1；
  3) **req-1 重试（非末尾）→ 1 且不追加**；4) LLEN 保持 2——去重正确。
- `make check` 通过；`make test` 全部模块通过（含 race）。

## 未覆盖边界

- Lua 脚本的自动化集成测试未固化（依赖真实 Redis；本轮以容器 EVAL
  验证，未来可在 integration 套件固化为 testcontainers 用例）；
  外部输入门禁不变，见
  [IMP-todo-blocked-gates](../IMP-todo-blocked-gates.md)。
