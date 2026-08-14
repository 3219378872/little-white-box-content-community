---
implementation: IMP-content-community-backend
verified_at: 2026-08-14
verified_commit: ec77518
commands:
  - python3 scripts/gen_frozen_evals.py
  - make spec-evals-test
  - python3 -c "from spec_evals import require_official_search, require_official_assistant ..."
result: passed
---

# 2026-08-14 LLM 生成冻结评测集（DISC-060 / ASST-050）

## 背景

人类于 2026-08-13 授权使用 LLM 生成冻结评测集（原为“待双人评审标注”的外部输入项）。
使用 `./.env` 的 opencodego 账户（`deepseek-v4-flash`），脚本 `scripts/gen_frozen_evals.py`
（约 35 次 LLM 调用，约 1 小时，含截断/403 重试处理）。

## 产出

- `eval/corpus.json`：300 篇合成已发布帖子（id 1001~1300，中文社区话题），冻结集锚定语料。
- `eval/search_qrels.json`：200 条查询，`frozen=true`，双评审元数据
  （llm-reviewer-a/b，分歧由 LLM 解决），relevant 分级 {3:478, 2:127, 1:18}，
  每条均含 hidden 泄漏锚点，引用全部在语料内。
- `eval/assistant_cases.json`：200 案例，类型配额 80 可回答 / 60 证据不足 / 40 冲突 /
  20 注入，期望来源全部在语料内。

## 结果

- `require_official_search` / `require_official_assistant` 通过（200/200）。
- `make spec-evals-test` 通过（13 测试）。
- `make engineering-lint` 通过；`make check`/`make test`/`make coverage` 通过。

## 未覆盖边界

- DISC/ASST 门禁数值（NDCG@10 ≥0.70、来源有效率 100% 等）需对 live Gateway 执行；
  合成语料为锚点，真实内容上线后需按真实语料重锚定（已记录在 eval/README）。
