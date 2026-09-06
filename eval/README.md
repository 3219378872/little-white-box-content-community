# 规范质量评测数据

本目录当前只包含合成开发数据，不包含可关闭正式质量门禁的人类冻结集。LLM 生成的数据集中放在
`dev/*.synthetic.json`，根目录不保留容易被误用为正式门禁输入的同名 qrels、cases 或 samples。

## 正式门禁契约

`scripts/spec_evals.py search/assistant/recommend` 只接受同时满足以下元数据的数据：

- `frozen: true` 与 `dataset_role: official`；
- `review_provenance: human`；
- `independent_review: true` 与至少两名不同的 `reviewers`；
- `disagreements_resolved: true`。

两个不同的 LLM reviewer 名称不等于两名人类独立评审。搜索集还须满足 DISC-060 的 200 条查询、
0～3 级标注和分歧解决要求；Assistant 集按 AGENT-A13 延续既有人类冻结集的案例规模、类型配额、
逐项事实和来源核验要求。推荐集还须声明至少 10,000 次有效曝光和 1,000 个有效身份，并为每个时间
留出样本提供人类标注、学习模型排序和规则基线排序。

## 当前开发数据

- `corpus.json`：300 篇 LLM 合成帖子（id 1001～1300），仅用于本地种子和评测工具干运行。
- `dev/search_qrels.synthetic.json`：200 条 LLM 合成查询与相关性标签，明确为
  `dataset_role: development`、`review_provenance: synthetic`、`frozen: false`。
- `dev/assistant_cases.synthetic.json`：200 个 LLM 合成案例，期望事实由合成语料确定性派生；不能证明
  实际回答质量。
- `dev/recommend_samples.synthetic.json`：200 个合成会话，`model_ranked` 等于规则基线，不能关闭
  DISC-063，也不支持学习排序改善声明。
- `dev/search_qrels.dev.json` 与 `dev/assistant_cases.dev.json`：与上述生成数据不同、且不依赖它们的
  确定性测试夹具。
- `dev/corpus_2000.json`：2000 篇本地联调种子帖（id 2001～4000），不参与正式门禁。

历史命令 `make gen-frozen-evals` 为兼容保留，当前只会生成 `dev/*.synthetic.json` development 数据；
底层脚本为 `scripts/gen_frozen_evals.py`。`make gen-recommend-samples` 同样只生成合成开发样本。两者
需要本地 LLM 环境变量，不得把生成结果改写为 official/human。

## 历史运行边界

2026-08-14 曾在合成语料栈运行搜索与 Assistant 评测，用于发现分页、中文分词和检索参数缺陷；记录见
`docs/knowledge/implementation/evidence/2026-08-14-content-community-live-gates.md`。这些结果属于旧格式
历史证据，只证明当时的合成流程与暴露问题，不关闭当前 DISC-060、DISC-063 或 AGENT-A13 门禁。

正式数据准备完成后，使用：

```bash
python3 scripts/spec_evals.py search --qrels <official-qrels.json> --base-url http://127.0.0.1:8888
python3 scripts/spec_evals.py assistant --cases <official-cases.json> --base-url http://127.0.0.1:8888 --token <JWT>
python3 scripts/spec_evals.py recommend --samples <official-recommend-samples.json>
```
