---
id: IMP-agent-watch
layer: implementation
title: Agent Watch 实现映射
status: active
owner: agent
updated_at: 2026-09-25
code_paths:
- pkg/interceptor
- app/assistant/watch
- app/assistant/mq
- app/assistant/internal/runtime
- app/assistant/internal/store
- app/assistant/internal/tool
- app/gateway/internal/logic/assistant
- proto/assistant/assistant.proto
- deploy/sql/xbh_assistant.sql
---

# Agent Watch 实现映射

Watch 条件匹配、两分钟 bucket、配额、恢复与主动 Assistant 消息。
本页每行只归属一个活跃规格条款；`aligned` 只引用当前 active/passed EVD，
`unknown` 表示证据不足，`diverged` 表示已知未满足。源码与契约事实高于本页。

| requirement | design | state | evidence or gap |
| --- | --- | --- | --- |
| WCH-001 | DES-assistant-agent-runtime | aligned | EVD-20260925-quality-remediation |
| WCH-002 | DES-assistant-agent-runtime | diverged | gap: 四种规则匹配与 discussion_spike 预筛选已实现，但实际 matcher 未注入 SpikeJudge；阈值达标仅记录 failed，不能产生模型判定命中。 |
| WCH-003 | DES-assistant-agent-runtime | aligned | EVD-20260925-quality-remediation |
| WCH-004 | DES-assistant-agent-runtime | aligned | EVD-20260925-quality-remediation |
| WCH-010 | DES-assistant-agent-runtime | aligned | EVD-20260925-quality-remediation |
| WCH-011 | DES-assistant-agent-runtime | aligned | EVD-20260925-quality-remediation |
| WCH-012 | DES-assistant-agent-runtime | aligned | EVD-20260925-quality-remediation |
| WCH-013 | DES-assistant-agent-runtime | aligned | EVD-20260925-quality-remediation |
| WCH-014 | DES-assistant-agent-runtime | unknown | gap: 结构化发布与回源已实现；主动消息语义质量仍缺真实场景评审。 |
| WCH-020 | DES-assistant-agent-runtime | aligned | EVD-20260925-quality-remediation |
| WCH-021 | DES-assistant-agent-runtime | aligned | EVD-20260925-quality-remediation |
| WCH-022 | DES-assistant-agent-runtime | aligned | EVD-20260925-quality-remediation |
| WCH-023 | DES-assistant-agent-runtime | aligned | EVD-20260925-quality-remediation |
| WCH-024 | DES-assistant-agent-runtime | aligned | EVD-20260925-quality-remediation |
| WCH-A01 | DES-assistant-agent-runtime | diverged | gap: 单测使用注入的 SpikeJudge；实际 matcher 未接入该判定器，不能以 fixture 通过证明 discussion_spike 已交付。 |
| WCH-A02 | DES-assistant-agent-runtime | aligned | EVD-20260925-quality-remediation |
| WCH-A03 | DES-assistant-agent-runtime | unknown | gap: 只读工具与抢占路径有测试；真实 SQL 取消交错缺专门集成覆盖。 |
| WCH-A04 | DES-assistant-agent-runtime | aligned | EVD-20260925-quality-remediation |
| WCH-A05 | DES-assistant-agent-runtime | unknown | gap: CRUD 与停用有测试；90 天恢复和不可见内容补投缺集成验证。 |
| WCH-A06 | DES-assistant-agent-runtime | aligned | EVD-20260925-quality-remediation |

## 代码边界

具体入口由 frontmatter 的 `code_paths` 给出；路径均相对仓库根目录。

## 证据边界

当前确定性验证见 `EVD-20260925-quality-remediation`。覆盖 Watch 调度、预算、恢复及删除历史时的任务隔离；Watch 定义不随历史删除而清除。
证据在固定实现提交运行全模块 race、静态检查与专用 MySQL/Redis 隔离集成，随后更新本映射；
历史 EVD 保留原观察结果，不改写为当前证明。原有 unknown/diverged 及其 gap 不提升。
本轮没有真实模型、浏览器、设备、容量或生产验证，这些范围不能从本地门禁推导。
