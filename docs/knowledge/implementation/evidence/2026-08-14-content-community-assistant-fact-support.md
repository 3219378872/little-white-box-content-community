---
implementation: IMP-content-community-backend
verified_at: 2026-08-14
verified_commit: 3436d90
commands:
  - make spec-evals-test
  - make check
  - make test
  - make python-unit
result: passed
---

# 2026-08-14 Assistant 事实陈述支持率门禁补齐（ASST-051）

## 背景

ASST-051 要求「事实陈述支持率不低于 95%」，但 `scripts/spec_evals.py` 的
assistant 门禁此前只测量来源有效率、证据不足召回、可回答误拒率与注入越界，
**事实陈述支持率完全未测量**——工具与规范存在偏离。

## 本批次改动

- `scripts/spec_evals.py`：
  - `AssistantEvalResult` 新增 `facts_supported/facts_total` 与
    `fact_support_rate`；
  - 新增确定性判定 `fact_supported`：期望事实的字符 bigram 在回答文本中的
    覆盖率 ≥ 0.5 视为支持（中文场景、无分词依赖、可复现；阈值基于冻结语料
    120 个 answerable/conflict 案例标定：逐字转写全部 ≥ 0.5，无关文本全部
    < 0.5）；
  - `evaluate_assistant` 对 answerable/conflict 案例统计期望事实支持；
  - `live_assistant` 捕获 `token` 事件文本作为回答输入；
  - `report_assistant` 输出并门禁 `fact_support_rate ≥ 0.95`；无
    `expected_facts` 时视为未测量，门禁失败；
  - `require_official_assistant` 校验 answerable/conflict 案例必须携带
    `expected_facts`。
- `scripts/gen_frozen_evals.py`：新增确定性派生 `derive_expected_facts`（每帖
  标题 + 最长标点切分句）与 `enrich_cases_with_facts`，`validate()` 写入；
  新增 `--only facts` 模式（无 LLM、可复现富集）。
- `eval/assistant_cases.json`：经生成器重写，120 个 answerable/conflict 案例
  共 514 条 `expected_facts`；corpus/qrels 字节不变。
- `scripts/test_spec_evals.py`：新增 5 项测试（判定正负例、混合案例计数、
  未测量失败、<95% 失败、全量达标通过），共 35 项。

## 审查证据

- 端到端离线模拟（冻结数据集 + 模拟 run_case）：回答逐字转写全部期望事实 →
  `fact_support_rate=1.000`，门禁通过；无关文本回答 → `0.000`，门禁失败。
- ASST-A02（推荐候选重读验证）：`app/assistant/rpc/internal/tool/registry.go`
  `recommendHandler` 仅用推荐选候选，随后 `visibility.PublishedByIDs` 重读正文
  并验证 published 后才成为证据；`TestRegistryRecommendRereadsAndVerifiesPostsBeforeEvidence`
  与 `TestRegistryRecommendFailsClosedWhenContentUnavailable` 覆盖。
- ASST-034（安全拒绝不泄露）：`blockedDirectives` 检测 + `toolErrorCode` 稳定
  错误码映射，客户端仅收到错误码不泄露内部细节。
- `gen_recommend_samples.py` / `gen_frozen_evals.py`：LLM 部分按已登记口径
  （温度 0.7 + 双评审元数据，冻结文件为准）；规则基线排名与本次事实派生均为
  确定性逻辑，重复执行结果一致。

## 结果

- `make spec-evals-test`：35 项全过。
- `make check`：通过（fmt/engineering-lint/vet/golangci-lint 0 issues）。
- `make test`：全部 module 通过（0 失败）。
- `make python-unit`：3 项全过。

## 未覆盖边界

- 事实判定为确定性代理（bigram 覆盖），非语义级判定；LLM judge 留作外部
  输入门禁（见 `IMP-todo-blocked-gates.md`）。
- live 门禁需真实 Gateway 与 token，未在本批重跑；冻结集结构已通过
  `require_official_assistant` 校验。
