---
title: knowledge transition register
owner: agent
status: active
observed_at: 2026-09-05
observed_commit: 482e0cc6d8527056a87b32a78d4c827206bde103
---

# 知识迁移登记

本页只记录迁移状态，不是意图、规范或设计。

## 已完成迁移（2026-08-13，历史快照）

- 意图层：`docs/knowledge/intent/INT-content-community-backend.md`（approved）。
- 规范层：`SPEC-community-core`、`SPEC-content-discovery`、`SPEC-grounded-assistant`、
  `SPEC-assistant-agent-mode`、`SPEC-agent-memory`、`SPEC-agent-watch`、
  `SPEC-feedback-reliability`（当时 approved；Assistant 两份旧规范现为 retired）。
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

## 2026-08-27 Agent 能力扩张（历史快照）

- 意图：`INT-content-community-backend` 将 Agent 从发帖工具扩展为内容搜索综述、
  结构化记忆、可解释推荐与条件追踪；明确不做游戏库/价格/商城与通用通知中心。
- 规范：修订 `SPEC-grounded-assistant`（评论证据、`ASST-042` 重批）、
  `SPEC-assistant-agent-mode`（分组白名单、`consent_version`）；新增
  `SPEC-agent-memory`、`SPEC-agent-watch`。这些是迁移前的历史版本，当前约束以
  `SPEC-assistant-agent` 及重写后的 Memory/Watch 规范为准。
- 设计与实现尚未对齐：活跃设计仍须在后续任务中引用新规范，实现状态在对齐前保持
  `diverged`。

## 2026-08-29 Hermes 式长期 Agent（历史修订）

- `INT-content-community-backend` 已批准 Assistant 虚拟消息线程、通用异步 Agent、主动 Watch、
  双文档自然语言记忆、compact/BM25 与可选来源语义。
- `SPEC-assistant-agent` 接替 retired/deprecated 的 `SPEC-grounded-assistant` 与
  `SPEC-assistant-agent-mode`；`SPEC-agent-memory`、`SPEC-agent-watch` 和可靠性规范同步重写。
- `DES-assistant-agent-runtime` 已改为 assistant-rpc read model + MySQL lease worker + Watch 调度设计。
  实现未全部验证前 `IMP-content-community-backend` 继续保持 `diverged`。

## 2026-09-05 社区优先的复杂需求助手

- 人类在当前对话中确认并要求发布：内容社区优先，Agent 是辅助使用社区的工具；通过
  `ask_questions` 优先以选择题分轮澄清，允许未知、无偏好、跳过和先搜索；社区不足时尝试互联网
  补充并如实说明缺口；最终用自然语言逐项关联帖子或网页 URL。
- 正式上游已更新：[意图](intent/INT-content-community-backend.md)、[Agent 规范](spec/SPEC-assistant-agent.md)、
  [发现边界](spec/SPEC-content-discovery.md)、[Watch 引用](spec/SPEC-agent-watch.md)和
  [可靠性口径](spec/SPEC-feedback-reliability.md)。普通闲聊仍不强制引用，不恢复旧模式或 Intent Router。
- 本次只发布意图、规格及必要的索引和对齐状态说明；未修改运行时代码、SOUL、`.api`、`.proto`、SQL、
  SDK、Flutter 或根编排。现有 Memory 删除/快照、异步恢复、授权、安全、预算、保留期和质量阈值不变。
- `AGENT-100`~`AGENT-115`、`AGENT-073`~`AGENT-075` 与 `AGENT-A10`~`AGENT-A13` 尚待下层承接。
  已有 source ledger 不等于逐项信息引用已交付，旧验收证据不能关闭新增门禁；实现状态保持 `diverged`。
- `DES-content-community-backend` 与 `DES-assistant-agent-runtime` 仅增加上游变更提示，本次不设计
  问答的提交接口、等待/恢复状态、事件 payload 或逐项引用的数据结构。这些跨端契约须后续单独设计，
  并核对后端生成契约与前端 `vendor/sdk_source` / `lib/sdk`，不能把文档修订当作接口已经存在。

## 后续待办

2026-09-05 后续实施：正式知识已重新区分产品意图、工程规格与内部设计；社区研究闭环已有实现，
新增问答/证据/展示表为增量迁移，不清空历史。工程验证见
[当前证据](implementation/evidence/2026-09-05-agent-community-research.md)。前文“只发布文档”为当天
前一阶段记录，不代表当前代码仍无该能力；真实端到端和语义质量不得由工程门禁替代。

1. 完成社区研究闭环的跨端、真实模型与语义质量验收，保留各自证据边界。
2. 两名人类评审者产出正式冻结评测集；现有 LLM 合成集不能关闭 DISC-060 / ASST-050。
3. 学习模型达到 DISC-062 门槛后复评 DISC-063。
4. 收集一个自然月的生产观测数据，关闭 REL-033 / REL-040~043 / REL-A05。
