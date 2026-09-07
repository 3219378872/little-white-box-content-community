---
title: knowledge transition register
owner: agent
status: active
observed_at: 2026-09-07
---

# 知识迁移登记

本页只记录治理迁移，不定义产品或工程要求。

## 2026-09-07 去重与分组证据

保留五层、六域、稳定 ID 与现有条款语义。IMP 矩阵成为唯一条款归属和符合性来源，页头只使用
`active/retired` 生命周期；DES 保留 tracks，SPEC 上游由工具推导。EVD 改用 requirements/paths
覆盖组，按组判断当前证明，历史结果不因输入变化被改写。层索引和双向导航由工具生成。

`scripts/knowledge.py` 使用安全 YAML 与 CommonMark AST，导出固定提交 JSON 供根仓校验；
`scripts/engineering-lint.py` 保留公共入口、授权策略、提案、链接与生成约束。以下为上一轮迁移记录，
其中手填双向引用和重复字段已由本轮推导关系替代。此次迁移不提升 unknown/diverged，也不关闭开放门禁。

## 2026-09-06 五层治理

旧结构把操作指南、全域实现台账和带日期验证混在 implementation 下，无法机器判断某条要求由谁负责、
证据观察了哪个提交，也容易把历史通过结果误用于新条款。当时结构改为：

```text
INT -> SPEC -> DES -> IMP <-> EVD
```

- 六份 approved SPEC 是当前规范上游；`SPEC-grounded-assistant` 与
  `SPEC-assistant-agent-mode` 已 retired，只保留历史语境。
- current DES 使用精确 `tracks` 承接全部 approved requirement；每条 requirement 恰有一个 current IMP
  owner。实现按 community-core、content-discovery、assistant-agent、agent-memory、agent-watch、
  feedback-reliability 六个领域拆分。
- current EVD 独立登记提交、命令、scope 和结果，并与 IMP 双向引用。旧
  `implementation/evidence/` 原样保留为 legacy，不参与 aligned 判定。
- 原 `IMP-architecture`、`IMP-development-quickstart`、`IMP-engineering-conventions` 与
  `IMP-todo-blocked-gates` 只保留 retired ID 指针；内容分别迁到 `guides/` 与 `status/`。
- `IMP-content-community-backend` 退役为六域索引指针，不再重复拥有 requirement。

`scripts/engineering-lint.py` 从 approved SPEC 动态抽取 requirement，校验 DES 覆盖、唯一 IMP owner、
逐行状态、IMP/EVD 双向引用、证据提交与 scope、跨仓引用格式和五层索引。公共入口仍是
`make engineering-lint`。

## 工具和数据边界

- Assistant 评测与性能客户端跟随当前契约：先 `POST /api/v2/assistant/messages`，再从
  `/api/v2/assistant/runs/{runId}/events` 读取持久 SSE；旧 `/api/v2/assistant/chat` 不再使用。
- `eval/corpus.json` 与 `eval/dev/*.synthetic.json` 均为 development/synthetic 数据；根目录不保留
  易被误认为正式门禁集的 qrels、Assistant cases 或推荐样本。历史生成 target 仅为兼容保留。
- 普通完成 45 秒是 observation-only；长任务不设统一完成门禁。Assistant accept、首持久事件和 Watch
  delivery 分别按 500ms、2s、5min 口径观察。

这些治理、评测工具和数据元数据变更不改变后端 API、proto、SQL 或业务运行语义。

## 仍开放的事项

1. 两名人类独立评审并解决分歧后，产出 official/human 冻结集，关闭 DISC-060 与 AGENT-A13。
2. 学习排序达到 DISC-062 数据门槛后，用真实时间留出排序复评 DISC-063。
3. 收集 UTC 自然月生产观测，并完成 REL-033、REL-040～043、REL-A05 与十二项 REL-054 故障注入。
4. 浏览器、设备、真实 provider 和生产边界必须各自留 EVD，不能由静态/单元/合成结果替代。
5. [楼中楼回复提案](proposals/PROP-20260822-comment-reply-thread.md) 仍等待人类决定，不是 approved SPEC，
   本次迁移不提升或改写其语义。
