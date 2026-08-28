---
implementation: IMP-content-community-backend
verified_at: 2026-08-28
verified_commit: 648360f48c25283bf283406744ec80661c4db39c
commands:
  - go test ./app/assistant/rpc/internal/agent/ ./app/assistant/rpc/internal/logic/ ./app/assistant/rpc/internal/memory/
  - python3 scripts/engineering-lint.py
result: passed
---

# 2026-08-28 Agent P0：授权、证据、多轮历史

## 范围

- RPC `requireAgentConsent`：未授权不落库、不跑 Runner（AGNT-002/006）。
- Agent 终答中和模型 `[post:]`/`[comment:]`，只附加本轮已验证 post/comment 来源（AGNT-011 / ASST-010～012）。
- 截断会话历史注入模型；`recommend` upsert 开放 Task，`continue_task` 不新建（MEM-006）。
- Runner 空 `Choices` 视为 LLM 不可用。

## 结果

agent / logic / memory 包测试通过；知识链路校验通过。

## 未覆盖边界

- 记忆仍无 LLM schema 抽取；`WATCH_HIT`/`ACTIONS` 未发出；`discussion_spike` 生产未接 Judge。
- 未用真实模型跑「还有吗」联调。
