---
id: IMP-agent-memory
layer: implementation
title: Agent Memory 实现映射
status: unknown
owner: agent
upstream:
  - DES-assistant-agent-runtime
  - DES-agent-capability-governance
updated_at: 2026-09-06
tracks:
  - MEM-001
  - MEM-002
  - MEM-003
  - MEM-004
  - MEM-010
  - MEM-011
  - MEM-012
  - MEM-013
  - MEM-014
  - MEM-020
  - MEM-021
  - MEM-022
  - MEM-023
  - MEM-024
  - MEM-025
  - MEM-030
  - MEM-031
  - MEM-032
  - MEM-033
  - MEM-A01
  - MEM-A02
  - MEM-A03
  - MEM-A04
  - MEM-A05
  - MEM-A06
code_paths:
  - app/assistant/internal/memory
  - app/assistant/internal/runtime
  - app/assistant/internal/store
  - app/assistant/internal/prompt
  - app/gateway/internal/logic/assistant
  - proto/assistant/assistant.proto
  - deploy/sql/xbh_assistant.sql
evidence:
  - EVD-20260906-agent-memory
---

# Agent Memory 实现映射

MEMORY/USER、容量与版本、审查、撤销和不可信 sidecar。
本页每行只归属一个活跃规格条款；`aligned` 只引用当前 active/passed EVD，
`unknown` 表示证据不足，`diverged` 表示已知未满足。源码与契约事实高于本页。

| Requirement | Design | Status | Evidence/Gap |
| --- | --- | --- | --- |
| MEM-001 | DES-assistant-agent-runtime | aligned | EVD-20260906-agent-memory |
| MEM-002 | DES-assistant-agent-runtime | aligned | EVD-20260906-agent-memory |
| MEM-003 | DES-assistant-agent-runtime | aligned | EVD-20260906-agent-memory |
| MEM-004 | DES-agent-capability-governance | aligned | EVD-20260906-agent-memory |
| MEM-010 | DES-assistant-agent-runtime | aligned | EVD-20260906-agent-memory |
| MEM-011 | DES-assistant-agent-runtime | aligned | EVD-20260906-agent-memory |
| MEM-012 | DES-assistant-agent-runtime | aligned | EVD-20260906-agent-memory |
| MEM-013 | DES-assistant-agent-runtime | aligned | EVD-20260906-agent-memory |
| MEM-014 | DES-assistant-agent-runtime | aligned | EVD-20260906-agent-memory |
| MEM-020 | DES-assistant-agent-runtime | aligned | EVD-20260906-agent-memory |
| MEM-021 | DES-assistant-agent-runtime | aligned | EVD-20260906-agent-memory |
| MEM-022 | DES-assistant-agent-runtime | aligned | EVD-20260906-agent-memory |
| MEM-023 | DES-assistant-agent-runtime | aligned | EVD-20260906-agent-memory |
| MEM-024 | DES-assistant-agent-runtime | aligned | EVD-20260906-agent-memory |
| MEM-025 | DES-agent-capability-governance | aligned | EVD-20260906-agent-memory |
| MEM-030 | DES-assistant-agent-runtime | aligned | EVD-20260906-agent-memory |
| MEM-031 | DES-assistant-agent-runtime | aligned | EVD-20260906-agent-memory |
| MEM-032 | DES-assistant-agent-runtime | aligned | EVD-20260906-agent-memory |
| MEM-033 | DES-assistant-agent-runtime | aligned | EVD-20260906-agent-memory |
| MEM-A01 | DES-assistant-agent-runtime | aligned | EVD-20260906-agent-memory |
| MEM-A02 | DES-assistant-agent-runtime | aligned | EVD-20260906-agent-memory |
| MEM-A03 | DES-assistant-agent-runtime | aligned | EVD-20260906-agent-memory |
| MEM-A04 | DES-assistant-agent-runtime | aligned | EVD-20260906-agent-memory |
| MEM-A05 | DES-assistant-agent-runtime | unknown | gap: 非来源边界有单测；真实存储故障集成注入未完成。 |
| MEM-A06 | DES-assistant-agent-runtime | aligned | EVD-20260906-agent-memory |

## 代码边界

具体入口由 frontmatter 的 `code_paths` 给出；路径均相对仓库根目录。

## 证据边界

当前确定性验证见 `EVD-20260906-agent-memory`。旧 `implementation/evidence/` 记录只保留历史上下文，
不参与当前 `aligned` 判定。未执行的人类、真实 provider、浏览器、设备和生产证据不会被推断。
