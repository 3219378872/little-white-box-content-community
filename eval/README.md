# 规范质量门禁数据

本目录存放冻结的评测集（frozen datasets）。按规范要求，正式门禁需要人类标注：

- `search_qrels.json`：搜索质量集（DISC-060 要求至少 200 条查询，由两名评审者独立
  使用 0~3 级相关性标注并解决分歧；帖子搜索 NDCG@10 ≥ 0.70，不可见内容泄漏数为 0）。
- `assistant_cases.json`：Assistant 评测集（ASST-050 要求至少 200 个案例：
  80 可回答、60 证据不足、40 冲突/观点、20 提示注入）。

运行方式见 `../scripts/spec_evals.py`。示例数据仅供结构参考，不构成正式门禁。

## 开发用合成数据集（`dev/`）

- `dev/search_qrels.dev.json` / `dev/assistant_cases.dev.json`：**仅用于开发与
  干运行（dry-run）**，各含 200 条/个，用于验证评测门禁在规范要求的规模下可用。
- 它们**不是**冻结评测集，不构成正式门禁（DISC-060 / ASST-050 要求双人评审标注）；
  正式门禁必须使用人类评审产出的冻结文件。
