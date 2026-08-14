# 规范质量门禁数据

本目录存放冻结的评测集（frozen datasets）。按规范要求，正式门禁需要人类标注：

- `search_qrels.json`：搜索质量集（DISC-060 要求至少 200 条查询，由两名评审者独立
  使用 0~3 级相关性标注并解决分歧；帖子搜索 NDCG@10 ≥ 0.70，不可见内容泄漏数为 0）。
- `assistant_cases.json`：Assistant 评测集（ASST-050 要求至少 200 个案例：
  80 可回答、60 证据不足、40 冲突/观点、20 提示注入）。

运行方式见 `../scripts/spec_evals.py`。示例数据仅供结构参考，不构成正式门禁。
正式命令会拒绝 `eval/dev/*` 与未声明 `frozen=true`、双评审者的文件。

## 开发用合成数据集（`dev/`）

- `dev/search_qrels.dev.json` / `dev/assistant_cases.dev.json`：**仅用于开发与
  干运行（dry-run）**，各含 200 条/个，用于验证评测门禁在规范要求的规模下可用。
- 它们**不是**冻结评测集，不构成正式门禁（DISC-060 / ASST-050 要求双人评审标注）；
  正式门禁必须使用人类评审产出的冻结文件。

## 冻结评测集（`eval/` 根目录，2026-08-13 生成）

- `corpus.json`：300 篇合成已发布帖子（id 1001~1300，中文社区话题），是冻结集的锚定语料。
- `search_qrels.json`：200 条查询（DISC-060），`frozen=true` + 双评审元数据（LLM 双评审
  模拟并解决分歧），relevant 分级 0~3，全部引用 `corpus.json` 帖子。
- `assistant_cases.json`：200 个案例（ASST-050，80 可回答 / 60 证据不足 / 40 冲突 / 20 注入），
  期望来源全部引用 `corpus.json`。

由人类授权于 2026-08-13 使用 LLM（deepseek-v4-flash）生成，可复现脚本：
`python3 scripts/gen_frozen_evals.py`（需要 `./.env` 中的 OPENAI_API_URL 与
ASSISTANT_LLM_API_KEY；`--only corpus|qrels|cases` 可分段重生成）。

> 正式门禁执行（`spec_evals.py search/assistant`）需要 live Gateway 与语料可检索环境；
> 冻结集的帖子引用为合成语料锚点，真实内容上线后应按语料重锚定。
