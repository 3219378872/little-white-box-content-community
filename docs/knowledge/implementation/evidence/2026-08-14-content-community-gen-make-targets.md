---
implementation: IMP-content-community-backend
verified_at: 2026-08-14
verified_commit: 7ae1115
commands:
  - make help
  - make -n gen-slo-synthetic
  - make check
result: passed
---

# 2026-08-14 评测数据生成器统一 Makefile 入口（工具质量）

## 缺陷

`scripts/gen_frozen_evals.py` / `gen_recommend_samples.py` /
`gen_slo_synthetic.py` 是 DISC/ASST/REL 正式门禁的数据生成器
（冻结评测集、推荐样本、SLO 合成观测），仅被文档（eval/README、
证据）引用，无 Makefile 目标——与「所有日常检查统一通过根目录
Makefile」的约定不一致，工具可发现性差。

## 改动

Makefile 新增三个目标（均转发 ARGS，与现有目标风格一致）：

- `make gen-frozen-evals`：LLM 重新生成冻结评测集（corpus/qrels/cases，
  支持 `--only` 分段）。
- `make gen-recommend-samples`：重新生成冻结推荐样本集。
- `make gen-slo-synthetic`：重新生成确定性合成 SLO 观测。

## 验证

- `make help` 正确展示三个目标及说明。
- `make -n gen-slo-synthetic` 语法/命令正确。
- `make check` 通过。

## 未覆盖边界

- 生成器仍需 `.env` 的 LLM 密钥（人类授权生成），非日常门禁；
  外部输入门禁不变，见 [IMP-todo-blocked-gates](../IMP-todo-blocked-gates.md)。
