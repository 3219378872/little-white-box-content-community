---
id: IMP-feedback-reliability
layer: implementation
title: 反馈与可靠性实现映射
status: active
owner: agent
updated_at: 2026-09-07
code_paths:
- app/behavior
- app/pipeline/behaviorlog
- app/recommend/mq
- pkg/outboxx
- app/gateway
- app/assistant
- deploy/loki/loki-config.yaml
- deploy/docker-compose.production.yml
- scripts/spec_evals.py
- scripts/gateway_performance.py
---

# 反馈与可靠性实现映射

行为闭环、隐私保留、SLO、恢复、可观测性与十二项故障降级。
本页每行只归属一个活跃规格条款；`aligned` 只引用当前 active/passed EVD，
`unknown` 表示证据不足，`diverged` 表示已知未满足。源码与契约事实高于本页。

| requirement | design | state | evidence or gap |
| --- | --- | --- | --- |
| REL-001 | DES-content-community-backend | aligned | EVD-20260907-watch-contract |
| REL-002 | DES-content-community-backend | aligned | EVD-20260907-watch-contract |
| REL-003 | DES-content-community-backend | aligned | EVD-20260907-watch-contract |
| REL-004 | DES-content-community-backend | aligned | EVD-20260907-watch-contract |
| REL-005 | DES-content-community-backend | aligned | EVD-20260907-watch-contract |
| REL-006 | DES-content-community-backend | aligned | EVD-20260907-watch-contract |
| REL-007 | DES-content-community-backend | aligned | EVD-20260907-watch-contract |
| REL-008 | DES-content-community-backend | aligned | EVD-20260907-watch-contract |
| REL-010 | DES-content-community-backend | aligned | EVD-20260907-watch-contract |
| REL-011 | DES-content-community-backend | aligned | EVD-20260907-watch-contract |
| REL-012 | DES-content-community-backend | aligned | EVD-20260907-watch-contract |
| REL-013 | DES-content-community-backend | aligned | EVD-20260907-watch-contract |
| REL-020 | DES-content-community-backend | aligned | EVD-20260907-watch-contract |
| REL-021 | DES-content-community-backend | aligned | EVD-20260907-watch-contract |
| REL-022 | DES-content-community-backend | aligned | EVD-20260907-watch-contract |
| REL-023 | DES-content-community-backend | aligned | EVD-20260907-watch-contract |
| REL-024 | DES-content-community-backend | aligned | EVD-20260907-watch-contract |
| REL-030 | DES-content-community-backend | unknown | gap: 口径/指标或恢复机制已实现；缺真实 UTC 自然月生产观测。 |
| REL-031 | DES-content-community-backend | unknown | gap: 口径/指标或恢复机制已实现；缺真实 UTC 自然月生产观测。 |
| REL-032 | DES-content-community-backend | unknown | gap: 口径/指标或恢复机制已实现；缺真实 UTC 自然月生产观测。 |
| REL-033 | DES-content-community-backend | unknown | gap: 口径/指标或恢复机制已实现；缺真实 UTC 自然月生产观测。 |
| REL-040 | DES-content-community-backend | unknown | gap: 口径/指标或恢复机制已实现；缺真实 UTC 自然月生产观测。 |
| REL-041 | DES-content-community-backend | unknown | gap: 口径/指标或恢复机制已实现；缺真实 UTC 自然月生产观测。 |
| REL-042 | DES-content-community-backend | unknown | gap: 口径/指标或恢复机制已实现；缺真实 UTC 自然月生产观测。 |
| REL-043 | DES-content-community-backend | unknown | gap: 口径/指标或恢复机制已实现；缺真实 UTC 自然月生产观测。 |
| REL-044 | DES-content-community-backend | aligned | EVD-20260907-watch-contract |
| REL-045 | DES-content-community-backend | aligned | EVD-20260907-watch-contract |
| REL-050 | DES-content-community-backend | aligned | EVD-20260907-watch-contract |
| REL-051 | DES-content-community-backend | aligned | EVD-20260907-watch-contract |
| REL-052 | DES-content-community-backend | aligned | EVD-20260907-watch-contract |
| REL-053 | DES-content-community-backend | aligned | EVD-20260907-watch-contract |
| REL-054 | DES-content-community-backend | unknown | gap: 十二项均已逐条登记，但完整故障注入矩阵尚未关闭。 |
| REL-054-01 | DES-content-community-backend | aligned | EVD-20260907-watch-contract |
| REL-054-02 | DES-content-community-backend | aligned | EVD-20260907-watch-contract |
| REL-054-03 | DES-content-community-backend | aligned | EVD-20260907-watch-contract |
| REL-054-04 | DES-content-community-backend | aligned | EVD-20260907-watch-contract |
| REL-054-05 | DES-content-community-backend | aligned | EVD-20260907-watch-contract |
| REL-054-06 | DES-content-community-backend | aligned | EVD-20260907-watch-contract |
| REL-054-07 | DES-content-community-backend | aligned | EVD-20260907-watch-contract |
| REL-054-08 | DES-content-community-backend | aligned | EVD-20260907-watch-contract |
| REL-054-09 | DES-content-community-backend | aligned | EVD-20260907-watch-contract |
| REL-054-10 | DES-content-community-backend | unknown | gap: 业务继续路径有设计；指标后端故障与监控缺口告警缺独立注入证据。 |
| REL-054-11 | DES-content-community-backend | aligned | EVD-20260907-watch-contract |
| REL-054-12 | DES-content-community-backend | aligned | EVD-20260907-watch-contract |
| REL-060 | DES-content-community-backend | aligned | EVD-20260907-watch-contract |
| REL-061 | DES-content-community-backend | aligned | EVD-20260907-watch-contract |
| REL-A01 | DES-content-community-backend | aligned | EVD-20260907-watch-contract |
| REL-A02 | DES-content-community-backend | unknown | gap: 未运行完整跨服务“推荐 → 曝光 → 动作 → 分析/特征”闭环。 |
| REL-A03 | DES-content-community-backend | diverged | gap: 尚未逐项注入 REL-054-01 至 REL-054-12 并验证响应、健康、指标和日志。 |
| REL-A04 | DES-content-community-backend | unknown | gap: 未运行全部保留期与关闭个性化后 24 小时清理集成。 |
| REL-A05 | DES-content-community-backend | unknown | gap: 缺真实 UTC 自然月生产观测；合成数据只验证报告管线。 |

## 代码边界

具体入口由 frontmatter 的 `code_paths` 给出；路径均相对仓库根目录，
跨模块行为通过上游 DES 与本表中的精确条款关联。

## 证据边界

当前确定性验证见 `EVD-20260907-watch-contract`。旧 `implementation/evidence/` 记录只保留历史上下文，
不参与当前 `aligned` 判定。人类、真实 provider、浏览器、设备和生产证据未执行时不会被推断。
