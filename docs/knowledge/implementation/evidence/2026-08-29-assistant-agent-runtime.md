---
implementation: IMP-content-community-backend
verified_at: 2026-08-29
verified_commit: c36e59555601dc37a54efe136525d04a98365a5c
commands:
  - go test ./app/assistant/... ./app/gateway/... ./deploy/... ./pkg/errx/...
result: passed
---

# Hermes 持久异步 Assistant Agent 硬切换

## 环境

Worktree `task-hermes-agent`。无 live LLM、无 testcontainers MySQL。验证为包级 `go test`。

## 命令与结果

```
go test ./app/assistant/... ./app/gateway/... ./deploy/... ./pkg/errx/...
```

全部通过。覆盖：

- disposition / FIFO 32 / consent
- confirm CAS
- source handle 本 run 绑定与 `present_sources`
- Memory 容量/扫描/undo/version
- 预算每维每级一次
- prompt snapshot 复用
- compact keep-20% 选择
- Chat Completions / Responses 工具调用 httptest
- readiness 在不支持工具时失败
- gateway PostMessage 鉴权、run events 流、REST 契约与 nginx/prometheus/compose

## 未覆盖边界

- 无 live provider；worker 端到端 lease crash recovery 未接真实 MySQL SKIP LOCKED
- ES rebuild/delete 与用户隔离未接真实集群
- `discussion_spike` 生产仍无 Judge
- 根真实栈（授权→异步发送→断线续流→删除确认→memory-review→Watch 主动消息）未跑
