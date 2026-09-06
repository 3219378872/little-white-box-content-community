---
id: IMP-agent-watch
layer: implementation
title: Agent Watch 实现映射
status: unknown
owner: agent
upstream:
  - DES-assistant-agent-runtime
updated_at: 2026-09-06
tracks:
  - WCH-001
  - WCH-002
  - WCH-003
  - WCH-004
  - WCH-010
  - WCH-011
  - WCH-012
  - WCH-013
  - WCH-014
  - WCH-020
  - WCH-021
  - WCH-022
  - WCH-023
  - WCH-024
  - WCH-A01
  - WCH-A02
  - WCH-A03
  - WCH-A04
  - WCH-A05
  - WCH-A06
code_paths:
  - app/assistant/watch
  - app/assistant/mq
  - app/assistant/internal/runtime
  - app/assistant/internal/store
  - app/gateway/internal/logic/assistant
  - proto/assistant/assistant.proto
  - deploy/sql/xbh_assistant.sql
evidence:
  - EVD-20260906-agent-watch
---

# Agent Watch 实现映射

Watch 条件匹配、两分钟 bucket、配额、恢复与主动 Assistant 消息。
本页每行只归属一个活跃规格条款；`aligned` 只引用当前 active/passed EVD，
`unknown` 表示证据不足，`diverged` 表示已知未满足。源码与契约事实高于本页。

| Requirement | Design | Status | Evidence/Gap |
| --- | --- | --- | --- |
| WCH-001 | DES-assistant-agent-runtime | aligned | EVD-20260906-agent-watch |
| WCH-002 | DES-assistant-agent-runtime | aligned | EVD-20260906-agent-watch |
| WCH-003 | DES-assistant-agent-runtime | aligned | EVD-20260906-agent-watch |
| WCH-004 | DES-assistant-agent-runtime | aligned | EVD-20260906-agent-watch |
| WCH-010 | DES-assistant-agent-runtime | aligned | EVD-20260906-agent-watch |
| WCH-011 | DES-assistant-agent-runtime | aligned | EVD-20260906-agent-watch |
| WCH-012 | DES-assistant-agent-runtime | aligned | EVD-20260906-agent-watch |
| WCH-013 | DES-assistant-agent-runtime | aligned | EVD-20260906-agent-watch |
| WCH-014 | DES-assistant-agent-runtime | unknown | gap: 结构化发布与回源已实现；主动消息语义质量仍缺真实场景评审。 |
| WCH-020 | DES-assistant-agent-runtime | aligned | EVD-20260906-agent-watch |
| WCH-021 | DES-assistant-agent-runtime | aligned | EVD-20260906-agent-watch |
| WCH-022 | DES-assistant-agent-runtime | aligned | EVD-20260906-agent-watch |
| WCH-023 | DES-assistant-agent-runtime | aligned | EVD-20260906-agent-watch |
| WCH-024 | DES-assistant-agent-runtime | aligned | EVD-20260906-agent-watch |
| WCH-A01 | DES-assistant-agent-runtime | aligned | EVD-20260906-agent-watch |
| WCH-A02 | DES-assistant-agent-runtime | aligned | EVD-20260906-agent-watch |
| WCH-A03 | DES-assistant-agent-runtime | unknown | gap: 只读工具与抢占路径有测试；真实 SQL 取消交错缺专门集成覆盖。 |
| WCH-A04 | DES-assistant-agent-runtime | aligned | EVD-20260906-agent-watch |
| WCH-A05 | DES-assistant-agent-runtime | unknown | gap: CRUD 与停用有测试；90 天恢复和不可见内容补投缺集成验证。 |
| WCH-A06 | DES-assistant-agent-runtime | aligned | EVD-20260906-agent-watch |

## 代码边界

具体入口由 frontmatter 的 `code_paths` 给出；路径均相对仓库根目录。

## 证据边界

当前确定性验证见 `EVD-20260906-agent-watch`。旧 `implementation/evidence/` 记录只保留历史上下文，
不参与当前 `aligned` 判定。未执行的人类、真实 provider、浏览器、设备和生产证据不会被推断。
