---
title: knowledge transition register
owner: agent
status: active
observed_at: 2026-08-15
observed_commit: a52eb89
---

# 知识迁移登记

本页只记录迁移状态，不是意图、规范或设计。

## 已完成迁移（2026-08-13）

- 意图层：`docs/knowledge/intent/INT-content-community-backend.md`（approved）。
- 规范层：`SPEC-community-core`、`SPEC-content-discovery`、`SPEC-grounded-assistant`、
  `SPEC-feedback-reliability`（approved）。
- 设计层：`docs/knowledge/design/DES-content-community-backend.md`（active）。
- 实现层：`IMP-content-community-backend`、`IMP-architecture`、
  `IMP-engineering-conventions`、`IMP-development-quickstart`、`IMP-todo-blocked-gates`
  与带日期证据。

旧过渡基线（docs/active 速查、顶层 DESIGN/SECURITY/RELIABILITY/QUALITY_SCORE
规范文档、ARCHITECTURE 服务架构文档、generated 旧快照）的内容已并入实现层页面并
从仓库移除；路由见 `docs/INDEX.md`。

## 当前边界

- 正式知识链：意图（human）→ 规范（human）→ 设计（agent）→ 实现（agent）。
- 源码、配置、`.api`、`.proto` 和测试是当前行为的权威事实。
- 实现页 `aligned/diverged` 状态必须引用活跃 `DES-*` 并列出真实 `tracks`、提交和日期。
- 未获授权的意图/规范建议只能写入 `docs/knowledge/proposals/`。

## 后续待办

1. 两名人类评审者产出正式冻结评测集；现有 LLM 合成集不能关闭 DISC-060 / ASST-050。
2. 学习模型达到 DISC-062 门槛后复评 DISC-063。
3. 收集一个自然月的生产观测数据，关闭 REL-033 / REL-040~043 / REL-A05。
