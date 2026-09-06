# 实现层

本目录由 agent 维护，把六份活跃规格的每个精确条款唯一映射到当前实现领域。源码、配置、`.api`、
`.proto` 和测试是当前行为的事实权威；IMP 不能覆盖它们，也不能反向修改上层语义。

每个非 retired `IMP-*` 的 `tracks` 只列精确 requirement ID，`code_paths` 列仓库相对源码入口，
`evidence` 双向引用独立 [EVD 证据层](../evidence/README.md)。权威表逐行使用
`aligned/diverged/unknown`：只有 `aligned` 必须由 active/passed EVD 覆盖；其余状态必须写明
`gap: ...`。权威表必须且只能有一个如下精确表头和分隔行；不得使用范围、斜线或合并 ID。

```text
| Requirement | Design | Status | Evidence/Gap |
| --- | --- | --- | --- |
```

## 当前六域映射

| 实现页 | 对应 approved SPEC | 状态 |
| --- | --- | --- |
| [IMP-community-core](IMP-community-core.md) | SPEC-community-core | unknown |
| [IMP-content-discovery](IMP-content-discovery.md) | SPEC-content-discovery | diverged |
| [IMP-assistant-agent](IMP-assistant-agent.md) | SPEC-assistant-agent | unknown |
| [IMP-agent-memory](IMP-agent-memory.md) | SPEC-agent-memory | unknown |
| [IMP-agent-watch](IMP-agent-watch.md) | SPEC-agent-watch | unknown |
| [IMP-feedback-reliability](IMP-feedback-reliability.md) | SPEC-feedback-reliability | diverged |

## 稳定 ID 迁移指针

以下正式 IMP 身份已退役；页面只保留稳定 ID 与当前非正式指南/状态入口，不参与覆盖：

- [IMP-content-community-backend](IMP-content-community-backend.md)
- [IMP-architecture](IMP-architecture.md)
- [IMP-development-quickstart](IMP-development-quickstart.md)
- [IMP-engineering-conventions](IMP-engineering-conventions.md)
- [IMP-todo-blocked-gates](IMP-todo-blocked-gates.md)

旧 [implementation/evidence](evidence/README.md) 原样保留为 legacy 历史。当前操作指南见
[guides](../guides/README.md)，当前开放门禁见 [status/open-gates](../status/open-gates.md)。
