---
implementation: IMP-content-community-backend
verified_at: 2026-08-29
verified_commit: task/agent-tool-context
commands:
  - go test ./app/assistant/internal/canonical ./app/assistant/internal/llm ./app/assistant/internal/prompt ./app/assistant/internal/runtime ./app/assistant/internal/tool
  - go test ./app/assistant/...
result: passed
---

# Assistant 工具轮上下文重放

## 环境

Worktree `task-agent-tool-context`。无 live provider。验证为包级 `go test`。

## 命令与结果

```
go test ./app/assistant/internal/canonical ./app/assistant/internal/llm ./app/assistant/internal/prompt ./app/assistant/internal/runtime ./app/assistant/internal/tool
go test ./app/assistant/...
```

全部通过。覆盖：

- Responses `arguments` JSON 字符串剥壳后与对象参数一致
- Chat Completions `tool_calls` + `role=tool`，Responses `function_call` + `function_call_output`
- 隐藏 tool sidecar 进入下一轮 `Complete()`，用户正文不重复
- 崩溃后未完成 tool call 在下一轮 LLM 前补执行
- compact 只标记未保留消息；`kind=tool` 强制保留
- `result_json` 写入合法 JSON 对象

## 未覆盖边界

- 无 live GLM/Responses 联调；根真实栈未重跑
- 已在跑的旧 run 没有 tool sidecar，需取消后重发
