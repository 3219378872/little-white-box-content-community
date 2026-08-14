---
implementation: IMP-content-community-backend
verified_at: 2026-08-14
verified_commit: 4923ee8
commands:
  - python3 scripts/gen_recommend_samples.py
  - python3 scripts/spec_evals.py recommend --samples eval/recommend_samples.json
result: partial
---

# 2026-08-14 推荐门禁冻结样本集与执行（DISC-061/063）

## 背景

人类授权用 LLM 生成评测数据（2026-08-13/14）。`scripts/gen_recommend_samples.py`
由 LLM（deepseek-v4-flash）生成 200 个会话样本（时间覆盖 2026-07 整月，grades 引用
`eval/corpus.json`），脚本附加生产规则基线排序。

## 结果

- `recommend: cases=40（20% 时间留出） model_ndcg@20=0.0599 baseline=0.0599
  relative_improvement=0.0000 (require>=0.05) bootstrap95=0.0000..0.0000` → 如实未达标。
- 语义：生产当前仅服务规则模型（热榜），无学习排序模型；`model_ranked == baseline_ranked`
  是当前状态的如实表达。按 DISC-062，学习模型需 ≥10,000 有效曝光与 ≥1,000 有效身份
  才能进入效果门禁，此前不宣称学习模型改善——本结果即该合规状态。
- DISC-061：推荐样本集与搜索 qrels 独立，指标（NDCG@20 vs NDCG@10）与数据集分离。

## 说明

- 样本集冻结元数据：`frozen=true`、双评审者（LLM 模拟，人类授权）、生成器与种子
  口径（view_count=id%97 规则热榜）可复现。
- 待学习模型达到 DISC-062 门槛后，以真实 model/baseline 排序重生成并复评。
