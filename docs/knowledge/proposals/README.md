# 非权威提案区

本目录供 agent 提出意图或规范变更建议，内容不属于正式四层知识链。

- 提案使用 `PROP-YYYYMMDD-<slug>`，并声明 `target_layer: intent` 或 `spec`。
- 设计和实现不得把 `PROP-*` 当作 `upstream`。
- 人类接受建议时，可以亲自创建或通过当前对话授权 agent 创建、修改正式 `INT-*` / `SPEC-*`；
  移动文件或修改提案状态不会自动提升权威。
- 提案关闭后可保留为非权威历史，不能覆盖已批准文档。

新提案使用 `../templates/proposal.md`。

## 当前提案

- [PROP-20260813-core-revision-contract](PROP-20260813-core-revision-contract.md)：
  CORE-013 乐观锁与 CORE-062 向后兼容的契约收敛选项（closed：2026-08-13 采纳选项 B）。
- [PROP-20260813-slo-synthetic-observation](PROP-20260813-slo-synthetic-observation.md)：
  LLM 生成合成生产观测进行 SLO 报告管线干跑（open，待人类决定是否作为门禁关闭依据）。
- [PROP-20260822-comment-reply-thread](PROP-20260822-comment-reply-thread.md)：
  楼中楼回复读取与计数一致性（open，拟 CORE-070，待人类决定是否纳入规范）。
- [PROP-20260827-agent-spec-closure](PROP-20260827-agent-spec-closure.md)：
  08-27 Agent 扩张后意图与规范收口（open；SLO、网页来源、记忆/Watch 保留期等待人类逐项决定）。
