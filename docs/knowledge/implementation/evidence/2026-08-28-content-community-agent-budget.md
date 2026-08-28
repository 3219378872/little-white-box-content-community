---
implementation: IMP-content-community-backend
verified_at: 2026-08-28
verified_commit: faa68a9
commands:
  - go test ./app/assistant/rpc/internal/config/
  - go test ./app/assistant/rpc/internal/logic/ ./app/assistant/rpc/internal/agent/
  - python3 scripts/engineering-lint.py
result: passed
---

# 2026-08-28 Agent 单轮预算上调

## 范围

- `Agent.TurnTimeoutMs` 120s → 300s，`Agent.StepTimeoutMs` 30s → 90s（AGNT-033）。
- `LLM.MaxOutputTokens` 4096 → 32768（Agent runner `max_tokens` 与 enhanced_search 共用）。
- 未改 `LLM.TimeoutMs`、`MaxOutputRunes`、步数软/硬上限。

## 结果

配置加载断言与 assistant logic/agent 包测试通过；知识链路校验通过。

## 未覆盖边界

- 未用真实模型跑多工具长对话；现场需重启 assistant-rpc 后观察不再因 30s/120s 提前 `ASSISTANT_TIMEOUT`。
- enhanced_search 回答仍受 ASST-020 的 8000 rune 截断。
