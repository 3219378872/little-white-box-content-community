---
id: IMP-assistant-agent
layer: implementation
title: Assistant Agent 实现映射
status: active
owner: agent
updated_at: 2026-09-07
code_paths:
- app/assistant/internal/runtime
- app/assistant/internal/store
- app/assistant/internal/lease
- app/assistant/internal/llm
- app/assistant/internal/prompt
- app/assistant/internal/tool
- app/assistant/worker
- app/assistant/rpc
- app/gateway/internal/logic/assistant
- app/gateway/gateway.api
- proto/assistant/assistant.proto
- deploy/sql/xbh_assistant.sql
- deploy/sql/patches/20260905_agent_research.sql
- scripts/spec_evals.py
- eval/dev/assistant_cases.synthetic.json
---

# Assistant Agent 实现映射

持久异步 Assistant、provider/工具治理、社区研究、澄清与逐项来源。
本页每行只归属一个活跃规格条款；`aligned` 只引用当前 active/passed EVD，
`unknown` 表示证据不足，`diverged` 表示已知未满足。源码与契约事实高于本页。

| requirement | design | state | evidence or gap |
| --- | --- | --- | --- |
| AGENT-001 | DES-assistant-agent-runtime | aligned | EVD-20260907-watch-contract |
| AGENT-002 | DES-assistant-agent-runtime | aligned | EVD-20260907-watch-contract |
| AGENT-003 | DES-assistant-agent-runtime | aligned | EVD-20260907-watch-contract |
| AGENT-004 | DES-assistant-agent-runtime | aligned | EVD-20260907-watch-contract |
| AGENT-100 | DES-agent-community-research | unknown | gap: 协议、存储与工具路径已实现；真实 provider 轨迹和人类语义质量评审未关闭。 |
| AGENT-101 | DES-agent-community-research | unknown | gap: 协议、存储与工具路径已实现；真实 provider 轨迹和人类语义质量评审未关闭。 |
| AGENT-102 | DES-agent-community-research | unknown | gap: 协议、存储与工具路径已实现；真实 provider 轨迹和人类语义质量评审未关闭。 |
| AGENT-103 | DES-agent-community-research | unknown | gap: 协议、存储与工具路径已实现；真实 provider 轨迹和人类语义质量评审未关闭。 |
| AGENT-104 | DES-agent-community-research | unknown | gap: 协议、存储与工具路径已实现；真实 provider 轨迹和人类语义质量评审未关闭。 |
| AGENT-110 | DES-agent-community-research | unknown | gap: 协议、存储与工具路径已实现；真实 provider 轨迹和人类语义质量评审未关闭。 |
| AGENT-111 | DES-agent-community-research | unknown | gap: 协议、存储与工具路径已实现；真实 provider 轨迹和人类语义质量评审未关闭。 |
| AGENT-112 | DES-agent-community-research | unknown | gap: 协议、存储与工具路径已实现；真实 provider 轨迹和人类语义质量评审未关闭。 |
| AGENT-113 | DES-agent-community-research | unknown | gap: 协议、存储与工具路径已实现；真实 provider 轨迹和人类语义质量评审未关闭。 |
| AGENT-114 | DES-agent-community-research | unknown | gap: 协议、存储与工具路径已实现；真实 provider 轨迹和人类语义质量评审未关闭。 |
| AGENT-115 | DES-agent-community-research | unknown | gap: 协议、存储与工具路径已实现；真实 provider 轨迹和人类语义质量评审未关闭。 |
| AGENT-010 | DES-assistant-agent-runtime | aligned | EVD-20260907-watch-contract |
| AGENT-011 | DES-assistant-agent-runtime | aligned | EVD-20260907-watch-contract |
| AGENT-012 | DES-assistant-agent-runtime | aligned | EVD-20260907-watch-contract |
| AGENT-013 | DES-assistant-agent-runtime | aligned | EVD-20260907-watch-contract |
| AGENT-014 | DES-assistant-agent-runtime | aligned | EVD-20260907-watch-contract |
| AGENT-015 | DES-assistant-agent-runtime | aligned | EVD-20260907-watch-contract |
| AGENT-020 | DES-assistant-agent-runtime | aligned | EVD-20260907-watch-contract |
| AGENT-021 | DES-assistant-agent-runtime | aligned | EVD-20260907-watch-contract |
| AGENT-022 | DES-assistant-agent-runtime | aligned | EVD-20260907-watch-contract |
| AGENT-023 | DES-assistant-agent-runtime | aligned | EVD-20260907-watch-contract |
| AGENT-024 | DES-assistant-agent-runtime | aligned | EVD-20260907-watch-contract |
| AGENT-025 | DES-assistant-agent-runtime | aligned | EVD-20260907-watch-contract |
| AGENT-026 | DES-assistant-agent-runtime | aligned | EVD-20260907-watch-contract |
| AGENT-030 | DES-agent-capability-governance | aligned | EVD-20260907-watch-contract |
| AGENT-031 | DES-agent-capability-governance | aligned | EVD-20260907-watch-contract |
| AGENT-032 | DES-agent-capability-governance | aligned | EVD-20260907-watch-contract |
| AGENT-033 | DES-agent-capability-governance | aligned | EVD-20260907-watch-contract |
| AGENT-034 | DES-agent-capability-governance | aligned | EVD-20260907-watch-contract |
| AGENT-035 | DES-agent-capability-governance | aligned | EVD-20260907-watch-contract |
| AGENT-036 | DES-agent-capability-governance | aligned | EVD-20260907-watch-contract |
| AGENT-037 | DES-agent-capability-governance | aligned | EVD-20260907-watch-contract |
| AGENT-040 | DES-agent-capability-governance | aligned | EVD-20260907-watch-contract |
| AGENT-041 | DES-agent-capability-governance | aligned | EVD-20260907-watch-contract |
| AGENT-042 | DES-agent-capability-governance | aligned | EVD-20260907-watch-contract |
| AGENT-043 | DES-agent-capability-governance | aligned | EVD-20260907-watch-contract |
| AGENT-044 | DES-agent-capability-governance | aligned | EVD-20260907-watch-contract |
| AGENT-045 | DES-agent-capability-governance | aligned | EVD-20260907-watch-contract |
| AGENT-050 | DES-assistant-agent-runtime | aligned | EVD-20260907-watch-contract |
| AGENT-051 | DES-assistant-agent-runtime | aligned | EVD-20260907-watch-contract |
| AGENT-052 | DES-assistant-agent-runtime | aligned | EVD-20260907-watch-contract |
| AGENT-053 | DES-assistant-agent-runtime | aligned | EVD-20260907-watch-contract |
| AGENT-054 | DES-assistant-agent-runtime | aligned | EVD-20260907-watch-contract |
| AGENT-060 | DES-assistant-agent-runtime | aligned | EVD-20260907-watch-contract |
| AGENT-061 | DES-assistant-agent-runtime | aligned | EVD-20260907-watch-contract |
| AGENT-062 | DES-assistant-agent-runtime | aligned | EVD-20260907-watch-contract |
| AGENT-063 | DES-assistant-agent-runtime | aligned | EVD-20260907-watch-contract |
| AGENT-070 | DES-agent-community-research | unknown | gap: 来源 ledger、回源与结构化发布已实现；逐项语义支持仍需人类评审。 |
| AGENT-071 | DES-agent-community-research | unknown | gap: 来源 ledger、回源与结构化发布已实现；逐项语义支持仍需人类评审。 |
| AGENT-072 | DES-agent-community-research | unknown | gap: 来源 ledger、回源与结构化发布已实现；逐项语义支持仍需人类评审。 |
| AGENT-073 | DES-agent-community-research | unknown | gap: 来源 ledger、回源与结构化发布已实现；逐项语义支持仍需人类评审。 |
| AGENT-074 | DES-agent-community-research | unknown | gap: 来源 ledger、回源与结构化发布已实现；逐项语义支持仍需人类评审。 |
| AGENT-075 | DES-agent-community-research | unknown | gap: 来源 ledger、回源与结构化发布已实现；逐项语义支持仍需人类评审。 |
| AGENT-080 | DES-assistant-agent-runtime | aligned | EVD-20260907-watch-contract |
| AGENT-081 | DES-assistant-agent-runtime | aligned | EVD-20260907-watch-contract |
| AGENT-082 | DES-assistant-agent-runtime | aligned | EVD-20260907-watch-contract |
| AGENT-083 | DES-assistant-agent-runtime | aligned | EVD-20260907-watch-contract |
| AGENT-090 | DES-assistant-agent-runtime | unknown | gap: 接收/首事件实现可测；45 秒完成仅观察，Watch 5 分钟与生产 p95 未验证。 |
| AGENT-A01 | DES-assistant-agent-runtime | unknown | gap: 确定性 fixture/单测覆盖核心路径；外部 live provider 与生产边界未关闭。 |
| AGENT-A02 | DES-assistant-agent-runtime | unknown | gap: 确定性 fixture/单测覆盖核心路径；外部 live provider 与生产边界未关闭。 |
| AGENT-A03 | DES-assistant-agent-runtime | unknown | gap: 确定性 fixture/单测覆盖核心路径；外部 live provider 与生产边界未关闭。 |
| AGENT-A04 | DES-assistant-agent-runtime | unknown | gap: 确定性 fixture/单测覆盖核心路径；外部 live provider 与生产边界未关闭。 |
| AGENT-A05 | DES-assistant-agent-runtime | unknown | gap: 确定性 fixture/单测覆盖核心路径；外部 live provider 与生产边界未关闭。 |
| AGENT-A06 | DES-assistant-agent-runtime | unknown | gap: 确定性 fixture/单测覆盖核心路径；外部 live provider 与生产边界未关闭。 |
| AGENT-A07 | DES-assistant-agent-runtime | unknown | gap: 确定性 fixture/单测覆盖核心路径；外部 live provider 与生产边界未关闭。 |
| AGENT-A08 | DES-assistant-agent-runtime | unknown | gap: 确定性 fixture/单测覆盖核心路径；外部 live provider 与生产边界未关闭。 |
| AGENT-A09 | DES-assistant-agent-runtime | unknown | gap: 确定性 fixture/单测覆盖核心路径；外部 live provider 与生产边界未关闭。 |
| AGENT-A10 | DES-agent-community-research | unknown | gap: 协议、存储与工具路径已实现；真实 provider 轨迹和人类语义质量评审未关闭。 |
| AGENT-A11 | DES-agent-community-research | unknown | gap: 协议、存储与工具路径已实现；真实 provider 轨迹和人类语义质量评审未关闭。 |
| AGENT-A12 | DES-agent-community-research | unknown | gap: 协议、存储与工具路径已实现；真实 provider 轨迹和人类语义质量评审未关闭。 |
| AGENT-A13 | DES-agent-community-research | unknown | gap: 协议、存储与工具路径已实现；真实 provider 轨迹和人类语义质量评审未关闭。 |

## 代码边界

具体入口由 frontmatter 的 `code_paths` 给出；路径均相对仓库根目录。

## 证据边界

当前确定性验证见 `EVD-20260907-watch-contract`。旧 `implementation/evidence/` 记录只保留历史上下文，
不参与当前 `aligned` 判定。未执行的人类、真实 provider、浏览器、设备和生产证据不会被推断。
