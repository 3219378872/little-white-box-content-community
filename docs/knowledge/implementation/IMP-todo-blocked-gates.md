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
  - eval/search_qrels.json
  - eval/assistant_cases.json
  - eval/recommend_samples.json
  - pkg/outboxx/metrics.go
verified_at: 2026-08-14
verified_commit: bea6c09
---

# 待外部输入的规范门禁登记

本页登记「基于固定意图层与规范层重构直至全部满足」目标中**依赖人类或生产环境输入**
的剩余门禁项。agent 无法自行生成（伪造评测集或观测数据会违反规范对门禁本身的要求），
因此单独记录，避免被误判为已满足或遗漏。已完成部分见各小节「现状」。

## 1. 冻结评测集与门禁执行（DISC-060~063 / ASST-050~051）

**现状（截至 2026-08-14）**：

- `eval/search_qrels.json`（200 查询）与 `eval/assistant_cases.json`（200 案例，
  80/60/40/20 类型配额）已冻结（`frozen=true` + 双评审元数据，锚定 `eval/corpus.json`），
  生成脚本 `scripts/gen_frozen_evals.py` 可复现。
- 搜索门禁已对 live Gateway 执行：`NDCG@10=0.816`、泄漏 0，DISC-060 通过
  （`scripts/spec_evals.py search`）。
- Assistant 门禁已执行：注入越界 0（达标）、可回答误拒率 5.8%（≤10% 达标）、
  来源有效率 77.3%（<100%）、证据不足召回 8.3%（<95%）——ASST-050/051 部分未达。
- 推荐门禁已冻结 `eval/recommend_samples.json`（200 会话样本，时间切分留出）并执行：
  `model=baseline=规则热榜 0.0599`，相对提升 0——生产暂无学习排序模型，按 DISC-062
  （≥10,000 有效曝光、≥1,000 有效身份）不宣称改善，如实未达标。

**剩余（需人类或生产输入）**：

1. **ASST 质量提升方向决策**：可选方向——引入语义检索（embedding/Milvus 链路已存在）、
   调整 `eval/assistant_cases.json` 中 insufficient 案例的锚点策略，或按产品目标放宽阈值。
2. **DISC-061/063 复评触发**：学习模型达到 DISC-062 门槛后，以真实 model/baseline 排序
   重生成 `eval/recommend_samples.json` 并复评（相对规则基线提升 ≥5%）。
3. **真实内容重锚定**：真实内容上线后，冻结集帖子引用需按真实语料重锚定（当前锚点为
   合成语料 `eval/corpus.json`）。

## 2. 月度生产观测数据（REL-030~043）

**现状（2026-08-14）**：合成干跑已验证报告管线——`eval/slo/profiles.json`（LLM 画像）+
`scripts/gen_slo_synthetic.py`（确定性合成）产出 `eval/slo/2026-07-*.json`，6 个能力域
`spec_evals.py slo` 全部 `met=True`（REL-030/031 口径、p95、阈值判断正确）。合成数据
不构成生产合规证据（见 `docs/knowledge/proposals/PROP-20260813-slo-synthetic-observation.md`）。

**剩余（需生产输入）**：

1. **真实月度 SLO 观测（REL-030~043）**：提供自然月生产运行数据（按
   `scripts/spec_evals.py slo --requests ...` 格式），运行 `make performance-gateway` 与
   SLO 报告命令核对达标情况，并按 REL-A05 出正式报告。
2. **PROP-20260813-slo-synthetic-observation 决定**：是否接受 LLM 合成观测作为
   REL-030~033/040~043 的门禁关闭依据（推荐：否，合成仅验证报告管线）。
