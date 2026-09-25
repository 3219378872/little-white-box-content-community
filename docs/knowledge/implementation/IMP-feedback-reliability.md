---
id: IMP-feedback-reliability
layer: implementation
title: 反馈与可靠性实现映射
status: active
owner: agent
updated_at: 2026-09-25
code_paths:
- app/recommend/rpc
- pkg/interceptor
- app/behavior
- app/pipeline/behaviorlog
- app/recommend/mq
- pkg/outboxx
- app/gateway
- app/assistant
- app/content
- app/interaction
- app/message
- app/media
- app/user
- app/feed
- deploy/log_retention_test.go
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
| REL-001 | DES-content-community-backend | aligned | EVD-20260925-quality-remediation |
| REL-002 | DES-content-community-backend | aligned | EVD-20260925-quality-remediation |
| REL-003 | DES-content-community-backend | aligned | EVD-20260925-quality-remediation |
| REL-004 | DES-content-community-backend | aligned | EVD-20260925-quality-remediation |
| REL-005 | DES-content-community-backend | aligned | EVD-20260925-quality-remediation |
| REL-006 | DES-content-community-backend | aligned | EVD-20260925-quality-remediation |
| REL-007 | DES-content-community-backend | aligned | EVD-20260925-quality-remediation |
| REL-008 | DES-content-community-backend | aligned | EVD-20260925-quality-remediation |
| REL-010 | DES-content-community-backend | aligned | EVD-20260925-quality-remediation |
| REL-011 | DES-content-community-backend | aligned | EVD-20260925-quality-remediation |
| REL-012 | DES-content-community-backend | aligned | EVD-20260925-quality-remediation |
| REL-013 | DES-content-community-backend | aligned | EVD-20260925-quality-remediation |
| REL-020 | DES-content-community-backend | aligned | EVD-20260925-quality-remediation |
| REL-021 | DES-content-community-backend | aligned | EVD-20260925-quality-remediation |
| REL-022 | DES-content-community-backend | aligned | EVD-20260925-quality-remediation |
| REL-023 | DES-content-community-backend | aligned | EVD-20260925-quality-remediation |
| REL-024 | DES-content-community-backend | aligned | EVD-20260925-quality-remediation |
| REL-030 | DES-content-community-backend | unknown | gap: 口径/指标或恢复机制已实现；缺真实 UTC 自然月生产观测。 |
| REL-031 | DES-content-community-backend | unknown | gap: 口径/指标或恢复机制已实现；缺真实 UTC 自然月生产观测。 |
| REL-032 | DES-content-community-backend | unknown | gap: 口径/指标或恢复机制已实现；缺真实 UTC 自然月生产观测。 |
| REL-033 | DES-content-community-backend | unknown | gap: 口径/指标或恢复机制已实现；缺真实 UTC 自然月生产观测。 |
| REL-040 | DES-content-community-backend | unknown | gap: 口径/指标或恢复机制已实现；缺真实 UTC 自然月生产观测。 |
| REL-041 | DES-content-community-backend | unknown | gap: 口径/指标或恢复机制已实现；缺真实 UTC 自然月生产观测。 |
| REL-042 | DES-content-community-backend | unknown | gap: 口径/指标或恢复机制已实现；缺真实 UTC 自然月生产观测。 |
| REL-043 | DES-content-community-backend | unknown | gap: 口径/指标或恢复机制已实现；缺真实 UTC 自然月生产观测。 |
| REL-044 | DES-content-community-backend | aligned | EVD-20260925-quality-remediation |
| REL-045 | DES-content-community-backend | aligned | EVD-20260925-quality-remediation |
| REL-050 | DES-content-community-backend | aligned | EVD-20260925-quality-remediation |
| REL-051 | DES-content-community-backend | aligned | EVD-20260925-quality-remediation |
| REL-052 | DES-content-community-backend | aligned | EVD-20260925-quality-remediation |
| REL-053 | DES-content-community-backend | aligned | EVD-20260925-quality-remediation |
| REL-054 | DES-content-community-backend | unknown | gap: 十二项均已逐条登记，但完整故障注入矩阵尚未关闭。 |
| REL-054-01 | DES-content-community-backend | aligned | EVD-20260925-quality-remediation |
| REL-054-02 | DES-content-community-backend | aligned | EVD-20260925-quality-remediation |
| REL-054-03 | DES-content-community-backend | aligned | EVD-20260925-quality-remediation |
| REL-054-04 | DES-content-community-backend | aligned | EVD-20260925-quality-remediation |
| REL-054-05 | DES-content-community-backend | aligned | EVD-20260925-quality-remediation |
| REL-054-06 | DES-content-community-backend | aligned | EVD-20260925-quality-remediation |
| REL-054-07 | DES-content-community-backend | aligned | EVD-20260925-quality-remediation |
| REL-054-08 | DES-content-community-backend | aligned | EVD-20260925-quality-remediation |
| REL-054-09 | DES-content-community-backend | aligned | EVD-20260925-quality-remediation |
| REL-054-10 | DES-content-community-backend | unknown | gap: 业务继续路径有设计；指标后端故障与监控缺口告警缺独立注入证据。 |
| REL-054-11 | DES-content-community-backend | aligned | EVD-20260925-quality-remediation |
| REL-054-12 | DES-content-community-backend | aligned | EVD-20260925-quality-remediation |
| REL-060 | DES-content-community-backend | aligned | EVD-20260925-quality-remediation |
| REL-061 | DES-content-community-backend | aligned | EVD-20260925-quality-remediation |
| REL-A01 | DES-content-community-backend | aligned | EVD-20260925-quality-remediation |
| REL-A02 | DES-content-community-backend | unknown | gap: 未运行完整跨服务“推荐 → 曝光 → 动作 → 分析/特征”闭环。 |
| REL-A03 | DES-content-community-backend | diverged | gap: 尚未逐项注入 REL-054-01 至 REL-054-12 并验证响应、健康、指标和日志。 |
| REL-A04 | DES-content-community-backend | unknown | gap: 未运行全部保留期与关闭个性化后 24 小时清理集成。 |
| REL-A05 | DES-content-community-backend | unknown | gap: 缺真实 UTC 自然月生产观测；合成数据只验证报告管线。 |

## 代码边界

具体入口由 frontmatter 的 `code_paths` 给出；路径均相对仓库根目录，
跨模块行为通过上游 DES 与本表中的精确条款关联。

## 证据边界

当前确定性验证见 `EVD-20260925-quality-remediation`。覆盖缺失/过期关闭标记时的持久偏好检查、定时清理、作者分发和既有可靠性回归。
证据在固定实现提交运行全模块 race、静态检查与专用 MySQL/Redis 隔离集成，随后更新本映射；
历史 EVD 保留原观察结果，不改写为当前证明。原有 unknown/diverged 及其 gap 不提升。
本轮没有真实模型、浏览器、设备、容量或生产验证，这些范围不能从本地门禁推导。
