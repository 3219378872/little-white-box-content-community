# 待外部输入的规范门禁

本页是非正式状态汇总，不是实现台账或证据。精确状态以六个
[domain IMP](../implementation/README.md) 及其 [EVD](../evidence/README.md) 为准。

## 人类质量评审

当前 `eval/dev/search_qrels.synthetic.json`、`eval/dev/assistant_cases.synthetic.json`、
`eval/dev/recommend_samples.synthetic.json` 与锚定语料均为 `dataset_role: development`、
`review_provenance: synthetic`、`frozen: false`。LLM reviewer 名称不是人类独立评审，
2026-08-14 的合成栈结果只保留为 legacy 调试记录。

- DISC-060：至少 200 条真实查询，由两名人类独立做 0～3 级标注并解决分歧；在可检索环境复现
  NDCG@10 ≥ 0.70 且不可见内容泄漏为 0。
- AGENT-A10～A13：用真实 provider 与人类评审覆盖社区充分/不足/故障、分轮澄清、站内外补充、
  多源冲突及逐项 URL 支持关系；不能用来源存在性或合成 facts 代替语义判断。
- WCH-014/WCH-A05 等主动消息质量边界同样需要真实场景评审。

正式数据必须显式声明 `frozen: true`、`dataset_role: official`、`review_provenance: human`、
`independent_review: true`、至少两名 reviewer 及 `disagreements_resolved: true`。

## 推荐效果

当前合成样本中 `model_ranked == baseline_ranked`，相对提升为 0，DISC-063 已知未达。学习排序先达到
DISC-062 的 10,000 次有效曝光与 1,000 个有效身份门槛，再用真实时间留出样本、规则基线和 bootstrap
95% 区间复评；不得重用搜索 qrels 或以规则模型和自身对比宣称改善。

## 生产 SLO 与长任务

`eval/slo/` 是 synthetic 管线夹具，不是生产观测。关闭 REL-030～043/REL-A05 需要 UTC 自然月数据，
并按能力分别计算有效请求、可用性和 p95：

| 能力 | p95 | 门禁语义 |
| --- | ---: | --- |
| 社区核心读取 / 行为接收 | 300 ms | 正式门禁 |
| 关注流、搜索、推荐 | 800 ms | 正式门禁 |
| Assistant 接收 | 500 ms | 正式门禁 |
| Assistant 首个持久事件 | 2 s | 正式门禁 |
| Assistant 普通完成 | 45 s | 只观察，不判长任务不可用 |
| Watch 命中到主动消息 | 5 min | 正式门禁 |

长任务必须继续观察心跳、elapsed/idle、queue age 和阶段，不设单一完成 SLO。浏览器、设备、真实
provider 和生产结果须分别记录 scope，不能由静态、单元、集成或 synthetic 证据推断。

## 故障矩阵

REL-054-01 至 REL-054-12 已全部进入 DES/IMP，但 REL-A03 仍需逐项故障注入并核验响应、就绪/健康、
指标和日志。尤其 REL-054-10 的指标后端故障与监控缺口告警尚缺独立注入证据。

[楼中楼回复提案](../proposals/PROP-20260822-comment-reply-thread.md) 仍为 `open`，等待人类决定；现有代码或
历史证据不能把拟议 CORE-070 自动提升为 approved requirement。
