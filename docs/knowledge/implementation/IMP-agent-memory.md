---
id: IMP-agent-memory
layer: implementation
title: Agent Memory 实现映射
status: active
owner: agent
updated_at: 2026-09-25
code_paths:
- pkg/rpcx
- pkg/httpx
- pkg/configx
- pkg/logging
- pkg/sqlstore
- pkg/redisstore
- pkg/cachedstore
- kitex_gen
- pkg/interceptor
- app/assistant/internal/memory
- app/assistant/internal/runtime
- app/assistant/internal/store
- app/assistant/internal/prompt
- app/gateway/internal/logic/assistant
- proto/assistant/assistant.proto
- deploy/sql/xbh_assistant.sql
---

# Agent Memory 实现映射

MEMORY/USER、容量与版本、审查、撤销和不可信 sidecar。
本页每行只归属一个活跃规格条款；`aligned` 只引用当前 active/passed EVD，
`unknown` 表示证据不足，`diverged` 表示已知未满足。源码与契约事实高于本页。

| requirement | design | state | evidence or gap |
| --- | --- | --- | --- |
| MEM-001 | DES-assistant-agent-runtime | unknown | gap: 框架与生成契约已迁移，待新提交上的对应验收证据。 |
| MEM-002 | DES-assistant-agent-runtime | unknown | gap: 框架与生成契约已迁移，待新提交上的对应验收证据。 |
| MEM-003 | DES-assistant-agent-runtime | unknown | gap: 框架与生成契约已迁移，待新提交上的对应验收证据。 |
| MEM-004 | DES-agent-capability-governance | unknown | gap: 框架与生成契约已迁移，待新提交上的对应验收证据。 |
| MEM-010 | DES-assistant-agent-runtime | unknown | gap: 框架与生成契约已迁移，待新提交上的对应验收证据。 |
| MEM-011 | DES-assistant-agent-runtime | unknown | gap: 框架与生成契约已迁移，待新提交上的对应验收证据。 |
| MEM-012 | DES-assistant-agent-runtime | unknown | gap: 框架与生成契约已迁移，待新提交上的对应验收证据。 |
| MEM-013 | DES-assistant-agent-runtime | unknown | gap: 框架与生成契约已迁移，待新提交上的对应验收证据。 |
| MEM-014 | DES-assistant-agent-runtime | unknown | gap: 框架与生成契约已迁移，待新提交上的对应验收证据。 |
| MEM-020 | DES-assistant-agent-runtime | unknown | gap: 框架与生成契约已迁移，待新提交上的对应验收证据。 |
| MEM-021 | DES-assistant-agent-runtime | unknown | gap: 框架与生成契约已迁移，待新提交上的对应验收证据。 |
| MEM-022 | DES-assistant-agent-runtime | unknown | gap: 框架与生成契约已迁移，待新提交上的对应验收证据。 |
| MEM-023 | DES-assistant-agent-runtime | unknown | gap: 框架与生成契约已迁移，待新提交上的对应验收证据。 |
| MEM-024 | DES-assistant-agent-runtime | unknown | gap: 框架与生成契约已迁移，待新提交上的对应验收证据。 |
| MEM-025 | DES-agent-capability-governance | unknown | gap: 框架与生成契约已迁移，待新提交上的对应验收证据。 |
| MEM-030 | DES-assistant-agent-runtime | unknown | gap: 框架与生成契约已迁移，待新提交上的对应验收证据。 |
| MEM-031 | DES-assistant-agent-runtime | unknown | gap: 框架与生成契约已迁移，待新提交上的对应验收证据。 |
| MEM-032 | DES-assistant-agent-runtime | unknown | gap: 框架与生成契约已迁移，待新提交上的对应验收证据。 |
| MEM-033 | DES-assistant-agent-runtime | unknown | gap: 框架与生成契约已迁移，待新提交上的对应验收证据。 |
| MEM-A01 | DES-assistant-agent-runtime | unknown | gap: 框架与生成契约已迁移，待新提交上的对应验收证据。 |
| MEM-A02 | DES-assistant-agent-runtime | unknown | gap: 框架与生成契约已迁移，待新提交上的对应验收证据。 |
| MEM-A03 | DES-assistant-agent-runtime | unknown | gap: 框架与生成契约已迁移，待新提交上的对应验收证据。 |
| MEM-A04 | DES-assistant-agent-runtime | unknown | gap: 框架与生成契约已迁移，待新提交上的对应验收证据。 |
| MEM-A05 | DES-assistant-agent-runtime | unknown | gap: 非来源边界有单测；真实存储故障集成注入未完成。 |
| MEM-A06 | DES-assistant-agent-runtime | unknown | gap: 框架与生成契约已迁移，待新提交上的对应验收证据。 |

## 代码边界

具体入口由 frontmatter 的 `code_paths` 给出；路径均相对仓库根目录。

## 证据边界

当前确定性验证见 `EVD-20260925-quality-remediation`。覆盖 MEMORY/USER 回归及清空历史后记忆保留；在当前提交重验既有 runtime 输入，修复 MEM-001 证据过期。
证据在固定实现提交运行全模块 race、静态检查与专用 MySQL/Redis 隔离集成，随后更新本映射；
历史 EVD 保留原观察结果，不改写为当前证明。原有 unknown/diverged 及其 gap 不提升。
本轮没有真实模型、浏览器、设备、容量或生产验证，这些范围不能从本地门禁推导。
