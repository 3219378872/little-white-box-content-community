---
implementation: IMP-content-community-backend
verified_at: 2026-08-31
verified_commit: 7424525d32f9490ca3736f20131d84895680bad7
commands:
  - go test ./app/assistant/internal/runtime/ -count=1
  - go test ./app/assistant/internal/runtime/ ./app/assistant/internal/tool/ ./app/assistant/internal/llm/ -count=1
  - python3 scripts/engineering-lint.py
result: passed
---

# present_sources 保留已流式回答

## 范围

验证模型在已流式输出用户可见正文后只调用 `present_sources` 时不写 `response_reset`，该正文作为
可见 assistant 消息落库；`get_memory` 等检索类工具轮仍 reset 前导草稿。AGENT-026 的 retry /
redirect / lease 接管 reset 路径未改。

## 命令与结果

```text
go test ./app/assistant/internal/runtime/ -count=1 -timeout 120s
# ok esx/app/assistant/internal/runtime 0.290s

go test ./app/assistant/internal/runtime/ ./app/assistant/internal/tool/ ./app/assistant/internal/llm/ -count=1 -timeout 180s
# ok runtime 0.282s / tool 0.020s / llm 0.142s

python3 scripts/engineering-lint.py
# engineering-lint: all checks passed
```

`TestStreamingPresentSourcesKeepsStreamedAnswer`：event 重放无 reset，可见消息为
`full report` 与 `sources shown`。`TestStreamingToolRoundResetsPreambleBeforeFinalAnswer` 与
`TestStreamingRetryReplayKeepsOnlyWinningAttempt` 仍要求 reset。

## 未覆盖

未跑 `make check` / `make test` 全量、integration、live provider，也未在根编排仓对真实
`present_sources` 会话做 e2e。结果按本工作树单测记 `passed`。
