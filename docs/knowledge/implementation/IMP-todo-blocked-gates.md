---
id: IMP-todo-blocked-gates
layer: implementation
title: 待外部输入的规范门禁登记
owner: agent
status: unknown
upstream:
  - DES-content-community-backend
tracks:
  - scripts/spec_evals.py
  - eval/search_qrels.example.json
  - eval/assistant_cases.example.json
  - pkg/outboxx/metrics.go
verified_at: 2026-08-13
verified_commit: c9e350d
---

# 待外部输入的规范门禁登记

以下两项是「基于固定意图层与规范层重构直至全部满足」目标剩余的收尾项。二者均依赖
**人类或生产环境的输入**，agent 无法自行生成（伪造评测集或观测数据会违反规范对
门禁本身的要求），因此单独记录，避免被误判为已满足或遗漏。

## 1. 冻结评测集（DISC-060~063 / ASST-050~051）

**输入**：由两名评审者独立标注并解决分歧的冻结数据集。

- 搜索质量集：至少 200 条查询，0~3 级相关性标注；帖子搜索要求 `NDCG@10 ≥ 0.70`、
  不可见内容泄漏数为 0（`SPEC-content-discovery` DISC-060）。
- Assistant 评测集：至少 200 个案例（80 可回答 / 60 证据不足 / 40 冲突或观点 /
  20 提示注入），案例、期望证据和分类结果可复现；来源有效率 100%、事实陈述支持率
  ≥95%、证据不足召回率 ≥95%、可回答误拒率 ≤10%、注入越界 0 次
  （`SPEC-grounded-assistant` ASST-050/051）。

**已就绪**：`scripts/spec_evals.py`（search/assistant/recommend/slo 四个子命令）、
`eval/` 目录下的示例数据集结构、`make spec-evals-test`。

**待办**：人类评审按 `eval/search_qrels.example.json` 与
`eval/assistant_cases.example.json` 的结构产出正式冻结集（各 ≥200 条/个），随后运行
`python3 scripts/spec_evals.py search --qrels ...` 与
`assistant --cases ...` 完成门禁；推荐门禁还需冻结样本集
（DISC-061/062/063：时间切分留出集 + bootstrap 95% 置信区间，相对规则基线提升 ≥5%）。

> 说明：正式冻结集建议命名 `search_qrels.json` 与 `assistant_cases.json`（不含
> `.example`），路径固定在 `eval/` 下，待人类评审产出后回填。

## 2. 月度生产观测数据（REL-030~043）

**输入**：一个月度窗口的生产运行数据。

- SLO 可用性/延迟（REL-030~033）：社区核心读写 99.9%/300/500ms、行为接收 99.9%/300ms、
  发现 99.5%/800ms、Assistant 首事件/完成 99.0%/2s/12s。
- 异步延迟（REL-040~042）：outbox 投递 p95 30s / p99 5m；行为到特征 p95 60s / p99 5m；
  内容到搜索 p95 30s / p99 2m。
- RPO 0 / RTO 30 分钟（REL-043）。

**已就绪**：所有 MQ 消费者与 outbox relay 已埋点（outcome 计数、event-lag 直方图、
`esx_outbox_delivery_latency_seconds`），`scripts/spec_evals.py slo` 已实现月度口径
（分母只统计满足公开契约的请求；明确标记的降级与正确拒答计为可用）。

**待办**：收集一个自然月、按 `scripts/spec_evals.py slo --requests ...` 输入格式导出的
观测数据，运行 `make performance-gateway` 与 SLO 报告命令核对达标情况。
