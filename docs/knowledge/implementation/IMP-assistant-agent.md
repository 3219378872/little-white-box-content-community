---
id: IMP-assistant-agent
layer: implementation
title: Assistant Agent 实现映射
status: unknown
owner: agent
upstream:
  - DES-assistant-agent-runtime
  - DES-agent-community-research
  - DES-agent-capability-governance
updated_at: 2026-09-06
tracks:
  - AGENT-001
  - AGENT-002
  - AGENT-003
  - AGENT-004
  - AGENT-100
  - AGENT-101
  - AGENT-102
  - AGENT-103
  - AGENT-104
  - AGENT-110
  - AGENT-111
  - AGENT-112
  - AGENT-113
  - AGENT-114
  - AGENT-115
  - AGENT-010
  - AGENT-011
  - AGENT-012
  - AGENT-013
  - AGENT-014
  - AGENT-015
  - AGENT-020
  - AGENT-021
  - AGENT-022
  - AGENT-023
  - AGENT-024
  - AGENT-025
  - AGENT-026
  - AGENT-030
  - AGENT-031
  - AGENT-032
  - AGENT-033
  - AGENT-034
  - AGENT-035
  - AGENT-036
  - AGENT-037
  - AGENT-040
  - AGENT-041
  - AGENT-042
  - AGENT-043
  - AGENT-044
  - AGENT-045
  - AGENT-050
  - AGENT-051
  - AGENT-052
  - AGENT-053
  - AGENT-054
  - AGENT-060
  - AGENT-061
  - AGENT-062
  - AGENT-063
  - AGENT-070
  - AGENT-071
  - AGENT-072
  - AGENT-073
  - AGENT-074
  - AGENT-075
  - AGENT-080
  - AGENT-081
  - AGENT-082
  - AGENT-083
  - AGENT-090
  - AGENT-A01
  - AGENT-A02
  - AGENT-A03
  - AGENT-A04
  - AGENT-A05
  - AGENT-A06
  - AGENT-A07
  - AGENT-A08
  - AGENT-A09
  - AGENT-A10
  - AGENT-A11
  - AGENT-A12
  - AGENT-A13
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
evidence:
  - EVD-20260906-assistant-agent
---

# Assistant Agent 实现映射

持久异步 Assistant、provider/工具治理、社区研究、澄清与逐项来源。
本页每行只归属一个活跃规格条款；`aligned` 只引用当前 active/passed EVD，
`unknown` 表示证据不足，`diverged` 表示已知未满足。源码与契约事实高于本页。

| Requirement | Design | Status | Evidence/Gap |
| --- | --- | --- | --- |
| AGENT-001 | DES-assistant-agent-runtime | unknown | gap: validation pending. |
| AGENT-002 | DES-assistant-agent-runtime | unknown | gap: validation pending. |
| AGENT-003 | DES-assistant-agent-runtime | unknown | gap: validation pending. |
| AGENT-004 | DES-assistant-agent-runtime | unknown | gap: validation pending. |
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
| AGENT-010 | DES-assistant-agent-runtime | unknown | gap: validation pending. |
| AGENT-011 | DES-assistant-agent-runtime | unknown | gap: validation pending. |
| AGENT-012 | DES-assistant-agent-runtime | unknown | gap: validation pending. |
| AGENT-013 | DES-assistant-agent-runtime | unknown | gap: validation pending. |
| AGENT-014 | DES-assistant-agent-runtime | unknown | gap: validation pending. |
| AGENT-015 | DES-assistant-agent-runtime | unknown | gap: validation pending. |
| AGENT-020 | DES-assistant-agent-runtime | unknown | gap: validation pending. |
| AGENT-021 | DES-assistant-agent-runtime | unknown | gap: validation pending. |
| AGENT-022 | DES-assistant-agent-runtime | unknown | gap: validation pending. |
| AGENT-023 | DES-assistant-agent-runtime | unknown | gap: validation pending. |
| AGENT-024 | DES-assistant-agent-runtime | unknown | gap: validation pending. |
| AGENT-025 | DES-assistant-agent-runtime | unknown | gap: validation pending. |
| AGENT-026 | DES-assistant-agent-runtime | unknown | gap: validation pending. |
| AGENT-030 | DES-agent-capability-governance | unknown | gap: validation pending. |
| AGENT-031 | DES-agent-capability-governance | unknown | gap: validation pending. |
| AGENT-032 | DES-agent-capability-governance | unknown | gap: validation pending. |
| AGENT-033 | DES-agent-capability-governance | unknown | gap: validation pending. |
| AGENT-034 | DES-agent-capability-governance | unknown | gap: validation pending. |
| AGENT-035 | DES-agent-capability-governance | unknown | gap: validation pending. |
| AGENT-036 | DES-agent-capability-governance | unknown | gap: validation pending. |
| AGENT-037 | DES-agent-capability-governance | unknown | gap: validation pending. |
| AGENT-040 | DES-agent-capability-governance | unknown | gap: validation pending. |
| AGENT-041 | DES-agent-capability-governance | unknown | gap: validation pending. |
| AGENT-042 | DES-agent-capability-governance | unknown | gap: validation pending. |
| AGENT-043 | DES-agent-capability-governance | unknown | gap: validation pending. |
| AGENT-044 | DES-agent-capability-governance | unknown | gap: validation pending. |
| AGENT-045 | DES-agent-capability-governance | unknown | gap: validation pending. |
| AGENT-050 | DES-assistant-agent-runtime | unknown | gap: validation pending. |
| AGENT-051 | DES-assistant-agent-runtime | unknown | gap: validation pending. |
| AGENT-052 | DES-assistant-agent-runtime | unknown | gap: validation pending. |
| AGENT-053 | DES-assistant-agent-runtime | unknown | gap: validation pending. |
| AGENT-054 | DES-assistant-agent-runtime | unknown | gap: validation pending. |
| AGENT-060 | DES-assistant-agent-runtime | unknown | gap: validation pending. |
| AGENT-061 | DES-assistant-agent-runtime | unknown | gap: validation pending. |
| AGENT-062 | DES-assistant-agent-runtime | unknown | gap: validation pending. |
| AGENT-063 | DES-assistant-agent-runtime | unknown | gap: validation pending. |
| AGENT-070 | DES-agent-community-research | unknown | gap: 来源 ledger、回源与结构化发布已实现；逐项语义支持仍需人类评审。 |
| AGENT-071 | DES-agent-community-research | unknown | gap: 来源 ledger、回源与结构化发布已实现；逐项语义支持仍需人类评审。 |
| AGENT-072 | DES-agent-community-research | unknown | gap: 来源 ledger、回源与结构化发布已实现；逐项语义支持仍需人类评审。 |
| AGENT-073 | DES-agent-community-research | unknown | gap: 来源 ledger、回源与结构化发布已实现；逐项语义支持仍需人类评审。 |
| AGENT-074 | DES-agent-community-research | unknown | gap: 来源 ledger、回源与结构化发布已实现；逐项语义支持仍需人类评审。 |
| AGENT-075 | DES-agent-community-research | unknown | gap: 来源 ledger、回源与结构化发布已实现；逐项语义支持仍需人类评审。 |
| AGENT-080 | DES-assistant-agent-runtime | unknown | gap: validation pending. |
| AGENT-081 | DES-assistant-agent-runtime | unknown | gap: validation pending. |
| AGENT-082 | DES-assistant-agent-runtime | unknown | gap: validation pending. |
| AGENT-083 | DES-assistant-agent-runtime | unknown | gap: validation pending. |
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

当前确定性验证见 `EVD-20260906-assistant-agent`。旧 `implementation/evidence/` 记录只保留历史上下文，
不参与当前 `aligned` 判定。未执行的人类、真实 provider、浏览器、设备和生产证据不会被推断。
