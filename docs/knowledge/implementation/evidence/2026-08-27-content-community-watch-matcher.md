---
implementation: IMP-content-community-backend
verified_at: 2026-08-27
verified_commit: 237612ab4190b272870355cbd01af6f79e0fef39
commands:
  - go test -count=1 ./app/assistant/watch/... ./app/assistant/mq/... ./app/assistant/rpc/internal/agent/... ./app/assistant/rpc/internal/logic/... ./deploy/
  - python3 scripts/engineering-lint.py
result: passed
---

# 2026-08-27 Watch matcher 接入 post-* 事件

## 范围

把规则 Watch 匹配接到 RocketMQ：新增 `app/assistant/mq` 进程，订阅
`post-create` / `post-update` / `post-delete`，消费组
`assistant-watch-matcher-group`。`watch` 包从 `rpc/internal` 挪到
`app/assistant/watch`，供 RPC 与 matcher 共用。`RecordHit` 对
`(task_id, event_key)` 使用 `INSERT IGNORE`，避免重试产生重复未读命中。

## 结果

- `go test` 上述包全部 `ok`。
- `python3 scripts/engineering-lint.py` passed。

## 未覆盖边界

- `discussion_spike` 仍不消费 `user-behavior-v2`、无预筛选/模型判定。
- 命中列表仍不回源过滤不可见帖；下次 Agent 对话仍未注入未读摘要。
- 本地编排仓需把 matcher 进程加入 `stack.sh` MQ_SERVICES 后才会在 `just app-up` 拉起。
