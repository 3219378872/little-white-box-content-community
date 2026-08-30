---
implementation: IMP-content-community-backend
verified_at: 2026-08-30
verified_commit: 0db627fbb8a94401e4aecf84c33e508b127c9a0f
commands:
  - go test ./app/assistant/internal/runtime ./app/assistant/internal/prompt ./app/assistant/internal/store
  - go test ./app/assistant/...
result: passed
---

# Watch 注入 sidecar 进入 provider 历史

## 环境

Worktree `task-watch-input-sidecar`。无 live provider。验证为包级 `go test`。

## 命令与结果

```
go test ./app/assistant/internal/runtime ./app/assistant/internal/prompt ./app/assistant/internal/store
go test ./app/assistant/...
```

全部通过。覆盖：

- Watch 命中 JSON 落成隐藏 `watch_input` sidecar，不进入可见私信
- `HistoryTurns` 重放全部 `api_content`，包括隐藏 Watch 注入
- 第二轮 `Complete()` 顺序为 hits → tool_call → tool_result，命中 JSON 只出现一次
- 当前 Watch run 的 sidecar 在 compact 时强制保留；已完成工具轮仍可压缩
- `ensureWatchInput` 幂等，恢复不重复插入

## 未覆盖边界

- 无 live GLM/Responses 联调；根真实栈未重跑
- 已在跑、没有 sidecar 的旧 Watch run 仍可能先插入后重排；重复读保险丝保留
