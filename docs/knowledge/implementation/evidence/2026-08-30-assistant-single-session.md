---
implementation: IMP-content-community-backend
verified_at: 2026-08-30
verified_commit: da9a823c8484d3989272933770f408b1a51e5eb8
commands:
  - go test ./app/assistant/... ./app/gateway/internal/handler/assistant ./app/gateway/internal/logic/assistant ./app/gateway -count=1
  - go test -race ./app/assistant/internal/runtime ./app/assistant/internal/prompt ./app/assistant/rpc/internal/logic ./app/gateway/internal/logic/assistant -count=1
result: passed
---

# 永久前台 session 与 30 分钟冷拼接

## 环境

后端 worktree `task/agent-single-session`。没有调用 live LLM，没有根真实栈。

## 已验证结果

- `POST /assistant/sessions` 从 proto、gateway `.api`、路由和 RPC 中硬删除；决策表成功规则 60。
- 30 分钟可见空闲后下一次新建 user run 保持同一 `sessionId`，`prompt_epoch+1`，snapshot 载入最新 MEMORY/USER；热对话（29 分钟）不拼接。
- 空线程首次发消息不算冷对话。redirect 不拼接。遗留 `closed` session reopen，不新建 id。
- 冷拼接保留 `compact_summary`。Watch 调度在冷线程上拼接且不创建第二 session。
- 未 compact 的历史消息仍留在同一 session，由 live `ListSessionMessages` 进入 prompt。

## 未覆盖

- 未在真实 MySQL 上跑 30 分钟时钟。
- 未跑 live LLM 或根 e2e（根仓另测清历史与 404）。
