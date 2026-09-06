---
title: knowledge transition register
owner: agent
status: active
observed_at: 2026-09-06
---

# 知识迁移登记

本页只记录治理迁移，不定义产品或工程要求。

## 2026-09-06 五层治理

旧结构把操作指南、全域实现台账和带日期验证混在 implementation 下，无法机器判断某条要求由谁负责、
证据观察了哪个提交，也容易把历史通过结果误用于新条款。当前结构改为：

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
